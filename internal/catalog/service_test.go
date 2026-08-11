package catalog

import (
	"context"
	"testing"
	"time"
)

type fakeWriteStore struct {
	created Service
}

func (store *fakeWriteStore) ListServices(context.Context) ([]Service, error) {
	return nil, nil
}

func (store *fakeWriteStore) ServiceByID(context.Context, string) (Service, error) {
	return Service{}, ErrNotFound
}

func (store *fakeWriteStore) AuthorizedService(context.Context, string, [32]byte, time.Time) (Service, error) {
	return Service{}, ErrNotFound
}

func (store *fakeWriteStore) TemporaryServiceBySlug(context.Context, string, [32]byte, time.Time) (Service, error) {
	return Service{}, ErrNotFound
}

func (store *fakeWriteStore) RevokeTemporaryServices(context.Context, [32]byte, time.Time) error {
	return nil
}

func (store *fakeWriteStore) CreateService(_ context.Context, service Service, _ time.Time) error {
	store.created = service
	return nil
}

func (store *fakeWriteStore) CreateTemporaryService(_ context.Context, service Service, _ [32]byte, _, _, _ time.Time) error {
	store.created = service
	return nil
}

func TestValidateService(t *testing.T) {
	valid := Service{
		ID:          "AQIDBAUGBwgJCgsMDQ4PEA",
		AgentID:     "ERITFBUWFxgZGhscHR4fIA",
		Slug:        "openwrt",
		DisplayName: "OpenWrt",
		Kind:        "https",
		PolicyRef:   "openwrt-luci",
		Enabled:     true,
	}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Service){
		"slug":       func(value *Service) { value.Slug = "Bad.Host" },
		"kind":       func(value *Service) { value.Kind = "tcp" },
		"display":    func(value *Service) { value.DisplayName = "spoof\nname" },
		"policy ref": func(value *Service) { value.PolicyRef = "" },
		"identifier": func(value *Service) { value.ID = "predictable" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := Validate(candidate); err == nil {
				t.Fatal("invalid service was accepted")
			}
		})
	}
}

func TestManagerCreatesOpaqueServiceWithoutTargetAddress(t *testing.T) {
	store := &fakeWriteStore{}
	manager := NewManager(store)
	created, err := manager.Create(t.Context(), CreateRequest{
		AgentID: "ERITFBUWFxgZGhscHR4fIA", Slug: "openwrt", DisplayName: "OpenWrt",
		Kind: "https", PolicyRef: "openwrt-luci",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !validOpaqueID(created.ID) || store.created != created {
		t.Fatal("manager did not persist an opaque service record")
	}
}

func TestManagerCreatesTemporaryServiceWithIndependent128BitHostname(t *testing.T) {
	store := &fakeWriteStore{}
	created, err := NewManager(store).CreateTemporary(t.Context(), [32]byte{1}, CreateTemporaryRequest{
		AgentID: "ERITFBUWFxgZGhscHR4fIA", DisplayName: "Temporary", Kind: "https",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !created.Temporary || len(created.Slug) != len("tmp-")+26 || !slugPattern.MatchString(created.Slug) || created.ID == created.Slug {
		t.Fatalf("unexpected temporary service: %#v", created)
	}
}
