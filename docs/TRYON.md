# AI-примерка

## Как работает

1. Клиент собирает образ в примерочной (`/fitting`): по одной вещи на зону манекена (головной убор, верх, низ, обувь). «Двойка» (см. ниже) занимает зоны «верх» и «низ». Выбор хранится в браузере (`localStorage`, ключ `fitting_v1`), добавление считается через `POST /v1/fitting/adds`: счётчик копится в Redis и раз в 30 с пишется в `products.fitting_adds` одним батчем (нет горячей строки).
2. Мастер `/fitting/new`: правила съёмки и согласие → 3 фото (спереди, сбоку, сзади; загрузка `POST /v1/uploads?purpose=body` или сохранённые фото) → необязательные параметры тела → подтверждение цены.
3. `POST /v1/tryon/generations` (заголовок `Idempotency-Key`) в **одной транзакции** проверяет согласие, регистрацию, лимит активных генераций, товары (опубликованы, одна вещь на зону), списывает токены (`generation_charge`) и ставит задание в очередь (`generations.status = queued`). Ответ `202`.
4. Воркер берёт задание (`UPDATE … FROM (SELECT … FOR UPDATE SKIP LOCKED)`), «арендует» его до `lease_until` и вызывает провайдера. Два сценария:
   - **по ракурсам** (`gemini`, `fashn`, `http`, `mock`): для каждого ракурса параллельно вызывается `Provider.Generate`, три картинки склеиваются `Compose()` в **одно изображение** (ТЗ 6.8);
   - **один запрос** (`openai`, интерфейс `CompositeProvider`): три фото клиента и фото всех выбранных вещей уходят в **один** вызов `GenerateComposite`, модель сама рисует три ракурса на одной картинке, `Compose()` не вызывается.

   Результат сохраняется в хранилище биометрии.
5. Фронт опрашивает `GET /v1/tryon/generations/{id}` каждые 3 с (ожидает → выполняется → готово / ошибка).
6. Ошибки: временные (сеть, 5xx, 429) — повтор с экспоненциальной задержкой до `TRYON_MAX_ATTEMPTS`; постоянные (нет человека на фото, модерация, фото удалено) — сразу. В обоих случаях итог `failed` + **возврат токенов** (`generation_refund`, уникальный индекс не даёт вернуть дважды). Если воркер упал, после истечения аренды задание подхватит другой.

Нагрузка: очередь в PostgreSQL сглаживает пики (5000 одновременных пользователей не превращаются в 5000 запросов к AI). Пропускная способность = `TRYON_WORKERS` × число процессов воркера; лимит на клиента — `TRYON_MAX_ACTIVE_PER_USER` активных генераций и 30 запусков в час. Метрики: `tryon_generations_total{outcome}`, `tryon_generation_seconds`, `tryon_queue_wait_seconds`.

## Двойки (парные комплекты)

«Двойка» — верх и низ одного комплекта на одном фото товара (костюм, спортивный комплект). Магазин отмечает товар галочкой «Двойка» в форме товара (сайт и приложение магазина) или столбцом `set` (да/нет) в Excel-импорте. Флаг `products.is_set` (миграция `00032`) допустим только у категорий зоны `top`: попытка поставить его товару другой зоны — `422 {is_set: top_only}`, а при переносе товара в категорию другой зоны сервер сам сбрасывает флаг.

- Примерочная (сайт и Android) не выделяет двойки в отдельный режим и подстраивается под выбор сама: двойки показываются в общем списке «Верх»; если выбрана двойка, зона и вкладка «Низ» скрываются и ранее выбранный низ убирается; если выбрать обычный верх, «Низ» снова появляется; отдельный низ, выбранный позже, вытесняет двойку. Решение пользователя от 2026-11-06: эту логику не выделять нигде в системе.
- `POST /v1/tryon/generations`: двойка вместе с низом — `422 {product_ids[i]: zone_taken}`.
- Провайдеры получают `Garment.Set=true`: OpenAI — подпись `TWO-PIECE SET` и правило «надеть обе части», низ не считается «не выбранным»; Gemini — подпись «two-piece set»; FASHN — категория `one-pieces`; свой сервер (`http`) — поле `"set": true` у вещи.
- `is_set` есть в карточке и списке товаров, в кабинете магазина и в `products[]` генерации.

## Провайдеры (`TRYON_PROVIDER`)

