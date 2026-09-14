# LedgerWallet — Story Hook + Per-Function One-Liners (short)

Use this as a ~45s **intro** placed BEFORE your existing technical voice-over. You do NOT need to
re-record the rest — just add this hook (and optionally the four one-liners over each demo).

## HOOK (record this, ~40–50s)
"Every time you send money in an app, four quiet things have to go right — or someone loses money, or
gets charged twice. LedgerWallet is a small wallet-and-transfer service built to guarantee exactly those
four things, even when hundreds of requests hit the same account at the same instant. It's live, it's on
free infrastructure, and money is always counted in whole paise — never decimals that can drift. Let me
show you the four, under real load."

## THE FOUR — one line each (narrate over each gate)
1. Race-free get-or-create:
   "One — your phone retries 'create wallet' on a flaky network. You should get one wallet, not five.
   Fifty simultaneous taps → exactly one wallet."

2. Exactly-once transfer:
   "Two — you tap Pay once, but the app retries. You must be charged once. Thirty identical retries → a
   single debit; the same key with a different amount → rejected with a 409, never a second charge."

3. Conservation + no overdraft:
   "Three — money can't be created or destroyed, and you can't spend what you don't have. Hundreds of
   transfers both directions, some overdrawing → the total never changes and nothing goes negative."

4. Reversal / refund:
   "Four — a refund returns the exact amount, once. Reverse twice → one refund; reverse an
   already-refunded payment → rejected."

## CLOSE (short)
"Four guarantees, holding under load, genuinely deployed and observable. Repo and live URL are in the
description."

---
Tip: keep your existing deep-dive narration (mechanism, idempotency-in-same-transaction, CAP choice,
AI disclosure) AFTER the demo — the hook pulls people in, the demo proves it, the deep-dive earns trust.
