// Package orders — оформление заказа, окно доставки и статусы.
// Реализуется треком «Окно доставки и статусы заказа» поверх схемы из
// migrations/00001_init.sql (orders, order_items, order_deliveries, order_status_history).
//
// Правила, которые обязан соблюдать модуль:
//   - один checkout => по заказу на каждый магазин (общий checkout_id);
//   - клиент видит только свои заказы (user_id = principal.UserID),
//     магазин — только свои (store_id = principal.StoreID);
//   - магазин видит из ПД клиента только то, что нужно для доставки этого заказа;
//   - photo_key места доставки принимается только если media.OwnsKey(user, PurposeDeliveryPhoto, key);
//   - каждый переход статуса пишется в order_status_history в той же транзакции;
//   - POST оформления заказа требует заголовок Idempotency-Key (таблица idempotency_keys).
//
// Монтирование роутов — internal/server/server.go (см. TODO(orders)).
package orders
