-- +goose Up
-- Ускоряет поиск фото доставки с истёкшим сроком хранения (janitor).
CREATE INDEX order_deliveries_photo_idx ON order_deliveries (photo_key) WHERE photo_key IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS order_deliveries_photo_idx;
