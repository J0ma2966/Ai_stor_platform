-- +goose Up
-- Базовая схема маркетплейса бутиков.
-- Деньги: BIGINT в тийинах (1 сум = 100 тийин). Время: timestamptz (UTC).

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TYPE user_role       AS ENUM ('client', 'store', 'admin');
CREATE TYPE user_status     AS ENUM ('active', 'blocked');
CREATE TYPE gender          AS ENUM ('male', 'female');
CREATE TYPE ui_lang         AS ENUM ('ru', 'uz');
CREATE TYPE store_status    AS ENUM ('pending_activation', 'active', 'blocked', 'archived');
CREATE TYPE product_status  AS ENUM ('draft', 'published', 'unpublished', 'blocked');
CREATE TYPE photo_angle     AS ENUM ('front', 'side', 'back');
CREATE TYPE category_zone   AS ENUM ('headwear', 'top', 'bottom', 'shoes');
CREATE TYPE order_status    AS ENUM ('pending', 'confirmed', 'rejected', 'assembling', 'in_delivery', 'delivered', 'cancelled');
CREATE TYPE token_op        AS ENUM ('welcome_bonus', 'purchase', 'generation_charge', 'generation_refund', 'admin_adjustment');
CREATE TYPE payment_provider AS ENUM ('click', 'payme');
CREATE TYPE payment_status  AS ENUM ('pending', 'paid', 'declined', 'cancelled');
CREATE TYPE generation_status AS ENUM ('queued', 'running', 'succeeded', 'failed');

-- updated_at автоматически
-- +goose StatementBegin
CREATE FUNCTION touch_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at := now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- ===================== Магазины и пользователи =====================

CREATE TABLE stores (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug          text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,62}$'),
    name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description   text NOT NULL DEFAULT '',
    logo_key      text,
    cover_key     text,
    phone         text,
    contacts      jsonb NOT NULL DEFAULT '{}'::jsonb,
    address       text,
    lat           numeric(9,6) CHECK (lat BETWEEN -90 AND 90),
    lng           numeric(9,6) CHECK (lng BETWEEN -180 AND 180),
    status        store_status NOT NULL DEFAULT 'pending_activation',
    activated_at  timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER stores_touch BEFORE UPDATE ON stores FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    role           user_role NOT NULL,
    status         user_status NOT NULL DEFAULT 'active',
    phone          text UNIQUE CHECK (phone ~ '^\+[1-9][0-9]{7,14}$'),
    login          text UNIQUE CHECK (login ~ '^[a-z0-9._-]{3,64}$'),
    password_hash  text,
    store_id       uuid UNIQUE REFERENCES stores(id) ON DELETE RESTRICT, -- один аккаунт на магазин
    gender         gender,
    lang           ui_lang NOT NULL DEFAULT 'ru',
    token_balance  bigint NOT NULL DEFAULT 0 CHECK (token_balance >= 0),
    height_cm      smallint CHECK (height_cm BETWEEN 50 AND 260),
    weight_kg      numeric(5,1) CHECK (weight_kg BETWEEN 20 AND 400),
    body_params    jsonb NOT NULL DEFAULT '{}'::jsonb,
    pd_consent_at  timestamptz,           -- согласие на обработку ПД и изображений
    last_login_at  timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_client_has_phone  CHECK (role <> 'client' OR phone IS NOT NULL),
    CONSTRAINT users_staff_has_login   CHECK (role = 'client' OR (login IS NOT NULL AND password_hash IS NOT NULL)),
    CONSTRAINT users_store_has_store   CHECK ((role = 'store') = (store_id IS NOT NULL))
);
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX users_role_created_idx ON users (role, created_at DESC);

-- Ротируемые refresh-токены. Хранится только SHA-256 хэш.
CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    family_id   uuid NOT NULL,
    token_hash  bytea NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,     -- проставляется при ротации; повторное использование => отзыв семьи
    revoked_at  timestamptz,
    user_agent  text,
    ip          inet,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON refresh_tokens (user_id);

-- ===================== Каталог =====================

CREATE TABLE categories (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text NOT NULL UNIQUE,
    parent_id   uuid REFERENCES categories(id) ON DELETE RESTRICT,
    zone        category_zone NOT NULL,  -- зона манекена в примерочной
    name_ru     text NOT NULL,
    name_uz     text NOT NULL,
    sort        int NOT NULL DEFAULT 0,
    is_active   boolean NOT NULL DEFAULT true
);

