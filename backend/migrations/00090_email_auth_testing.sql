-- ВРЕМЕННО: вход клиента по почте и паролю на период тестирования (auth/email_temp.go).
-- Номер 00090 вне диапазонов модулей (00010–00039), чтобы не мешать другим веткам.
-- Чтобы убрать функцию из уже работающей базы, добавьте новую миграцию с телом Down.

-- +goose Up
ALTER TABLE users ADD COLUMN email text
    CONSTRAINT users_email_format CHECK (email = lower(email) AND email ~ '^[^@[:space:]]+@[^@[:space:]]+$');
CREATE UNIQUE INDEX users_email_key ON users (email) WHERE email IS NOT NULL;
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone
    CHECK (role <> 'client' OR phone IS NOT NULL OR email IS NOT NULL OR deleted_at IS NOT NULL);

-- +goose Down
-- Тестовые аккаунты без телефона обезличиваются так же, как при удалении аккаунта
-- (заказы и журнал токенов ссылаются на users, удалять строки нельзя).
UPDATE users SET status = 'blocked', deleted_at = now(), gender = NULL, height_cm = NULL, weight_kg = NULL,
    body_params = '{}'::jsonb, pd_consent_at = NULL
    WHERE role = 'client' AND phone IS NULL AND deleted_at IS NULL;
UPDATE users SET password_hash = NULL WHERE role = 'client';
UPDATE refresh_tokens SET revoked_at = now()
    WHERE revoked_at IS NULL AND user_id IN (SELECT id FROM users WHERE role = 'client' AND email IS NOT NULL);
ALTER TABLE users DROP CONSTRAINT users_client_has_phone;
ALTER TABLE users ADD CONSTRAINT users_client_has_phone CHECK (role <> 'client' OR phone IS NOT NULL OR deleted_at IS NOT NULL);
DROP INDEX users_email_key;
ALTER TABLE users DROP COLUMN email;
