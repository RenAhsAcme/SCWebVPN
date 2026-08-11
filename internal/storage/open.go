package storage

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

func OpenSQLite(ctx context.Context, path string) (*sql.DB, *SQLiteStore, error) {
	dsn, err := SQLiteDSN(path)
	if err != nil {
		return nil, nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("open SQLite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("connect SQLite: %w", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	if err := store.ApplyMigrations(ctx); err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, store, nil
}
