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

Клиенты входят через Telegram-бота, по почте и через Google — подробно в [AUTH.md](AUTH.md).
Каждый способ возвращает одинаковую сессию `{access_token, expires_at, is_new_user, user}` + cookie `rt`.

1. `GET /v1/auth/methods` — какие способы включены; затем, например,
   `POST /v1/auth/email/login {"email","password"}` или Telegram/Google (AUTH.md).
3. Новый клиент: `POST /v1/me/complete-registration {"gender":"female","pd_consent":true,"lang":"ru"}` → начисляется 20 токенов (один раз).
4. Запросы: `Authorization: Bearer <access_token>`.
5. При `401 token_invalid`: `POST /v1/auth/refresh` (cookie отправляется автоматически, `credentials: "include"`) → новый access-токен. Если и он 401 — на экран входа.
6. `POST /v1/auth/logout`.

### Пароль клиента и вход по SMS (только при `SMS_AUTH_ENABLED=true`, в prod выключено)

SMS стоит денег, поэтому код нужен только для регистрации и восстановления пароля.

- После регистрации по SMS клиент обязан задать пароль: `GET /v1/me` → `needs_password: true`,
  фронт ведёт на `/account/password` (после `/account/welcome`).
- `POST /v1/auth/password {"password"}` (client) — первый пароль, 10–128 символов.
  Смена: `{"password","current_password"}`; без текущего → `422 current_password_required`,
  неверный → `422 current_password_wrong`. Лимит 10/15 мин на пользователя.
- `POST /v1/auth/password/login {"phone","password"}` → сессия, как у `/otp/verify`.
  Клиент без пароля, неизвестный номер и неверный пароль дают одинаковый `401 bad_credentials`.
  Лимиты: 20/15 мин с IP, 10/15 мин на номер+IP, 50/15 мин на номер (если исчерпан — вход по SMS).
- «Забыли пароль?»: `POST /v1/auth/otp/request`, затем
  `POST /v1/auth/password/reset {"phone","code","password"}` → новая сессия. Неизвестный номер
  при этом регистрируется (`is_new_user: true`), чтобы ответ не выдавал наличие аккаунта.
- Смена и сброс пароля отзывают все refresh-токены клиента: другие устройства выходят,
  когда истечёт их access-токен (≤ 15 мин). Текущее устройство получает новую сессию и cookie.
- Удаление аккаунта стирает пароль.

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
| GET | `/v1/auth/methods` | — |
| POST | `/v1/auth/login`, `/v1/auth/refresh`, `/v1/auth/logout` | — |
| POST | `/v1/auth/email/code`, `/v1/auth/email/register`, `/v1/auth/email/login`, `/v1/auth/email/reset` (при `EMAIL_AUTH_ENABLED`) | — |
| POST | `/v1/auth/google/` (при `GOOGLE_CLIENT_IDS`), `/v1/auth/telegram/start`, `/v1/auth/telegram/poll` (при настроенном боте) | — |
| POST | `/v1/auth/otp/request`, `/v1/auth/otp/verify`, `/v1/auth/password/login`, `/v1/auth/password/reset` (только при `SMS_AUTH_ENABLED`) | — |
| POST | `/v1/auth/password` | client |
| GET | `/v1/categories`, `/v1/products`, `/v1/products/{id}` | — |
| POST | `/v1/products/{id}/view` — браузер отмечает открытие карточки (204; 60/мин с IP). `GET /v1/products/{id}` просмотры не считает: его кэширует серверный рендеринг сайта | — |
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
| POST/GET | `/v1/orders`, `/v1/orders/{id}` (позиция: `{"product_id","size","qty"}`; `size` обязателен у товара с размерами) | client |
| POST | `/v1/orders/{id}/cancel` `{"reason"?}` — отмена клиентом в статусах pending/confirmed, иначе 409 `cannot_cancel`; остатки размеров возвращаются | client |
| GET/POST | `/v1/store/orders`, `/v1/store/orders/{id}`, `/v1/store/orders/{id}/status` | store |
| GET/POST | `/v1/admin/orders`, `/v1/admin/orders/{id}`, `/v1/admin/orders/{id}/status` | admin |
| GET | `/v1/tariffs` | — |
| GET/POST | `/v1/payments`, `/v1/payments/{id}` | client |
| GET/POST/PATCH | `/v1/admin/tariffs`, `/v1/admin/tariffs/{id}`; GET `/v1/admin/payments` | admin |
| POST | `/v1/payments/click/prepare`, `/v1/payments/click/complete`, `/v1/payments/payme` | Click / Payme |

| GET | `/v1/stores`, `/v1/stores/{slug}` | — (публичные бутики) |
| GET | `/v1/platform` | — (стоимость генерации, контакты поддержки) |
| DELETE | `/v1/me` `{"confirm":true}` | client (удаление аккаунта и ПД) |
| GET/PUT/DELETE | `/v1/me/cart`, `/v1/me/cart/{product_id}?size=M` `{"qty"}` (ошибки `size_required`, `size_unknown`, `out_of_stock`, `not_enough_stock`) | client |
| GET/PUT/DELETE | `/v1/me/favorites`, `/v1/me/favorites/ids`, `/v1/me/favorites/{product_id}` | client |
| GET/POST | `/v1/me/notifications`, `/v1/me/notifications/unread-count`, `/v1/me/notifications/read` `{"ids":[..]}` или `{"all":true}` | любая |
| GET/PATCH | `/v1/store/profile` | store |
| GET | `/v1/store/stats?from=YYYY-MM-DD&to=YYYY-MM-DD` | store |
| GET/PATCH/DELETE | `/v1/store/products/{id}` (`"sizes":[{"size":"M","stock":3}]`, `[]` — убрать размеры; при создании тоже можно передать `sizes`) | store |
| GET/POST/DELETE | `/v1/store/telegram`, `/v1/store/telegram/link` — статус, одноразовая ссылка t.me/<бот>?start=<код> (15 мин), отключение | store |
| POST | `/v1/telegram/webhook` — обновления Telegram, заголовок `X-Telegram-Bot-Api-Secret-Token` | Telegram |
| GET | `/v1/store/products/import/template` (xlsx) | store |
| POST | `/v1/store/products/import` (тело — .xlsx, до 5 МБ, 2000 строк) | store |
| POST | `/v1/store/products/images/bulk` `{"items":[{"filename":"SKU_front.jpg","key":"..."}]}` | store |
| GET/POST | `/v1/admin/products`, `/v1/admin/products/{id}/status` `{"status":"unpublished|blocked|draft","reason"}` | admin |
| GET/POST/PATCH | `/v1/admin/categories`, `/v1/admin/categories/{id}` | admin |
| GET/PUT | `/v1/admin/settings` | admin |

