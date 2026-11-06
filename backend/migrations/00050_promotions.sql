-- +goose Up
-- Реклама товаров магазинов (docs/PROMOTIONS.md). Диапазон миграций рекламы: 00050–00059.
--
-- Магазин подаёт заявку на продвижение товара (pending). Суперадмин одобряет её
-- со сроком показа (approved + starts_at/ends_at), отклоняет с причиной (rejected)
-- или останавливает уже идущую рекламу (stopped). Администратор может и сам
-- запустить рекламу товара без заявки (магазин договорился по телефону).
-- Товар показывается в карусели «Реклама» на главной, пока
-- status = 'approved' AND starts_at <= now() < ends_at, товар опубликован, а магазин активен.
CREATE TYPE promotion_status AS ENUM ('pending', 'approved', 'rejected', 'stopped');

CREATE TABLE product_promotions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    store_id      uuid NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    status        promotion_status NOT NULL DEFAULT 'pending',
    days          smallint NOT NULL CHECK (days BETWEEN 1 AND 90),            -- срок, который просит магазин
    store_note    text NOT NULL DEFAULT '' CHECK (length(store_note) <= 500),  -- комментарий магазина к заявке
    admin_reason  text CHECK (length(admin_reason) <= 500),                    -- причина отказа или остановки
    starts_at     timestamptz,
    ends_at       timestamptz,
    decided_at    timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_promotions_period CHECK (
        (status = 'pending' AND starts_at IS NULL AND ends_at IS NULL)
        OR (status = 'rejected')
        OR (status IN ('approved', 'stopped') AND starts_at IS NOT NULL AND ends_at > starts_at)
    )
);
CREATE TRIGGER product_promotions_touch BEFORE UPDATE ON product_promotions FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
-- Витрина: идущая реклама (фильтр по времени — в запросе).
CREATE INDEX product_promotions_live_idx ON product_promotions (ends_at, starts_at) WHERE status = 'approved';
CREATE INDEX product_promotions_product_idx ON product_promotions (product_id);
CREATE INDEX product_promotions_store_idx ON product_promotions (store_id, created_at DESC, id DESC);
CREATE INDEX product_promotions_created_idx ON product_promotions (created_at DESC, id DESC);

-- +goose Down
DROP TABLE IF EXISTS product_promotions;
DROP TYPE IF EXISTS promotion_status;
