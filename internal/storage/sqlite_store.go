package storage

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/RenAhsAcme/SCWebVPN/internal/audit"
	"github.com/RenAhsAcme/SCWebVPN/internal/binding"
	"github.com/RenAhsAcme/SCWebVPN/internal/catalog"
	"github.com/RenAhsAcme/SCWebVPN/internal/identity"
	"github.com/RenAhsAcme/SCWebVPN/internal/presence"
	"github.com/RenAhsAcme/SCWebVPN/internal/session"
)

func (store *SQLiteStore) RecordAudit(ctx context.Context, event audit.Event) error {
	var actor any
	if event.ActorDigest != nil {
		actor = event.ActorDigest[:]
	}
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO audit_events(
            occurred_at, actor_digest, agent_id, service_id, event_type,
            result_code, candidate_type, latency_bucket, byte_bucket
         ) VALUES(?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''))`,
		event.OccurredAt.Unix(), actor, event.AgentID, event.ServiceID, event.Type,
		event.ResultCode, event.Candidate, event.Latency, event.Bytes,
	)
	return err
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("SQLite database is required")
	}
	return &SQLiteStore{db: db}, nil
}

func (store *SQLiteStore) ApplyMigrations(ctx context.Context) error {
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA busy_timeout = 5000",
		`CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            applied_at INTEGER NOT NULL
        ) STRICT`,
	} {
		if _, err := store.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configure SQLite: %w", err)
		}
	}

	var current int
	if err := store.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&current); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if current > len(Migrations) {
		return fmt.Errorf("database schema version %d is newer than supported version %d", current, len(Migrations))
	}
	for _, migration := range Migrations {
		if migration.Version <= current {
			continue
		}
		transaction, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}
		if _, err := transaction.ExecContext(ctx, migration.SQL); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("apply migration %d: %w", migration.Version, err)
		}
		if _, err := transaction.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", migration.Version, time.Now().UTC().Unix()); err != nil {
			_ = transaction.Rollback()
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
		if err := transaction.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}
	return nil
}

func (store *SQLiteStore) SaveBindingCode(ctx context.Context, digest [32]byte, expiresAt, createdAt time.Time) error {
	_, err := store.db.ExecContext(ctx,
		"INSERT INTO binding_codes(digest, expires_at, created_at) VALUES(?, ?, ?)",
		digest[:], expiresAt.Unix(), createdAt.Unix(),
	)
	return err
}

func (store *SQLiteStore) ConsumeBindingCode(ctx context.Context, digest [32]byte, agent binding.Agent, now time.Time) error {
	transaction, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx,
		`UPDATE binding_codes SET consumed_at = ?
         WHERE digest = ? AND consumed_at IS NULL AND expires_at > ?`,
		now.Unix(), digest[:], now.Unix(),
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return binding.ErrInvalidCode
	}
	if _, err := transaction.ExecContext(ctx,
		`INSERT INTO agents(id, display_name, public_key, state, created_at)
         VALUES(?, ?, ?, 'active', ?)`,
		agent.ID, agent.DisplayName, []byte(agent.PublicKey), agent.CreatedAt.Unix(),
	); err != nil {
		return err
	}
	return transaction.Commit()
}

func (store *SQLiteStore) AgentPublicKey(ctx context.Context, agentID string) (ed25519.PublicKey, error) {
	var publicKey []byte
	if err := store.db.QueryRowContext(ctx,
		"SELECT public_key FROM agents WHERE id = ? AND state = 'active'", agentID,
	).Scan(&publicKey); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, identity.ErrAgentNotFound
		}
		return nil, err
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("stored Agent public key has an invalid size")
	}
	return ed25519.PublicKey(publicKey), nil
}

func (store *SQLiteStore) TouchAgent(ctx context.Context, agentID string, seen time.Time) error {
	result, err := store.db.ExecContext(ctx,
		"UPDATE agents SET last_seen_at = ? WHERE id = ? AND state = 'active'", seen.Unix(), agentID,
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return presence.ErrNotFound
	}
	return nil
}

func (store *SQLiteStore) AgentLastSeen(ctx context.Context, agentID string) (time.Time, error) {
	var seen sql.NullInt64
	if err := store.db.QueryRowContext(ctx,
		"SELECT last_seen_at FROM agents WHERE id = ? AND state = 'active'", agentID,
	).Scan(&seen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, presence.ErrNotFound
		}
		return time.Time{}, err
	}
	if !seen.Valid {
		return time.Time{}, nil
	}
	return time.Unix(seen.Int64, 0).UTC(), nil
}

func (store *SQLiteStore) SaveNonce(ctx context.Context, agentID string, digest [32]byte, expiresAt, createdAt time.Time) error {
	result, err := store.db.ExecContext(ctx,
		`INSERT INTO agent_nonces(digest, agent_id, expires_at)
         SELECT ?, id, ? FROM agents WHERE id = ? AND state = 'active'`,
		digest[:], expiresAt.Unix(), agentID,
	)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted != 1 {
		return identity.ErrAgentNotFound
	}
	_ = createdAt // The nonce lifetime is sufficient; creation time is deliberately not persisted.
	return nil
}

func (store *SQLiteStore) ConsumeNonce(ctx context.Context, agentID string, digest [32]byte, now time.Time) error {
	result, err := store.db.ExecContext(ctx,
		`UPDATE agent_nonces SET consumed_at = ?
         WHERE digest = ? AND agent_id = ? AND consumed_at IS NULL AND expires_at > ?`,
		now.Unix(), digest[:], agentID, now.Unix(),
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return identity.ErrNonceUsed
	}
	return nil
}

func (store *SQLiteStore) ListServices(ctx context.Context) ([]catalog.Service, error) {
	rows, err := store.db.QueryContext(ctx,
		`SELECT id, agent_id, slug, display_name, kind, policy_ref, enabled
		 FROM services WHERE enabled = 1 AND owner_digest IS NULL ORDER BY display_name, id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	services := make([]catalog.Service, 0)
	for rows.Next() {
		var current catalog.Service
		if err := rows.Scan(&current.ID, &current.AgentID, &current.Slug, &current.DisplayName, &current.Kind, &current.PolicyRef, &current.Enabled); err != nil {
			return nil, err
		}
		services = append(services, current)
	}
	return services, rows.Err()
}

