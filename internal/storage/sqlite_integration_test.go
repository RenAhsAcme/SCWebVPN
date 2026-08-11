package storage

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/audit"
	"github.com/RenAhsAcme/SCWebVPN/internal/binding"
	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/session"
)

func TestSQLiteControlStateAndReplayProtection(t *testing.T) {
	ctx := context.Background()
	db, store, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "controller.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	assertPragmas(t, store)

	bindings := binding.NewService(store)
	code, _, err := bindings.Issue(ctx)
	if err != nil {
		t.Fatal(err)
	}
	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := bindings.Bind(ctx, code, "OpenWrt", publicKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bindings.Bind(ctx, code, "Replay", publicKey); !errors.Is(err, binding.ErrInvalidCode) {
		t.Fatalf("binding replay was not rejected: %v", err)
	}

	challenges := identity.NewChallengeIssuer(store)
	nonce, _, err := challenges.Issue(ctx, agent.ID)
	if err != nil {
		t.Fatal(err)
	}
	nonceDigest := sha256.Sum256([]byte("scwebvpn-nonce-v1\x00" + nonce))
	if err := store.ConsumeNonce(ctx, agent.ID, nonceDigest, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := store.ConsumeNonce(ctx, agent.ID, nonceDigest, time.Now().UTC()); !errors.Is(err, identity.ErrNonceUsed) {
		t.Fatalf("nonce replay was not rejected: %v", err)
	}

	manager := catalog.NewManager(store)
	service, err := manager.Create(ctx, catalog.CreateRequest{
		AgentID: agent.ID, Slug: "openwrt", DisplayName: "OpenWrt", Kind: "https", PolicyRef: "openwrt-luci",
	})
	if err != nil {
		t.Fatal(err)
	}
	owner := [32]byte{9}
	temporary, err := manager.CreateTemporary(ctx, owner, catalog.CreateTemporaryRequest{
		AgentID: agent.ID, DisplayName: "Temporary HTTPS", Kind: "https",
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListServices(ctx)
	if err != nil || len(listed) != 1 || listed[0].ID != service.ID {
		t.Fatalf("temporary service leaked into persistent catalog: %#v err=%v", listed, err)
	}
	if _, err := store.AuthorizedService(ctx, temporary.ID, [32]byte{8}, time.Now().UTC()); !errors.Is(err, catalog.ErrNotFound) {
		t.Fatalf("another user authorized temporary service: %v", err)
	}
	authorized, err := store.TemporaryServiceBySlug(ctx, temporary.Slug, owner, time.Now().UTC())
	if err != nil || authorized.ID != temporary.ID || !authorized.Temporary {
		t.Fatalf("temporary service owner could not resolve mapping: %#v err=%v", authorized, err)
	}
	sessions := session.NewService(store)
	created, err := sessions.Create(ctx, [32]byte{1}, service.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Lookup(ctx, created.Record.ID, created.Record.UserDigest, created.Token); err != nil {
		t.Fatal(err)
	}
	temporarySession, err := sessions.Create(ctx, owner, temporary.ID)
	if err != nil || !temporarySession.Record.Temporary {
		t.Fatalf("temporary session was not marked: %#v err=%v", temporarySession.Record, err)
	}
	if err := store.RevokeTemporaryServices(ctx, owner, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Lookup(ctx, temporarySession.Record.ID, owner, temporarySession.Token); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("revoked temporary mapping remained usable: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE browser_sessions SET expires_at = ? WHERE id = ?", time.Now().Add(-time.Minute).Unix(), created.Record.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Lookup(ctx, created.Record.ID, created.Record.UserDigest, created.Token); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("expired session was not rejected: %v", err)
	}
	recorder := audit.NewRecorder(store)
	if err := recorder.Record(ctx, audit.Event{AgentID: agent.ID, ServiceID: service.ID, Type: "integration_test", ResultCode: "ok"}); err != nil {
		t.Fatal(err)
	}
	var auditCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_events").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit metadata was not persisted: count=%d err=%v", auditCount, err)
	}
}

func assertPragmas(t *testing.T, store *SQLiteStore) {
	t.Helper()
	for _, test := range []struct {
		query string
		want  string
	}{
		{"PRAGMA foreign_keys", "1"},
		{"PRAGMA busy_timeout", "5000"},
		{"PRAGMA journal_mode", "wal"},
	} {
		var value string
		if err := store.db.QueryRow(test.query).Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value != test.want {
			t.Fatalf("%s = %q, want %q", test.query, value, test.want)
		}
	}
}
