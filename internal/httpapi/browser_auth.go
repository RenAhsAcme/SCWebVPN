package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	InternalAuthHeader = "X-WebVPN-Internal-Auth"
	UserHeader         = "X-WebVPN-User"
	SessionHeader      = "X-WebVPN-Session"
)

type browserIdentityKey struct{}

type BrowserIdentity struct {
	Username      string
	UserDigest    [32]byte
	SessionDigest [32]byte
}

type BrowserAuth struct {
	secret []byte
}

func NewBrowserAuth(secret []byte) (*BrowserAuth, error) {
	if len(secret) < 32 {
		return nil, errors.New("internal authentication secret must contain at least 32 bytes")
	}
	return &BrowserAuth{secret: append([]byte(nil), secret...)}, nil
}

func (auth *BrowserAuth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !loopbackRemote(request.RemoteAddr) || !equalSecret(auth.secret, []byte(request.Header.Get(InternalAuthHeader))) {
			writeError(writer, http.StatusUnauthorized, "authentication required")
			return
		}
		username := strings.TrimSpace(request.Header.Get(UserHeader))
		sessionToken := request.Header.Get(SessionHeader)
		if username == "" || len(username) > 254 || strings.ContainsAny(username, "\r\n\x00") || sessionToken == "" || len(sessionToken) > 4096 || strings.ContainsAny(sessionToken, "\r\n\x00") {
			writeError(writer, http.StatusUnauthorized, "invalid authenticated identity")
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead && request.Method != http.MethodOptions && !sameOrigin(request) {
			writeError(writer, http.StatusForbidden, "same-origin request required")
			return
		}
		identity := BrowserIdentity{Username: username}
		userMAC := hmac.New(sha256.New, auth.secret)
		_, _ = userMAC.Write([]byte("scwebvpn-user-v1\x00" + username))
		copy(identity.UserDigest[:], userMAC.Sum(nil))
		sessionMAC := hmac.New(sha256.New, auth.secret)
		_, _ = sessionMAC.Write([]byte("scwebvpn-session-v1\x00" + username + "\x00" + sessionToken))
		copy(identity.SessionDigest[:], sessionMAC.Sum(nil))
		request.Header.Del(InternalAuthHeader)
		request.Header.Del(UserHeader)
		request.Header.Del(SessionHeader)
		ctx := context.WithValue(request.Context(), browserIdentityKey{}, identity)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

func sameOrigin(request *http.Request) bool {
	origin, err := url.Parse(request.Header.Get("Origin"))
	if err != nil || origin.Scheme != "https" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return false
	}
	return strings.EqualFold(origin.Host, request.Host)
}

func BrowserFromContext(ctx context.Context) (BrowserIdentity, bool) {
	identity, ok := ctx.Value(browserIdentityKey{}).(BrowserIdentity)
	return identity, ok
}

func loopbackRemote(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return false
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func equalSecret(expected, provided []byte) bool {
	return len(expected) == len(provided) && subtle.ConstantTimeCompare(expected, provided) == 1
}
