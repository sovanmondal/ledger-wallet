# LedgerWallet — Demo Video Cheat-Sheet (run sheet)

A ~6–8 minute screen recording. Goal: show it's **live, correct under load, observable, and well-reasoned**.
Keep two windows ready: (1) a **browser** on the live URL + repo, (2) a **terminal** in the project dir.

- Live URL:  https://ledger-wallet-latest.onrender.com/
- Repo:      https://github.com/sovanmondal/ledger-wallet
- Terminal:  cd into the `ledger-wallet` folder

## BEFORE you hit record (setup, ~30s)
1. Warm the service (free instance sleeps): 
   `curl https://ledger-wallet-latest.onrender.com/healthz`
2. Open the log view you'll show (Render Logs tab, or Better Stack live tail once set up).
3. Have this file open to read talking points.

---

## RUN SHEET

### 0:00 — Intro (30s)
Say: "This is LedgerWallet — a wallet and P2P transfer service. It's live on free infrastructure for ₹0.
The point isn't features; it's that money stays **correct under concurrency and failure**. Go + PostgreSQL,
money is always integer paise, and every invariant is enforced in the database."

### 0:30 — The repo & container hygiene (60s)
Show the GitHub repo. Point at:
- `Dockerfile` — "multi-stage, **distroless, runs as non-root**, has a `HEALTHCHECK`, ~13.6MB image."
- `docker-compose.yml` — "app + Postgres come up with one command."
- Commit history — "incremental, human commits."
Say: "It's genuinely containerized and deployed — not a design on paper."

### 1:30 — The live dashboard (60s)
Open https://ledger-wallet-latest.onrender.com/ 
- Register a user → Create wallet (use the **copy button** for the id) → Top up (e.g. 100000 paise = ₹1,000).
- Do one transfer to another wallet.
Say: "Money shows as rupees for readability, but it's stored and moved as **integer paise** — never floats.
The live metrics panel on the right updates from `/metrics`."

### 2:30 — The invariants under load — THE MAIN EVENT (2–3 min)
In the terminal, run the burst against the LIVE url and narrate as it prints:
```
BASE_URL=https://ledger-wallet-latest.onrender.com ./scripts/burst.sh
```
Narrate each gate as it passes:
- **Gate 1 — race-free get-or-create:** "50 concurrent create-wallet calls → **exactly one wallet**.
  Mechanism: unique constraint on user_id plus INSERT … ON CONFLICT DO NOTHING."
- **Gate 2 — exactly-once:** "30 concurrent transfers with the **same idempotency key** → **one** debit and
  one credit, identical responses; and the same key with a **different body** → **409**. The key is
  committed in the **same transaction** as the money movement, so there's no double-charge."
- **Gate 3 — conservation + no-overdraft:** "300 concurrent transfers, including A→B and B→A at the same
  time and some that would overdraw. **Total is unchanged**, **no negative balances**, overdrawing ones
  decline cleanly. The debit is an atomic conditional UPDATE, and both rows are locked in a fixed order
  so the cross can't deadlock."
- **Reversal probe:** "Reversing twice with the same key → **one refund**; reversing an already-reversed
  transfer → **409**."
End on the green: **"ALL GATES PASSED."**

### 5:00 — Observability (60–90s)
- Switch to the **logs** view and run a small action (or re-run part of the burst). Point at a
  `transfer.declined_insufficient_funds` line and read its **correlation_id** — "every request is traceable."
- Open `https://ledger-wallet-latest.onrender.com/metrics` (or the UI panel). Point at the **domain
  counters** (transfers created / declined / idempotent replays / conflicts / reversals) and the
  **conservation gauge**. "Not a default metrics dump — these are the business events."

### 6:30 — The reasoning (60s)
Open `docs/WRITEUP.md`. Hit the highlights:
- "Simplest-correct mechanism: **conditional UPDATE**. I rejected `SELECT … FOR UPDATE` (extra locking
  and round-trips) and `SERIALIZABLE` (needs a retry loop) — overkill here."
- "Idempotency lives on a unique key committed **in the same transaction** as the ledger write."
- "It's a money workload, so I chose **consistency over availability** (single Postgres primary)."
- "AI usage disclosed: I directed the architecture and the mechanism; I let the AI decide code structure
  and the exact SQL."

### 7:30 — Close (15s)
"Everything you saw runs on free tiers for ₹0. Repo and live URL are in the description. Thanks."

---

## DEBRIEF Q&A — quick answers (in case they ask live)
- **"A→B and B→A at the same instant?"** → "Both transactions lock the two rows in the **same order
  (by wallet id)**, so no deadlock. And the conditional UPDATE already does the check+write atomically."
- **"Where is the idempotency key's uniqueness committed?"** → "**Same transaction** as the ledger
  movement — otherwise there's a TOCTOU window and you'd double-apply under a retry storm."
- **"Conditional UPDATE vs SELECT FOR UPDATE vs SERIALIZABLE?"** → "Conditional UPDATE: fewest moving
  parts that is correct. FOR UPDATE needs manual sorted locking + round-trips; SERIALIZABLE needs a
  40001 retry loop. I picked the simplest correct one."
- **"Show a declined log line and trace its id."** → pull it live from the logs by correlation_id.

## R3 live add-on (already built, so it's a redeploy, not a scramble)
`POST /transfers/{id}/reverse` already exists: reuses the same conditional-debit primitive with roles
swapped, its own idempotency key, and `UNIQUE(reverses_transfer_id)` blocks double-refund. If asked to
"add reversal live," it's already there — demo it and, if needed, make a tiny visible tweak + redeploy.
