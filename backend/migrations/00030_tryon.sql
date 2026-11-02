-- +goose Up
-- AI-примерка: очередь генераций на PostgreSQL, сохранённые фото клиента, образы.
-- Биометрия (фото клиента и результаты) хранится только в хранилище в Узбекистане
-- (BIOMETRIC_S3_*), в БД — лишь ключи объектов.

-- Согласие на обработку фото для AI-примерки (в т.ч. передачу AI-провайдеру).
ALTER TABLE users ADD COLUMN tryon_consent_at timestamptz;

-- Сохранённые фото клиента: не больше одного на ракурс.
CREATE TABLE body_photos (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    angle         photo_angle NOT NULL,
    key           text NOT NULL UNIQUE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_used_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, angle)
);

-- Генерация = задание очереди.
ALTER TABLE generations
    ADD COLUMN body           jsonb NOT NULL DEFAULT '{}'::jsonb,   -- рост/вес/параметры на момент запуска
    ADD COLUMN gender         gender,
    ADD COLUMN provider       text,
    ADD COLUMN attempts       int NOT NULL DEFAULT 0,
    ADD COLUMN next_run_at    timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN lease_until    timestamptz,                          -- воркер «держит» задание до этого времени
    ADD COLUMN started_at     timestamptz,
    ADD COLUMN error_code     text,                                 -- стабильный код для фронта
    ADD COLUMN refunded       boolean NOT NULL DEFAULT false,
    ADD COLUMN saved_at       timestamptz,                          -- «Сохранить образ»
    ADD COLUMN photos_purged_at timestamptz,                        -- исходные фото удалены по сроку
    ADD COLUMN result_purged_at timestamptz,                        -- результат удалён по сроку/клиентом
    ADD COLUMN deleted_at     timestamptz;                          -- удалено клиентом (скрыто из истории)

-- Очередь: выборка следующего задания и поиск «зависших» по истечению аренды.
CREATE INDEX generations_queue_idx ON generations (next_run_at) WHERE status = 'queued';
CREATE INDEX generations_lease_idx ON generations (lease_until) WHERE status = 'running';
-- Сроки хранения.
CREATE INDEX generations_finished_idx ON generations (finished_at)
    WHERE status IN ('succeeded', 'failed') AND (photos_purged_at IS NULL OR result_purged_at IS NULL);
CREATE INDEX generations_products_gin ON generations USING gin (product_ids);

-- +goose Down
DROP INDEX IF EXISTS generations_products_gin, generations_finished_idx, generations_lease_idx, generations_queue_idx;
ALTER TABLE generations
    DROP COLUMN body, DROP COLUMN gender, DROP COLUMN provider, DROP COLUMN attempts,
    DROP COLUMN next_run_at, DROP COLUMN lease_until, DROP COLUMN started_at, DROP COLUMN error_code,
    DROP COLUMN refunded, DROP COLUMN saved_at, DROP COLUMN photos_purged_at, DROP COLUMN result_purged_at,
    DROP COLUMN deleted_at;
DROP TABLE IF EXISTS body_photos;
ALTER TABLE users DROP COLUMN IF EXISTS tryon_consent_at;
