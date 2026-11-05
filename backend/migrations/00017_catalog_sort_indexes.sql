-- +goose NO TRANSACTION
-- +goose Up
-- Индексы под сортировки каталога. Без них «Популярные» (сортировка по умолчанию
-- на главной и в каталоге) и «Новинки» читали все опубликованные товары и сортировали
-- их на каждый запрос: на 50 000 товаров ~60 мс CPU БД на запрос.
-- Каждый индекс повторяет ORDER BY запроса целиком (столбец сортировки, затем p.id),
-- чтобы курсорная пагинация (столбец, id) < (...) тоже шла по индексу.
-- CONCURRENTLY: строится без блокировки записи в products на живой базе.
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_popular_idx ON products (views_count DESC, id DESC) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_new_idx ON products (created_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_price_id_idx ON products (price, id) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_cat_popular_idx ON products (category_id, views_count DESC, id DESC) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_cat_new_idx ON products (category_id, created_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_pub_store_popular_idx ON products (store_id, views_count DESC, id DESC) WHERE status = 'published';
-- заменены индексами выше (те же столбцы + id)
DROP INDEX CONCURRENTLY IF EXISTS products_public_price_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_public_idx;

-- +goose Down
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_public_idx ON products (category_id, created_at DESC) WHERE status = 'published';
CREATE INDEX CONCURRENTLY IF NOT EXISTS products_public_price_idx ON products (price) WHERE status = 'published';
DROP INDEX CONCURRENTLY IF EXISTS products_pub_store_popular_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_pub_cat_new_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_pub_cat_popular_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_pub_price_id_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_pub_new_idx;
DROP INDEX CONCURRENTLY IF EXISTS products_pub_popular_idx;
