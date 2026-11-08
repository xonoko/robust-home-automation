// Package store gives access to Postgres (docs/adr/0004).
//
// Postgres holds configuration and history only; the hot path never depends
// on it, so its readiness check is not critical.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"entgo.io/ent/dialect"

	"github.com/xonoko/robust-home-automation/internal/store/ent"
)

// Store wraps the ent client and the underlying connection pool.
type Store struct {
	Client *ent.Client
	db     *sql.DB
}

// Open prepares a connection pool. It does not wait for Postgres to be
// reachable: the first query (or Check) does.
func Open(url string) (*Store, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	return &Store{Client: client, db: db}, nil
}

// Check implements a readiness check.
func (s *Store) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres unreachable: %w", err)
	}
	return nil
}

// Close closes the connection pool.
func (s *Store) Close() error { return s.Client.Close() }
