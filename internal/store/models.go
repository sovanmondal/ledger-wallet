package store

import (
	"errors"
	"time"
)

// User is an authenticated principal identified by a bearer token.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

// Wallet holds an integer-paise balance for exactly one user.
type Wallet struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	BalancePaise int64  `json:"balance_paise"`
}

// Transfer is a ledger header describing one money movement (or a reversal).
type Transfer struct {
	ID                 string    `json:"id"`
	IdempotencyKey     string    `json:"idempotency_key"`
	FromWallet         string    `json:"from"`
	ToWallet           string    `json:"to"`
	AmountPaise        int64     `json:"amount_paise"`
	Kind               string    `json:"kind"`
	Status             string    `json:"status"`
	DeclineReason      *string   `json:"decline_reason,omitempty"`
	ReversesTransferID *string   `json:"reverses_transfer_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`

	fingerprint string // internal; used for same-key/different-body detection
}

// Sentinel errors mapped to HTTP status codes by the handler layer.
var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("idempotency key reused with a different request body")
	ErrAlreadyReversed = errors.New("transfer has already been reversed")
	ErrNotReversible   = errors.New("transfer is not reversible")
)

// movementParams is the internal input to the shared movement primitive.
type movementParams struct {
	idempotencyKey string
	fromWallet     string
	toWallet       string
	amountPaise    int64
	kind           string  // "transfer" | "reversal"
	reversesID     *string // set only for reversals
}
