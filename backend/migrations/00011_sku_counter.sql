-- +goose Up
-- Счётчик автоартикулов: если магазин не ввёл артикул, сервер выдаёт следующий
-- номер своего магазина (A-000001, A-000002, ...). Отдельная таблица, а не столбец
-- stores, чтобы выдача номера не трогала stores.updated_at и не блокировала профиль.
CREATE TABLE store_sku_counters (
    store_id  uuid PRIMARY KEY REFERENCES stores(id) ON DELETE CASCADE,
    last      bigint NOT NULL DEFAULT 0 CHECK (last >= 0)
);

-- +goose Down
DROP TABLE store_sku_counters;
