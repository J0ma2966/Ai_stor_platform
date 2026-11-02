# Boutique Market — маркетплейс бутиков с AI-примеркой

Монорепозиторий: бэкенд на Go, фронтенд на Next.js (папка `web/`, добавляется отдельно).

| Папка | Что внутри |
|---|---|
| `backend/cmd/api` | точка входа: сервер, `migrate`, `create-admin` |
| `backend/internal/*` | модули: `auth`, `users`, `products`, `media`, `tokens`, `admin`, `orders`, `audit`, `httpx`, `ratelimit`, `platform/*` |
| `backend/migrations` | SQL-миграции goose (встраиваются в бинарник) |
| `deploy/` | docker-compose для разработки, скрипты бэкапа и восстановления |
| `docs/` | [архитектура](docs/ARCHITECTURE.md), [соглашения API](docs/API.md), [бэкапы](docs/BACKUPS.md) |

## Быстрый старт

```bash
cp .env.example .env            # заполнить секреты
make up                         # postgres, redis, minio, api, backup
ADMIN_PASSWORD=... docker compose -f deploy/docker-compose.yml exec -e ADMIN_PASSWORD api /api create-admin root
curl localhost:8080/readyz
```

### Windows (PowerShell, без make)

Нужны [Docker Desktop](https://www.docker.com/products/docker-desktop/) (запущенный) и, для сайта, [Node.js 20+](https://nodejs.org/). `make` ставить не нужно, всё делает скрипт `scripts\dev.ps1`:

```powershell
.\scripts\dev.ps1 up            # создаст .env, если его нет, поднимет всё и дождётся /readyz
.\scripts\dev.ps1 admin root    # создать администратора, пароль спросит (минимум 10 символов)
.\scripts\dev.ps1 web           # сайт на http://localhost:3000 (в отдельном окне)
.\scripts\dev.ps1 otp           # последние SMS-коды для входа по телефону (в dev они только в логах)
.\scripts\dev.ps1 logs          # логи API; .\scripts\dev.ps1 logs postgres — логи другого сервиса
.\scripts\dev.ps1 down          # остановить; reset — остановить и удалить все локальные данные
```

Если PowerShell пишет «выполнение сценариев отключено», запускайте так:
`powershell -ExecutionPolicy Bypass -File .\scripts\dev.ps1 up`.

То же самое без скрипта:

```powershell
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml exec -e ADMIN_PASSWORD="ВашПароль123" api /api create-admin root
cd web; npm ci; $env:NEXT_PUBLIC_API_URL="http://localhost:8080"; $env:NEXT_PUBLIC_MEDIA_ORIGIN="http://localhost:9000"; npm run dev
```

Тесты (нужна PostgreSQL 16, Redis не нужен — используется miniredis):

```bash
make test        # go test -race ./... с интеграционными тестами
make lint        # vet + staticcheck + govulncheck
```
