# Заказы, окно доставки и статусы

Модуль `backend/internal/orders`. Тесты: `internal/orders/orders_test.go` (логика) и `internal/server/orders_test.go` (API на реальной БД).

## Как это работает

- Одно оформление (checkout) создаёт **отдельный заказ на каждый магазин** из корзины (общий `checkout_id`). Каждый бутик видит только свои заказы.
- Цена, название, бренд и фото товара копируются в заказ на момент оформления (снимок).
- Оформленные товары удаляются из корзины клиента.
- Статусы меняет только магазин этого заказа или админ, строго по схеме:

```
pending ─┬─> confirmed ──> assembling ──> in_delivery ──> delivered
         ├─> rejected            (причина обязательна)
         └─> cancelled  (из pending / confirmed / assembling, причина обязательна)
```

Каждый переход пишется в `order_status_history` (кто, когда, из какого в какой, причина) и в `audit_log` в той же транзакции. Строка заказа блокируется (`FOR UPDATE`), поэтому две вкладки магазина не могут сделать переход одновременно; поле `from` позволяет фронту получить `409 status_changed`, если статус уже изменили.

Для клиента статус показывается шкалой `step`: 1 подтверждён, 2 собирается, 3 передан в доставку, 4 доставлен; 0 означает «ожидает подтверждения», «отклонён» или «отменён» (смотреть `status`).

## Эндпоинты

| Метод | Путь | Роль | Что делает |
|---|---|---|---|
| POST | `/v1/orders` | client | Оформить заказ. Заголовок `Idempotency-Key` обязателен. |
| GET | `/v1/orders?status=&cursor=&limit=` | client | Мои заказы. |
| GET | `/v1/orders/{id}` | client | Мой заказ: товары, адрес, статус, история. |
| GET | `/v1/store/orders?status=pending,confirmed&cursor=` | store | Заказы моего магазина. |
| GET | `/v1/store/orders/{id}` | store | Окно доставки: адрес, локация, фото, комментарии, `copy_text`, `next_statuses`. |
| POST | `/v1/store/orders/{id}/status` | store | `{"status":"confirmed","from":"pending","reason":""}` |
| GET/POST | `/v1/admin/orders`, `/v1/admin/orders/{id}`, `/v1/admin/orders/{id}/status` | admin | То же для всех магазинов. |

### Оформление

```json
POST /v1/orders
Idempotency-Key: 7b0c...-uuid
{
  "items": [{"product_id": "uuid", "qty": 1}],
  "customer_name": "Азиз",
  "customer_phone": "+998901234567",          // необязательно, по умолчанию телефон аккаунта
  "customer_comment": "Позвоните за час",      // комментарий к заказу, до 1000 символов
  "delivery_time_comment": "после 18:00",      // комментарий ко времени доставки, до 500
  "delivery": {
    "street": "ул. Навои", "house": "5А",       // обязательно
    "entrance": "2", "floor": "7", "apartment": "41", "intercom": "41К",
    "address_comment": "вход со двора",
    "lat": 41.311081, "lng": 69.240562,        // оба или ни одного
    "photo_key": "u/<user>/delivery/<hex>.jpg" // из POST /v1/uploads?purpose=delivery
  }
}
→ 201 {"data": {"checkout_id": "...", "orders": [{"id","number","store_id","store_name","status","items_total"}]}}
```

Ошибки: `400 idempotency_key_required`, `422 validation_failed` (поля `delivery.house`, `delivery.location`, `items[0].qty`, ...), `422 idempotency_key_reused`, `409 products_unavailable` (в `fields` по id товара: `out_of_stock` или `unavailable`), `429 rate_limited` (20 оформлений в час на клиента).

## Безопасность

- Адрес, геолокация, телефон и фото места доставки — персональные данные. Их видят только клиент заказа, магазин этого заказа и админ. Проверка идёт в SQL (`o.user_id = $principal` / `o.store_id = $principal.store`); чужой заказ отвечает `404`, а не `403`.
- Фото: загружается через общий модуль `media` (проверка сигнатуры файла, лимит размера, перекодирование в JPEG без EXIF/GPS, приватный бакет). При оформлении принимается только ключ, загруженный этим клиентом с `purpose=delivery`. Ссылка на фото подписанная и живёт 5 минут; в `copy_text` она не попадает.
- Тексты: удаляются управляющие и невидимые символы (в т.ч. смена направления текста U+202E), лимиты длины совпадают с CHECK в БД, хранятся как обычный текст. Фронт выводит их только как текст (React экранирует), `copy_text` кладётся в буфер обмена как текст.
- Фото места доставки хранится **30 дней** после завершения заказа (доставлен, отклонён или отменён), затем janitor удаляет объект из хранилища и обнуляет `photo_key` (`orders.PurgeDeliveryPhotos`, раз в час). Если одно фото прикреплено к нескольким заказам одного оформления, оно удаляется, когда срок истёк у всех.
- Ответы с ПД отдаются с `Cache-Control: no-store`.
- Неизвестные поля в JSON отклоняются (нельзя передать `status` или `items_total` при оформлении).
