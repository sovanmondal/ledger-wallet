// Package store is the data-access layer: connection pool, migrations, and all
// transactional business logic that upholds the money invariants.
package store

import (
	"context"
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TreasuryWalletID is the genesis wallet that funds all top-ups (seeded in 0001_init.sql).
const TreasuryWalletID = "00000000-0000-0000-0000-000000000002"

//go:embed migrations/*.sql
var migrationFS embed.FS

// Store wraps a pgx connection pool.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a connection pool from a DATABASE_URL.
func New(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool.
func (s *Store) Close() { s.pool.Close() }

// Ping checks DB readiness.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Migrate applies embedded SQL migrations in filename order, tracked in schema_migrations.
func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version int PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	applied := map[int]bool{}
	rows, err := s.pool.Query(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied[v] = true
	}
	rows.Close()

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := parseVersion(name)
		if err != nil {
			return fmt.Errorf("bad migration name %q: %w", name, err)
		}
		if applied[version] {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		if err := s.applyMigration(ctx, version, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) applyMigration(ctx context.Context, version int, sql string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// parseVersion extracts the leading integer from a filename like "0001_init.sql".
func parseVersion(name string) (int, error) {
	i := 0
	for i < len(name) && name[i] >= '0' && name[i] <= '9' {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("no numeric prefix")
	}
	return strconv.Atoi(name[:i])
}

// TotalBalance returns the global sum of all wallet balances (conservation probe).
func (s *Store) TotalBalance(ctx context.Context) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(balance_paise), 0) FROM wallets`).Scan(&total)
	return total, err
}

// LedgerSum returns the sum of all ledger deltas — must always be zero.
func (s *Store) LedgerSum(ctx context.Context) (int64, error) {
	var total int64
	err := s.pool.QueryRow(ctx, `SELECT COALESCE(sum(delta_paise), 0) FROM ledger_entries`).Scan(&total)
	return total, err
}
