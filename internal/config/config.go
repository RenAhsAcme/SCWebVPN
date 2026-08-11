package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Controller struct {
	Listen                 string   `json:"listen"`
	DatabasePath           string   `json:"database_path"`
	InternalAuthSecretFile string   `json:"internal_auth_secret_file"`
	PublicBaseURL          string   `json:"public_base_url"`
	STUNURLs               []string `json:"stun_urls"`
}

func LoadController(path string) (Controller, error) {
	file, err := os.Open(path)
	if err != nil {
		return Controller{}, err
	}
	defer file.Close()

	var cfg Controller
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Controller{}, fmt.Errorf("decode controller config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Controller{}, errors.New("controller config must contain one valid JSON value")
	}
	if err := cfg.Validate(); err != nil {
		return Controller{}, err
	}
	return cfg, nil
}

func (cfg Controller) Validate() error {
	host, _, err := net.SplitHostPort(cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen must include an IP and port: %w", err)
	}
	address := net.ParseIP(host)
	if address == nil || !address.IsLoopback() {
		return errors.New("controller must listen on a loopback IP")
	}
	if strings.TrimSpace(cfg.DatabasePath) == "" || strings.ContainsAny(cfg.DatabasePath, "\x00\r\n") ||
		!strings.HasPrefix(cfg.DatabasePath, "/") && !filepath.IsAbs(cfg.DatabasePath) {
		return errors.New("database_path must be absolute")
	}
	if strings.TrimSpace(cfg.InternalAuthSecretFile) == "" {
		return errors.New("internal_auth_secret_file is required")
	}
	base, err := url.Parse(cfg.PublicBaseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return errors.New("public_base_url must be an HTTPS origin without credentials, query, or fragment")
	}
	if base.Path != "" && base.Path != "/" {
		return errors.New("public_base_url must not contain a path")
	}
	if err := ValidateSTUNURLs(cfg.STUNURLs); err != nil {
		return err
	}
	return nil
}

func ValidateSTUNURLs(values []string) error {
	if len(values) == 0 {
		return errors.New("at least one STUN URL is required")
	}
	for _, value := range values {
		lower := strings.ToLower(value)
		if value != strings.TrimSpace(value) || !strings.HasPrefix(lower, "stun:") || len(value) <= len("stun:") {
			return fmt.Errorf("only credential-free stun: URLs are allowed: %q", value)
		}
		if err := validateSTUNAuthority(value[len("stun:"):]); err != nil {
			return fmt.Errorf("invalid public STUN URL %q: %w", value, err)
		}
	}
	return nil
}

func validateSTUNAuthority(authority string) error {
	if strings.ContainsAny(authority, "/?#@") {
		return errors.New("credentials, paths, queries, and fragments are forbidden")
	}
	host := authority
	if strings.HasPrefix(authority, "[") {
		closing := strings.IndexByte(authority, ']')
		if closing < 0 {
			return errors.New("invalid bracketed IPv6 address")
		}
		host = authority[1:closing]
		if suffix := authority[closing+1:]; suffix != "" {
			if !strings.HasPrefix(suffix, ":") || !validPort(suffix[1:]) {
				return errors.New("invalid STUN port")
			}
		}
	} else if strings.Count(authority, ":") == 1 {
		var port string
		var err error
		host, port, err = net.SplitHostPort(authority)
		if err != nil || !validPort(port) {
			return errors.New("invalid STUN host or port")
		}
	} else if strings.Contains(authority, ":") {
		return errors.New("IPv6 STUN addresses must be bracketed")
	}
	if host == "" {
		return errors.New("STUN host is empty")
	}
	if address := net.ParseIP(host); address != nil {
		if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
			return errors.New("STUN IP must be public")
		}
		return nil
	}
	lowerHost := strings.ToLower(host)
	if !strings.Contains(lowerHost, ".") || lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".localhost") || strings.HasSuffix(lowerHost, ".lan") || strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".home.arpa") {
		return errors.New("STUN DNS name must be public")
	}
	if !validDNSName(host) {
		return errors.New("invalid STUN DNS name")
	}
	return nil
}

func validPort(value string) bool {
	port, err := strconv.ParseUint(value, 10, 16)
	return err == nil && port != 0
}

func validDNSName(value string) bool {
	if len(value) > 253 || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func ReadSecret(path string) ([]byte, error) {
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	value = []byte(strings.TrimSpace(string(value)))
	if len(value) < 32 {
		return nil, errors.New("internal authentication secret must contain at least 32 bytes")
	}
	return value, nil
}
