export TEST_DATABASE_URL ?= postgres://postgres@localhost:5432/postgres?sslmode=disable

.PHONY: up down test lint migrate backup verify-backup run

up:            ## поднять всё окружение
	docker compose -f deploy/docker-compose.yml up -d --build
down:
	docker compose -f deploy/docker-compose.yml down
run:           ## запустить API локально (нужен .env)
	cd backend && set -a && . ../.env && set +a && go run ./cmd/api
test:          ## все тесты, включая интеграционные (нужна PostgreSQL)
	cd backend && go test -race -count=1 -coverprofile=coverage.out ./...
lint:
	cd backend && go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./... && go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
migrate:       ## накатить миграции
	cd backend && go run ./cmd/api migrate up
backup:        ## разовый бэкап локальной базы
	docker compose -f deploy/docker-compose.yml exec backup backup.sh
verify-backup: ## бэкап + восстановление во временную базу + сверка
	DATABASE_URL=$${DATABASE_URL} ADMIN_DATABASE_URL=$${ADMIN_DATABASE_URL} deploy/backup/verify-restore.sh
