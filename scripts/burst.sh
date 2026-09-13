#!/usr/bin/env bash
# LedgerWallet correctness burst — reproduces every hard gate against a deployed URL.
# Portable across macOS (BSD xargs) and Linux (GNU xargs).
#
#   BASE_URL=https://your-app.onrender.com ./scripts/burst.sh
#
# Gate 1: race-free get-or-create   (50 concurrent POST /wallets -> exactly 1 wallet)
# Gate 2: idempotent exactly-once   (30 concurrent same-key transfers -> 1 debit/credit)
# Gate 3: conservation + no-overdraft under contention (incl. A<->B cross + overdraws)
# Probe : reversal double-fire + reverse-already-reversed
#
# Dependencies: bash, curl, coreutils (no jq / python required).

set -u

BASE="${BASE_URL:-http://localhost:8080}"
CONC_WALLETS="${CONC_WALLETS:-50}"
CONC_IDEMP="${CONC_IDEMP:-30}"
CONTENTION_JOBS="${CONTENTION_JOBS:-300}"
TMP="$(mktemp -d)"
FAILURES=0
export BASE TMP

trap 'rm -rf "$TMP"' EXIT

green() { printf "\033[32m%s\033[0m\n" "$1"; }
red()   { printf "\033[31m%s\033[0m\n" "$1"; }
hd()    { printf "\n\033[1m=== %s ===\033[0m\n" "$1"; }
pass()  { green "PASS: $1"; }
fail()  { red "FAIL: $1"; FAILURES=$((FAILURES+1)); }

# --- flat-JSON field extractors (no external deps) ---
jstr() { grep -o "\"$1\":\"[^\"]*\"" | head -1 | sed -e "s/\"$1\"://" -e 's/^"//' -e 's/"$//'; }
jnum() { grep -o "\"$1\":-\{0,1\}[0-9][0-9]*" | head -1 | sed "s/\"$1\"://"; }

register()   { curl -s -X POST "$BASE/auth/register" -H 'Content-Type: application/json' -d "{\"username\":\"$1\"}"; }
mk_wallet()  { curl -s -X POST "$BASE/wallets" -H "Authorization: Bearer $1"; }
get_wallet() { curl -s "$BASE/wallets/$2" -H "Authorization: Bearer $1"; }
topup()      { curl -s -X POST "$BASE/wallets/$2/topup" -H "Authorization: Bearer $1" -H 'Content-Type: application/json' -d "{\"amount_paise\":$3}"; }
balance()    { # retry transient empty reads (free hosts can briefly return an empty body)
  local b
  for _ in 1 2 3 4; do
    b="$(get_wallet "$1" "$2" | jnum balance_paise)"
    [ -n "$b" ] && { printf '%s' "$b"; return; }
    sleep 0.4
  done
  printf ''
}

# --- portable concurrency helpers (BSD & GNU xargs both support -P/-n from stdin) ---
cat > "$TMP/h_wallet.sh" <<'EOF'
curl -s -X POST "$BASE/wallets" -H "Authorization: Bearer $TOK" -o "$TMP/g1_$1.json"
EOF
cat > "$TMP/h_idem.sh" <<'EOF'
curl -s -X POST "$BASE/transfers" -H "Authorization: Bearer $TA" -H "Content-Type: application/json" \
  -d "{\"from\":\"$WA\",\"to\":\"$WB\",\"amount_paise\":$AMT,\"idempotency_key\":\"$KEY\"}" -o "$TMP/g2_$1.json"
EOF
cat > "$TMP/h_job.sh" <<'EOF'
curl -s -X POST "$BASE/transfers" -H "Authorization: Bearer $1" -H "Content-Type: application/json" \
  -d "{\"from\":\"$2\",\"to\":\"$3\",\"amount_paise\":$4,\"idempotency_key\":\"$5\"}" >/dev/null
EOF
cat > "$TMP/h_reverse.sh" <<'EOF'
curl -s -X POST "$BASE/transfers/$TID/reverse" -H "Authorization: Bearer $TS" -H "Content-Type: application/json" \
  -d "{\"idempotency_key\":\"$RKEY\"}" -o "$TMP/rev_$1.json"
