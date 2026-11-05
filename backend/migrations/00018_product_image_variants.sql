-- +goose Up
-- Уменьшенные копии фото товара (800 и 1600 px), см. media.MakeVariants.
-- pending — ещё не построены (витрина отдаёт оригинал), ready — построены,
-- failed — оригинал не читается (витрина отдаёт оригинал, повторов нет).
-- Копии строит фоновый products.Variants: и для новых фото, и для уже загруженных.
ALTER TABLE product_images ADD COLUMN variants text NOT NULL DEFAULT 'pending'
    CHECK (variants IN ('pending', 'ready', 'failed'));
CREATE INDEX product_images_variants_pending_idx ON product_images (product_id) WHERE variants = 'pending';

-- Замена фото (тот же ракурс, новый ключ) во всех местах кода (карточка, Excel,
-- массовая загрузка) сбрасывает готовность копий: их нужно построить заново.
-- +goose StatementBegin
CREATE FUNCTION product_images_reset_variants() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.key IS DISTINCT FROM OLD.key THEN
        NEW.variants := 'pending';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER product_images_reset_variants BEFORE UPDATE OF key ON product_images
    FOR EACH ROW EXECUTE FUNCTION product_images_reset_variants();

-- +goose Down
DROP TRIGGER product_images_reset_variants ON product_images;
DROP FUNCTION product_images_reset_variants();
ALTER TABLE product_images DROP COLUMN variants;
