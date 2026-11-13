#!/usr/bin/env bash
# Генерирует согласованные .env для всех серверов со случайными секретами.
# Результат НЕ коммитится: содержимое файлов вставляется в секреты окружения GitHub
# (ENV_APP, ENV_DB, ENV_REPLICA, ENV_EDGE, AGE_IDENTITY, TELEGRAM_BOT_TOKEN).
#
#   init-env.sh <out_dir> <домен> <acme_email> [staging]
#
# Схема (TOPOLOGY):
#   single (по умолчанию) — один сервер в UZ: все роли на нём, адрес 10.77.0.10, без реплики.
#   multi                 — app/db/replica в UZ + edge в Германии (адреса ниже).
#
# Windows (PowerShell, нужен Docker Desktop):
#   docker run --rm -it -v ${PWD}:/w -w /w alpine:3.22 sh -c "apk add -q bash openssl age && bash deploy/prod/scripts/init-env.sh secrets-out shop.optolink.uz admin@optolink.uz"
#
# Адреса WireGuard по умолчанию (как в ansible/inventory.example.yml):
#   APP 10.77.0.10, DB 10.77.0.20, REPLICA 10.77.0.21, EDGE 10.77.0.30
set -euo pipefail
out="${1:?usage: init-env.sh <out_dir> <domain> <acme_email> [staging]}"
domain="${2:?domain}"; email="${3:?acme email}"; envname="${4:-production}"
registry="${REGISTRY:-ghcr.io/j0ma2966/ai_stor_platform}"
topology="${TOPOLOGY:-single}"
case "$topology" in
  single) APP_IP="${APP_IP:-10.77.0.10}"; DB_IP="$APP_IP"; EDGE_IP="$APP_IP"; REPLICA_IP="" ;;
  multi)  APP_IP="${APP_IP:-10.77.0.10}" DB_IP="${DB_IP:-10.77.0.20}" REPLICA_IP="${REPLICA_IP:-10.77.0.21}" EDGE_IP="${EDGE_IP:-10.77.0.30}" ;;
  *) echo "TOPOLOGY must be single or multi" >&2; exit 2 ;;
esac
[[ -e "$out" ]] && { echo "$out already exists — refusing to overwrite secrets" >&2; exit 1; }
mkdir -p "$out"; umask 077

rnd() { openssl rand -hex "${1:-24}"; }
APP_DB_PASSWORD=$(rnd) OWNER_DB_PASSWORD=$(rnd) POSTGRES_PASSWORD=$(rnd) REPLICATOR_PASSWORD=$(rnd) MONITOR_PASSWORD=$(rnd)
REDIS_PASSWORD=$(rnd) MINIO_ROOT_PASSWORD=$(rnd) MINIO_APP_ACCESS_KEY="app$(rnd 8)" MINIO_APP_SECRET_KEY=$(rnd)
JWT_SECRET=$(rnd 32) OTP_PEPPER=$(rnd 24) INTERNAL_API_TOKEN=$(rnd 32) WALG_LIBSODIUM_KEY=$(rnd 32) GRAFANA_ADMIN_PASSWORD=$(rnd 16)
age-keygen -o "$out/age-identity.txt" 2>/dev/null
AGE_RECIPIENT=$(age-keygen -y "$out/age-identity.txt")

if [[ "$envname" == "staging" ]]; then
  site="staging.$domain"; api="api.staging.$domain"; s3="s3.staging.$domain"; media="media.staging.$domain"
  appenv=staging; sms=log
else
  appenv=prod; sms=eskiz
  site="$domain"; api="api.$domain"; s3="s3.$domain"; media="media.$domain"
fi