EOF

hd "Warm-up"
curl -s "$BASE/healthz" >/dev/null && green "service reachable at $BASE" || { red "cannot reach $BASE"; exit 1; }

# =====================================================================================
hd "GATE 1 — race-free get-or-create ($CONC_WALLETS concurrent POST /wallets, fresh user)"
TOK="$(register "race_user_$(date +%s)_$RANDOM" | jstr token)"; export TOK
seq 1 "$CONC_WALLETS" | xargs -P"$CONC_WALLETS" -n1 bash "$TMP/h_wallet.sh"
DISTINCT="$(cat "$TMP"/g1_*.json | grep -o '"id":"[^"]*"' | sort -u | wc -l | tr -d ' ')"
echo "distinct wallet ids returned: $DISTINCT (expected 1)"
[ "$DISTINCT" = "1" ] && pass "exactly one wallet under $CONC_WALLETS concurrent creates" || fail "got $DISTINCT wallets — race in get-or-create"

# =====================================================================================
hd "GATE 2 — idempotent exactly-once ($CONC_IDEMP concurrent same-key transfers)"
TA="$(register "idem_A_$(date +%s)_$RANDOM" | jstr token)"
TB="$(register "idem_B_$(date +%s)_$RANDOM" | jstr token)"
WA="$(mk_wallet "$TA" | jstr id)"
WB="$(mk_wallet "$TB" | jstr id)"
topup "$TA" "$WA" 1000000 >/dev/null
AMT=250
KEY="idem-$(date +%s)-$RANDOM"
export TA TB WA WB AMT KEY
A_BEFORE="$(balance "$TA" "$WA")"; B_BEFORE="$(balance "$TB" "$WB")"
seq 1 "$CONC_IDEMP" | xargs -P"$CONC_IDEMP" -n1 bash "$TMP/h_idem.sh"
TID_DISTINCT="$(cat "$TMP"/g2_*.json | grep -o '"id":"[^"]*"' | sort -u | wc -l | tr -d ' ')"
A_AFTER="$(balance "$TA" "$WA")"; B_AFTER="$(balance "$TB" "$WB")"
echo "distinct transfer ids: $TID_DISTINCT (expected 1)"
echo "A: $A_BEFORE -> $A_AFTER   B: $B_BEFORE -> $B_AFTER   (amount $AMT)"
[ "$TID_DISTINCT" = "1" ] && pass "single transfer id across $CONC_IDEMP retries" || fail "multiple transfer ids ($TID_DISTINCT) — idempotency broken"
[ "$A_AFTER" = "$((A_BEFORE - AMT))" ] && pass "exactly one debit applied" || fail "debit applied $((A_BEFORE - A_AFTER)) != $AMT"
[ "$B_AFTER" = "$((B_BEFORE + AMT))" ] && pass "exactly one credit applied" || fail "credit applied $((B_AFTER - B_BEFORE)) != $AMT"
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/transfers" -H "Authorization: Bearer $TA" \
  -H 'Content-Type: application/json' -d "{\"from\":\"$WA\",\"to\":\"$WB\",\"amount_paise\":$((AMT+1)),\"idempotency_key\":\"$KEY\"}")"
[ "$CODE" = "409" ] && pass "same key + different body -> 409" || fail "expected 409, got $CODE"

# =====================================================================================
hd "GATE 3 — conservation + no-overdraft under contention ($CONTENTION_JOBS concurrent)"
N=5
WALLETS=(); TOKENS=()
for i in $(seq 1 $N); do
  t="$(register "conc_${i}_$(date +%s)_$RANDOM" | jstr token)"
  w="$(mk_wallet "$t" | jstr id)"
  topup "$t" "$w" 100000 >/dev/null
  WALLETS+=("$w"); TOKENS+=("$t")
done
SUM_BEFORE=0
for i in $(seq 0 $((N-1))); do SUM_BEFORE=$((SUM_BEFORE + $(balance "${TOKENS[$i]}" "${WALLETS[$i]}"))); done
echo "sum before: $SUM_BEFORE paise"

JOBS="$TMP/jobs"; : > "$JOBS"
for j in $(seq 1 "$CONTENTION_JOBS"); do
  a=$((RANDOM % N)); b=$((RANDOM % N))
  while [ "$b" = "$a" ]; do b=$((RANDOM % N)); done
  if [ $((j % 3)) = 0 ]; then a=0; b=1; fi   # heavy A<->B cross to stress deadlock ordering
  if [ $((j % 6)) = 0 ]; then a=1; b=0; fi
  amt=$(( (RANDOM % 50) + 1 ))
  if [ $((j % 10)) = 0 ]; then amt=999999999; fi   # overdraw attempts (must decline)
  printf '%s %s %s %s %s\n' "${TOKENS[$a]}" "${WALLETS[$a]}" "${WALLETS[$b]}" "$amt" "g3-$j-$RANDOM" >> "$JOBS"
done
# stdin redirect + -n5 groups 5 args per call: portable on BSD & GNU xargs
xargs -P50 -n5 bash "$TMP/h_job.sh" < "$JOBS"

SUM_AFTER=0; NEG=0
for i in $(seq 0 $((N-1))); do
  bal="$(balance "${TOKENS[$i]}" "${WALLETS[$i]}")"
  SUM_AFTER=$((SUM_AFTER + bal))
  [ "$bal" -lt 0 ] && NEG=$((NEG+1))
  echo "  wallet $i balance: $bal"
done
echo "sum after:  $SUM_AFTER paise"
[ "$SUM_BEFORE" = "$SUM_AFTER" ] && pass "conservation held (sum unchanged)" || fail "conservation broken: $SUM_BEFORE -> $SUM_AFTER"
[ "$NEG" = "0" ] && pass "no negative balances" || fail "$NEG wallet(s) went negative"

# =====================================================================================
hd "PROBE — reversal: double-fire + reverse-already-reversed"
TS="$(register "rev_S_$(date +%s)_$RANDOM" | jstr token)"
TRR="$(register "rev_R_$(date +%s)_$RANDOM" | jstr token)"
WS="$(mk_wallet "$TS" | jstr id)"
WR="$(mk_wallet "$TRR" | jstr id)"
topup "$TS" "$WS" 500000 >/dev/null
S0="$(balance "$TS" "$WS")"; R0="$(balance "$TRR" "$WR")"
RX=1000
TID="$(curl -s -X POST "$BASE/transfers" -H "Authorization: Bearer $TS" -H 'Content-Type: application/json' \
  -d "{\"from\":\"$WS\",\"to\":\"$WR\",\"amount_paise\":$RX,\"idempotency_key\":\"revsetup-$RANDOM\"}" | jstr id)"
echo "original transfer $TID moved $RX paise S->R"
RKEY="reverse-$RANDOM"; export TS TID RKEY
seq 1 2 | xargs -P2 -n1 bash "$TMP/h_reverse.sh"
S1="$(balance "$TS" "$WS")"; R1="$(balance "$TRR" "$WR")"
echo "S: $S0 -> (after transfer) -> $S1   R: $R0 -> $R1   (reversed twice, same key)"
[ "$S1" = "$S0" ] && pass "sender restored to pre-transfer balance (single refund)" || fail "sender balance $S1 != $S0"
[ "$R1" = "$R0" ] && pass "recipient restored (no double refund)" || fail "recipient balance $R1 != $R0"
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X POST "$BASE/transfers/$TID/reverse" -H "Authorization: Bearer $TS" \
  -H 'Content-Type: application/json' -d "{\"idempotency_key\":\"reverse-again-$RANDOM\"}")"
[ "$CODE" = "409" ] && pass "reverse-already-reversed -> 409" || fail "expected 409, got $CODE"

# =====================================================================================
hd "RESULT"
if [ "$FAILURES" = "0" ]; then green "ALL GATES PASSED ✅"; exit 0; else red "$FAILURES check(s) failed ❌"; exit 1; fi
