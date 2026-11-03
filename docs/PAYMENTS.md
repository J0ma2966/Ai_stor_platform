# SMS и оплата токенов (Eskiz, Click, Payme)

## Что где лежит

| Что | Код |
|---|---|
| Отправка SMS-кода | `backend/internal/sms` (Eskiz), подключение в `cmd/api/main.go` |
| Тарифы, платежи, вебхуки | `backend/internal/payments` |
| Таблицы | `payments` (миграция 00001 + 00020), `payment_transactions` (00020), `token_ledger` |
| Страница покупки | `web/src/app/[locale]/account/balance`, `web/src/components/BalancePage.tsx` |

## Как проходит покупка

1. Клиент открывает `/account/balance`, выбирает тариф и Click или Payme.
2. Фронт вызывает `POST /v1/payments {tariff_id, provider}` с `Idempotency-Key`. Сервер копирует в платёж цену и число токенов тарифа (позже тариф можно менять, оплаченные платежи не изменятся) и возвращает `checkout_url`.
3. Клиент платит на странице провайдера, провайдер вызывает наш вебхук, мы зачисляем токены.
4. Провайдер возвращает клиента на `PAYMENT_RETURN_URL?payment=<id>`, страница опрашивает `GET /v1/payments/{id}` и показывает итог.

Брошенные неоплаченные платежи janitor закрывает через 24 часа (`cancelled`), вместе с незавершённой подготовкой Click: поздний Complete получит -9.

## Защита от двойного зачисления

- Каждый вебхук работает в одной транзакции и сначала блокирует строку платежа (`FOR UPDATE`), поэтому параллельные повторы выполняются по очереди.
- Токены зачисляются только при переходе `pending → paid`, в той же транзакции.
- В БД: уникальный индекс `token_ledger_ref_once` (одно зачисление на платёж), `payment_transactions_one_active` (одна живая транзакция провайдера на платёж), `UNIQUE (provider, external_id)`.
- Повтор Complete (Click) и PerformTransaction (Payme) возвращает тот же ответ без нового зачисления.
- `scripts/consistency-check.sql` проверяет: каждый оплаченный платёж зачислен ровно один раз и имеет ровно одну проведённую транзакцию, суммы совпадают.
- Тесты гоняют по 10 параллельных подтверждений и проверяют, что зачисление одно.

## Eskiz (SMS)

Нужно: email и пароль от кабинета my.eskiz.uz, одобренный шаблон SMS.

```
SMS_PROVIDER=eskiz
ESKIZ_EMAIL=...
ESKIZ_PASSWORD=...
ESKIZ_FROM=4546                 # или своё имя отправителя после регистрации
SMS_OTP_TEMPLATE=Kod dlya vhoda v Butiki: %s
```

Пока аккаунт в тестовом режиме, Eskiz пропускает только текст `Bu Eskiz dan test` (без кода). Для проверки связки можно поставить `SMS_OTP_TEMPLATE=Bu Eskiz dan test`, но войти так нельзя. Для реальной работы отправьте шаблон на модерацию в кабинете, текст в `SMS_OTP_TEMPLATE` должен совпадать с одобренным, а код стоит на месте `%s`. Токен Eskiz живёт 30 дней, сервер перевыпускает его сам при ответе 401. В prod сервер не стартует с `SMS_PROVIDER=log`.

## Click (SHOP API)

Нужно из кабинета merchant.click.uz: `service_id`, `merchant_id`, `secret_key`.

В настройках сервиса указать:
- Prepare URL: `https://<api-домен>/v1/payments/click/prepare`
- Complete URL: `https://<api-домен>/v1/payments/click/complete`

Подпись: `md5(click_trans_id + service_id + secret_key + merchant_trans_id [+ merchant_prepare_id] + amount + action + sign_time)`. `merchant_trans_id` — id платежа (uuid), `merchant_prepare_id`/`merchant_confirm_id` — номер нашей транзакции. Сумма у Click в сумах, у нас в тийинах.

| error | когда |
|---|---|
| 0 | успех (в том числе повтор Prepare/Complete) |
| -1 | неверная подпись, провайдер выключен, IP не из списка |
| -2 | сумма не совпадает с платежом |
| -3 | неверный action |
| -4 | платёж уже оплачен |
| -5 | платёж не найден или создан для Payme |
| -6 | транзакция не найдена (Complete без Prepare) |
| -7 | наша внутренняя ошибка (Click повторит) |
| -8 | чужой service_id |
| -9 | транзакция отменена; Complete с error < 0 от Click отменяет платёж (`declined`) |

Проверка: в кабинете Click есть тестовый инструмент для Prepare/Complete, им можно прогнать все сценарии до боевого запуска.

## Payme (Merchant API)

Нужно из кабинета business.payme.uz: ID кассы (`PAYME_MERCHANT_ID`) и ключ (тестовый для песочницы, потом боевой) в `PAYME_KEY`.

В настройках кассы:
- Endpoint URL: `https://<api-домен>/v1/payments/payme`
- Поле account: `payment_id` (или своё имя, тогда `PAYME_ACCOUNT_KEY`)

Авторизация: Basic `Paycom:<ключ>`, иначе `-32504`. Поддержаны все методы: CheckPerformTransaction, CreateTransaction, PerformTransaction, CancelTransaction, CheckTransaction, GetStatement. Транзакция в состоянии 1 дольше 12 часов отменяется (reason 4). Отмена проведённой транзакции возвращает `-31007`: токены зачисляются сразу и могли быть потрачены, возврат делается через поддержку (корректировка баланса админом). Вторая транзакция на тот же платёж возвращает `-31099`.

Песочница: `PAYME_TEST=true` (ссылки на checkout.test.paycom.uz; по умолчанию true вне prod и false в prod), затем прогнать все сценарии на test.paycom.uz. Перед боевым запуском: боевой ключ, `PAYME_TEST=false`.

Фискальный чек: Payme требует ИКПУ (МХИК) для позиций. Задайте `PAYME_IKPU`, `PAYME_PACKAGE_CODE`, `PAYME_VAT_PERCENT` — тогда CheckPerformTransaction отдаёт `detail` с позицией «<тариф> (N AI-токенов)».

## Ограничение по IP (обязательно в prod)

`CLICK_ALLOWED_IPS` и `PAYME_ALLOWED_IPS` — списки IP/подсетей через запятую (адреса выдают провайдеры). В prod сервер не стартует, если провайдер включён, а список пуст: поле `error` в Click Complete не входит в подпись, а ключ Payme статичен. В dev пусто — проверяется только подпись/ключ. За балансировщиком нужен `TRUSTED_PROXY=true`, иначе сервер увидит IP балансировщика.

## API

| Метод | Путь | Кто |
|---|---|---|
| GET | `/v1/tariffs` | все (опубликованные, кэш 60 с) |
| POST | `/v1/payments` `{tariff_id, provider}` + `Idempotency-Key` | client → 201 (повтор с тем же ключом → 200, тот же платёж) |
| GET | `/v1/payments`, `/v1/payments/{id}` | client, только свои |
| GET/POST | `/v1/admin/tariffs` | admin |
| PATCH | `/v1/admin/tariffs/{id}` | admin |
| GET | `/v1/admin/payments?status=&provider=&user_id=&cursor=` | admin |
| POST | `/v1/payments/click/prepare`, `/v1/payments/click/complete` | Click (подпись) |
| POST | `/v1/payments/payme` | Payme (Basic) |

Ошибки создания платежа: `422 provider_unavailable` (ключи провайдера не заданы), `422 tariff_not_found`, `422 idempotency_key_reused`, `429 rate_limited` (10 в минуту).
