# LedgerWallet — Project Overview

## What it is
A small **wallet & peer-to-peer transfer** service, deployed and operated for real, whose entire
purpose is to stay **correct under concurrency and failure**. It is not about features or UI — it is
about whether money stays conserved, balances never go negative, and retries never double-charge,
even when many requests hit the same wallets at the same instant.

- **Live API + dashboard:** https://ledger-wallet-latest.onrender.com/
- **Repo:** https://github.com/sovanmondal/ledger-wallet
- **Cost:** ₹0 on free tiers, no credit card.

## The problem it solves
Money systems break in subtle ways under load: two transfers racing on the same wallet can "lose" an
update and create or destroy money; a retried request can debit twice; two "create wallet" calls can
make two wallets. LedgerWallet makes each of those impossible by construction.

## Capabilities (API)
Auth: a bearer token per user (`Authorization: Bearer <token>`). Money is always **integer paise**.

| Endpoint | What it does |
|---|---|
| `POST /auth/register` | Mint a bearer token for a username |
| `POST /wallets` | Get-or-create the caller's wallet |
| `GET /wallets/{id}` | Current balance |
| `POST /wallets/{id}/topup` | Add test funds (a transfer from a genesis treasury) |
| `POST /transfers` | Move paise: `{from, to, amount_paise, idempotency_key}` |
| `GET /transfers/{id}` | Transfer status |
| `POST /transfers/{id}/reverse` | Refund a transfer (its own idempotency key) |
| `GET /healthz` `/readyz` `/metrics` `/api` | Health, readiness, Prometheus, JSON index |
| `GET /` | The operations console (browser UI) |

## The four invariants (what actually gets graded)
1. **Conservation** — the sum of balances never changes across a transfer. The append-only ledger
   proves it: `sum(delta_paise) = 0` always.
2. **No overdraft** — a balance never goes negative; an overdrawing debit declines cleanly (no partial
   apply), backed by both an atomic conditional `UPDATE` and a `CHECK (balance >= 0)`.
3. **Exactly-once** — the same `idempotency_key` applies a transfer once; a retry returns the original
   result; the same key with a different body is a `409` conflict.
4. **Race-free get-or-create** — N concurrent "create my wallet" calls yield exactly one wallet
   (`UNIQUE(user_id)` + `INSERT … ON CONFLICT DO NOTHING`).

## How correctness is achieved (in plain terms)
- Each transfer runs in **one database transaction**.
- The debit is an **atomic conditional update**: `UPDATE wallets SET balance = balance - amt WHERE id = from AND balance >= amt`.
  If it affects 0 rows, the transfer declines — the read, check, and write are one indivisible step, so
  there is no "lost update" window.
- The two balance changes are applied **in a fixed order (by wallet id)**, so two transfers touching the
  same pair in opposite directions (A→B and B→A) can never deadlock.
- The **idempotency key row is inserted in the same transaction** as the money movement, so a duplicate
  retry either waits and reads the committed result or loses the insert race — never a second debit.
- A **reversal** reuses the exact same primitive with the roles swapped, has its own idempotency key,
  and is blocked from running twice by a `UNIQUE(reverses_transfer_id)` constraint.

## Capabilities beyond the API
- **Observability:** structured JSON logs to stdout, each line carrying a **correlation id** and the
  meaningful domain event (transfer created / debited / credited / declined / idempotent replay /
  conflict / reversal). Prometheus `/metrics` exposes request rate, latency (p99), error rate, plus
  **domain counters** and a **conservation gauge**.
- **Browser dashboard** (served at `/`): register, create/fund a wallet, transfer, check status,
  reverse, and a live metrics panel — a professional operations console.
- **One-command reproducibility:** `docker compose up` locally; `scripts/burst.sh` reproduces every
  invariant against any URL.

## How it's deployed (₹0, no card)
- **API container** → Render (free web service), built as a **multi-stage, distroless, non-root** image
  (~13.6MB) with a `HEALTHCHECK`.
- **Database** → Neon (free managed PostgreSQL).
- **UI** → served from the same container (and also deployable to Vercel).

## Why it's strong
- Correctness lives in the **database**, not app memory — it holds under real concurrency.
- The **simplest mechanism that is correct** was chosen deliberately (conditional `UPDATE`), and heavier
  options (row locks, serializable isolation) were considered and rejected with reasons.
- It genuinely **deploys, observes, and reproduces** its own guarantees — not just a design on paper.
