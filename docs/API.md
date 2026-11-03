# Соглашения API

База: `/v1`. Формат: JSON, UTF-8. Деньги — целые числа в тийинах (`150000000` = 1 500 000 сум). Время — RFC 3339 UTC.

## Ответы

Успех:
```json
{ "data": { ... } }
```
Список (курсорная пагинация):
```json
{ "data": [ ... ], "next_cursor": "opaque-string" }
```
Следующая страница: `?cursor=<next_cursor>`; нет `next_cursor` — это последняя страница. `?limit=` ограничен сервером.

Ошибка:
```json
{ "error": { "code": "validation_failed", "message": "...", "fields": { "phone": "e164" } }, "request_id": "a1b2c3..." }
```
`code` стабилен — фронт переводит его на ru/uz. `request_id` показывается пользователю в сообщении об ошибке для поддержки.

| HTTP | Типичные `code` |
|---|---|
| 400 | `invalid_json`, `bad_cursor`, `bad_filter`, `bad_sort` |
| 401 | `unauthorized`, `token_invalid` (нужен refresh), `token_revoked` (аккаунт заблокирован, на экран входа), `otp_invalid`, `bad_credentials`, `refresh_invalid` |
| 403 | `forbidden`, `account_blocked`, `store_not_active` |
| 404 | `not_found` (в том числе для чужих объектов) |
| 409 | `conflict`, `sku_taken`, `slug_taken`, `login_taken` |
| 413/415 | `payload_too_large`, `file_too_large`, `unsupported_type` |
| 422 | `validation_failed` (+ `fields`), `invalid_purpose`, `insufficient_tokens`, `phone_not_allowed` |
| 429 | `rate_limited`, `otp_cooldown` (+ заголовок `Retry-After`) |
| 500/503 | `internal`, `unavailable` |

## Аутентификация

1. `POST /v1/auth/otp/request {"phone":"+998901234567"}` → 202.
2. `POST /v1/auth/otp/verify {"phone":"...","code":"123456"}` → `{access_token, expires_at, is_new_user, user}` + cookie `rt`.
3. Новый клиент: `POST /v1/me/complete-registration {"gender":"female","pd_consent":true,"lang":"ru"}` → начисляется 20 токенов (один раз).
4. Запросы: `Authorization: Bearer <access_token>`.
5. При `401 token_invalid`: `POST /v1/auth/refresh` (cookie отправляется автоматически, `credentials: "include"`) → новый access-токен. Если и он 401 — на экран входа.
6. `POST /v1/auth/logout`.

Магазин и админ: `POST /v1/auth/login {"login","password"}`.

## Идемпотентность

Операции, которые нельзя повторять (оформление заказа, запуск генерации, создание платежа), должны принимать заголовок `Idempotency-Key: <uuid>`. Повтор с тем же ключом возвращает первый ответ. Фронт генерирует ключ при открытии формы подтверждения.

## Загрузка файлов

`POST /v1/uploads?purpose=<product|store|body|delivery>`, тело — сам файл (JPEG/PNG/WebP, до 10 МБ). Ответ `{key, url}`. Для `product` и `store` `url` постоянный (CDN), для `body` и `delivery` это подписанная ссылка на 15 минут. Затем `key` передаётся в бизнес-запрос (товар, заказ). Чужие ключи отклоняются. Одна загрузка на пользователя одновременно (`429 upload_in_progress`). Если сервер перегружен, ответ `503 upload_busy` с `Retry-After`.

| purpose | кто может |
|---|---|
| `body`, `delivery` | клиент |
| `product`, `store` | магазин, админ |

## Реализованные эндпоинты

| Метод | Путь | Роль |
|---|---|---|
| GET | `/healthz`, `/readyz` | — |
| POST | `/v1/auth/otp/request`, `/v1/auth/otp/verify`, `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout` | — |
| GET | `/v1/categories`, `/v1/products`, `/v1/products/{id}` | — |
| GET | `/v1/me` | любая |
| PATCH | `/v1/me` | client |
| POST | `/v1/me/complete-registration` | client |
| GET | `/v1/me/tokens` | client |
| POST | `/v1/uploads` | любая |
| GET/POST | `/v1/store/products` | store |
| PATCH | `/v1/store/products/{id}/status` | store |
| POST | `/v1/admin/stores`, `/v1/admin/stores/{id}/status`, `/v1/admin/users/{id}/status`, `/v1/admin/users/{id}/tokens` | admin |
| POST | `/v1/admin/stores/{id}/password` — новый пароль магазина, все его сессии завершаются → 204 | admin |
| GET | `/v1/admin/stats` — сводка для панели (магазины, клиенты, заказы по статусам, токены за 30 дней) | admin |
| GET | `/v1/admin/stores?status=&q=`, `/v1/admin/stores/{id}` (+ последние 20 записей журнала) | admin |
| GET | `/v1/admin/users?role=client\|store\|admin&status=&q=`, `/v1/admin/users/{id}` (+ движение токенов и журнал) | admin |
| GET | `/v1/admin/audit?action=&entity_type=&entity_id=&actor_id=` (`action=store.` — все действия с префиксом; курсор — id записи) | admin |
| POST/GET | `/v1/orders`, `/v1/orders/{id}` | client |
| GET/POST | `/v1/store/orders`, `/v1/store/orders/{id}`, `/v1/store/orders/{id}/status` | store |
| GET/POST | `/v1/admin/orders`, `/v1/admin/orders/{id}`, `/v1/admin/orders/{id}/status` | admin |
| GET | `/v1/tariffs` | — |
| GET/POST | `/v1/payments`, `/v1/payments/{id}` | client |
| GET/POST/PATCH | `/v1/admin/tariffs`, `/v1/admin/tariffs/{id}`; GET `/v1/admin/payments` | admin |
| POST | `/v1/payments/click/prepare`, `/v1/payments/click/complete`, `/v1/payments/payme` | Click / Payme |

Заказы и окно доставки подробно: [ORDERS.md](ORDERS.md). Оплата токенов и SMS: [PAYMENTS.md](PAYMENTS.md).

Веб-панель суперадмина: `/{ru|uz}/admin` (вход по логину и паролю администратора, созданного командой `api create-admin`). Тексты панели — `web/messages/admin.{ru,uz}.json`.

Фильтры каталога: `q`, `category`, `store`, `brand`, `price_min`, `price_max`, `in_stock=true`, `sort=new|price_asc|price_desc|popular`.

## Как добавить модуль

1. Миграция `backend/migrations/0000N_<name>.sql` (`-- +goose Up` / `-- +goose Down`).
2. Пакет `backend/internal/<module>` с `Handlers{DB ...}` и методом `Routes(chi.Router)`.
3. Монтирование в `internal/server/server.go` в нужной группе ролей.
4. Каждый запрос магазина/клиента фильтруется по `auth.MustFrom(ctx)`.
5. Интеграционный тест в стиле `internal/server/server_test.go`.
