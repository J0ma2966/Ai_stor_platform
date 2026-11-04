-- +goose Up
-- Отдельное информированное согласие на передачу фото тела внешнему AI-провайдеру
-- (OpenAI, Google Gemini, FASHN) — ТЗ интеграции OpenAI §12. Согласие привязано к
-- конкретному получателю: при смене TRYON_PROVIDER его нужно дать заново.
ALTER TABLE users
    ADD COLUMN tryon_transfer_consent_at timestamptz,
    ADD COLUMN tryon_transfer_provider text;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS tryon_transfer_provider, DROP COLUMN IF EXISTS tryon_transfer_consent_at;
