# Временный вход по почте (только на период тестирования)

Пока нет боевого SMS-провайдера, клиент может зарегистрироваться и войти по
электронной почте и паролю. Это **временная** функция: после окончания
тестирования её удаляют.

## Как включить и выключить

| Переменная | По умолчанию | Что делает |
|---|---|---|
| `EMAIL_AUTH_ENABLED` | `true` в dev/test, `false` в prod | Открывает `/v1/auth/email/register` и `/v1/auth/email/login`; форма на `/login` появляется сама |

На тестовом prod-сервере можно включить явно: `EMAIL_AUTH_ENABLED=true` в
`deploy/prod/app/.env` и перезапустить API. Сервер тогда пишет в лог
предупреждение. Для реального запуска уберите переменную.

## Как работает

- `GET /v1/auth/email/status` → `{"enabled": true|false}` (отвечает всегда).
- `POST /v1/auth/email/register` `{email, password}` — пароль от 10 символов;
  почта приводится к нижнему регистру; занятая почта → `409 email_taken`.
  Ответ как у `/v1/auth/otp/verify` (`is_new_user: true`), дальше обычный путь
  нового клиента: выбор пола, согласие, приветственный бонус.
- `POST /v1/auth/email/login` `{email, password}` → сессия или `401 bad_credentials`.
- Лимиты: регистрация 10/час с IP; вход 20/15 мин с IP и 10/15 мин на почту.
- Почта **не подтверждается**, восстановления пароля нет.
- У такого клиента нет телефона: при оформлении заказа телефон получателя
  обязателен (это уже так работает).
- Удаление аккаунта стирает почту и пароль, адрес можно зарегистрировать заново.

## Как удалить

1. Удалить `backend/internal/auth/email_temp.go`, `backend/internal/server/email_auth_test.go`.
2. В `backend/internal/server/server.go` убрать строки с пометкой «ВРЕМЕННО»
   (`EmailRoutes`, `PurgeEmail`, предупреждение в логе); в `security_test.go` — три
   маршрута `/v1/auth/email/*`; в `server_test.go` — `EmailAuthEnabled: true`.
3. В `backend/internal/config/config.go` убрать `EmailAuthEnabled`.
4. Добавить новую миграцию с телом раздела Down из
   `backend/migrations/00090_email_auth_testing.sql`: аккаунты без телефона
   обезличиваются (как при удалении аккаунта), колонка `users.email` удаляется.
5. Удалить `web/src/components/EmailAuthForm.tsx`, `web/src/__tests__/EmailAuthForm.test.tsx`,
   подключение в `web/src/app/login/page.tsx`, ключи `emailAuth` и
   `errors.email_taken` в `web/messages/ru.json` и `uz.json`.
6. Удалить `EMAIL_AUTH_ENABLED` из `.env.example` и этот файл.

Поиск всех мест: `grep -rn "email_temp\|EmailAuth\|email/status\|ВРЕМЕННО" backend web/src`.