func (store *SQLiteStore) ServiceByID(ctx context.Context, serviceID string) (catalog.Service, error) {
	var service catalog.Service
	if err := store.db.QueryRowContext(ctx,
		`SELECT id, agent_id, slug, display_name, kind, policy_ref, enabled, owner_digest IS NOT NULL
		 FROM services WHERE id = ? AND enabled = 1`, serviceID,
	).Scan(&service.ID, &service.AgentID, &service.Slug, &service.DisplayName, &service.Kind, &service.PolicyRef, &service.Enabled, &service.Temporary); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return catalog.Service{}, catalog.ErrNotFound
		}
		return catalog.Service{}, err
	}
	return service, nil
}

func (store *SQLiteStore) AuthorizedService(ctx context.Context, serviceID string, owner [32]byte, now time.Time) (catalog.Service, error) {
	if err := store.expireTemporaryServices(ctx, now); err != nil {
		return catalog.Service{}, err
	}
	var service catalog.Service
	var absolute sql.NullInt64
	err := store.db.QueryRowContext(ctx,
		`SELECT id, agent_id, slug, display_name, kind, policy_ref, enabled,
		        owner_digest IS NOT NULL, absolute_expires_at
		 FROM services
		 WHERE id = ? AND enabled = 1
		   AND (owner_digest IS NULL OR (owner_digest = ? AND expires_at > ? AND absolute_expires_at > ?))`,
		serviceID, owner[:], now.Unix(), now.Unix(),
	).Scan(&service.ID, &service.AgentID, &service.Slug, &service.DisplayName, &service.Kind, &service.PolicyRef, &service.Enabled, &service.Temporary, &absolute)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return catalog.Service{}, catalog.ErrNotFound
		}
		return catalog.Service{}, err
	}
	if service.Temporary {
		next := now.Add(30 * time.Minute)
		limit := time.Unix(absolute.Int64, 0).UTC()
		if next.After(limit) {
			next = limit
		}
		result, err := store.db.ExecContext(ctx,
			`UPDATE services SET expires_at = ?
			 WHERE id = ? AND owner_digest = ? AND enabled = 1 AND expires_at > ? AND absolute_expires_at >= ?`,
			next.Unix(), service.ID, owner[:], now.Unix(), next.Unix(),
		)
		if err != nil {
			return catalog.Service{}, err
		}
		updated, err := result.RowsAffected()
		if err != nil || updated != 1 {
			return catalog.Service{}, catalog.ErrNotFound
		}
	}
	return service, nil
}

