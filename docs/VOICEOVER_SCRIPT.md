# LedgerWallet — Voice-Over Script (word-for-word, ~6–7 min)

Format: **[SHOW]** = what's on screen · **SAY** = read this aloud. No fixed time limit is specified in
the brief; target ~6–7 minutes. Speak calmly; the burst passing is the money shot.

## Before recording (not on camera)
- Warm the API: `curl https://ledger-wallet-latest.onrender.com/healthz`
- Tabs open: (1) the live dashboard `/`, (2) the GitHub repo, (3) Better Stack live tail.
- Terminal in the project folder, ready to paste the burst command.

---

### Scene 1 — Intro  [0:00–0:30]
**[SHOW]** The live dashboard at https://ledger-wallet-latest.onrender.com/
**SAY:** "This is LedgerWallet — a wallet and peer-to-peer transfer service. It's deployed and running
on free infrastructure for zero rupees. The goal of this round isn't features or UI — it's whether the
money stays correct under concurrency and failure, and whether it's genuinely deployed and observable.
It's built in Go on PostgreSQL, and money is always integer paise — never floating point."

### Scene 2 — Repo & container hygiene  [0:30–1:15]
**[SHOW]** GitHub repo; open the `Dockerfile`, then `docker-compose.yml`, then the commit list.
**SAY:** "Here's the public repo. The Dockerfile is multi-stage — it compiles a static Go binary and ships
it on a distroless base that runs as a non-root user, with a healthcheck. The final image is about
thirteen megabytes. docker-compose brings up the app and Postgres with a single command. The commit
history is incremental — scaffold, database, core logic, HTTP, observability, deploy."

### Scene 3 — Quick dashboard tour  [1:15–2:00]
**[SHOW]** In the UI: Register a user → Create wallet (click the copy button on the id) → Top up 100000.
**SAY:** "Quick tour: I register a user to get a bearer token, create my wallet — this is race-free, I'll
prove that in a second — and top it up. Notice balances show in rupees for readability, but every amount
is stored and moved as integer paise. On the right is a live metrics panel reading from the metrics
endpoint."

### Scene 4 — THE INVARIANTS UNDER LOAD  [2:00–4:30]  ← the main event
**[SHOW]** Terminal. Run:
`BASE_URL=https://ledger-wallet-latest.onrender.com ./scripts/burst.sh`
Narrate as each gate prints:
**SAY (Gate 1):** "First, race-free get-or-create — fifty concurrent create-wallet calls for a brand-new
user. Exactly one wallet. That's a unique constraint on the user id plus insert-on-conflict-do-nothing —
the database decides the winner, not the application."
**SAY (Gate 2):** "Next, exactly-once. Thirty concurrent transfers with the same idempotency key —
one debit, one credit, identical responses. And the same key with a different body returns a 409, not a
second debit. The key is inserted in the same transaction as the money movement, so there's no window to
double-apply."
**SAY (Gate 3):** "Now conservation under contention — three hundred concurrent transfers among five
wallets, including A-to-B and B-to-A at the same instant, plus some that would overdraw. The total is
unchanged, no balance goes negative, and the overdrawing transfers decline cleanly. The debit is a single
conditional update — subtract only where balance is at least the amount — and both rows are locked in a
fixed order by wallet id, so the opposite-direction pair can never deadlock."
**SAY (Reversal):** "And a reversal probe — reversing twice with the same key gives one refund, and
reversing an already-reversed transfer returns 409."
**[SHOW]** the final green line.
**SAY:** "All gates passed — against the live public URL."

### Scene 5 — Logs streaming with correlation ids  [4:30–5:30]
**[SHOW]** Better Stack live tail (logs from the burst are visible). Click a
`transfer.declined_insufficient_funds` line; copy its `correlation_id`; search it.
**SAY:** "Logs are structured JSON with a correlation id per request, streamed publicly. Here are the
domain events — created, debited, credited, declined, idempotent replay, conflict, reversal. I'll take a
declined transfer, grab its correlation id, and trace the whole request end to end."

### Scene 6 — Metrics  [5:30–6:15]
**[SHOW]** `https://ledger-wallet-latest.onrender.com/metrics` (or the UI panel).
**SAY:** "Metrics are Prometheus format: request rate and status for error rate, a latency histogram for
p99, and the domain counters that matter — transfers created, declined for insufficient funds, idempotent
replays, conflicts, and reversals — plus a conservation gauge for the total balance. Not a default dump —
these are the business events."

### Scene 7 — Reasoning + honesty  [6:15–7:00]
**[SHOW]** `docs/WRITEUP.md`.
**SAY:** "On the design: I chose the simplest mechanism that's correct — a conditional update. I
considered and rejected select-for-update, which needs manual sorted locking and extra round-trips, and
serializable isolation, which needs a retry loop — both are overkill here. Idempotency lives on a unique
key committed in the same transaction as the ledger write. Because this is money, I chose consistency over
availability — a single Postgres primary as the source of truth. And to disclose AI use honestly: I
directed the architecture and the correctness mechanism; I let the assistant decide code structure and the
exact SQL. The whole thing runs on free tiers for zero rupees, no card."

### Scene 8 — Close  [7:00–7:15]
**SAY:** "That's LedgerWallet — correct under load, genuinely deployed and observed. The repo and live URL
are in the description. Thanks for watching."

---

## If asked to "add a feature live" (R3)
The reversal endpoint already exists: `POST /transfers/{id}/reverse`. It reuses the same conditional-debit
primitive with roles swapped, has its own idempotency key, and a unique constraint blocks double-refund.
Demo it live; if they want a visible change, tweak and redeploy the image.
