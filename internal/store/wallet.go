package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// GetOrCreateWallet returns the caller's wallet, creating it if absent.
//
// Race-free by construction: the UNIQUE(user_id) constraint means that among N
// concurrent callers, at most one INSERT wins; the rest hit ON CONFLICT DO NOTHING
// (0 rows) and fall through to the SELECT, all observing the single winning row.
// Returns (wallet, created).
func (s *Store) GetOrCreateWallet(ctx context.Context, userID string) (Wallet, bool, error) {
	var w Wallet
	err := s.pool.QueryRow(ctx,
		`INSERT INTO wallets (user_id) VALUES ($1)
		 ON CONFLICT (user_id) DO NOTHING
		 RETURNING id, user_id, balance_paise`,
		userID,
	).Scan(&w.ID, &w.UserID, &w.BalancePaise)
	if err == nil {
		return w, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.pool.QueryRow(ctx,
			`SELECT id, user_id, balance_paise FROM wallets WHERE user_id = $1`, userID,
		).Scan(&w.ID, &w.UserID, &w.BalancePaise)
		return w, false, err
	}
	return Wallet{}, false, err
}

// GetWallet returns a wallet by id, or ErrNotFound.
func (s *Store) GetWallet(ctx context.Context, id string) (Wallet, error) {
	var w Wallet
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, balance_paise FROM wallets WHERE id = $1`, id,
	).Scan(&w.ID, &w.UserID, &w.BalancePaise)
	if errors.Is(err, pgx.ErrNoRows) {
		return Wallet{}, ErrNotFound
	}
	return w, err
}
