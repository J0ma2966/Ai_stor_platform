# AI-примерка

## Как работает

1. Клиент собирает образ в примерочной (`/fitting`): по одной вещи на зону манекена (головной убор, верх, низ, обувь). Выбор хранится в браузере (`localStorage`, ключ `fitting_v1`), добавление считается в `products.fitting_adds` через `POST /v1/fitting/adds`.
2. Мастер `/fitting/new`: правила съёмки и согласие → 3 фото (спереди, сбоку, сзади; загрузка `POST /v1/uploads?purpose=body` или сохранённые фото) → необязательные параметры тела → подтверждение цены.
3. `POST /v1/tryon/generations` (заголовок `Idempotency-Key`) в **одной транзакции** проверяет согласие, регистрацию, лимит активных генераций, товары (опубликованы, одна вещь на зону), списывает токены (`generation_charge`) и ставит задание в очередь (`generations.status = queued`). Ответ `202`.
4. Воркер берёт задание (`UPDATE … FROM (SELECT … FOR UPDATE SKIP LOCKED)`), «арендует» его до `lease_until`, для каждого ракурса параллельно вызывает AI-провайдера и склеивает три ракурса в **одно изображение** (ТЗ 6.8). Результат сохраняется в хранилище биометрии.
5. Фронт опрашивает `GET /v1/tryon/generations/{id}` каждые 3 с (ожидает → выполняется → готово / ошибка).
6. Ошибки: временные (сеть, 5xx, 429) — повтор с экспоненциальной задержкой до `TRYON_MAX_ATTEMPTS`; постоянные (нет человека на фото, модерация, фото удалено) — сразу. В обоих случаях итог `failed` + **возврат токенов** (`generation_refund`, уникальный индекс не даёт вернуть дважды). Если воркер упал, после истечения аренды задание подхватит другой.

Нагрузка: очередь в PostgreSQL сглаживает пики (5000 одновременных пользователей не превращаются в 5000 запросов к AI). Пропускная способность = `TRYON_WORKERS` × число процессов воркера; лимит на клиента — `TRYON_MAX_ACTIVE_PER_USER` активных генераций и 30 запусков в час. Метрики: `tryon_generations_total{outcome}`, `tryon_generation_seconds`, `tryon_queue_wait_seconds`.

## Провайдеры (`TRYON_PROVIDER`)

| Значение | Что это | Зоны | Ключ |
|---|---|---|---|
| `mock` | демо без AI: фото клиента + карточки товаров, полоса DEMO. Только dev, в prod запрещён | все | не нужен |
| `gemini` | Google Gemini API, модель `TRYON_MODEL` (по умолчанию `gemini-2.5-flash-image`), один запрос на ракурс со всеми вещами | все 4 | `TRYON_API_KEY` (Google AI Studio) |
| `fashn` | FASHN AI `tryon-v1.6`, вещи надеваются цепочкой (низ → верх); `return_base64=true` — у FASHN результат живёт ≤ 60 мин | верх, низ | `TRYON_API_KEY` (fashn.ai) |
| `http` | свой сервер инференса (open-source модель на GPU в Узбекистане — биометрия не покидает страну) | по серверу | `TRYON_ENDPOINT`, опц. `TRYON_API_KEY` |

Контракт `http`: `POST TRYON_ENDPOINT` с JSON `{"angle","gender","body","person_image":"<base64>","garments":[{"zone","title","brand","image":"<base64>"}]}` → `200` и картинка в теле; `422 {"code":"photo_rejected|content_blocked|unsupported_items"}` — без повтора; `429/5xx` — повтор.

Фронт получает `supported_zones` из `GET /v1/tryon/quote` и предупреждает, какие вещи модель не переоденет.

## Биометрия и закон о локализации

Фото тела и результаты — биометрические персональные данные. Ключи объектов `priv/u/<user>/body/…` и `priv/u/<user>/tryon/…` маршрутизируются (`media.Routed`) в **отдельное хранилище** `BIOMETRIC_S3_*`, которое должно физически находиться в Узбекистане (ЗРУ «О персональных данных», ст. 27-1). В prod сервер не стартует без `BIOMETRIC_S3_ENDPOINT` и `BIOMETRIC_STORAGE_COUNTRY=UZ`. Остальные файлы (товары, доставка) могут лежать в основном хранилище (например, в Германии).

Провайдерам `gemini`/`fashn` фото передаются только на время генерации; клиент даёт отдельное согласие на это (`POST /v1/tryon/consent`, текст в мастере). Для полного исключения трансграничной передачи — провайдер `http` на своём сервере в Узбекистане. Администратор видит журнал генераций и ошибки, но **не** изображения.

## Сроки хранения (по умолчанию; согласовать с юристом — ТЗ, раздел 20)

| Данные | Срок | Переменная |
|---|---|---|
| Несохранённые фото | 24 ч после завершения генерации | `TRYON_PHOTO_TTL` |
| Загруженные, но не использованные фото | 24 ч | — |
| Сохранённые фото | 180 дней с последнего использования | `TRYON_SAVED_PHOTO_TTL` |
| Результат без «Сохранить образ» | 30 дней | `TRYON_RESULT_TTL` |
| Сохранённый образ | 365 дней с сохранения | `TRYON_SAVED_LOOK_TTL` |

Удаляет janitor (раз в час, один инстанс по advisory lock). Клиент может в любой момент удалить примерку, фото или отозвать согласие (`DELETE /v1/tryon/consent`) — тогда удаляется вся его биометрия. Для удаления аккаунта есть `tryon.PurgeUser`.

## API

Клиент (`/v1/tryon`, роль client): `GET /quote`, `POST|DELETE /consent`, `GET /photos`, `PUT /photos/{front|side|back}` `{key}`, `DELETE /photos/{angle}`, `POST /generations`, `GET /generations?saved=true&cursor=`, `GET /generations/{id}`, `POST|DELETE /generations/{id}/save`, `DELETE /generations/{id}`.

Публично: `POST /v1/fitting/adds {product_id}`.

Админ: `GET /v1/admin/tryon/generations?status=&user_id=&cursor=`, `GET /v1/admin/tryon/stats`.

Коды ошибок генерации (`error_code`): `photo_rejected`, `content_blocked`, `unsupported_items`, `photo_missing`, `product_unavailable`, `provider_error`.

## Запуск

- dev: `TRYON_PROVIDER=mock` (по умолчанию), воркер внутри API (`TRYON_WORKER_INPROCESS=true`).
- prod: API с `TRYON_WORKER_INPROCESS=false` + отдельный процесс `api worker` (можно несколько реплик).
