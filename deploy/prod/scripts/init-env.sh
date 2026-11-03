#!/usr/bin/env bash
# Генерирует согласованные .env для всех серверов со случайными секретами.
# Результат НЕ коммитится: содержимое файлов вставляется в секреты окружения GitHub
# (ENV_APP, ENV_DB, ENV_REPLICA, ENV_EDGE, AGE_IDENTITY, TELEGRAM_BOT_TOKEN).
#
#   init-env.sh <out_dir> <домен> <acme_email> [staging]
#
# Windows (PowerShell, нужен Docker Desktop):
#   docker run --rm -it -v ${PWD}:/w -w /w alpine:3.22 sh -c "apk add -q bash openssl age && bash deploy/prod/scripts/init-env.sh secrets-out shop.uz admin@shop.uz"
#
# Адреса WireGuard по умолчанию (как в ansible/inventory.example.yml):
#   APP 10.77.0.10, DB 10.77.0.20, REPLICA 10.77.0.21, EDGE 10.77.0.30
set -euo pipefail
out="${1:?usage: init-env.sh <out_dir> <domain> <acme_email> [staging]}"
domain="${2:?domain}"; email="${3:?acme email}"; envname="${4:-production}"
registry="${REGISTRY:-ghcr.io/j0ma2966/ai_stor_platform}"
APP_IP="${APP_IP:-10.77.0.10}" DB_IP="${DB_IP:-10.77.0.20}" REPLICA_IP="${REPLICA_IP:-10.77.0.21}" EDGE_IP="${EDGE_IP:-10.77.0.30}"
[[ -e "$out" ]] && { echo "$out already exists — refusing to overwrite secrets" >&2; exit 1; }
mkdir -p "$out"; umask 077

rnd() { openssl rand -hex "${1:-24}"; }
APP_DB_PASSWORD=$(rnd) OWNER_DB_PASSWORD=$(rnd) POSTGRES_PASSWORD=$(rnd) REPLICATOR_PASSWORD=$(rnd) MONITOR_PASSWORD=$(rnd)
REDIS_PASSWORD=$(rnd) MINIO_ROOT_PASSWORD=$(rnd) MINIO_APP_ACCESS_KEY="app$(rnd 8)" MINIO_APP_SECRET_KEY=$(rnd)
JWT_SECRET=$(rnd 32) OTP_PEPPER=$(rnd 24) WALG_LIBSODIUM_KEY=$(rnd 32) GRAFANA_ADMIN_PASSWORD=$(rnd 16)
age-keygen -o "$out/age-identity.txt" 2>/dev/null
AGE_RECIPIENT=$(age-keygen -y "$out/age-identity.txt")

if [[ "$envname" == "staging" ]]; then
  site="staging.$domain"; api="api.staging.$domain"; s3="s3.staging.$domain"; media="media.staging.$domain"
else
  site="$domain"; api="api.$domain"; s3="s3.$domain"; media="media.$domain"
fi

cat > "$out/app.env" <<E
# Сервер приложения (UZ). Секрет GitHub: ENV_APP
APP_ENV=prod
REGISTRY=$registry
PRIVATE_IP=$APP_IP
DB_PRIVATE_IP=$DB_IP
EDGE_PRIVATE_IP=$EDGE_IP
GRAFANA_ADMIN_PASSWORD=$GRAFANA_ADMIN_PASSWORD
ACME_EMAIL=$email
TLS_MODE=acme
SITE_DOMAIN=$site
API_DOMAIN=$api
S3_DOMAIN=$s3
MEDIA_DOMAIN=$media
S3_BUCKET=boutique-media
APP_DB_PASSWORD=$APP_DB_PASSWORD
OWNER_DB_PASSWORD=$OWNER_DB_PASSWORD
DB_MAX_CONNS=25
REDIS_PASSWORD=$REDIS_PASSWORD
REDIS_MAXMEMORY=1gb
MINIO_APP_ACCESS_KEY=$MINIO_APP_ACCESS_KEY
MINIO_APP_SECRET_KEY=$MINIO_APP_SECRET_KEY
JWT_SECRET=$JWT_SECRET
OTP_PEPPER=$OTP_PEPPER
OTP_ALLOWED_PREFIXES=+998
UPLOAD_CONCURRENCY=4
API_CPUS=2
API_MEMORY=2g
# --- ключи SMS-провайдера, Click/Payme, AI-примерки добавляются сюда (см. docs/DEPLOY.md) ---
E