cat > "$out/app.env" <<E
# Роль app (UZ): API, Caddy, Redis, логи. Секрет GitHub: ENV_APP
APP_ENV=$appenv
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
# общий секрет с сайтом (тот же в ENV_EDGE): серверный рендеринг не упирается в лимит по IP
INTERNAL_API_TOKEN=$INTERNAL_API_TOKEN
OTP_ALLOWED_PREFIXES=+998
UPLOAD_CONCURRENCY=4
# AI-примерка в первом релизе выключена: не нужны воркер, ключи Gemini/FASHN и биометрическое хранилище.
# Включение: другой провайдер + TRYON_API_KEY, BIOMETRIC_S3_* (см. docs/DEPLOY.md) и TRYON_ENABLED=true у сайта.
TRYON_PROVIDER=off
API_CPUS=2
API_MEMORY=2g
# SMS-коды входа через Eskiz: без логина и пароля API в prod не стартует (так и задумано)
SMS_PROVIDER=$sms
ESKIZ_EMAIL=
ESKIZ_PASSWORD=
ESKIZ_FROM=4546
# Уведомления о заказах (docs/NOTIFICATIONS.md): бот магазинов включается, когда задан токен.
SITE_URL=https://$site
TELEGRAM_BOT_TOKEN=
TELEGRAM_BOT_USERNAME=
TELEGRAM_WEBHOOK_SECRET=$(rnd 24)
TELEGRAM_WEBHOOK_URL=
ORDER_SMS_ENABLED=false
# Оплата: куда вернуть клиента после оплаты. Click/Payme включаются, когда заполнены их ключи;
# тогда *_ALLOWED_IPS обязательны (адреса из договора с провайдером).
PAYMENT_RETURN_URL=https://$site/account/balance
CLICK_SERVICE_ID=
CLICK_MERCHANT_ID=
CLICK_SECRET_KEY=
CLICK_ALLOWED_IPS=
PAYME_MERCHANT_ID=
PAYME_KEY=
PAYME_ALLOWED_IPS=
PAYME_IKPU=
PAYME_PACKAGE_CODE=
E
if [[ "$topology" == single ]]; then
  cat >> "$out/app.env" <<E
# Один сервер: этот Caddy отдаёт и сайт (инстансы web1..web6 из edge/compose.single.yml)
CADDY_SITES=single
WEB_UPSTREAMS="$APP_IP:3001 $APP_IP:3002 $APP_IP:3003 $APP_IP:3004 $APP_IP:3005 $APP_IP:3006"
E
fi

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
{ echo "# Роль db, primary (UZ). Секрет GitHub: ENV_DB"
  # один сервер: учебное восстановление дампа тоже здесь (реплики нет)
  if [[ "$topology" == single ]]; then echo "COMPOSE_PROFILES=primary,drill"; echo "REPLICA_SLOT="; else echo "COMPOSE_PROFILES=primary"; fi
  echo "PG_ROLE=primary"; echo "PRIVATE_IP=$DB_IP"
  db_common
  echo "MINIO_DATA_DIR=/srv/minio"
  echo "MINIO_ROOT_USER=root$(rnd 6)"; echo "MINIO_ROOT_PASSWORD=$MINIO_ROOT_PASSWORD"
  echo "MINIO_APP_ACCESS_KEY=$MINIO_APP_ACCESS_KEY"; echo "MINIO_APP_SECRET_KEY=$MINIO_APP_SECRET_KEY"
  echo "S3_BUCKET=boutique-media"
} > "$out/db.env"
[[ "$topology" == multi ]] && { echo "# Сервер реплики БД (UZ). Секрет GitHub: ENV_REPLICA"
  echo "COMPOSE_PROFILES=replica,drill"; echo "PG_ROLE=replica"; echo "PRIVATE_IP=$REPLICA_IP"; echo "PRIMARY_HOST=$DB_IP"
  db_common
  # не используются на реплике, но обязательны для разбора compose.yml
  echo "MINIO_ROOT_USER=unused"; echo "MINIO_ROOT_PASSWORD=unused"; echo "MINIO_APP_ACCESS_KEY=unused"; echo "MINIO_APP_SECRET_KEY=unused"
} > "$out/replica.env"

cat > "$out/edge.env" <<E
# Роль edge: сайт и мониторинг (один сервер: тот же сервер в UZ; иначе Германия). Секрет GitHub: ENV_EDGE
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
# тот же секрет, что в ENV_APP: заголовок X-Internal-Token в запросах серверного рендеринга
INTERNAL_API_TOKEN=$INTERNAL_API_TOKEN
# chat id группы/канала в Telegram (бот должен быть добавлен в группу). Заполнить!
TELEGRAM_CHAT_ID=CHANGE-ME
E
[[ "$topology" == single ]] && printf 'SINGLE_HOST=1\n# 6 инстансов сайта по 1 CPU (edge/compose.single.yml)\nWEB_CPUS=1\nWEB_MEMORY=1g\n' >> "$out/edge.env"

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
# один сервер: APP_SSH_HOST = DB_SSH_HOST = EDGE_SSH_HOST = его адрес, REPLICA_* не задавать
E
echo "Готово ($topology): $out (app.env, db.env, $([[ $topology == multi ]] && echo "replica.env, ")edge.env, age-identity.txt, github-variables.txt)"
echo "Приватный ключ age (age-identity.txt) сохраните ещё и офлайн: без него зашифрованные дампы не прочитать."
