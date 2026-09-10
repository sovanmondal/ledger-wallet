-- Verifies the exact SQL mechanisms LedgerWallet's Go code relies on, against real Postgres.
\set ON_ERROR_STOP on

-- seed two users + wallets, give 'a' a balance of 1000 paise
INSERT INTO users(username, token) VALUES ('a','ta'),('b','tb');
INSERT INTO wallets(user_id) SELECT id FROM users WHERE username IN ('a','b');
UPDATE wallets SET balance_paise = 1000 WHERE user_id = (SELECT id FROM users WHERE username='a');

-- GATE 1: race-free get-or-create — two inserts, one wallet
INSERT INTO users(username, token) VALUES ('gc','tgc');
WITH u AS (SELECT id FROM users WHERE username='gc')
INSERT INTO wallets(user_id) SELECT id FROM u ON CONFLICT (user_id) DO NOTHING;
WITH u AS (SELECT id FROM users WHERE username='gc')
INSERT INTO wallets(user_id) SELECT id FROM u ON CONFLICT (user_id) DO NOTHING;
SELECT 'GATE1 one wallet per user' AS check,
       (SELECT count(*) FROM wallets w JOIN users u ON u.id=w.user_id WHERE u.username='gc') = 1 AS pass;

-- GATE 2: idempotency key uniqueness — second insert with same key returns nothing
DO $$
DECLARE wa uuid; wb uuid; tid uuid; tid2 uuid; n int;
BEGIN
  SELECT w.id INTO wa FROM wallets w JOIN users u ON u.id=w.user_id WHERE u.username='a';
  SELECT w.id INTO wb FROM wallets w JOIN users u ON u.id=w.user_id WHERE u.username='b';
  INSERT INTO transfers(idempotency_key,from_wallet,to_wallet,amount_paise,kind,status,request_fingerprint)
    VALUES ('K',wa,wb,100,'transfer','pending','fp') ON CONFLICT (idempotency_key) DO NOTHING RETURNING id INTO tid;
  INSERT INTO transfers(idempotency_key,from_wallet,to_wallet,amount_paise,kind,status,request_fingerprint)
    VALUES ('K',wa,wb,100,'transfer','pending','fp') ON CONFLICT (idempotency_key) DO NOTHING RETURNING id INTO tid2;
  SELECT count(*) INTO n FROM transfers WHERE idempotency_key='K';
  RAISE NOTICE 'GATE2 idempotency: one_row=%  second_insert_null=%', (n=1), (tid2 IS NULL);
END $$;

-- GATE 3a: conditional debit declines cleanly when it would overdraw (0 rows, balance unchanged)
DO $$
DECLARE aff int; bal bigint;
BEGIN
  UPDATE wallets SET balance_paise = balance_paise - 999999
    WHERE user_id=(SELECT id FROM users WHERE username='a') AND balance_paise >= 999999;
  GET DIAGNOSTICS aff = ROW_COUNT;
  SELECT balance_paise INTO bal FROM wallets WHERE user_id=(SELECT id FROM users WHERE username='a');
  RAISE NOTICE 'GATE3 overdraw: declined_rows0=%  balance_unchanged=%', (aff=0), (bal=1000);
END $$;

-- GATE 3b: successful ordered debit/credit writes a zero-sum ledger pair (conservation)
DO $$
DECLARE wa uuid; wb uuid; tid uuid; s bigint;
BEGIN
  SELECT w.id INTO wa FROM wallets w JOIN users u ON u.id=w.user_id WHERE u.username='a';
  SELECT w.id INTO wb FROM wallets w JOIN users u ON u.id=w.user_id WHERE u.username='b';
  INSERT INTO transfers(idempotency_key,from_wallet,to_wallet,amount_paise,kind,status,request_fingerprint)
    VALUES ('K2',wa,wb,300,'transfer','pending','fp2') RETURNING id INTO tid;
  UPDATE wallets SET balance_paise = balance_paise - 300 WHERE id=wa AND balance_paise >= 300;
  UPDATE wallets SET balance_paise = balance_paise + 300 WHERE id=wb;
  INSERT INTO ledger_entries(transfer_id,wallet_id,delta_paise) VALUES (tid,wa,-300),(tid,wb,300);
  UPDATE transfers SET status='completed' WHERE id=tid;
  SELECT sum(delta_paise) INTO s FROM ledger_entries;
  RAISE NOTICE 'GATE3 conservation: ledger_sum_zero=%', (s=0);
END $$;

-- REVERSAL: UNIQUE(reverses_transfer_id) blocks a second, different reversal of the same transfer
DO $$
DECLARE wa uuid; wb uuid; orig uuid; blocked boolean := false;
BEGIN
  SELECT id, from_wallet, to_wallet INTO orig, wa, wb FROM transfers WHERE idempotency_key='K2';
  INSERT INTO transfers(idempotency_key,from_wallet,to_wallet,amount_paise,kind,status,reverses_transfer_id,request_fingerprint)
    VALUES ('R1',wb,wa,300,'reversal','completed',orig,'fpr1');
  BEGIN
    INSERT INTO transfers(idempotency_key,from_wallet,to_wallet,amount_paise,kind,status,reverses_transfer_id,request_fingerprint)
      VALUES ('R2',wb,wa,300,'reversal','completed',orig,'fpr2');
  EXCEPTION WHEN unique_violation THEN blocked := true;
  END;
  RAISE NOTICE 'REVERSAL double-reverse blocked=%', blocked;
END $$;

-- CHECK constraint prevents any negative balance write
DO $$
DECLARE blocked boolean := false;
BEGIN
  BEGIN
    UPDATE wallets SET balance_paise = -1 WHERE user_id=(SELECT id FROM users WHERE username='b');
  EXCEPTION WHEN check_violation THEN blocked := true;
  END;
  RAISE NOTICE 'NO-OVERDRAFT check-constraint blocks negative=%', blocked;
END $$;

-- treasury genesis present
SELECT 'TREASURY seeded' AS check, (SELECT count(*) FROM wallets WHERE id='00000000-0000-0000-0000-000000000002')=1 AS pass;