| Значение | Что это | Зоны | Ключ |
|---|---|---|---|
| `off` | примерка выключена целиком: `/v1/tryon/*` и `/v1/fitting/adds` отвечают `503 tryon_disabled`, загрузка фото тела запрещена, воркер не запускается; ключи и `BIOMETRIC_S3_*` не нужны даже в prod | — | не нужен |
| `mock` | демо без AI: фото клиента + карточки товаров, полоса DEMO. Только dev, в prod запрещён | все | не нужен |
| `gemini` | Google Gemini API, модель `TRYON_MODEL` (по умолчанию `gemini-2.5-flash-image`), один запрос на ракурс со всеми вещами | все 4 | `TRYON_API_KEY` (Google AI Studio) |
| `fashn` | FASHN AI `tryon-v1.6`, вещи надеваются цепочкой (низ → верх); `return_base64=true` — у FASHN результат живёт ≤ 60 мин | верх, низ | `TRYON_API_KEY` (fashn.ai) |
| `openai` | OpenAI Images API `POST /v1/images/edits`, модель `OPENAI_IMAGE_MODEL` (по умолчанию `gpt-image-2.5-sunburst`), **один запрос на всю генерацию** — см. раздел ниже | все 4 | `OPENAI_API_KEY` |
| `http` | свой сервер инференса (open-source модель на GPU в Узбекистане — биометрия не покидает страну) | по серверу | `TRYON_ENDPOINT`, опц. `TRYON_API_KEY` |

Контракт `http`: `POST TRYON_ENDPOINT` с JSON `{"angle","gender","body","person_image":"<base64>","garments":[{"zone","title","brand","image":"<base64>"}]}` → `200` и картинка в теле; `422 {"code":"photo_rejected|content_blocked|unsupported_items"}` — без повтора; `429/5xx` — повтор.

Фронт получает `supported_zones` из `GET /v1/tryon/quote` и предупреждает, какие вещи модель не переоденет.

## OpenAI: однозапросная генерация

### Как устроено

- Код: `backend/internal/tryon/openai.go` (провайдер, промпт, разбор ответа), `provider.go` (`CompositeRequest`, `CompositeProvider`), `worker.go` (`processComposite`). Интерфейс `Provider` не менялся; остальные провайдеры работают по-старому.
- Одна генерация = одна попытка воркера = **один** HTTP-запрос `POST https://api.openai.com/v1/images/edits` (multipart). Исключение: если OpenAI отверг параметр `size`, тот же запрос повторяется один раз с `size=auto` (ТЗ §2).
- В запросе 3–7 изображений в фиксированном порядке: `FRONT`, `SIDE`, `BACK`, затем выбранные вещи в порядке `HEADWEAR`, `TOP`, `BOTTOM`, `SHOES`. Промпт подписывает каждое изображение («Image 4: TOP garment reference») и перечисляет категории без референса — модель оставляет одежду клиента в этих зонах. Названия товаров (текст продавцов) в промпт не попадают.
- Для вещи берётся фронтальное фото товара. Вещей 1–4, не больше одной на зону — проверяется и при создании заказа (`422`), и в провайдере.
- Подготовка входа: каждое фото проверяется (не пустое, JPEG/PNG/WebP по сигнатуре, декодируется, не «бомба»), уменьшается до 2048 px по длинной стороне и перекодируется в JPEG. Поворот из EXIF Orientation применяется ещё при загрузке (`media.Process`).
- Параметры: `model`, `prompt`, `n=1`, `size`, `quality`, `output_format`; `input_fidelity` для семейства gpt-image-2 не передаётся.
- Ответ: `data[0].b64_json` → base64 → проверка, что это настоящее изображение разумного размера → перекодирование в JPEG → обычное сохранение результата (`priv/u/<user>/tryon/…` в хранилище биометрии), статусы и ссылка — существующим механизмом. Фронт получает `result_url` и `download_url` (с `Content-Disposition: attachment` для кнопки «Скачать»).
- Метрики: `tryon_openai_requests_total{status}` (0 — сеть/таймаут), `tryon_openai_tokens_total{kind=input|output}` — для контроля расходов.

### Ошибки

| Ответ OpenAI | Итог | Повтор |
|---|---|---|
| 400 модерация (`moderation_blocked`, safety system) | `content_blocked`, возврат токенов | нет |
| 400 по `size` | повтор с `size=auto` в той же попытке | — |
| прочие 400, 404 | `provider_error`, возврат токенов | нет |
| 401 (неверный ключ) | `provider_error`; текст ответа не сохраняется (в нём бывает фрагмент ключа) | нет |
| 403 (нет доступа к модели, организация не верифицирована) | `provider_error` | нет |
| 429 `insufficient_quota` / billing | `provider_error` (кончились деньги на счёте) | нет |
| 429 rate limit, 408, 409, 5xx | — | да, с backoff до `TRYON_MAX_ATTEMPTS` |
| таймаут `OPENAI_TIMEOUT_SECONDS`, сеть | — | да |
| пустой ответ, битый base64, не изображение | — | да; после последней попытки `provider_error` + возврат |

