package storage

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
)

func SQLiteDSN(path string) (string, error) {
	if path == "" || strings.ContainsAny(path, "\x00\r\n") || !absoluteDatabasePath(path) {
		return "", errors.New("SQLite database path must be absolute")
	}
	normalized := filepath.ToSlash(filepath.Clean(path))
	if len(normalized) >= 2 && normalized[1] == ':' {
		normalized = "/" + normalized
	}
	location := url.URL{Scheme: "file", Path: normalized}
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "journal_mode(WAL)")
	location.RawQuery = query.Encode()
	return location.String(), nil
}

func absoluteDatabasePath(path string) bool {
	return strings.HasPrefix(path, "/") || filepath.IsAbs(path)
}