func (store *SQLiteStore) TemporaryServiceBySlug(ctx context.Context, slug string, owner [32]byte, now time.Time) (catalog.Service, error) {
	if err := store.expireTemporaryServices(ctx, now); err != nil {
		return catalog.Service{}, err
	}
	var id string
	if err := store.db.QueryRowContext(ctx,
		`SELECT id FROM services
		 WHERE slug = ? AND owner_digest = ? AND enabled = 1 AND expires_at > ? AND absolute_expires_at > ?`,
		slug, owner[:], now.Unix(), now.Unix(),
	).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return catalog.Service{}, catalog.ErrNotFound
		}
		return catalog.Service{}, err
	}
	return store.AuthorizedService(ctx, id, owner, now)
}

func (store *SQLiteStore) RevokeTemporaryServices(ctx context.Context, owner [32]byte, now time.Time) error {
	_, err := store.db.ExecContext(ctx,
		`UPDATE services SET enabled = 0, owner_digest = NULL, expires_at = NULL, absolute_expires_at = NULL
		 WHERE owner_digest = ? AND enabled = 1`, owner[:],
	)
	_ = now
	return err
}

func (store *SQLiteStore) expireTemporaryServices(ctx context.Context, now time.Time) error {
	_, err := store.db.ExecContext(ctx,
		`UPDATE services SET enabled = 0, owner_digest = NULL, expires_at = NULL, absolute_expires_at = NULL
		 WHERE owner_digest IS NOT NULL AND enabled = 1 AND (expires_at <= ? OR absolute_expires_at <= ?)`,
		now.Unix(), now.Unix(),
	)
	return err
}

func (store *SQLiteStore) CreateService(ctx context.Context, service catalog.Service, createdAt time.Time) error {
	if err := catalog.Validate(service); err != nil {
		return err
	}
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO services(id, agent_id, slug, display_name, kind, policy_ref, enabled, created_at)
         VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		service.ID, service.AgentID, service.Slug, service.DisplayName, service.Kind, service.PolicyRef, service.Enabled, createdAt.Unix(),
	)
	return err
}

func (store *SQLiteStore) CreateTemporaryService(ctx context.Context, service catalog.Service, owner [32]byte, createdAt, expiresAt, absoluteExpiresAt time.Time) error {
	if err := catalog.Validate(service); err != nil || !service.Temporary || owner == [32]byte{} || !createdAt.Before(expiresAt) || !expiresAt.Before(absoluteExpiresAt) {
		if err != nil {
			return err
		}
		return errors.New("invalid temporary service")
	}
	if err := store.expireTemporaryServices(ctx, createdAt); err != nil {
		return err
	}
	result, err := store.db.ExecContext(ctx,
		`INSERT INTO services(
		    id, agent_id, slug, display_name, kind, policy_ref, enabled, created_at,
		    owner_digest, expires_at, absolute_expires_at
		 ) SELECT ?, id, ?, ?, ?, ?, 1, ?, ?, ?, ? FROM agents WHERE id = ? AND state = 'active'`,
		service.ID, service.Slug, service.DisplayName, service.Kind, service.PolicyRef,
		createdAt.Unix(), owner[:], expiresAt.Unix(), absoluteExpiresAt.Unix(),
		service.AgentID,
	)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil || inserted != 1 {
		return catalog.ErrNotFound
	}
	return nil
}