CREATE TABLE products (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id          uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    category_id       uuid NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    sku               text NOT NULL CHECK (length(sku) BETWEEN 1 AND 64),
    title             text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
    description       text NOT NULL CHECK (length(description) BETWEEN 1 AND 10000),
    brand             text NOT NULL CHECK (length(brand) BETWEEN 1 AND 120),
    price             bigint NOT NULL CHECK (price > 0),
    in_stock          boolean NOT NULL DEFAULT true,
    status            product_status NOT NULL DEFAULT 'draft',
    moderation_reason text,
    views_count       bigint NOT NULL DEFAULT 0,
    fitting_adds      bigint NOT NULL DEFAULT 0,
    published_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (store_id, sku)
);
CREATE TRIGGER products_touch BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX products_store_idx ON products (store_id, created_at DESC);
CREATE INDEX products_public_idx ON products (category_id, created_at DESC) WHERE status = 'published';
CREATE INDEX products_public_price_idx ON products (price) WHERE status = 'published';
CREATE INDEX products_search_trgm ON products USING gin (lower(title || ' ' || brand) gin_trgm_ops);

CREATE TABLE product_images (
    product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    angle       photo_angle NOT NULL,
    key         text NOT NULL,
    PRIMARY KEY (product_id, angle)
);

CREATE TABLE favorites (
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, product_id)
);

CREATE TABLE cart_items (
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    qty         int NOT NULL CHECK (qty BETWEEN 1 AND 99),
    added_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, product_id)
);

-- ===================== Заказы и доставка =====================
-- Одно оформление (checkout) создаёт по заказу на каждый магазин.

CREATE SEQUENCE order_number_seq START 100001;

CREATE TABLE orders (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number                 bigint NOT NULL UNIQUE DEFAULT nextval('order_number_seq'),
    checkout_id            uuid NOT NULL,
    user_id                uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    store_id               uuid NOT NULL REFERENCES stores(id) ON DELETE RESTRICT,
    status                 order_status NOT NULL DEFAULT 'pending',
    items_total            bigint NOT NULL CHECK (items_total >= 0),
    customer_name          text NOT NULL CHECK (length(customer_name) BETWEEN 1 AND 120),
    customer_phone         text NOT NULL CHECK (customer_phone ~ '^\+[1-9][0-9]{7,14}$'),
    customer_comment       text CHECK (length(customer_comment) <= 1000),   -- комментарий к заказу
    delivery_time_comment  text CHECK (length(delivery_time_comment) <= 500), -- комментарий ко времени доставки
    rejection_reason       text CHECK (length(rejection_reason) <= 500),
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER orders_touch BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX orders_user_idx ON orders (user_id, created_at DESC);
CREATE INDEX orders_store_status_idx ON orders (store_id, status, created_at DESC);
CREATE INDEX orders_checkout_idx ON orders (checkout_id);

CREATE TABLE order_items (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id    uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    product_id  uuid REFERENCES products(id) ON DELETE SET NULL,
    title       text NOT NULL,   -- снимок на момент заказа
    brand       text NOT NULL,
    image_key   text,
    unit_price  bigint NOT NULL CHECK (unit_price > 0),
    qty         int NOT NULL CHECK (qty BETWEEN 1 AND 99)
);
CREATE INDEX order_items_order_idx ON order_items (order_id);

CREATE TABLE order_deliveries (
    order_id         uuid PRIMARY KEY REFERENCES orders(id) ON DELETE CASCADE,
    street           text NOT NULL CHECK (length(street) BETWEEN 1 AND 200),
    house            text NOT NULL CHECK (length(house) BETWEEN 1 AND 30),
    entrance         text CHECK (length(entrance) <= 20),   -- подъезд
    floor            text CHECK (length(floor) <= 10),      -- этаж
    apartment        text CHECK (length(apartment) <= 20),
    intercom         text CHECK (length(intercom) <= 30),
    address_comment  text CHECK (length(address_comment) <= 1000),
    lat              numeric(9,6) CHECK (lat BETWEEN -90 AND 90),
    lng              numeric(9,6) CHECK (lng BETWEEN -180 AND 180),
    photo_key        text,      -- фото места доставки (необязательно)
    CONSTRAINT order_deliveries_latlng_pair CHECK ((lat IS NULL) = (lng IS NULL))
);

CREATE TABLE order_status_history (
    id           bigserial PRIMARY KEY,
    order_id     uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status  order_status,
    to_status    order_status NOT NULL,
    actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_role   user_role,
    reason       text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_status_history_order_idx ON order_status_history (order_id, created_at);

-- ===================== Токены, тарифы, платежи, генерации =====================

CREATE TABLE token_ledger (
    id              bigserial PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    op              token_op NOT NULL,
    amount          bigint NOT NULL CHECK (amount <> 0),
    balance_after   bigint NOT NULL CHECK (balance_after >= 0),
    reference_type  text,
    reference_id    uuid,
    reason          text,
    actor_id        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT token_ledger_admin_reason CHECK (op <> 'admin_adjustment' OR (reason IS NOT NULL AND actor_id IS NOT NULL))
);
CREATE INDEX token_ledger_user_idx ON token_ledger (user_id, id DESC);
-- приветственный бонус строго один раз на аккаунт
CREATE UNIQUE INDEX token_ledger_welcome_once ON token_ledger (user_id) WHERE op = 'welcome_bonus';
-- одна операция на одну ссылку (платёж/генерацию) и тип — защита от двойного зачисления/списания
CREATE UNIQUE INDEX token_ledger_ref_once ON token_ledger (op, reference_type, reference_id) WHERE reference_id IS NOT NULL;

-- Ledger только на добавление
-- +goose StatementBegin
CREATE FUNCTION forbid_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER token_ledger_append_only BEFORE UPDATE OR DELETE ON token_ledger FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER token_ledger_no_truncate BEFORE TRUNCATE ON token_ledger FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

CREATE TABLE tariffs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name_ru      text NOT NULL,
    name_uz      text NOT NULL,
    description  text NOT NULL DEFAULT '',
    tokens       int NOT NULL CHECK (tokens > 0),
    price        bigint NOT NULL CHECK (price > 0),
    highlighted  boolean NOT NULL DEFAULT false,
    is_published boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER tariffs_touch BEFORE UPDATE ON tariffs FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TABLE payments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    tariff_id        uuid NOT NULL REFERENCES tariffs(id) ON DELETE RESTRICT,
    provider         payment_provider NOT NULL,
    provider_txn_id  text,
    amount           bigint NOT NULL CHECK (amount > 0),
    tokens           int NOT NULL CHECK (tokens > 0),
    status           payment_status NOT NULL DEFAULT 'pending',
    paid_at          timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_txn_id)
);
CREATE TRIGGER payments_touch BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX payments_user_idx ON payments (user_id, created_at DESC);