Заказы и окно доставки подробно: [ORDERS.md](ORDERS.md).
Заказы и окно доставки подробно: [ORDERS.md](ORDERS.md). Оплата токенов и SMS: [PAYMENTS.md](PAYMENTS.md).

Веб-панель суперадмина: `/{ru|uz}/admin` (вход по логину и паролю администратора, созданного командой `api create-admin`). Тексты панели — `web/messages/admin.{ru,uz}.json`.

## Корзина, избранное, уведомления

- Корзина серверная: добавить можно только опубликованный товар активного магазина в наличии. Если товар потом закончился или снят, позиция остаётся с `available=false` и `unavailable_reason` (`out_of_stock` / `unavailable`), а `can_checkout=false`. После оформления заказа оформленные позиции удаляются из корзины.
- Уведомления: `kind` + `data` (без ПД), тексты переводит фронт. Модули создают их через `notify.Send` / `notify.SendToStore` в своей транзакции. Виды: `order_created` (магазину), `order_status` (клиенту), `product_moderated` (магазину), `payment_paid`, `payment_failed`, `generation_succeeded`, `generation_failed`, `review_created` (магазину), `review_reply`, `review_hidden` (клиенту), `system`. Хранятся 180 дней.

## Товары магазина

- Опубликовать можно только товар с тремя ракурсами фото (`422 images_required`).
- `is_set` в `POST`/`PATCH /v1/store/products` — «двойка»: верх и низ на одном фото. Только для категорий зоны `top` (иначе `422 {is_set: top_only}`), при переносе в категорию другой зоны сбрасывается. Подробнее — docs/TRYON.md, «Двойки».
- Фото в ответах: `images` — большое фото для страницы товара (1600 px), `thumbs` — карточки и миниатюры (800 px). Копии строит фоновая задача (`products.Variants`, `product_images.variants`) в течение ~15 с после сохранения товара; пока их нет, обе ссылки ведут на оригинал (до 4096 px). Оригинал остаётся для AI-примерки.
- Модерация: `unpublished` — снят администратором с причиной; магазин видит причину, после правки товар становится черновиком и его можно опубликовать снова. `blocked` — магазин не может ни редактировать, ни удалять, ни публиковать.
- Удаление товара: в заказах остаётся снимок (название, цена, фото), файлы, на которые ссылаются заказы, не удаляются.
- Импорт Excel: столбцы `sku, title, description, category, brand, price, in_stock`. `category` — slug или название ru/uz, `price` — в сумах, `in_stock` — да/нет; необязательные `sizes` и `set` (двойка: да/нет, только категории верха). Новые товары создаются черновиками, существующие (по `sku`) обновляются. Пустой `sku` — новый товар с автоартикулом. Ответ: `{total, created, updated, failed, errors:[{row, field, code}], generated:[{row, sku}]}`.
- Артикул (`sku`) уникален в пределах магазина. В `POST /v1/store/products` он необязателен: если пуст, сервер выдаёт следующий номер магазина `A-000001`, `A-000002`, … (счётчик `store_sku_counters`, номера, занятые вручную, пропускаются). Ответ создания: `{id, sku, status}`.
- Массовые фото: файлы загружаются через `/v1/uploads?purpose=product`, затем ключи привязываются по имени `АРТИКУЛ_ракурс.jpg` (ракурс: `front|side|back`, `1|2|3`, `спереди|сбоку|сзади`).
- Статистика: просмотры и добавления в примерочную по дням (`product_stats_daily`), примерки — успешные генерации с товарами магазина, заказы и выручка (доставленные) за период; топ-10 товаров.

Фильтры каталога: `q`, `category`, `store`, `brand`, `price_min`, `price_max`, `in_stock=true`, `sort=new|price_asc|price_desc|popular|rating`. В товаре есть `rating_avg` (0 — отзывов нет) и `rating_count`, в карточке ещё `rating_dist` (число отзывов на 1…5 звёзд).

Отзывы и оценки: [REVIEWS.md](REVIEWS.md).
Реклама товаров магазинов (заявки, решения суперадмина, карусель на главной): [PROMOTIONS.md](PROMOTIONS.md).

## Как добавить модуль

1. Миграция `backend/migrations/0000N_<name>.sql` (`-- +goose Up` / `-- +goose Down`).
2. Пакет `backend/internal/<module>` с `Handlers{DB ...}` и методом `Routes(chi.Router)`.
3. Монтирование в `internal/server/server.go` в нужной группе ролей.
4. Каждый запрос магазина/клиента фильтруется по `auth.MustFrom(ctx)`.
5. Интеграционный тест в стиле `internal/server/server_test.go`.
