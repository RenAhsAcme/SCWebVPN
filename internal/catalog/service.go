package catalog

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("service not found")
	slugPattern  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	allowedKinds = map[string]struct{}{"http": {}, "https": {}, "guacamole": {}}
)

type Service struct {
	ID          string `json:"id"`
	AgentID     string `json:"agent_id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	PolicyRef   string `json:"policy_ref"`
	Enabled     bool   `json:"enabled"`
	Temporary   bool   `json:"temporary,omitempty"`
}

type Store interface {
	ListServices(context.Context) ([]Service, error)
	ServiceByID(context.Context, string) (Service, error)
	AuthorizedService(context.Context, string, [32]byte, time.Time) (Service, error)
	TemporaryServiceBySlug(context.Context, string, [32]byte, time.Time) (Service, error)
	RevokeTemporaryServices(context.Context, [32]byte, time.Time) error
}

type WriteStore interface {
	Store
	CreateService(context.Context, Service, time.Time) error
	CreateTemporaryService(context.Context, Service, [32]byte, time.Time, time.Time, time.Time) error
}

type CreateRequest struct {
	AgentID     string `json:"agent_id"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
	PolicyRef   string `json:"policy_ref"`
}

type CreateTemporaryRequest struct {
	AgentID     string `json:"agent_id"`
	DisplayName string `json:"display_name"`
	Kind        string `json:"kind"`
}

type Manager struct {
	store WriteStore
	now   func() time.Time
}

func NewManager(store WriteStore) *Manager {
	return &Manager{store: store, now: time.Now}
}

func (manager *Manager) Create(ctx context.Context, request CreateRequest) (Service, error) {
	id, err := randomID()
	if err != nil {
		return Service{}, err
	}
	service := Service{
		ID: id, AgentID: request.AgentID, Slug: request.Slug, DisplayName: request.DisplayName,
		Kind: request.Kind, PolicyRef: request.PolicyRef, Enabled: true,
	}
	if err := Validate(service); err != nil {
		return Service{}, err
	}
	if err := manager.store.CreateService(ctx, service, manager.now().UTC()); err != nil {
		return Service{}, err
	}
	return service, nil
}

func (manager *Manager) CreateTemporary(ctx context.Context, owner [32]byte, request CreateTemporaryRequest) (Service, error) {
	if owner == [32]byte{} {
		return Service{}, errors.New("temporary service owner is required")
	}
	id, err := randomID()
	if err != nil {
		return Service{}, err
	}
	slug, err := randomTemporarySlug()
	if err != nil {
		return Service{}, err
	}
	service := Service{
		ID: id, AgentID: request.AgentID, Slug: slug, DisplayName: request.DisplayName,
		Kind: request.Kind, PolicyRef: "temporary", Enabled: true, Temporary: true,
	}
	if err := Validate(service); err != nil {
		return Service{}, err
	}
	now := manager.now().UTC()
	if err := manager.store.CreateTemporaryService(ctx, service, owner, now, now.Add(30*time.Minute), now.Add(2*time.Hour)); err != nil {
		return Service{}, err
	}
	return service, nil
}

func Validate(service Service) error {
	if !validOpaqueID(service.ID) || !validOpaqueID(service.AgentID) {
		return errors.New("service and Agent IDs must be 128-bit opaque values")
	}
	if !slugPattern.MatchString(service.Slug) {
		return errors.New("service slug is invalid")
	}
	if service.DisplayName != strings.TrimSpace(service.DisplayName) || service.DisplayName == "" || len(service.DisplayName) > 80 || strings.ContainsAny(service.DisplayName, "\r\n\x00") {
		return errors.New("service display name must contain 1 to 80 safe bytes")
	}
	if _, ok := allowedKinds[service.Kind]; !ok {
		return errors.New("service kind is invalid")
	}
	if service.PolicyRef != strings.TrimSpace(service.PolicyRef) || service.PolicyRef == "" || len(service.PolicyRef) > 120 || strings.ContainsAny(service.PolicyRef, "\r\n\x00") {
		return errors.New("service policy reference must contain 1 to 120 safe bytes")
	}
	return nil
}

func validOpaqueID(value string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(decoded) == 16
}

func randomID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomTemporarySlug() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "tmp-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}
