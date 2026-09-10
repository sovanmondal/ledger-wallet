# LedgerWallet — Design Write-up (one page)

## Data model
Four tables (`internal/store/migrations/0001_init.sql`). Money is **integer paise** (`bigint`) everywhere.

- **`users`** `(id, username unique, token unique)` — bearer-token identity.
- **`wallets`** `(id, user_id UNIQUE, balance_paise bigint CHECK >= 0)` — the `UNIQUE(user_id)` is what
  makes get-or-create race-free; the `CHECK` is a last-line no-overdraft guard.
- **`transfers`** `(id, idempotency_key UNIQUE, from, to, amount_paise CHECK > 0, kind, status,
  decline_reason, reverses_transfer_id UNIQUE, request_fingerprint, ...)` — one row per money movement.
- **`ledger_entries`** `(transfer_id, wallet_id, delta_paise)` — append-only double entry. Every completed
  movement writes **two rows summing to zero**, so conservation is *provable*: `SELECT sum(delta_paise)` is
  always `0`. Top-ups are modelled as transfers **from a genesis treasury wallet**, so the global sum of
  all balances is invariant across funding too.

## Simplest-correct mechanism for conservation + no-overdraft
A transfer runs in **one transaction** (`READ COMMITTED`):
1. **Atomic conditional debit:** `UPDATE wallets SET balance = balance - $amt WHERE id = $from AND balance >= $amt`.
   `RowsAffected = 0` ⇒ the debit would overdraw ⇒ **clean decline**, never a partial apply. The read,
   the check, and the write are one statement — no lost-update window.
2. **Credit:** `UPDATE ... SET balance = balance + $amt WHERE id = $to`.
3. Both balance UPDATEs are applied **in ascending wallet-id order**, so two transfers touching the same
   pair (A→B and B→A at once) always acquire the row locks in the *same* global order ⇒ **no deadlock**.
4. On decline we `ROLLBACK TO SAVEPOINT` (undoing any credit already applied) but keep the transfer header,
   then commit it as `declined` — so the outcome is durable and idempotent.

**Why this is the simplest correct thing, and what I rejected:**
- **`SELECT … FOR UPDATE` then update** — correct, but needs me to *manually* lock both rows in sorted
  order and adds round-trips. The conditional `UPDATE` already gets the row lock *and* does the check
  atomically, so explicit `FOR UPDATE` is redundant work.
- **`SERIALIZABLE` isolation** — correct, but pushes conflict handling onto a **retry loop** for
  `40001 serialization_failure`, which is more code and worse under the exact contention the graders
  fire. Overkill for a single-row-per-side debit/credit.
- **Read-modify-write in app memory** — rejected outright: the classic lost-update that creates/destroys
  money under concurrency.

## Where idempotency lives
On the **`transfers.idempotency_key` UNIQUE constraint**, and the row is inserted **in the same
transaction as the balance movement** (`executeMovement` in `internal/store/transfer.go`). The flow is
`INSERT ... ON CONFLICT (idempotency_key) DO NOTHING RETURNING id`:
- **Winner** proceeds to the movement and commits key + balances together.
- **Concurrent duplicate** blocks on the unique index until the winner commits, then sees 0 rows, re-reads
  the committed transfer, and returns it — **exactly one debit/credit** across K concurrent retries. No
  TOCTOU, because the key is never checked in a *separate* transaction before the debit.
- **Same key, different body** → the stored `request_fingerprint` (hash of from/to/amount/kind) mismatches
  → **409**, not a silent second debit.
- **Reversal** reuses the same primitive with roles swapped and its own key; `UNIQUE(reverses_transfer_id)`
  means a *second, different* reversal of the same transfer loses the race → **409 already-reversed**
  (no double refund). If the recipient already spent the funds, the reversal's conditional debit declines
  cleanly (policy: decline rather than force a negative balance).

## Consistency vs availability (it's money)
**Consistency (CP).** A single Postgres primary is the linearizable source of truth; correctness invariants
are DB constraints, so they can't be violated even under a stampede. The conscious trade-off: if the
primary is unavailable, the service returns errors rather than accepting writes it can't prove correct —
we **give up availability during a partition** rather than risk double-spend or lost money. For a wallet,
a rejected request is recoverable; a created-or-destroyed rupee is not.

## AI: directed vs decided
- **Directed (human decided the approach, AI implemented):** the tech stack (Go + Neon + Vercel) and the
  rejection of Java/serverless for free-tier fit; the decision to model conservation with a treasury +
  double-entry ledger; the decision to build the reversal endpoint up front; the choice of conditional
  `UPDATE` as the mechanism.
- **Let AI decide (accepted its design):** the concrete code structure and package layout; the exact SQL
  and the `SAVEPOINT` decline-rollback technique; the deadlock-avoidance-by-sorted-id detail; middleware
  design (correlation id, metrics route labels); Prometheus metric names and the UI.

## Free-tier cost
**₹0, no credit card.** Neon Postgres free (no card, permanent), Render free web service (no card; sleeps
when idle), Vercel free static hosting. Verified current terms before choosing. Fly.io was rejected
because it now requires a card.
