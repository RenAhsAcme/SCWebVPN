package storage

import (
	"strings"
	"testing"
)

func TestMigrationsAvoidSensitiveAddressColumns(t *testing.T) {
	for _, migration := range Migrations {
		lower := strings.ToLower(migration.SQL)
		for _, forbidden := range []string{"source_ip", "target_ip", "candidate_address", "authorization", "cookie", "sdp text"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("migration %d contains forbidden sensitive field %q", migration.Version, forbidden)
			}
		}
	}
}

func TestMigrationsDoNotPersistSignalingDescriptions(t *testing.T) {
	for _, migration := range Migrations {
		lower := strings.ToLower(migration.SQL)
		for _, forbidden := range []string{"create table signals", " offer ", " answer ", "sdp"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("migration %d persists signaling data through %q", migration.Version, forbidden)
			}
		}
	}
}

func TestMigrationVersionsAreContiguous(t *testing.T) {
	for index, migration := range Migrations {
		if migration.Version != index+1 {
			t.Fatalf("migration at index %d has version %d", index, migration.Version)
		}
	}
}
