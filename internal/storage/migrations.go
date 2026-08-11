package storage

type Migration struct {
	Version int
	SQL     string
}

var Migrations = []Migration{{
	Version: 1,
	SQL: `
CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at INTEGER NOT NULL
) STRICT;

CREATE TABLE agents (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 80),
    public_key BLOB NOT NULL UNIQUE CHECK(length(public_key) = 32),
    state TEXT NOT NULL CHECK(state IN ('active', 'revoked')),
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER
) STRICT;

CREATE TABLE binding_codes (
    digest BLOB PRIMARY KEY CHECK(length(digest) = 32),
    expires_at INTEGER NOT NULL,
    consumed_at INTEGER,
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE agent_nonces (
    digest BLOB PRIMARY KEY CHECK(length(digest) = 32),
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    consumed_at INTEGER
) STRICT;
CREATE INDEX agent_nonces_expiry ON agent_nonces(expires_at);

CREATE TABLE services (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    slug TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 80),
    kind TEXT NOT NULL CHECK(kind IN ('http', 'https', 'guacamole')),
    policy_ref TEXT NOT NULL CHECK(length(policy_ref) BETWEEN 1 AND 120),
    enabled INTEGER NOT NULL CHECK(enabled IN (0, 1)),
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE browser_sessions (
    id TEXT PRIMARY KEY,
    token_digest BLOB NOT NULL UNIQUE CHECK(length(token_digest) = 32),
    user_digest BLOB NOT NULL CHECK(length(user_digest) = 32),
    agent_id TEXT NOT NULL REFERENCES agents(id),
    service_id TEXT NOT NULL REFERENCES services(id),
    temporary_host TEXT UNIQUE,
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    absolute_expires_at INTEGER NOT NULL,
    revoked_at INTEGER
) STRICT;
CREATE INDEX browser_sessions_expiry ON browser_sessions(expires_at, absolute_expires_at);

CREATE TABLE audit_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at INTEGER NOT NULL,
    actor_digest BLOB CHECK(actor_digest IS NULL OR length(actor_digest) = 32),
    agent_id TEXT,
    service_id TEXT,
    event_type TEXT NOT NULL CHECK(length(event_type) BETWEEN 1 AND 64),
    result_code TEXT NOT NULL CHECK(length(result_code) BETWEEN 1 AND 64),
    candidate_type TEXT CHECK(candidate_type IN ('host', 'srflx', 'prflx')),
    latency_bucket TEXT,
    byte_bucket TEXT
) STRICT;
CREATE INDEX audit_events_time ON audit_events(occurred_at);
`,
}, {
	Version: 2,
	SQL: `
ALTER TABLE services ADD COLUMN owner_digest BLOB CHECK(owner_digest IS NULL OR length(owner_digest) = 32);
ALTER TABLE services ADD COLUMN expires_at INTEGER;
ALTER TABLE services ADD COLUMN absolute_expires_at INTEGER;
CREATE UNIQUE INDEX services_temporary_slug ON services(slug) WHERE owner_digest IS NOT NULL;
CREATE INDEX services_temporary_expiry ON services(expires_at, absolute_expires_at) WHERE owner_digest IS NOT NULL;
`,
}}
