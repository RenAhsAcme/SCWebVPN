package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserAuthRejectsExternalAndSpoofedRequests(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	auth, err := NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, ok := BrowserFromContext(request.Context()); !ok {
			t.Fatal("authenticated identity missing from context")
		}
		if request.Header.Get(InternalAuthHeader) != "" || request.Header.Get(UserHeader) != "" || request.Header.Get(SessionHeader) != "" {
			t.Fatal("trusted identity headers reached the application handler")
		}
		writer.WriteHeader(http.StatusNoContent)
	}))

	for _, test := range []struct {
		name       string
		remote     string
		secret     string
		user       string
		session    string
		wantStatus int
	}{
		{"valid", "127.0.0.1:32100", string(secret), "owner", "authelia-session", http.StatusNoContent},
		{"external", "192.0.2.10:32100", string(secret), "owner", "authelia-session", http.StatusUnauthorized},
		{"bad secret", "127.0.0.1:32100", "attacker", "owner", "authelia-session", http.StatusUnauthorized},
		{"missing user", "127.0.0.1:32100", string(secret), "", "authelia-session", http.StatusUnauthorized},
		{"missing session", "127.0.0.1:32100", string(secret), "owner", "", http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "https://vpn.example/api", nil)
			request.RemoteAddr = test.remote
			request.Header.Set(InternalAuthHeader, test.secret)
			request.Header.Set(UserHeader, test.user)
			request.Header.Set(SessionHeader, test.session)
			request.Header.Set("Origin", "https://vpn.example")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("got %d, want %d", response.Code, test.wantStatus)
			}
		})
	}
}

func TestBrowserAuthRejectsCrossOriginWrite(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	auth, err := NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "https://vpn.example/api", nil)
	request.RemoteAddr = "127.0.0.1:32100"
	request.Header.Set(InternalAuthHeader, string(secret))
	request.Header.Set(UserHeader, "owner")
	request.Header.Set(SessionHeader, "authelia-session")
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin write returned %d", response.Code)
	}
}

func TestBrowserAuthSeparatesUserAndSessionDigests(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	auth, err := NewBrowserAuth(secret)
	if err != nil {
		t.Fatal(err)
	}
	identities := make([]BrowserIdentity, 0, 2)
	handler := auth.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		identity, _ := BrowserFromContext(request.Context())
		identities = append(identities, identity)
		writer.WriteHeader(http.StatusNoContent)
	}))
	for _, token := range []string{"session-a", "session-b"} {
		request := httptest.NewRequest(http.MethodGet, "https://vpn.example/api", nil)
		request.RemoteAddr = "127.0.0.1:32100"
		request.Header.Set(InternalAuthHeader, string(secret))
		request.Header.Set(UserHeader, "owner")
		request.Header.Set(SessionHeader, token)
		handler.ServeHTTP(httptest.NewRecorder(), request)
	}
	if identities[0].UserDigest != identities[1].UserDigest || identities[0].SessionDigest == identities[1].SessionDigest {
		t.Fatal("Authelia sessions were not isolated from the stable audit identity")
	}
}
