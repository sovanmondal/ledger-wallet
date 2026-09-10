package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const transferCols = `id, idempotency_key, from_wallet, to_wallet, amount_paise, kind, status, ` +
	`COALESCE(decline_reason, '') AS decline_reason, ` +
	`COALESCE(reverses_transfer_id::text, '') AS reverses_transfer_id, ` +
	`request_fingerprint, created_at`

func scanTransfer(row pgx.Row) (Transfer, error) {
	var t Transfer
	var declineReason, reversesID string
	err := row.Scan(
		&t.ID, &t.IdempotencyKey, &t.FromWallet, &t.ToWallet, &t.AmountPaise,
		&t.Kind, &t.Status, &declineReason, &reversesID, &t.fingerprint, &t.CreatedAt,
	)
	if err != nil {
		return Transfer{}, err
	}
	if declineReason != "" {
		t.DeclineReason = &declineReason
	}
	if reversesID != "" {
		t.ReversesTransferID = &reversesID
	}
	return t, nil
}

// CreateTransfer moves amount paise from one wallet to another, exactly once per idempotency key.
// Returns (transfer, replay). A same-key/different-body request returns ErrConflict.
func (s *Store) CreateTransfer(ctx context.Context, key, from, to string, amount int64) (Transfer, bool, error) {
	return s.executeMovement(ctx, movementParams{
		idempotencyKey: key, fromWallet: from, toWallet: to, amountPaise: amount, kind: "transfer",
	})
}

// Topup funds a wallet by moving paise FROM the treasury (a normal transfer under the hood),
// keeping the global balance sum invariant. Returns the movement and the refreshed wallet.
func (s *Store) Topup(ctx context.Context, walletID string, amount int64) (Transfer, Wallet, error) {
	t, _, err := s.executeMovement(ctx, movementParams{
		idempotencyKey: "topup:" + randToken(), fromWallet: TreasuryWalletID, toWallet: walletID,
		amountPaise: amount, kind: "transfer",
	})
	if err != nil {
		return Transfer{}, Wallet{}, err
	}
	w, err := s.GetWallet(ctx, walletID)
	if err != nil {
		return Transfer{}, Wallet{}, err
	}
	return t, w, nil
}

// ReverseTransfer refunds a completed transfer by reusing the movement primitive with roles
// swapped. Its own idempotency key guards double-fire; UNIQUE(reverses_transfer_id) guards
// double-reversal (returns ErrAlreadyReversed). If the recipient already spent the funds, the
// reversal declines cleanly (documented policy: decline rather than force a negative balance).
func (s *Store) ReverseTransfer(ctx context.Context, originalID, key string) (Transfer, bool, error) {
	orig, err := s.GetTransfer(ctx, originalID)
	if err != nil {
		return Transfer{}, false, err // ErrNotFound
	}
	if orig.Kind != "transfer" || orig.Status != "completed" {
		return Transfer{}, false, ErrNotReversible
	}
	rid := orig.ID
	return s.executeMovement(ctx, movementParams{
		idempotencyKey: key, fromWallet: orig.ToWallet, toWallet: orig.FromWallet,
		amountPaise: orig.AmountPaise, kind: "reversal", reversesID: &rid,
	})
}

