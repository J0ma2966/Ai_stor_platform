-- +goose Up
-- Витрина, кабинеты, уведомления (docs/ROADMAP.md, разделы 3–7).

-- Уведомления для всех ролей: статусы заказов, генераций, платежей, модерации, системные.
CREATE TABLE notifications (
    id          bigserial PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text NOT NULL CHECK (kind ~ '^[a-z_]{3,64}$'),
    data        jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, id DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
CREATE INDEX notifications_created_idx ON notifications (created_at);

-- Статистика по дням: просмотры и добавления в примерочную за выбранный период.
CREATE TABLE product_stats_daily (
    product_id    uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    day           date NOT NULL,
    views         bigint NOT NULL DEFAULT 0 CHECK (views >= 0),
    fitting_adds  bigint NOT NULL DEFAULT 0 CHECK (fitting_adds >= 0),
    PRIMARY KEY (product_id, day)
);

-- Удаление аккаунта клиентом (право субъекта ПД): телефон и параметры стираются,
-- заказы и журнал токенов остаются обезличенными для учёта.
ALTER TABLE users ADD COLUMN deleted_at timestamptz;
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone CHECK (role <> 'client' OR phone IS NOT NULL OR deleted_at IS NOT NULL);

-- Поиск по бренду (фильтр каталога) и витрина бутиков.
CREATE INDEX products_brand_idx ON products (lower(brand)) WHERE status = 'published';
CREATE INDEX products_store_public_idx ON products (store_id, created_at DESC) WHERE status = 'published';
CREATE INDEX stores_active_idx ON stores (name) WHERE status = 'active';
CREATE INDEX cart_items_product_idx ON cart_items (product_id);
CREATE INDEX favorites_product_idx ON favorites (product_id);
-- проверка «файл ещё используется» перед удалением фото товара
CREATE INDEX product_images_key_idx ON product_images (key);
CREATE INDEX order_items_image_key_idx ON order_items (image_key) WHERE image_key IS NOT NULL;

INSERT INTO platform_settings (key, value) VALUES
    ('support_phone', '""'),
    ('support_email', '""'),
    ('support_telegram', '""')
ON CONFLICT (key) DO NOTHING;

-- +goose Down
DELETE FROM platform_settings WHERE key IN ('support_phone', 'support_email', 'support_telegram');
DROP INDEX IF EXISTS order_items_image_key_idx;
DROP INDEX IF EXISTS product_images_key_idx;
DROP INDEX IF EXISTS favorites_product_idx;
DROP INDEX IF EXISTS cart_items_product_idx;
DROP INDEX IF EXISTS stores_active_idx;
DROP INDEX IF EXISTS products_store_public_idx;
DROP INDEX IF EXISTS products_brand_idx;
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone CHECK (role <> 'client' OR phone IS NOT NULL);
ALTER TABLE users DROP COLUMN deleted_at;
DROP TABLE IF EXISTS product_stats_daily;
DROP TABLE IF EXISTS notifications;
