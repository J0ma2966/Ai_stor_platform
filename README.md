# Boutique Market — маркетплейс бутиков с AI-примеркой

Монорепозиторий: бэкенд на Go (`backend/`), сайт на Next.js (`web/`), окружение для разработки в Docker (`deploy/`).

| Папка | Что внутри |
|---|---|
| `backend/cmd/api` | точка входа: сервер, `migrate`, `create-admin` |
| `backend/internal/*` | модули: `auth`, `users`, `products`, `media`, `tokens`, `admin`, `orders`, `audit`, `httpx`, `ratelimit`, `platform/*` |
| `backend/migrations` | SQL-миграции goose (встраиваются в бинарник и накатываются при старте) |
| `web/` | сайт: Next.js 15, React 19, TypeScript, языки ru/uz ([подробнее](web/README.md)) |
| `deploy/` | docker-compose для разработки, скрипты бэкапа и восстановления |
| `deploy/prod/`, `ansible/` | продакшен: один сервер в UZ (с путём роста до нескольких), TLS, мониторинг, бэкапы с PITR, выкатка ([как запустить](docs/DEPLOY.md), [runbook](docs/RUNBOOK.md)) |
| `scripts/dev.ps1` | запуск всего на Windows без `make` |
| `docs/` | [архитектура](docs/ARCHITECTURE.md), [соглашения API](docs/API.md), [бэкапы](docs/BACKUPS.md), [продакшен](docs/DEPLOY.md), [runbook](docs/RUNBOOK.md) |

## Содержание

