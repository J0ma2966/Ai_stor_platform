-- +goose Up
-- Отзывы и оценки товаров (docs/REVIEWS.md). Диапазон миграций отзывов: 00040–00049.

CREATE TYPE review_status AS ENUM ('visible', 'hidden');

-- Один отзыв клиента на товар. Оставить отзыв можно только после доставки заказа
-- с этим товаром (проверяет бэкенд). Отзыв публикуется сразу; администратор может
-- скрыть его с причиной. Магазин отвечает на отзыв, но скрыть его не может.
CREATE TABLE product_reviews (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id        uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating            smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body              text NOT NULL DEFAULT '' CHECK (length(body) <= 2000),
    author_name       text NOT NULL DEFAULT '' CHECK (length(author_name) <= 60), -- «Азиз К.» из заказа; '' — «Покупатель»
    status            review_status NOT NULL DEFAULT 'visible',
    moderation_reason text CHECK (length(moderation_reason) <= 500),
    reply             text CHECK (length(reply) BETWEEN 1 AND 2000),            -- ответ магазина
    reply_at          timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, user_id),
    CONSTRAINT product_reviews_reply_pair CHECK ((reply IS NULL) = (reply_at IS NULL))
);
CREATE TRIGGER product_reviews_touch BEFORE UPDATE ON product_reviews FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX product_reviews_public_idx ON product_reviews (product_id, created_at DESC, id DESC) WHERE status = 'visible';
CREATE INDEX product_reviews_user_idx ON product_reviews (user_id);
CREATE INDEX product_reviews_created_idx ON product_reviews (created_at DESC, id DESC);

-- Средняя оценка и число видимых отзывов — в карточке товара и для сортировки каталога.
-- Пересчитываются в той же транзакции, что и изменение отзыва (reviews.Refresh).
ALTER TABLE products ADD COLUMN rating_avg numeric(3,2) NOT NULL DEFAULT 0 CHECK (rating_avg BETWEEN 0 AND 5);
ALTER TABLE products ADD COLUMN rating_count int NOT NULL DEFAULT 0 CHECK (rating_count >= 0);
CREATE INDEX products_rating_idx ON products (rating_avg DESC, id DESC) WHERE status = 'published';

-- Поиск «покупал ли клиент этот товар»: заказы клиента + позиции по товару.
CREATE INDEX order_items_product_idx ON order_items (product_id) WHERE product_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS order_items_product_idx;
DROP INDEX IF EXISTS products_rating_idx;
ALTER TABLE products DROP COLUMN IF EXISTS rating_count;
ALTER TABLE products DROP COLUMN IF EXISTS rating_avg;
DROP TABLE IF EXISTS product_reviews;
DROP TYPE IF EXISTS review_status;
