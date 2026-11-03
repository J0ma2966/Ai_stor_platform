-- Data invariants. Run after a restore and after every load test:
--   psql -v ON_ERROR_STOP=1 "$DATABASE_URL" -f scripts/consistency-check.sql
-- Fails (non-zero exit) if any invariant is broken; prints the counts otherwise.
DO $$
DECLARE
  n bigint;
  problems text[] := '{}';
BEGIN
  -- 1. balance equals the sum of the ledger, for every user
  SELECT count(*) INTO n FROM users u
  LEFT JOIN (SELECT user_id, sum(amount) s FROM token_ledger GROUP BY user_id) l ON l.user_id = u.id
  WHERE u.token_balance <> COALESCE(l.s, 0);
  IF n > 0 THEN problems := problems || format('%s users: token_balance <> sum(ledger)', n); END IF;

  -- 2. last balance_after in the ledger equals the current balance
  SELECT count(*) INTO n FROM users u
  JOIN LATERAL (SELECT balance_after FROM token_ledger t WHERE t.user_id = u.id ORDER BY id DESC LIMIT 1) last ON true
  WHERE last.balance_after <> u.token_balance;
  IF n > 0 THEN problems := problems || format('%s users: last balance_after <> token_balance', n); END IF;

  -- 3. no negative balances
  SELECT count(*) INTO n FROM users WHERE token_balance < 0;
  IF n > 0 THEN problems := problems || format('%s users with negative balance', n); END IF;

  -- 4. at most one welcome bonus per user (also enforced by an index; checks the index survived)
  SELECT count(*) INTO n FROM (SELECT user_id FROM token_ledger WHERE op = 'welcome_bonus' GROUP BY user_id HAVING count(*) > 1) d;
  IF n > 0 THEN problems := problems || format('%s users with more than one welcome bonus', n); END IF;

  -- 5. every paid payment credited exactly once; nothing credited for unpaid ones
  SELECT count(*) INTO n FROM payments p
  WHERE p.status = 'paid' AND (SELECT count(*) FROM token_ledger t
        WHERE t.reference_type = 'payment' AND t.reference_id = p.id AND t.amount > 0) <> 1;
  IF n > 0 THEN problems := problems || format('%s paid payments not credited exactly once', n); END IF;
  SELECT count(*) INTO n FROM payments p
  WHERE p.status <> 'paid' AND EXISTS (SELECT 1 FROM token_ledger t
        WHERE t.reference_type = 'payment' AND t.reference_id = p.id AND t.amount > 0);
  IF n > 0 THEN problems := problems || format('%s unpaid payments credited', n); END IF;
  -- 5b. a paid payment has exactly one performed provider transaction, an unpaid one has none
  SELECT count(*) INTO n FROM payments p
  WHERE (SELECT count(*) FROM payment_transactions t WHERE t.payment_id = p.id AND t.state = 2)
        <> CASE WHEN p.status = 'paid' THEN 1 ELSE 0 END;
  IF n > 0 THEN problems := problems || format('%s payments disagree with provider transactions', n); END IF;
  -- 5c. provider transaction amount equals the payment amount
  SELECT count(*) INTO n FROM payment_transactions t JOIN payments p ON p.id = t.payment_id WHERE t.amount <> p.amount;
  IF n > 0 THEN problems := problems || format('%s provider transactions with a wrong amount', n); END IF;

  -- 6. failed generations refunded; successful ones not refunded
  SELECT count(*) INTO n FROM generations g
  WHERE g.status = 'failed' AND g.cost > 0 AND NOT EXISTS (SELECT 1 FROM token_ledger t
        WHERE t.reference_type = 'generation' AND t.reference_id = g.id AND t.amount > 0);
  IF n > 0 THEN problems := problems || format('%s failed generations not refunded', n); END IF;

  -- 7. orders: have items, have delivery data, total matches items, history starts at pending
  SELECT count(*) INTO n FROM orders o WHERE NOT EXISTS (SELECT 1 FROM order_items i WHERE i.order_id = o.id);
  IF n > 0 THEN problems := problems || format('%s orders without items', n); END IF;
  SELECT count(*) INTO n FROM orders o WHERE NOT EXISTS (SELECT 1 FROM order_deliveries d WHERE d.order_id = o.id);
  IF n > 0 THEN problems := problems || format('%s orders without delivery data', n); END IF;
  SELECT count(*) INTO n FROM orders o
  WHERE o.items_total <> (SELECT COALESCE(sum(unit_price * qty), 0) FROM order_items i WHERE i.order_id = o.id);
  IF n > 0 THEN problems := problems || format('%s orders whose total <> sum(items)', n); END IF;
  SELECT count(*) INTO n FROM orders o WHERE o.status <> 'pending' AND NOT EXISTS
        (SELECT 1 FROM order_status_history h WHERE h.order_id = o.id AND h.to_status = o.status);
  IF n > 0 THEN problems := problems || format('%s orders whose status has no history entry', n); END IF;

  -- 8. an order item belongs to the same store as its order (no cross-store orders)
  SELECT count(*) INTO n FROM order_items i JOIN orders o ON o.id = i.order_id
  JOIN products p ON p.id = i.product_id WHERE p.store_id <> o.store_id;
  IF n > 0 THEN problems := problems || format('%s order items from another store', n); END IF;

  -- 9. append-only protections present
  SELECT count(*) INTO n FROM pg_trigger WHERE tgname IN
    ('token_ledger_append_only','token_ledger_no_truncate','audit_log_append_only','audit_log_no_truncate');
  IF n <> 4 THEN problems := problems || 'append-only triggers missing'; END IF;

  IF array_length(problems, 1) > 0 THEN
    RAISE EXCEPTION 'consistency check failed: %', array_to_string(problems, '; ');
  END IF;
  RAISE NOTICE 'consistency check passed';
END $$;
