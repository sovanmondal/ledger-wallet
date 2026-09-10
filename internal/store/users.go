package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/jackc/pgx/v5"
)

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// RegisterUser creates a user (idempotent by username) and returns it with its token.
// Re-registering an existing username returns the existing token — convenient for tests.
func (s *Store) RegisterUser(ctx context.Context, username string) (User, error) {
	token := randToken()
	var u User
	err := s.pool.QueryRow(ctx,
		`INSERT INTO users (username, token) VALUES ($1, $2)
		 ON CONFLICT (username) DO NOTHING
		 RETURNING id, username, token`,
		username, token,
	).Scan(&u.ID, &u.Username, &u.Token)
	if err == nil {
		return u, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// Username already existed — return the existing record.
		err = s.pool.QueryRow(ctx,
			`SELECT id, username, token FROM users WHERE username = $1`, username,
		).Scan(&u.ID, &u.Username, &u.Token)
		return u, err
	}
	return User{}, err
}

// UserByToken resolves a bearer token to a user. Returns ErrNotFound if unknown.
func (s *Store) UserByToken(ctx context.Context, token string) (User, error) {
	var u User
	err := s.pool.QueryRow(ctx,
		`SELECT id, username, token FROM users WHERE token = $1`, token,
	).Scan(&u.ID, &u.Username, &u.Token)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}