// GetTransfer returns a transfer by id, or ErrNotFound.
func (s *Store) GetTransfer(ctx context.Context, id string) (Transfer, error) {
	t, err := scanTransfer(s.pool.QueryRow(ctx, `SELECT `+transferCols+` FROM transfers WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Transfer{}, ErrNotFound
	}
	return t, err
}

// executeMovement is the single primitive behind transfers, top-ups, and reversals.
// The idempotency-key claim and the balance movement are committed in ONE transaction.
func (s *Store) executeMovement(ctx context.Context, p movementParams) (Transfer, bool, error) {
	fp := fingerprint(p)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Transfer{}, false, err
	}
	defer tx.Rollback(ctx) // no-op after a successful commit

	// (1) Claim the idempotency key by inserting the header — SAME tx as the movement.
	var id string
	err = tx.QueryRow(ctx,
		`INSERT INTO transfers
		   (idempotency_key, from_wallet, to_wallet, amount_paise, kind, status, reverses_transfer_id, request_fingerprint)
		 VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7)
		 ON CONFLICT (idempotency_key) DO NOTHING
		 RETURNING id`,
		p.idempotencyKey, p.fromWallet, p.toWallet, p.amountPaise, p.kind, p.reversesID, fp,
	).Scan(&id)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Key already committed by a prior/concurrent request → serve the original result.
		existing, lerr := scanTransfer(tx.QueryRow(ctx,
			`SELECT `+transferCols+` FROM transfers WHERE idempotency_key = $1`, p.idempotencyKey))
		if lerr != nil {
			return Transfer{}, false, lerr
		}
		_ = tx.Rollback(ctx)
		if existing.fingerprint != fp {
			return Transfer{}, false, ErrConflict
		}
		return existing, true, nil // idempotent replay
	case err != nil:
		// A conflict on reverses_transfer_id (a DIFFERENT unique index, not swallowed by
		// ON CONFLICT) means the original was already reversed by another reversal.
		if isUniqueViolation(err, "reverses_transfer_id") {
			return Transfer{}, false, ErrAlreadyReversed
		}
		return Transfer{}, false, err
	}

	// (2) Apply balances in a savepoint so a decline rolls back the movement but keeps the header.
	if _, err := tx.Exec(ctx, `SAVEPOINT mv`); err != nil {
		return Transfer{}, false, err
	}
	declineReason, err := applyBalances(ctx, tx, p)
	if err != nil {
		return Transfer{}, false, err
	}

	if declineReason != "" {
		if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT mv`); err != nil {
			return Transfer{}, false, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE transfers SET status = 'declined', decline_reason = $1, updated_at = now() WHERE id = $2`,
			declineReason, id); err != nil {
			return Transfer{}, false, err
		}
	} else {
		// Balanced double-entry pair (sums to zero → conservation), then mark completed.
		if _, err := tx.Exec(ctx,
			`INSERT INTO ledger_entries (transfer_id, wallet_id, delta_paise)
			 VALUES ($1, $2, $3), ($1, $4, $5)`,
			id, p.fromWallet, -p.amountPaise, p.toWallet, p.amountPaise); err != nil {
			return Transfer{}, false, err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE transfers SET status = 'completed', updated_at = now() WHERE id = $1`, id); err != nil {
			return Transfer{}, false, err
		}
	}

	final, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferCols+` FROM transfers WHERE id = $1`, id))
	if err != nil {
		return Transfer{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, false, err
	}
	return final, false, nil
}

// applyBalances performs the debit and credit in ascending wallet-id order. Because every
// transaction acquires the two row locks in the SAME global order, A→B and B→A can never
// deadlock. The debit is an atomic conditional UPDATE: 0 rows affected ⇒ clean decline.
func applyBalances(ctx context.Context, tx pgx.Tx, p movementParams) (string, error) {
	type op struct {
		wallet string
		debit  bool
	}
	ops := []op{{p.fromWallet, true}, {p.toWallet, false}}
	if p.fromWallet > p.toWallet { // deterministic order by wallet id
		ops[0], ops[1] = ops[1], ops[0]
	}

	for _, o := range ops {
		if o.debit {
			ct, err := tx.Exec(ctx,
				`UPDATE wallets SET balance_paise = balance_paise - $1 WHERE id = $2 AND balance_paise >= $1`,
				p.amountPaise, o.wallet)
			if err != nil {
				return "", err
			}
			if ct.RowsAffected() == 0 {
				var x bool
				e := tx.QueryRow(ctx, `SELECT true FROM wallets WHERE id = $1`, o.wallet).Scan(&x)
				if errors.Is(e, pgx.ErrNoRows) {
					return "from_wallet_not_found", nil
				}
				if e != nil {
					return "", e
				}
				return "insufficient_funds", nil
			}
		} else {
			ct, err := tx.Exec(ctx,
				`UPDATE wallets SET balance_paise = balance_paise + $1 WHERE id = $2`,
				p.amountPaise, o.wallet)
			if err != nil {
				return "", err
			}
			if ct.RowsAffected() == 0 {
				return "to_wallet_not_found", nil
			}
		}
	}
	return "", nil
}

func fingerprint(p movementParams) string {
	rev := ""
	if p.reversesID != nil {
		rev = *p.reversesID
	}
	raw := fmt.Sprintf("%s|%s|%d|%s|%s", p.fromWallet, p.toWallet, p.amountPaise, p.kind, rev)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func isUniqueViolation(err error, constraintSubstr string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, constraintSubstr)
	}
	return false
}
