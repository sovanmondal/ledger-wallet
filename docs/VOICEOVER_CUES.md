# LedgerWallet — Voice-Over Cue Sheet (copy-paste)

Read the **SAY** blocks aloud; do the **SHOW** action on screen. ~6–7 min. Warm the API first:
`curl https://ledger-wallet-latest.onrender.com/healthz`

---

**SHOW:** Live dashboard — https://ledger-wallet-latest.onrender.com/

**SAY:**
This is LedgerWallet — a wallet and peer-to-peer transfer service, deployed and running on free infrastructure for zero rupees. This round isn't about features or UI. It's about whether the money stays correct under concurrency and failure, and whether it's genuinely deployed and observable. It's built in Go on PostgreSQL, and money is always integer paise — never floating point.

---

**SHOW:** GitHub repo — open Dockerfile, docker-compose.yml, then the commit list

**SAY:**
Here's the public repo. The Dockerfile is multi-stage: it compiles a static Go binary and ships it on a distroless base that runs as a non-root user, with a healthcheck. The final image is about thirteen megabytes. docker-compose brings up the app and Postgres with a single command. And the commit history is incremental — scaffold, database, core logic, HTTP, observability, deploy.

---

**SHOW:** In the UI — register a user, create wallet (click the copy button), top up 100000

**SAY:**
Quick tour. I register a user to get a bearer token, create my wallet — which is race-free, I'll prove that in a moment — and top it up. Balances show in rupees for readability, but every amount is stored and moved as integer paise. On the right is a live metrics panel reading from the metrics endpoint.

---

**SHOW:** Terminal — run:
`BASE_URL=https://ledger-wallet-latest.onrender.com ./scripts/burst.sh`

**SAY (Gate 1, as it prints):**
First — race-free get-or-create. Fifty concurrent create-wallet calls for a brand-new user, and we get exactly one wallet. That's a unique constraint on the user id plus insert-on-conflict-do-nothing — the database picks the winner, not the application.

**SAY (Gate 2):**
Next — exactly-once. Thirty concurrent transfers with the same idempotency key: one debit, one credit, identical responses. And the same key with a different body returns a 409, not a second debit. The key is inserted in the same transaction as the money movement, so there's no window to double-apply.

**SAY (Gate 3):**
Now conservation under contention. Three hundred concurrent transfers among five wallets — including A-to-B and B-to-A at the same instant, plus some that would overdraw. The total is unchanged, no balance goes negative, and the overdrawing transfers decline cleanly. The debit is a single conditional update — subtract only where balance is at least the amount — and both rows are locked in a fixed order by wallet id, so opposite-direction transfers can never deadlock.

**SAY (Reversal):**
And a reversal probe: reversing twice with the same key gives one refund, and reversing an already-reversed transfer returns 409.

**SHOW:** the final green line
**SAY:**
All gates passed — against the live public URL.

---

**SHOW:** Better Stack live tail — click a declined line, copy its correlation_id, search it

**SAY:**
Logs are structured JSON with a correlation id per request, streamed publicly. Here are the domain events — created, debited, credited, declined, idempotent replay, conflict, reversal. I'll take a declined transfer, grab its correlation id, and trace the whole request end to end.

---

**SHOW:** https://ledger-wallet-latest.onrender.com/metrics (or the UI metrics panel)

**SAY:**
Metrics are in Prometheus format: request rate and status for the error rate, a latency histogram for p99, and the domain counters that matter — transfers created, declined for insufficient funds, idempotent replays, conflicts, and reversals — plus a conservation gauge for the total balance. Not a default dump — these are the business events.

---

**SHOW:** docs/WRITEUP.md

**SAY:**
On the design: I chose the simplest mechanism that's correct — a conditional update. I considered and rejected select-for-update, which needs manual sorted locking and extra round-trips, and serializable isolation, which needs a retry loop — both overkill here. Idempotency lives on a unique key committed in the same transaction as the ledger write. Because this is money, I chose consistency over availability — a single Postgres primary as the source of truth. And to be honest about AI use: I directed the architecture and the correctness mechanism; I let the assistant decide the code structure and the exact SQL. The whole thing runs on free tiers for zero rupees, no card.

---

**SAY (close):**
That's LedgerWallet — correct under load, genuinely deployed and observed. The repo and live URL are in the description. Thanks for watching.

===================================================================
## FULL NARRATION — read straight through (for a one-take voiceover)
===================================================================

This is LedgerWallet — a wallet and peer-to-peer transfer service, deployed and running on free infrastructure for zero rupees. This round isn't about features or UI. It's about whether the money stays correct under concurrency and failure, and whether it's genuinely deployed and observable. It's built in Go on PostgreSQL, and money is always integer paise — never floating point.

Here's the public repo. The Dockerfile is multi-stage: it compiles a static Go binary and ships it on a distroless base that runs as a non-root user, with a healthcheck. The final image is about thirteen megabytes. docker-compose brings up the app and Postgres with a single command, and the commit history is incremental.

Quick tour. I register a user to get a bearer token, create my wallet — which is race-free — and top it up. Balances show in rupees for readability, but every amount is stored and moved as integer paise.

Now the real test — correctness under load. First, race-free get-or-create: fifty concurrent create-wallet calls for a brand-new user, and we get exactly one wallet — a unique constraint plus insert-on-conflict-do-nothing.

Next, exactly-once: thirty concurrent transfers with the same idempotency key give one debit, one credit, identical responses; and the same key with a different body returns a 409, not a second debit — because the key is committed in the same transaction as the money movement.

Now conservation under contention: three hundred concurrent transfers among five wallets, including A-to-B and B-to-A at once, plus some that would overdraw. The total is unchanged, no balance goes negative, and overdraws decline cleanly. The debit is a single conditional update, and both rows are locked in a fixed order by wallet id, so opposite-direction transfers can't deadlock.

And a reversal probe: reversing twice with the same key gives one refund; reversing an already-reversed transfer returns 409. All gates passed — against the live public URL.

Logs are structured JSON with a correlation id per request, streamed publicly — created, debited, credited, declined, idempotent replay, conflict, reversal. I can take any declined transfer and trace its correlation id end to end.

Metrics are Prometheus format: request rate and status for error rate, a latency histogram for p99, and the domain counters that matter, plus a conservation gauge.

On the design, I chose the simplest mechanism that's correct — a conditional update — and rejected select-for-update and serializable isolation as overkill. Idempotency lives on a unique key committed in the same transaction as the ledger write. Because this is money, I chose consistency over availability. And to disclose AI use honestly: I directed the architecture and the mechanism; I let the assistant decide code structure and exact SQL. It all runs on free tiers for zero rupees.

That's LedgerWallet — correct under load, genuinely deployed and observed. Thanks for watching.
