-- Способы входа клиента: Telegram-бот, почта (с подтверждением кодом) и Google.
-- users.email (из 00090) становится постоянной колонкой; email_verified_at — когда
-- почта подтверждена кодом из письма или аккаунтом Google. Неподтверждённые адреса
-- остаются только у тестовых аккаунтов, созданных до этой миграции.
-- Номер 00091 — рядом с 00090, вне диапазонов модулей.

-- +goose Up
ALTER TABLE users
    ADD COLUMN email_verified_at timestamptz,
    ADD COLUMN telegram_user_id  bigint,
    ADD COLUMN google_sub        text CONSTRAINT users_google_sub_len CHECK (length(google_sub) BETWEEN 1 AND 255);
CREATE UNIQUE INDEX users_telegram_user_id_key ON users (telegram_user_id) WHERE telegram_user_id IS NOT NULL;
CREATE UNIQUE INDEX users_google_sub_key ON users (google_sub) WHERE google_sub IS NOT NULL;
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone
    CHECK (role <> 'client' OR phone IS NOT NULL OR email IS NOT NULL OR telegram_user_id IS NOT NULL
           OR google_sub IS NOT NULL OR deleted_at IS NOT NULL);

-- +goose Down
UPDATE users SET status = 'blocked', deleted_at = now(), gender = NULL, height_cm = NULL, weight_kg = NULL,
    body_params = '{}'::jsonb, pd_consent_at = NULL
    WHERE role = 'client' AND phone IS NULL AND email IS NULL AND deleted_at IS NULL;
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone
    CHECK (role <> 'client' OR phone IS NOT NULL OR email IS NOT NULL OR deleted_at IS NOT NULL);
DROP INDEX users_google_sub_key;
DROP INDEX users_telegram_user_id_key;
ALTER TABLE users DROP COLUMN google_sub, DROP COLUMN telegram_user_id, DROP COLUMN email_verified_at;
