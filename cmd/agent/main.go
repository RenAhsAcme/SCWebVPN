package main

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	webvpnagent "github.com/RenAhsAcme/SCWebVPN/internal/agent"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "webvpn-agent:", err)
		os.Exit(1)
	}
}

func run(arguments []string, stdin io.Reader, stdout io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("expected subcommand: bind, check, or run")
	}
	switch arguments[0] {
	case "bind":
		return bind(arguments[1:], stdin, stdout)
	case "check":
		return check(arguments[1:], stdout)
	case "run":
		return runAgent(arguments[1:])
	default:
		return fmt.Errorf("unknown subcommand %q", arguments[0])
	}
}

func runAgent(arguments []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "Agent JSON config path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("run requires -config")
	}
	config, err := webvpnagent.Load(*configPath)
	if err != nil {
		return err
	}
	privateKey, err := webvpnagent.LoadPrivateKey(config.PrivateKeyFile)
	if err != nil {
		return err
	}
	runtime, err := webvpnagent.NewRuntime(config, privateKey)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runtime.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func bind(arguments []string, stdin io.Reader, stdout io.Writer) error {
	flags := flag.NewFlagSet("bind", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	controllerURL := flags.String("controller", "", "Controller HTTPS origin")
	keyPath := flags.String("key", "", "absolute Agent private key path")
	displayName := flags.String("name", "OpenWrt", "Agent display name")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *controllerURL == "" || *keyPath == "" {
		return errors.New("bind requires -controller and -key; binding code is read from stdin")
	}
	code, err := readBindingCode(stdin)
	if err != nil {
		return err
	}
	privateKey, err := webvpnagent.LoadOrCreatePrivateKey(*keyPath)
	if err != nil {
		return err
	}
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return errors.New("Agent private key did not produce an Ed25519 public key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	agentID, err := webvpnagent.BindAgent(ctx, *controllerURL, code, *displayName, publicKey, nil)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, agentID)
	return err
}

func check(arguments []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "Agent JSON config path")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if flags.NArg() != 0 || *configPath == "" {
		return errors.New("check requires -config")
	}
	config, err := webvpnagent.Load(*configPath)
	if err != nil {
		return err
	}
	if _, err := webvpnagent.LoadPrivateKey(config.PrivateKeyFile); err != nil {
		return err
	}
	if _, err := webvpnagent.CompilePolicy(config); err != nil {
		return err
	}
	for _, service := range config.Services {
		if service.TLS != nil {
			if _, err := webvpnagent.BuildTLSConfig(*service.TLS); err != nil {
				return fmt.Errorf("service %q TLS policy: %w", service.PolicyRef, err)
			}
		}
	}
	_, err = fmt.Fprintln(stdout, "configuration valid")
	return err
}

func readBindingCode(reader io.Reader) (string, error) {
	buffered := bufio.NewReader(io.LimitReader(reader, 257))
	value, err := io.ReadAll(buffered)
	if err != nil {
		return "", err
	}
	if len(value) > 256 {
		return "", errors.New("binding code input is too large")
	}
	code := strings.TrimSpace(string(value))
	if code == "" || strings.ContainsAny(code, "\r\n\x00") {
		return "", errors.New("binding code input must contain one value")
	}
	return code, nil
}
