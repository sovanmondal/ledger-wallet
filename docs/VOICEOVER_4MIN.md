# LedgerWallet — Final Voice-Over (~4 min, product-style)

Target: **≤ 4:00** (4:20 hard max). Tone: describe the product, no exercise/meta framing.
Warm the API first: `curl https://ledger-wallet-latest.onrender.com/healthz`

Format: **[SHOW]** = on screen · **SAY** = read aloud. The FULL read-through is at the bottom for one take.

---

**[SHOW]** Live dashboard `/`
**SAY:** "This is LedgerWallet — a wallet and peer-to-peer money-transfer service. Whenever money moves in
an app, four things have to hold: you get one wallet even if you tap twice; a payment applies exactly once
even on retries; money is never created or lost; and a balance never goes negative. LedgerWallet guarantees
all four — even when hundreds of requests hit the same account at once. It's built in Go on PostgreSQL,
it's live, and every amount is whole paise — never decimals that can drift."

**[SHOW]** Repo — Dockerfile + docker-compose
**SAY:** "It ships as a small container: a multi-stage build on a distroless base that runs as a non-root
user, with a healthcheck — about thirteen megabytes. One command brings up the app and Postgres together."

**[SHOW]** UI — register, create wallet, top up
**SAY:** "Here's the app. I register a user, create my wallet, and add funds. Balances read in rupees, but
under the hood every value is integer paise."

**[SHOW]** Terminal — run: `BASE_URL=https://ledger-wallet-latest.onrender.com ./scripts/burst.sh`
**SAY:** "Now the real test — correctness under load.
Fifty simultaneous create-wallet calls for a new user — exactly one wallet. A unique constraint lets the
database pick the winner, not the app.
Thirty identical transfers fired at once with the same key — one debit, one credit, identical responses;
the same key with a different amount is rejected as a conflict. The key is committed in the same
transaction as the money movement, so there's no way to double-charge.
Then three hundred concurrent transfers among five wallets — both directions at once, some trying to
overspend. The total never changes, nothing goes negative, and overdrafts decline cleanly. The debit is a
single conditional update, and the rows lock in a fixed order, so opposing transfers never deadlock.
And refunds: reversing twice returns the money once; reversing an already-refunded payment is rejected."
**[SHOW]** the green result
**SAY:** "Every guarantee holds — against the live URL."

**[SHOW]** Better Stack live tail (or logs)
**SAY:** "Every request emits structured JSON with a correlation id, streamed live — created, debited,
credited, declined, replayed, reversed — so any transaction can be traced end to end."

**[SHOW]** `/metrics` (or UI panel)
**SAY:** "Metrics expose request rate, p99 latency, and error rate, plus the business counters that matter
and a running total-balance gauge."

**[SHOW]** `docs/WRITEUP.md`
**SAY:** "The design is deliberately simple: a conditional update is the least machinery that's correct —
heavier row-locking and serializable isolation were unnecessary here. Idempotency lives on a unique key
committed together with the ledger write. And because it's money, I chose consistency over availability —
one Postgres primary as the source of truth."

**SAY (close):** "LedgerWallet — four guarantees, holding under load, genuinely deployed and observed.
Repo and live URL are in the description."

=======================================================================
## FULL READ-THROUGH (one take)
=======================================================================

This is LedgerWallet — a wallet and peer-to-peer money-transfer service. Whenever money moves in an app,
four things have to hold: you get one wallet even if you tap twice; a payment applies exactly once even on
retries; money is never created or lost; and a balance never goes negative. LedgerWallet guarantees all
four — even when hundreds of requests hit the same account at once. It's built in Go on PostgreSQL, it's
live, and every amount is whole paise — never decimals that can drift.

It ships as a small container: a multi-stage build on a distroless base that runs as a non-root user, with
a healthcheck — about thirteen megabytes. One command brings up the app and Postgres together.

Here's the app. I register a user, create my wallet, and add funds. Balances read in rupees, but under the
hood every value is integer paise.

Now the real test — correctness under load. Fifty simultaneous create-wallet calls for a new user — exactly
one wallet; a unique constraint lets the database pick the winner, not the app. Thirty identical transfers
fired at once with the same key — one debit, one credit, identical responses; the same key with a different
amount is rejected as a conflict, because the key is committed in the same transaction as the money
movement. Then three hundred concurrent transfers among five wallets — both directions at once, some trying
to overspend: the total never changes, nothing goes negative, and overdrafts decline cleanly. The debit is
a single conditional update, and rows lock in a fixed order, so opposing transfers never deadlock. And
refunds — reversing twice returns the money once; reversing an already-refunded payment is rejected. Every
guarantee holds, against the live URL.

Every request emits structured JSON with a correlation id, streamed live — created, debited, credited,
declined, replayed, reversed — so any transaction can be traced end to end. Metrics expose request rate,
p99 latency, and error rate, plus the business counters that matter and a running total-balance gauge.

The design is deliberately simple: a conditional update is the least machinery that's correct — heavier
row-locking and serializable isolation were unnecessary here. Idempotency lives on a unique key committed
together with the ledger write. And because it's money, I chose consistency over availability — one Postgres
primary as the source of truth.

LedgerWallet — four guarantees, holding under load, genuinely deployed and observed. Repo and live URL are
in the description.

---
NOTE: The AI directed-vs-decided disclosure required by the brief is in docs/WRITEUP.md, so it does not
need to be spoken in the video. If you want one line, add: "Built with an AI assistant — I directed the
architecture and the correctness mechanism; it wrote much of the code."