1. [Что нужно установить](#1-что-нужно-установить)
2. [Что запускается и на каких адресах](#2-что-запускается-и-на-каких-адресах)
3. [Запуск на Windows](#3-запуск-на-windows-powershell)
4. [Запуск на Linux и macOS](#4-запуск-на-linux-и-macos)
5. [Настройка .env](#5-настройка-env)
6. [Администратор, магазин и вход на сайт](#6-администратор-магазин-и-вход-на-сайт)
7. [SMS-код для входа в режиме разработки](#7-sms-код-для-входа-в-режиме-разработки)
8. [Остановка, логи, сброс данных](#8-остановка-логи-сброс-данных)
9. [Тесты](#9-тесты)
10. [Частые проблемы](#10-частые-проблемы)

## 1. Что нужно установить

Бэкенд, база данных, Redis и хранилище файлов работают в Docker, поэтому Go и PostgreSQL ставить на компьютер **не нужно**. Для сайта нужен Node.js.

| Программа | Версия | Зачем | Где взять |
|---|---|---|---|
| Docker Desktop (Windows, macOS) или Docker Engine + Compose v2 (Linux) | Docker 24+, Compose 2.20+ | PostgreSQL, Redis, MinIO, API, бэкапы | https://www.docker.com/products/docker-desktop/ |
| Git | 2.30+ | скачать репозиторий | https://git-scm.com/downloads |
| Node.js (вместе с ним ставится npm) | 20 LTS или новее (проверено на 24), npm 10+ | сайт на http://localhost:3000 | https://nodejs.org/ |
| Go | 1.26+ | **не обязательно**: только для запуска тестов и API без Docker | https://go.dev/dl/ |
| make | любая | **не обязательно**: только Linux/macOS, на Windows вместо него `scripts\dev.ps1` | обычно уже есть |

Проверить, что всё установлено:

```bash
docker --version
docker compose version
git --version
node -v
npm -v
```

Библиотеки ставить вручную не нужно:

- **Бэкенд** (Go): зависимости из `backend/go.mod` (chi, pgx, go-redis, minio-go, goose, prometheus и др.) скачиваются автоматически при сборке Docker-образа.
- **Сайт**: зависимости из `web/package.json` (next 15, react 19, next-intl, typescript, vitest) ставит команда `npm ci`. Скрипт `dev.ps1 web` делает это сам при первом запуске.
- **Образы Docker** скачиваются при первом `up`: `postgres:16-alpine`, `redis:7-alpine`, `pgsty/minio`, `pgsty/mc`, `golang:1.26-alpine`, `distroless/static`. Первый запуск занимает 3–10 минут в зависимости от интернета.

Для Docker Desktop на Windows нужен включённый WSL 2 (установщик предложит его сам). Компьютеру нужно примерно 4 ГБ свободной памяти и 5 ГБ места на диске.

## 2. Что запускается и на каких адресах

| Сервис | Адрес | Примечание |
|---|---|---|
| Сайт | http://localhost:3000/ru (или `/uz`) | запускается отдельно через Node.js |
| API | http://localhost:8080, проверка: http://localhost:8080/readyz | ответ `{"data":{"db":"ok","redis":"ok"}}` значит всё в порядке |
| Метрики API | http://localhost:9090/metrics | Prometheus |
| Консоль MinIO (файлы, картинки) | http://localhost:9001 | логин `minioadmin`, пароль `minioadmin-dev` |
| PostgreSQL 16 | `localhost:5432`, база/пользователь `boutique`, пароль `devpassword` | только с этого компьютера |
| Redis 7 | `localhost:6379` | только с этого компьютера |

Все порты открыты только на `127.0.0.1`, из локальной сети они недоступны.

## 3. Запуск на Windows (PowerShell)

1. Установите программы из раздела 1 и **запустите Docker Desktop**. Дождитесь внизу окна статуса *Engine running*.
2. Скачайте проект:

   ```powershell
   git clone https://github.com/J0ma2966/Ai_stor_platform.git
   cd Ai_stor_platform
   ```

3. Поднимите бэкенд. Скрипт сам создаст `.env` из `.env.example`, соберёт и запустит контейнеры и дождётся готовности API:

   ```powershell
   .\scripts\dev.ps1 up
   ```

4. Создайте администратора (скрипт спросит пароль, минимум 10 символов):

   ```powershell
   .\scripts\dev.ps1 admin root
   ```

5. Запустите сайт **в отдельном окне PowerShell** и не закрывайте его, пока работаете:

   ```powershell
   .\scripts\dev.ps1 web
   ```

   При первом запуске установятся npm-пакеты, затем появится строка `Ready`. Откройте http://localhost:3000/ru.

Если PowerShell пишет «выполнение сценариев отключено в этой системе», запускайте скрипт так:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 up
```

Все команды скрипта:

| Команда | Что делает |
|---|---|
| `.\scripts\dev.ps1 up` | поднять postgres, redis, minio, api, backup и дождаться `/readyz` |
| `.\scripts\dev.ps1 web` | запустить сайт на http://localhost:3000 |
| `.\scripts\dev.ps1 admin <логин>` | создать администратора |
| `.\scripts\dev.ps1 otp` | показать последние SMS-коды для входа (только dev) |
| `.\scripts\dev.ps1 logs [сервис]` | логи API или другого сервиса (`postgres`, `redis`, `minio`, `backup`) |
| `.\scripts\dev.ps1 ps` | список контейнеров и их статус |
| `.\scripts\dev.ps1 health` | проверить `/readyz` |
| `.\scripts\dev.ps1 backup` | сделать разовый бэкап базы |
| `.\scripts\dev.ps1 test` | тесты бэкенда (нужен Go) |
| `.\scripts\dev.ps1 down` | остановить контейнеры, данные сохраняются |
| `.\scripts\dev.ps1 reset` | остановить и **удалить все локальные данные** (спросит подтверждение) |

То же самое без скрипта:

```powershell
Copy-Item .env.example .env
docker compose -f deploy/docker-compose.yml up -d --build
$env:ADMIN_PASSWORD = "ВашПароль123"
docker compose -f deploy/docker-compose.yml exec -e ADMIN_PASSWORD api /api create-admin root
Remove-Item Env:ADMIN_PASSWORD
cd web
npm ci
$env:NEXT_PUBLIC_API_URL = "http://localhost:8080"
$env:NEXT_PUBLIC_MEDIA_ORIGIN = "http://localhost:9000"
npm run dev
```

## 4. Запуск на Linux и macOS

1. Установите Docker (на macOS Docker Desktop, на Linux Docker Engine с плагином `docker compose`), Git и Node.js 20+. На Linux добавьте пользователя в группу `docker` (`sudo usermod -aG docker $USER`, затем перелогиньтесь), иначе команды придётся запускать через `sudo`.
2. Скачайте проект и создайте `.env`:

   ```bash
   git clone https://github.com/J0ma2966/Ai_stor_platform.git
   cd Ai_stor_platform
   cp .env.example .env
   ```

3. Поднимите бэкенд (через `make` или напрямую) и проверьте его:

   ```bash
   make up        # или: docker compose -f deploy/docker-compose.yml up -d --build
   curl localhost:8080/readyz
   ```

   API накатывает миграции при старте, поэтому первые 10–30 секунд `/readyz` может не отвечать.

4. Создайте администратора:

   ```bash
   read -rs ADMIN_PASSWORD && export ADMIN_PASSWORD     # введите пароль (не меньше 10 символов) и нажмите Enter
   docker compose -f deploy/docker-compose.yml exec -e ADMIN_PASSWORD api /api create-admin root
   unset ADMIN_PASSWORD
   ```

5. Запустите сайт в отдельном терминале:

   ```bash
   cd web
   npm ci
   NEXT_PUBLIC_API_URL=http://localhost:8080 NEXT_PUBLIC_MEDIA_ORIGIN=http://localhost:9000 npm run dev
   ```

   Откройте http://localhost:3000/ru.

Полезные команды:

```bash
docker compose -f deploy/docker-compose.yml ps                          # статус
docker compose -f deploy/docker-compose.yml logs -f --tail 200 api      # логи API
docker compose -f deploy/docker-compose.yml logs --tail 500 api | grep 'DEV OTP'   # SMS-коды
make down                                                               # остановить
docker compose -f deploy/docker-compose.yml down -v                     # остановить и удалить данные
```

## 5. Настройка .env

Файл `.env` лежит в корне проекта, в git не попадает. Для локальной разработки значения из `.env.example` подходят как есть, менять ничего не нужно.

| Переменная | По умолчанию | Что значит |
|---|---|---|
| `APP_ENV` | `dev` | в `dev` SMS не отправляются, а коды пишутся в лог API |
| `JWT_SECRET` | заглушка | ключ подписи токенов; для любого не-локального окружения сгенерируйте: `openssl rand -hex 32` |
| `OTP_PEPPER` | заглушка | секрет для хэширования SMS-кодов; тоже заменить вне локальной машины |
| `COOKIE_SECURE` | `false` | `true` только при работе по HTTPS |
| `CORS_ORIGINS` | `http://localhost:3000` | адреса сайта, которым разрешено ходить в API |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | `minioadmin`, `minioadmin-dev` | логин и пароль MinIO |
| `S3_BUCKET` | `boutique-media` | бакет для картинок |
| `PUBLIC_MEDIA_URL` | `http://localhost:9000/boutique-media` | откуда сайт берёт публичные картинки товаров |
| `OTP_ALLOWED_PREFIXES` | `+998` | коды стран, на номера которых можно запросить SMS-код |
| `UPLOAD_CONCURRENCY` | `4` | сколько картинок API обрабатывает одновременно |
| `AGE_RECIPIENT` | пусто | публичный ключ age для шифрования бэкапов, локально не нужен |

Переменные, которые читает сам docker-compose (задаются в окружении терминала перед `up`):

| Переменная | По умолчанию | Когда нужна |
|---|---|---|
| `POSTGRES_HOST_PORT` | `5432` | если порт 5432 уже занят локальным PostgreSQL |
| `REDIS_HOST_PORT` | `6379` | если порт 6379 уже занят |
| `POSTGRES_PASSWORD` | `devpassword` | пароль базы в контейнере |

Переменные сайта: `NEXT_PUBLIC_API_URL=http://localhost:8080` и `NEXT_PUBLIC_MEDIA_ORIGIN=http://localhost:9000`. Скрипт `dev.ps1 web` выставляет их сам.

## 6. Администратор, магазин и вход на сайт

В системе три роли:

| Роль | Как входит | Где работает |
|---|---|---|
| Клиент | по номеру телефона и SMS-коду на http://localhost:3000/ru/login | каталог, оформление заказа, «Мои заказы» |
| Магазин | по логину и паролю на http://localhost:3000/ru/store/login | заказы магазина: `/ru/store/orders` |
| Администратор | по логину и паролю на http://localhost:3000/ru/admin/login | панель `/ru/admin`: сводка, магазины, пользователи и токены, все заказы, тарифы, платежи, журнал действий |

**Админ-панель:** откройте http://localhost:3000/ru/admin, войдите как `root`, в разделе «Магазины» нажмите «Новый магазин». Пароль магазина сгенерируется и покажется один раз, галочка «Сразу активировать» включена по умолчанию. Дальше можно войти под этим магазином на `/ru/store/login`.

То же самое через API (например, для скриптов). Пример на Windows (PowerShell), после того как создан администратор `root`:

```powershell
# 1. Войти администратором и получить токен
$pass = Read-Host "Пароль администратора"
$login = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/auth/login `
  -ContentType 'application/json' -Body (@{ login = 'root'; password = $pass } | ConvertTo-Json)
$h = @{ Authorization = "Bearer $($login.data.access_token)" }

# 2. Создать магазин (логин и пароль для входа на /ru/store/login)
$store = Invoke-RestMethod -Method Post -Uri http://localhost:8080/v1/admin/stores -Headers $h `
  -ContentType 'application/json' -Body (@{ slug = 'demo'; name = 'Демо бутик'; login = 'demo'; password = 'demo-password-1' } | ConvertTo-Json)

# 3. Активировать магазин
Invoke-RestMethod -Method Post -Uri "http://localhost:8080/v1/admin/stores/$($store.data.store_id)/status" -Headers $h `
  -ContentType 'application/json' -Body '{"status":"active"}'
```

То же на Linux/macOS:

```bash
TOKEN=$(curl -s localhost:8080/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"login":"root","password":"ВАШ_ПАРОЛЬ"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
STORE=$(curl -s localhost:8080/v1/admin/stores -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"slug":"demo","name":"Демо бутик","login":"demo","password":"demo-password-1"}' | sed -E 's/.*"store_id":"([^"]+)".*/\1/')
curl -s localhost:8080/v1/admin/stores/$STORE/status -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  -d '{"status":"active"}'
```

После этого войдите на http://localhost:3000/ru/store/login с логином `demo` и паролем `demo-password-1`. Остальные эндпоинты описаны в [docs/API.md](docs/API.md).

## 7. SMS-код для входа в режиме разработки

При `APP_ENV=dev` SMS никуда не отправляются, код пишется в лог API.

1. Откройте http://localhost:3000/ru/login и введите номер в формате `+998901234567` (по умолчанию разрешены только номера `+998`, см. `OTP_ALLOWED_PREFIXES`).
2. Посмотрите код:

   ```powershell
   .\scripts\dev.ps1 otp
   ```

   или на Linux/macOS:

   ```bash
   docker compose -f deploy/docker-compose.yml logs --tail 500 api | grep 'DEV OTP'
   ```

   В строке будут последние цифры номера (`phone_tail`) и сам код (`code`).
3. Введите код на сайте. Новый клиент после входа заполняет короткую регистрацию и получает 20 токенов для AI-примерки.

Повторно запросить код можно не сразу: при слишком частых запросах API отвечает `otp_cooldown`, подождите минуту.

**AI-примерка локально.** Откройте http://localhost:3000/ru/fitting, выберите вещи, загрузите три фото и запустите. По умолчанию работает демо-режим `TRYON_PROVIDER=mock`: AI не вызывается, ключи не нужны, результат — ваши фото с карточками товаров. Чтобы подключить настоящую модель, в `.env` укажите `TRYON_PROVIDER=gemini` (или `fashn`) и `TRYON_API_KEY=...`, затем пересоберите API. Чтобы выключить примерку совсем (так prod запускается сейчас), задайте `TRYON_PROVIDER=off` для API и `NEXT_PUBLIC_TRYON_ENABLED=false` при сборке фронтенда. Подробности, сроки хранения фото и требования закона о локализации — в [docs/TRYON.md](docs/TRYON.md).

## 8. Остановка, логи, сброс данных

- Остановить, сохранив данные: `.\scripts\dev.ps1 down` или `make down`.
- Снова запустить: `.\scripts\dev.ps1 up` или `make up`. База, файлы и администратор сохранятся.
- Сайт останавливается сочетанием `Ctrl+C` в его окне.
- Полностью очистить всё (база, картинки, бэкапы): `.\scripts\dev.ps1 reset` или `docker compose -f deploy/docker-compose.yml down -v`.
- После изменений в коде бэкенда пересоберите API: `docker compose -f deploy/docker-compose.yml up -d --build api`. Сайт в режиме `npm run dev` подхватывает изменения сам.

## 9. Тесты

Нужен Go 1.26+ и запущенный PostgreSQL 16 (подойдёт тот, что в docker compose). Redis не нужен, в тестах используется miniredis.

```powershell
.\scripts\dev.ps1 test
```

```bash
make test        # go test -race ./... с интеграционными тестами
make lint        # vet + staticcheck + govulncheck
```

Тесты сайта:

```bash
cd web
npm test         # vitest
npm run lint     # проверка типов
```

## 10. Частые проблемы

**`Docker не запущен` / `error during connect` / `Cannot connect to the Docker daemon`.** Откройте Docker Desktop и дождитесь статуса *Engine running*. На Linux: `sudo systemctl start docker`.

**`Bind for 127.0.0.1:5432 failed: port is already allocated`.** Порт занят другим PostgreSQL (или Redis для 6379). Остановите его или пробросьте другой порт:

```powershell
$env:POSTGRES_HOST_PORT = "15432"; $env:REDIS_HOST_PORT = "16379"
.\scripts\dev.ps1 up
```

```bash
POSTGRES_HOST_PORT=15432 REDIS_HOST_PORT=16379 make up
```

Внутри Docker API всё равно подключается к стандартным портам, меняется только порт на вашем компьютере. Для `dev.ps1 test` тогда задайте `TEST_DATABASE_URL=postgres://boutique:devpassword@localhost:15432/boutique?sslmode=disable`.

**Порт 3000, 8080 или 9000 занят.** Найдите процесс: `Get-NetTCPConnection -LocalPort 3000` (Windows) или `lsof -i :3000` (Linux/macOS) и остановите его. Сайт можно запустить на другом порту: `npm run dev -- -p 3001`, но тогда добавьте `http://localhost:3001` в `CORS_ORIGINS` в `.env` и перезапустите API (`docker compose -f deploy/docker-compose.yml up -d api`).

**`pull access denied` или `manifest unknown` для образов MinIO.** Официальные образы `minio/minio` и `minio/mc` убраны с Docker Hub, поэтому используется совместимый форк `pgsty/minio` и `pgsty/mc`. Если у вас старая версия `deploy/docker-compose.yml`, обновите репозиторий (`git pull`).

**Сайт не открывается на http://localhost:3000.** Сайт не входит в docker compose, его нужно запустить отдельно (`.\scripts\dev.ps1 web` или `npm run dev` в папке `web`) и держать окно открытым. Дождитесь строки `Ready`. Если `npm ci` падает, проверьте версию Node.js (`node -v`, нужно 20+), удалите папку `web\node_modules` и запустите снова.

**Сайт открывается, но выдаёт ошибки сети или CORS.** Проверьте, что API отвечает на http://localhost:8080/readyz, что сайт запущен с `NEXT_PUBLIC_API_URL=http://localhost:8080` и что адрес сайта есть в `CORS_ORIGINS`.

**`/readyz` не отвечает после `up`.** Посмотрите логи: `.\scripts\dev.ps1 logs` или `docker compose -f deploy/docker-compose.yml logs api`. Частые причины: база ещё стартует (подождите 30 секунд) или в `.env` ошибка.

**`create-admin` пишет ошибку про пароль.** Пароль должен быть от 10 до 128 символов. Если логин уже занят, выберите другой.

**Картинки товаров не грузятся.** Проверьте, что контейнер `minio` запущен, а `minio-init` завершился с кодом 0 (`.\scripts\dev.ps1 ps`). Если нет, перезапустите: `docker compose -f deploy/docker-compose.yml up -d minio minio-init`.

**В PowerShell «выполнение сценариев отключено».** Запускайте `powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 <команда>` или один раз выполните `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned`.