CREATE TABLE generations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status        generation_status NOT NULL DEFAULT 'queued',
    cost          int NOT NULL CHECK (cost >= 0),
    product_ids   uuid[] NOT NULL,
    photo_keys    text[] NOT NULL,
    result_key    text,
    error         text,
    created_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz
);
CREATE INDEX generations_user_idx ON generations (user_id, created_at DESC);

-- Идемпотентность POST-запросов (генерация, оплата, оформление заказа)
CREATE TABLE idempotency_keys (
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope         text NOT NULL,
    key           text NOT NULL CHECK (length(key) BETWEEN 8 AND 128),
    request_hash  bytea NOT NULL,
    status_code   int,
    response      jsonb,
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, scope, key)
);

-- ===================== Платформа и аудит =====================

CREATE TABLE platform_settings (
    key         text PRIMARY KEY,
    value       jsonb NOT NULL,
    updated_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at  timestamptz NOT NULL DEFAULT now()
);
INSERT INTO platform_settings (key, value) VALUES
    ('generation_cost', '20'),
    ('welcome_bonus', '20');

CREATE TABLE audit_log (
    id           bigserial PRIMARY KEY,
    actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_role   user_role,
    action       text NOT NULL,
    entity_type  text NOT NULL,
    entity_id    text,
    data         jsonb NOT NULL DEFAULT '{}'::jsonb,
    ip           inet,
    request_id   text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_entity_idx ON audit_log (entity_type, entity_id, created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id, created_at DESC);
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log FOR EACH ROW EXECUTE FUNCTION forbid_mutation();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log FOR EACH STATEMENT EXECUTE FUNCTION forbid_mutation();

-- Стартовые категории первого этапа
INSERT INTO categories (slug, zone, name_ru, name_uz, sort) VALUES
    ('headwear', 'headwear', 'Головные уборы', 'Bosh kiyimlar', 10),
    ('tops',     'top',      'Верх и верхняя одежда', 'Ustki kiyimlar', 20),
    ('bottoms',  'bottom',   'Низ: брюки, джинсы, юбки', 'Pastki kiyimlar', 30),
    ('shoes',    'shoes',    'Обувь', 'Poyabzal', 40);

-- +goose Down
DROP TABLE IF EXISTS audit_log, platform_settings, idempotency_keys, generations, payments, tariffs,
    token_ledger, order_status_history, order_deliveries, order_items, orders, cart_items, favorites,
    product_images, products, categories, refresh_tokens, users, stores CASCADE;
DROP SEQUENCE IF EXISTS order_number_seq;
DROP FUNCTION IF EXISTS forbid_mutation, touch_updated_at;
DROP TYPE IF EXISTS generation_status, payment_status, payment_provider, token_op, order_status,
    category_zone, photo_angle, product_status, store_status, ui_lang, gender, user_status, user_role;
