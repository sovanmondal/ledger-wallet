-- LedgerWallet schema. Every correctness invariant is anchored to a DB constraint here.
-- Money is ALWAYS integer paise (bigint). No floats, ever.

CREATE EXTENSION IF NOT EXISTS pgcrypto; -- gen_random_uuid()

-- Users identified by an opaque bearer token.
CREATE TABLE IF NOT EXISTS users (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username   text NOT NULL UNIQUE,
    token      text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- One wallet per user. The UNIQUE(user_id) is what makes get-or-create race-free.
CREATE TABLE IF NOT EXISTS wallets (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL UNIQUE REFERENCES users(id),
    balance_paise bigint NOT NULL DEFAULT 0 CHECK (balance_paise >= 0), -- no-overdraft, enforced by the DB
    created_at    timestamptz NOT NULL DEFAULT now()
);

-- Transfers ledger header. UNIQUE(idempotency_key) makes exactly-once enforceable in the
-- same transaction as the balance movement. UNIQUE(reverses_transfer_id) blocks double-reversal.
CREATE TABLE IF NOT EXISTS transfers (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    idempotency_key      text NOT NULL UNIQUE,
    from_wallet          uuid NOT NULL REFERENCES wallets(id),
    to_wallet            uuid NOT NULL REFERENCES wallets(id),
    amount_paise         bigint NOT NULL CHECK (amount_paise > 0),
    kind                 text NOT NULL DEFAULT 'transfer' CHECK (kind IN ('transfer', 'reversal')),
    status               text NOT NULL CHECK (status IN ('pending', 'completed', 'declined')),
    decline_reason       text,
    reverses_transfer_id uuid UNIQUE REFERENCES transfers(id), -- NULL for normal transfers; unique when set
    request_fingerprint  text NOT NULL, -- hash(from,to,amount,kind,reverses) → same-key/different-body = 409
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);

-- Append-only double-entry ledger. For every completed movement we write two rows whose
-- deltas sum to zero, so global conservation is provable: SELECT sum(delta_paise) = 0 always.
CREATE TABLE IF NOT EXISTS ledger_entries (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id uuid NOT NULL REFERENCES transfers(id),
    wallet_id   uuid NOT NULL REFERENCES wallets(id),
    delta_paise bigint NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ledger_wallet ON ledger_entries(wallet_id);

-- Genesis treasury. Top-ups are modelled as transfers FROM the treasury, so the global
-- sum of all wallet balances is invariant across every operation (funding included).
INSERT INTO users (id, username, token)
VALUES ('00000000-0000-0000-0000-000000000001', '__treasury__', '__treasury_no_login__')
ON CONFLICT DO NOTHING;

INSERT INTO wallets (id, user_id, balance_paise)
VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001', 1000000000000000)
ON CONFLICT DO NOTHING;