func (store *SQLiteStore) CreateBrowserSession(ctx context.Context, record session.Record) error {
	_, err := store.db.ExecContext(ctx,
		`INSERT INTO browser_sessions(
            id, token_digest, user_digest, agent_id, service_id, temporary_host,
            created_at, last_seen_at, expires_at, absolute_expires_at
         ) VALUES(?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`,
		record.ID, record.TokenDigest[:], record.UserDigest[:], record.AgentID, record.ServiceID, record.TemporaryHost,
		record.CreatedAt.Unix(), record.LastSeenAt.Unix(), record.ExpiresAt.Unix(), record.AbsoluteExpiresAt.Unix(),
	)
	return err
}

func (store *SQLiteStore) BrowserSession(ctx context.Context, id string, userDigest [32]byte, now time.Time) (session.Record, error) {
	var (
		record        session.Record
		tokenDigest   []byte
		storedUser    []byte
		temporaryHost sql.NullString
		createdAt     int64
		lastSeenAt    int64
		expiresAt     int64
		absoluteAt    int64
	)
	if err := store.db.QueryRowContext(ctx,
		`SELECT browser_sessions.id, token_digest, user_digest, browser_sessions.agent_id,
		        browser_sessions.service_id, temporary_host, services.owner_digest IS NOT NULL,
		        browser_sessions.created_at, last_seen_at, browser_sessions.expires_at,
		        browser_sessions.absolute_expires_at
		 FROM browser_sessions JOIN services ON services.id = browser_sessions.service_id
		 WHERE browser_sessions.id = ? AND user_digest = ? AND revoked_at IS NULL AND services.enabled = 1
		   AND browser_sessions.expires_at > ? AND browser_sessions.absolute_expires_at > ?`,
		id, userDigest[:], now.Unix(), now.Unix(),
	).Scan(
		&record.ID, &tokenDigest, &storedUser, &record.AgentID, &record.ServiceID, &temporaryHost, &record.Temporary,
		&createdAt, &lastSeenAt, &expiresAt, &absoluteAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.Record{}, session.ErrNotFound
		}
		return session.Record{}, err
	}
	if len(tokenDigest) != 32 || len(storedUser) != 32 {
		return session.Record{}, errors.New("stored browser session digest has an invalid size")
	}
	copy(record.TokenDigest[:], tokenDigest)
	copy(record.UserDigest[:], storedUser)
	record.TemporaryHost = temporaryHost.String
	record.CreatedAt = time.Unix(createdAt, 0).UTC()
	record.LastSeenAt = time.Unix(lastSeenAt, 0).UTC()
	record.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	record.AbsoluteExpiresAt = time.Unix(absoluteAt, 0).UTC()
	return record, nil
}

func (store *SQLiteStore) TouchBrowserSession(ctx context.Context, id string, userDigest [32]byte, seenAt, expiresAt time.Time) error {
	result, err := store.db.ExecContext(ctx,
		`UPDATE browser_sessions SET last_seen_at = ?, expires_at = ?
         WHERE id = ? AND user_digest = ? AND revoked_at IS NULL
           AND expires_at > ? AND absolute_expires_at >= ?`,
		seenAt.Unix(), expiresAt.Unix(), id, userDigest[:], seenAt.Unix(), expiresAt.Unix(),
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return session.ErrNotFound
	}
	return nil
}

func (store *SQLiteStore) RevokeBrowserSessions(ctx context.Context, userDigest [32]byte, now time.Time) error {
	_, err := store.db.ExecContext(ctx,
		"UPDATE browser_sessions SET revoked_at = ? WHERE user_digest = ? AND revoked_at IS NULL",
		now.Unix(), userDigest[:],
	)
	return err
}

func (store *SQLiteStore) RevokeBrowserSession(ctx context.Context, id string, userDigest [32]byte, now time.Time) error {
	result, err := store.db.ExecContext(ctx,
		`UPDATE browser_sessions SET revoked_at = ?
         WHERE id = ? AND user_digest = ? AND revoked_at IS NULL`,
		now.Unix(), id, userDigest[:],
	)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil || updated != 1 {
		return session.ErrNotFound
	}
	return nil
}