db_common() {
cat <<E
REGISTRY=$registry
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
OWNER_DB_PASSWORD=$OWNER_DB_PASSWORD
APP_DB_PASSWORD=$APP_DB_PASSWORD
REPLICATOR_PASSWORD=$REPLICATOR_PASSWORD
MONITOR_PASSWORD=$MONITOR_PASSWORD
PG_DATA_DIR=/srv/pgdata
PG_SHARED_BUFFERS=8GB
PG_EFFECTIVE_CACHE_SIZE=24GB
PG_WORK_MEM=16MB
PG_MAINTENANCE_WORK_MEM=1GB
# Офсайт-хранилище S3 в Узбекистане (другой провайдер/ЦОД). Заполнить!
BACKUP_S3_ENDPOINT_URL=https://CHANGE-ME-offsite-s3.uz
BACKUP_S3_REGION=us-east-1
BACKUP_S3_ACCESS_KEY=CHANGE-ME
BACKUP_S3_SECRET_KEY=CHANGE-ME
WALG_S3_PREFIX=s3://boutique-backups/wal
PGDUMP_S3_URI=s3://boutique-backups/daily
MEDIA_MIRROR_URI=boutique-media-mirror
WALG_LIBSODIUM_KEY=$WALG_LIBSODIUM_KEY
AGE_RECIPIENT=$AGE_RECIPIENT
BACKUP_HOUR_UTC=21
E
}
{ echo "# Сервер БД, primary (UZ). Секрет GitHub: ENV_DB"
  echo "COMPOSE_PROFILES=primary"; echo "PG_ROLE=primary"; echo "PRIVATE_IP=$DB_IP"
  db_common
  echo "MINIO_DATA_DIR=/srv/minio"
  echo "MINIO_ROOT_USER=root$(rnd 6)"; echo "MINIO_ROOT_PASSWORD=$MINIO_ROOT_PASSWORD"
  echo "MINIO_APP_ACCESS_KEY=$MINIO_APP_ACCESS_KEY"; echo "MINIO_APP_SECRET_KEY=$MINIO_APP_SECRET_KEY"
  echo "S3_BUCKET=boutique-media"
} > "$out/db.env"
{ echo "# Сервер реплики БД (UZ). Секрет GitHub: ENV_REPLICA"
  echo "COMPOSE_PROFILES=replica,drill"; echo "PG_ROLE=replica"; echo "PRIVATE_IP=$REPLICA_IP"; echo "PRIMARY_HOST=$DB_IP"
  db_common
  # не используются на реплике, но обязательны для разбора compose.yml
  echo "MINIO_ROOT_USER=unused"; echo "MINIO_ROOT_PASSWORD=unused"; echo "MINIO_APP_ACCESS_KEY=unused"; echo "MINIO_APP_SECRET_KEY=unused"
} > "$out/replica.env"

cat > "$out/edge.env" <<E
# Сервер сайта и мониторинга (Германия). Секрет GitHub: ENV_EDGE
DEPLOY_ENV=$envname
REGISTRY=$registry
ACME_EMAIL=$email
TLS_MODE=acme
SITE_DOMAIN=$site
API_DOMAIN=$api
S3_DOMAIN=$s3
MEDIA_DOMAIN=$media
APP_PRIVATE_IP=$APP_IP
DB_PRIVATE_IP=$DB_IP
REPLICA_PRIVATE_IP=$REPLICA_IP
EDGE_PRIVATE_IP=$EDGE_IP
# chat id группы/канала в Telegram (бот должен быть добавлен в группу). Заполнить!
TELEGRAM_CHAT_ID=CHANGE-ME
E

cat > "$out/github-variables.txt" <<E
# Переменные окружения GitHub (Settings → Environments → $envname → Variables)
SITE_DOMAIN=$site
API_DOMAIN=$api
S3_DOMAIN=$s3
MEDIA_DOMAIN=$media
APP_PRIVATE_IP=$APP_IP
DB_PRIVATE_IP=$DB_IP
REPLICA_PRIVATE_IP=$REPLICA_IP
EDGE_PRIVATE_IP=$EDGE_IP
# публичные адреса SSH серверов (заполнить): APP_SSH_HOST, DB_SSH_HOST, REPLICA_SSH_HOST, EDGE_SSH_HOST
E
echo "Готово: $out (app.env, db.env, replica.env, edge.env, age-identity.txt, github-variables.txt)"
echo "Приватный ключ age (age-identity.txt) сохраните ещё и офлайн: без него зашифрованные дампы не прочитать."
