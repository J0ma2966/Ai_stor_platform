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
docker compose -f deploy/docker-compose.yml exec api /api create-admin root   # с ADMIN_PASSWORD в окружении
curl localhost:8080/readyz
```

Тесты (нужна PostgreSQL 16, Redis не нужен — используется miniredis):

```bash
make test        # go test -race ./... с интеграционными тестами
make lint        # vet + staticcheck + govulncheck
```
