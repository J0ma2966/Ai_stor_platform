-- +goose Up
-- Платежи Click/Payme: номер платежа (целое — Click требует числовой merchant_prepare_id),
-- ключ идемпотентности создания и журнал транзакций провайдеров.
ALTER TABLE payments
    ADD COLUMN number          bigint GENERATED ALWAYS AS IDENTITY,
    ADD COLUMN idempotency_key text CHECK (length(idempotency_key) BETWEEN 8 AND 128),
    ADD COLUMN cancelled_at    timestamptz;
ALTER TABLE payments ADD CONSTRAINT payments_number_key UNIQUE (number);
CREATE UNIQUE INDEX payments_idem_idx ON payments (user_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX payments_pending_idx ON payments (created_at) WHERE status = 'pending';
-- оплаченный платёж обязан иметь время и id транзакции провайдера
ALTER TABLE payments ADD CONSTRAINT payments_paid_consistent
    CHECK (status <> 'paid' OR (paid_at IS NOT NULL AND provider_txn_id IS NOT NULL));

-- Транзакция провайдера по платежу. state: 1 — создана/подготовлена, 2 — проведена,
-- -1 — отменена до проведения, -2 — отменена после проведения (Payme).
CREATE TABLE payment_transactions (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    payment_id     uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
    provider       payment_provider NOT NULL,
    external_id    text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 64),
    state          smallint NOT NULL CHECK (state IN (1, 2, -1, -2)),
    amount         bigint NOT NULL CHECK (amount > 0),
    provider_time  bigint,            -- время создания у провайдера, мс (Payme)
    created_at     timestamptz NOT NULL DEFAULT now(),
    performed_at   timestamptz,
    cancelled_at   timestamptz,
    cancel_reason  int,
    UNIQUE (provider, external_id)
);
-- не больше одной живой (созданной или проведённой) транзакции на платёж:
-- вторая оплата того же платежа невозможна на уровне БД
CREATE UNIQUE INDEX payment_transactions_one_active ON payment_transactions (payment_id) WHERE state IN (1, 2);
CREATE INDEX payment_transactions_payment_idx ON payment_transactions (payment_id);
CREATE INDEX payment_transactions_time_idx ON payment_transactions (provider, provider_time);

-- +goose Down
DROP TABLE IF EXISTS payment_transactions;
DROP INDEX IF EXISTS payments_pending_idx;
DROP INDEX IF EXISTS payments_idem_idx;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_paid_consistent;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_number_key;
ALTER TABLE payments DROP COLUMN IF EXISTS cancelled_at, DROP COLUMN IF EXISTS idempotency_key, DROP COLUMN IF EXISTS number;