Токены списываются один раз при создании заказа; повтор попытки не списывает их снова, возврат делается ровно один раз (уникальный индекс в `token_ledger`). Каждая попытка — платный запрос в OpenAI, поэтому `TRYON_MAX_ATTEMPTS` (по умолчанию 3) стоит держать небольшим.

### Настройка

1. Создать ключ: [platform.openai.com](https://platform.openai.com) → *API keys* → *Create new secret key* (лучше отдельный проект с лимитом расходов). Для моделей gpt-image OpenAI может потребовать верификацию организации (*Settings → Organization → Verify*), без неё будет 403.
2. В `.env` (не в коде, не во фронтенде, не в git):

```env
TRYON_PROVIDER=openai
OPENAI_API_KEY=sk-...
OPENAI_IMAGE_MODEL=gpt-image-2.5-sunburst   # или gpt-image-2.5-flare — быстрее
OPENAI_IMAGE_QUALITY=high                   # low|medium|high|xhigh|max|auto
OPENAI_IMAGE_SIZE=1536x1024                 # auto или WxH: кратно 16, 1:3…3:1, ≤ 3840x2160
OPENAI_IMAGE_FORMAT=jpeg                    # jpeg|png|webp (в хранилище всё равно JPEG)
OPENAI_TIMEOUT_SECONDS=180                  # на один HTTP-запрос; таймаут задания = max(TRYON_TIMEOUT, это + 30 с)
OPENAI_BASE_URL=                            # пусто = https://api.openai.com
```

3. Запуск локально (Windows): `.\scripts\dev.ps1` как обычно — API прочитает `.env`. Проверка без AI: `TRYON_PROVIDER=mock`.
4. При старте проверяется: ключ задан (иначе ошибка конфигурации даже в dev — фиктивной генерации не будет), допустимые quality/format/size, в prod — `OPENAI_PROD_APPROVED=true`. Ключ не выводится в логах и не попадает в тексты ошибок.

Пример запроса, который отправляет провайдер (для ручной диагностики):

```bash
curl https://api.openai.com/v1/images/edits \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -F model=gpt-image-2.5-sunburst -F n=1 -F size=1536x1024 -F quality=high -F output_format=jpeg \
  -F prompt="$(cat prompt.txt)" \
  -F "image[]=@front.jpg" -F "image[]=@side.jpg" -F "image[]=@back.jpg" -F "image[]=@top.jpg" \
  | jq -r '.data[0].b64_json' | base64 -d > result.jpg
```

Промпт можно получить из `tryon.OpenAIPrompt` (тест `TestOpenAIPrompt`). Интеграционный тест с настоящим API (платный): `OPENAI_API_KEY_TEST=sk-... OPENAI_TEST_DIR=/путь/к/фото go test ./internal/tryon -run TestOpenAIRealAPI -v` — в каталоге `front.jpg`, `side.jpg`, `back.jpg`, `top.jpg`, результат пишется в `result.jpg`.

### Диагностика

- Генерация `failed`, `error_code=provider_error`: причина в админке (журнал генераций, поле `error`) и в логе воркера `tryon failed`/`tryon retry` (`generation_id`, текст ошибки OpenAI без фото и ключа).
- `openai auth failed (401)` — неверный/отозванный ключ. `access denied (403)` — нет доступа к модели или не пройдена верификация организации. `quota exhausted (429)` (OpenAI: `type=insufficient_quota`, `code=credit_balance_exhausted` — «You have no credits remaining») — пополнить баланс на platform.openai.com → Settings → Billing / поднять лимит проекта; генерация завершается сразу, без повторов, токены возвращаются.
- Локально картинка результата или превью фото не открывается: ссылки подписываются на `S3_PUBLIC_ENDPOINT` (в `deploy/docker-compose.yml` это `localhost:${MINIO_HOST_PORT}`); если API запущен иначе, задайте `S3_PUBLIC_ENDPOINT` вручную — адрес `minio:9000` браузеру не виден.
- Много `retry` с `openai status 429` — упёрлись в rate limit: уменьшить `TRYON_WORKERS`.
- `context deadline exceeded` — генерация дольше `OPENAI_TIMEOUT_SECONDS`: поднять таймаут или перейти на `gpt-image-2.5-flare` / `quality=medium`.
- `content_blocked` — модерация OpenAI отклонила фото; клиенту показывается просьба загрузить другие фото, токены возвращены.
- `transfer_consent_required` — у клиента нет согласия на передачу фото этому провайдеру (например, сменили `TRYON_PROVIDER`, пока задание было в очереди). Фото не отправлялись, токены возвращены.

### Ограничения

- Качество «три ракурса на одной картинке» зависит от модели: возможны разные лица на панелях, обрезанные стопы, лишние фигуры. Нужен ручной прогон на 10–20 наборах фото до включения.
- При `1536x1024` на каждый ракурс приходится ~512×1024 px; для деталей можно поднять `OPENAI_IMAGE_SIZE` (например `2304x1536`) — дороже и дольше.
- Цена генерации считается OpenAI в токенах (вход — изображения, выход — картинка), стоимость входных изображений публично не документирована: смотреть `tryon_openai_tokens_total` и биллинг OpenAI на тестовых запросах.
- Sunburst генерирует дольше Flare; при `quality=high` 180 с может не хватить.

## Биометрия и закон о локализации

Фото тела и результаты — биометрические персональные данные. Ключи объектов `priv/u/<user>/body/…` и `priv/u/<user>/tryon/…` маршрутизируются (`media.Routed`) в **отдельное хранилище** `BIOMETRIC_S3_*`, которое должно физически находиться в Узбекистане (ЗРУ «О персональных данных», ст. 27-1). В prod сервер не стартует без `BIOMETRIC_S3_ENDPOINT` и `BIOMETRIC_STORAGE_COUNTRY=UZ`. Остальные файлы (товары, доставка) могут лежать в основном хранилище (например, в Германии).

Внешним провайдерам (`openai`, `gemini`, `fashn`) фото передаются только на время генерации и **только** с отдельным информированным согласием на передачу конкретному получателю: `POST /v1/tryon/consent/transfer` сохраняет дату и имя провайдера (`users.tryon_transfer_consent_at`, `tryon_transfer_provider`, миграция `00031`). При смене `TRYON_PROVIDER` согласие нужно дать заново; без него `POST /generations` отвечает `409 tryon_transfer_consent_required`, а воркер перед отправкой проверяет согласие ещё раз. `GET /quote` отдаёт `transfer: {required, recipient, consent}`, мастер показывает отдельную галочку. Отзыв: `DELETE /v1/tryon/consent/transfer` (или общий отзыв согласия).

**OpenAI и prod.** OpenAI обрабатывает данные в США; по умолчанию хранит запросы до 30 дней для контроля злоупотреблений, на обучение данные API не использует; Zero Data Retention — только по одобрению OpenAI (и даже с ним изображения, отмеченные фильтрами, могут храниться для проверки). Поэтому prod не стартует с `TRYON_PROVIDER=openai` без явного `OPENAI_PROD_APPROVED=true`, который ставится только после заключения юриста по передаче биометрии за рубеж (ст. 27-1). Хранилище биометрии в Узбекистане и его проверки остаются включёнными при любом провайдере. Для полного исключения трансграничной передачи — провайдер `http` на своём сервере в Узбекистане. Администратор видит журнал генераций и ошибки, но **не** изображения.

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

Клиент (`/v1/tryon`, роль client): `GET /quote`, `POST|DELETE /consent`, `POST|DELETE /consent/transfer`, `GET /photos`, `PUT /photos/{front|side|back}` `{key}`, `DELETE /photos/{angle}`, `POST /generations`, `GET /generations?saved=true&cursor=`, `GET /generations/{id}`, `POST|DELETE /generations/{id}/save`, `DELETE /generations/{id}`.

Публично: `POST /v1/fitting/adds {product_id}`.

Админ: `GET /v1/admin/tryon/generations?status=&user_id=&cursor=`, `GET /v1/admin/tryon/stats`.

Коды ошибок генерации (`error_code`): `photo_rejected`, `content_blocked`, `unsupported_items`, `photo_missing`, `product_unavailable`, `provider_error`, `transfer_consent_required`.

## Запуск

- dev: `TRYON_PROVIDER=mock` (по умолчанию), воркер внутри API (`TRYON_WORKER_INPROCESS=true`).
- prod без примерки (текущее решение владельца, 2026-11-03): `TRYON_PROVIDER=off` для API и сборка фронтенда с `NEXT_PUBLIC_TRYON_ENABLED=false` — страницы примерочной, мастера, истории и фото показывают заглушку, `addToFitting` ничего не делает. Gemini/FASHN/OpenAI в prod не включать, пока владелец не вернётся к вопросу (для OpenAI это ещё и защищено флагом `OPENAI_PROD_APPROVED`).
- prod: API с `TRYON_WORKER_INPROCESS=false` + отдельный процесс `api worker` (можно несколько реплик).
