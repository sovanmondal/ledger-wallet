# LedgerWallet — Architecture

## System overview
```mermaid
flowchart LR
  U["Browser UI / curl"] -->|"HTTPS + Bearer token"| API
  subgraph RENDER["Render — free web service (container)"]
    API["Go API<br/>middleware: correlation · auth · metrics · recover<br/>UI served at /"]
  end
  API -->|"pgx pool · one transaction per movement"| DB[("PostgreSQL<br/>Neon — free managed")]
  API -->|"structured JSON logs (stdout)"| LOG["Better Stack<br/>live tail — public"]
  API -->|"GET /metrics (Prometheus)"| MET["request rate · p99 latency<br/>domain counters · conservation gauge"]
  GHCR[("GHCR image<br/>ghcr.io/sovanmondal/ledger-wallet:latest")] -.->|"Render pulls & runs"| API
```

## The money movement — one transaction (the correctness core)
Every transfer, top-up, and reversal goes through the **same** primitive, in a single DB transaction.
```mermaid
flowchart TD
  S["POST /transfers"] --> BEGIN["BEGIN transaction"]
  BEGIN --> INS["INSERT transfer<br/>ON CONFLICT (idempotency_key) DO NOTHING"]
  INS -->|"key already used"| REPLAY["return original result<br/>(409 if body differs)"]
  INS -->|"inserted (we own it)"| SP["SAVEPOINT"]
  SP --> DEBIT["UPDATE debit<br/>SET balance = balance - amt<br/>WHERE id = from AND balance >= amt<br/>(rows locked in wallet-id order)"]
  DEBIT -->|"0 rows affected"| DECL["ROLLBACK TO SAVEPOINT<br/>status = declined"]
  DEBIT -->|"ok"| CREDIT["UPDATE credit<br/>SET balance = balance + amt"]
  CREDIT --> LEDGER["INSERT 2 ledger entries<br/>(-amt, +amt) → sum = 0"]
  LEDGER --> DONE["status = completed"]
  DECL --> COMMIT["COMMIT"]
  DONE --> COMMIT["COMMIT"]
```

## How each invariant is enforced
- **Race-free get-or-create** → `UNIQUE(user_id)` + `INSERT … ON CONFLICT DO NOTHING`.
- **Exactly-once** → `UNIQUE(idempotency_key)`, inserted in the **same transaction** as the movement.
- **No overdraft** → atomic conditional debit (`… WHERE balance >= amt`) + `CHECK (balance >= 0)`.
- **Conservation** → append-only `ledger_entries`; every movement writes a `(-amt, +amt)` pair, so
  `sum(delta_paise) = 0` always.
- **Deadlock-free** → the two balance rows are always updated in ascending wallet-id order.

## Data model (tables)
`users(id, username, token)` · `wallets(id, user_id UNIQUE, balance_paise CHECK >= 0)` ·
`transfers(id, idempotency_key UNIQUE, from, to, amount_paise, kind, status, reverses_transfer_id UNIQUE, request_fingerprint)` ·
`ledger_entries(transfer_id, wallet_id, delta_paise)`
