-- +goose Up
-- Размеры с остатками, отмена заказа клиентом, Telegram-бот магазина и исходящие SMS.

-- Размеры товара. Если у товара есть размеры, products.in_stock считается
-- автоматически: true, когда хотя бы у одного размера остаток > 0.
-- Товар без размеров (аксессуары, сумки) работает как раньше — по флажку in_stock.
CREATE TABLE product_sizes (
    product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    size        text NOT NULL CHECK (length(size) BETWEEN 1 AND 20),
    stock       int  NOT NULL CHECK (stock BETWEEN 0 AND 100000),
    sort        smallint NOT NULL DEFAULT 0,
    PRIMARY KEY (product_id, size)
);

-- +goose StatementBegin
CREATE FUNCTION product_sizes_sync_stock() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pid uuid := COALESCE(NEW.product_id, OLD.product_id);
BEGIN
    UPDATE products p SET in_stock = EXISTS (SELECT 1 FROM product_sizes s WHERE s.product_id = pid AND s.stock > 0)
    WHERE p.id = pid AND EXISTS (SELECT 1 FROM product_sizes s WHERE s.product_id = pid)
      AND p.in_stock IS DISTINCT FROM EXISTS (SELECT 1 FROM product_sizes s WHERE s.product_id = pid AND s.stock > 0);
    RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER product_sizes_sync AFTER INSERT OR UPDATE OR DELETE ON product_sizes
    FOR EACH ROW EXECUTE FUNCTION product_sizes_sync_stock();

-- Корзина: одна строка на товар и размер ('' — товар без размеров).
ALTER TABLE cart_items ADD COLUMN size text NOT NULL DEFAULT '' CHECK (length(size) <= 20);
ALTER TABLE cart_items DROP CONSTRAINT cart_items_pkey;
ALTER TABLE cart_items ADD PRIMARY KEY (user_id, product_id, size);

-- Снимок размера в заказе.
ALTER TABLE order_items ADD COLUMN size text CHECK (length(size) <= 20);

-- Telegram-бот магазина: chat_id привязывается по одноразовому коду из кабинета.
ALTER TABLE stores ADD COLUMN telegram_chat_id bigint;
ALTER TABLE stores ADD COLUMN telegram_link_code text UNIQUE;
ALTER TABLE stores ADD COLUMN telegram_link_expires_at timestamptz;

-- Напоминание магазину о неподтверждённом заказе отправляется один раз.
ALTER TABLE orders ADD COLUMN store_reminded_at timestamptz;
CREATE INDEX orders_pending_remind_idx ON orders (created_at) WHERE status = 'pending' AND store_reminded_at IS NULL;

-- Очередь исходящих сообщений (outbox): пишется в той же транзакции, что и событие,
-- отправляется фоновым воркером с повторами. Отправленные удаляются janitor через 7 дней.
CREATE TABLE outbound_messages (
    id          bigserial PRIMARY KEY,
    channel     text NOT NULL CHECK (channel IN ('telegram', 'sms')),
    recipient   text NOT NULL CHECK (length(recipient) BETWEEN 1 AND 64),
    payload     jsonb NOT NULL,
    attempts    int NOT NULL DEFAULT 0,
    next_at     timestamptz NOT NULL DEFAULT now(),
    sent_at     timestamptz,
    failed_at   timestamptz,
    last_error  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX outbound_messages_due_idx ON outbound_messages (next_at) WHERE sent_at IS NULL AND failed_at IS NULL;
CREATE INDEX outbound_messages_created_idx ON outbound_messages (created_at);

-- +goose Down
DROP TABLE IF EXISTS outbound_messages;
DROP INDEX IF EXISTS orders_pending_remind_idx;
ALTER TABLE orders DROP COLUMN IF EXISTS store_reminded_at;
ALTER TABLE stores DROP COLUMN IF EXISTS telegram_link_expires_at;
ALTER TABLE stores DROP COLUMN IF EXISTS telegram_link_code;
ALTER TABLE stores DROP COLUMN IF EXISTS telegram_chat_id;
ALTER TABLE order_items DROP COLUMN IF EXISTS size;
DELETE FROM cart_items a USING cart_items b
    WHERE a.user_id = b.user_id AND a.product_id = b.product_id AND a.size > b.size;
ALTER TABLE cart_items DROP CONSTRAINT cart_items_pkey;
ALTER TABLE cart_items DROP COLUMN size;
ALTER TABLE cart_items ADD PRIMARY KEY (user_id, product_id);
DROP TRIGGER IF EXISTS product_sizes_sync ON product_sizes;
DROP FUNCTION IF EXISTS product_sizes_sync_stock;
DROP TABLE IF EXISTS product_sizes;
