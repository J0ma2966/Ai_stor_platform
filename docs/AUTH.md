# Вход и регистрация клиентов

Решение от 2026-11-13: клиенты регистрируются и входят **через нашего Telegram-бота,
по почте и через Google**. Вход по номеру телефона и SMS выключен (код остался,
включается флагом `SMS_AUTH_ENABLED`). Магазины и админы входят по логину и паролю, как раньше.

Сайт и приложение спрашивают у API, какие способы показывать:
`GET /v1/auth/methods` → `{email, sms, google: {client_id}|null, telegram: {bot_username}|null}`.
Кнопка появляется сама, как только способ настроен на сервере.

## Что нужно настроить на сервере (`deploy/prod/app/.env`)

| Способ | Переменные | Где взять |
|---|---|---|
| Telegram | `TELEGRAM_BOT_TOKEN`, `TELEGRAM_BOT_USERNAME`, `TELEGRAM_WEBHOOK_SECRET`, `TELEGRAM_WEBHOOK_URL=https://api.<домен>/v1/telegram/webhook` | @BotFather → /newbot. Это тот же бот, что присылает магазинам заказы |
| Почта | `MAIL_PROVIDER=smtp`, `SMTP_HOST`, `SMTP_PORT` (587 или 465), `SMTP_USER`, `SMTP_PASSWORD`, `MAIL_FROM` | Любой SMTP: Яндекс 360 для бизнеса, Google Workspace, Mailgun, SendGrid. Для доставляемости настройте SPF/DKIM домена |
| Google | `GOOGLE_CLIENT_IDS=<веб-клиент>[,<другие>]` | Google Cloud Console → APIs & Services → Credentials (см. ниже) |
| SMS (выключено) | `SMS_AUTH_ENABLED=true`, `SMS_PROVIDER=eskiz`, `ESKIZ_*` | docs/PAYMENTS.md |

В prod API не стартует, если вход по почте включён, а SMTP не настроен (`MAIL_PROVIDER=log`
разрешён только в dev/staging: код пишется в лог API).

### Google: какие клиенты создать

1. OAuth consent screen: тип External, название магазина, домен сайта, политика конфиденциальности.
2. Credentials → Create OAuth client ID → **Web application**. Authorized JavaScript origins:
   `https://<домен>` (и `http://localhost:3000` для разработки). Его ID — первым в `GOOGLE_CLIENT_IDS`:
   сайт показывает кнопку с ним, а Android-приложение просит токен для него же (serverClientId).
3. Credentials → Create OAuth client ID → **Android**: пакет `uz.boutique.client` и SHA-1 ключа
   подписи (release-ключ и, для тестов, debug). Отдельно в `GOOGLE_CLIENT_IDS` его вписывать не нужно,
   но без него Google не выдаст токен приложению.

## Как работает

### Telegram (`backend/internal/auth/telegram_login.go`, `backend/internal/telegram/login.go`)

1. `POST /v1/auth/telegram/start` → `{login_id, poll_token, url, expires_at}`; ссылка
   `https://t.me/<бот>?start=login_<login_id>` живёт 5 минут.
2. Человек открывает ссылку. Знакомому бот показывает кнопку «✅ Войти». Новому — кнопку
   «📱 Поделиться номером»: Telegram отдаёт номер, привязанный к аккаунту, он сохраняется
   в профиль (нужен для доставки). Если клиент с этим номером уже есть (старый вход по SMS),
   Telegram привязывается к нему. Номер сотрудника магазина клиентский вход не даёт.
3. Сайт/приложение раз в 2 секунды спрашивают `POST /v1/auth/telegram/poll {login_id, poll_token}`:
   `{"status":"pending"}`, затем обычная сессия (один раз). Истекла — `410 telegram_login_expired`.

Защита: `poll_token` знает только устройство, начавшее вход; подтверждает только тот
пользователь Telegram, который открыл ссылку (чужой контакт и чужое нажатие не принимаются);
бот предупреждает, что нажимать нужно, только если вы сами сейчас входите.

### Почта (`backend/internal/auth/email.go`, `backend/internal/mail`)

- `POST /v1/auth/email/code {email, purpose: register|reset, lang}` → 202 всегда (без перечисления
  аккаунтов; код на восстановление уходит только существующему клиенту). Код 6 цифр, 10 минут,
  5 попыток, повтор не чаще раза в минуту, 6 писем в час на адрес.
- `POST /v1/auth/email/register {email, code, password}` — почта сразу подтверждена; занятый
  адрес → `409 email_taken`.
- `POST /v1/auth/email/login {email, password}`; `POST /v1/auth/email/reset {email, code, password}`
  (отзывает все сессии клиента).
- SMTP только с шифрованием: порт 465 — TLS, иначе обязательный STARTTLS.

### Google (`backend/internal/auth/google.go`)

`POST /v1/auth/google/ {id_token}`. API проверяет подпись по ключам Google, `iss`, срок,
`aud ∈ GOOGLE_CLIENT_IDS`. Аккаунт ищется по `google_sub`, затем по почте, если Google её
подтвердил. Если найден аккаунт с той же почтой, но **не подтверждённой** (тестовые регистрации
до 2026-11-13), Google привязывается, а старый пароль стирается и сессии отзываются.

Сайт: кнопка Google Identity Services (`web/src/components/auth/GoogleLogin.tsx`), CSP разрешает
только `https://accounts.google.com/gsi/`. Android: Credential Manager
(`mobile/app-client/.../GoogleSignIn.kt`).

## После входа

Новый клиент (`is_new_user: true` или `registration_complete: false`) проходит экран выбора пола
и согласия. Пароль обязателен только тем, кто вошёл по SMS (`/v1/me` → `needs_password`).
В `/v1/me` есть `email`, `has_telegram`, `has_google`. Удаление аккаунта стирает почту,
пароль и привязки Telegram и Google.

Таблица `users`: `email` (+ `email_verified_at`), `telegram_user_id`, `google_sub`
(миграция `00091_client_identities.sql`).
