package storage

import (
	"net/url"
	"testing"
)

func TestSQLiteDSNConfiguresEveryConnection(t *testing.T) {
	dsn, err := SQLiteDSN("/var/lib/scwebvpn/controller.sqlite3")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	values := parsed.Query()["_pragma"]
	expected := []string{"foreign_keys(1)", "busy_timeout(5000)", "journal_mode(WAL)"}
	if len(values) != len(expected) {
		t.Fatalf("unexpected DSN pragmas: %#v", values)
	}
	for index := range expected {
		if values[index] != expected[index] {
			t.Fatalf("unexpected DSN pragmas: %#v", values)
		}
	}
}

func TestSQLiteDSNRejectsRelativePath(t *testing.T) {
	if _, err := SQLiteDSN("controller.sqlite3"); err == nil {
		t.Fatal("relative SQLite path was accepted")
	}
}
