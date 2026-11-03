#!/usr/bin/env bash
# Деплой на сервер. Запускается на самом сервере (CD делает это по SSH):
#
#   deploy.sh host                 агенты мониторинга (каждый сервер)
#   deploy.sh db                   PostgreSQL/MinIO/бэкапы (profile из COMPOSE_PROFILES в db/.env)
#   deploy.sh app  <IMAGE_TAG>     миграции + API по одному инстансу (без простоя)
#   deploy.sh edge <IMAGE_TAG>     сайт по одному инстансу + мониторинг
#
# При неудаче нового инстанса возвращается предыдущий тег и скрипт завершается с ошибкой.
set -euo pipefail
role="${1:?usage: deploy.sh host|db|app|edge [IMAGE_TAG]}"
tag="${2:-}"
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/$role"
# CD кладёт свежий .env.new из секретов GitHub; текущий IMAGE_TAG сохраняем для отката
if [[ -f .env.new ]]; then
  keep="$(sed -n 's/^IMAGE_TAG=//p' .env 2>/dev/null | tail -1 || true)"
  mv .env.new .env
  [[ -n "$keep" ]] && echo "IMAGE_TAG=$keep" >> .env
fi
[[ -f .env ]] || { echo "$root/$role/.env not found" >&2; exit 1; }
chmod 600 .env

log() { printf '[deploy %s] %s\n' "$role" "$*"; }
compose() { docker compose "${COMPOSE_FILES[@]}" "$@"; }
COMPOSE_FILES=(-f compose.yml)
[[ -f compose.override.yml ]] && COMPOSE_FILES+=(-f compose.override.yml)

# IMAGE_TAG записывается в .env, чтобы ручные `docker compose ps/logs` тоже работали
set_tag() {
  export IMAGE_TAG="$1"
  if grep -q '^IMAGE_TAG=' .env; then sed -i "s/^IMAGE_TAG=.*/IMAGE_TAG=$1/" .env; else echo "IMAGE_TAG=$1" >> .env; fi
}
prev_tag() { sed -n 's/^IMAGE_TAG=//p' .env | tail -1; }

# ждёт, пока $2 (URL внутри сети стека) не ответит 200, проверяя из контейнера $1
wait_http() {
  local via="$1" url="$2" t="${3:-90}"
  for ((i = 0; i < t; i++)); do
    if compose exec -T "$via" wget -q -O /dev/null -T 2 "$url" 2>/dev/null; then return 0; fi
    sleep 1
  done
  return 1
}

# каталог textfile-метрик node_exporter: туда пишут задачи бэкапа (любой uid)
metrics_dir() {
  local d; d="$(sed -n 's/^METRICS_HOST_DIR=//p' .env | tail -1)"; d="${d:-/var/lib/boutique/metrics}"
  mkdir -p "$d" && chmod 1777 "$d"
}

pull() { [[ "${SKIP_PULL:-0}" == "1" ]] || compose pull -q --ignore-buildable; }

rolling() { # rolling <сервис-проверяльщик> <путь> <сервисы...>
  local via="$1" path="$2"; shift 2
  for s in "$@"; do
    log "updating $s -> $IMAGE_TAG"
    compose up -d --no-deps --force-recreate "$s"
    if ! wait_http "$via" "http://$s:${PORT}${path}" 120; then
      log "ERROR: $s is not ready, rolling back"
      return 1
    fi
    log "$s ready"
    sleep "${ROLL_PAUSE:-5}"   # балансировщик успевает вернуть инстанс в пул
  done
}

case "$role" in
  host)
    metrics_dir
    pull
    compose up -d --remove-orphans
    ;;

  db)
    metrics_dir
    pull
    # сначала хранилище и бакеты (архив WAL пишется туда с первой секунды)
    if [[ ",${COMPOSE_PROFILES:-$(sed -n 's/^COMPOSE_PROFILES=//p' .env)}," == *",primary,"* ]]; then
      compose up -d --wait minio
      compose run --rm -T minio-init
    fi
    # Postgres пересоздаётся только если поменялся образ/конфиг (тег образа = хэш его файлов)
    compose up -d --remove-orphans --wait --wait-timeout 300 postgres
    compose up -d --remove-orphans
    log "postgres: $(compose exec -T -u postgres postgres psql -U postgres -tAc 'select pg_is_in_recovery()' | sed 's/t/replica/;s/f/primary/')"
    ;;

  app)
    [[ -n "$tag" ]] || { echo "IMAGE_TAG required" >&2; exit 1; }
    old="$(prev_tag)"
    set_tag "$tag"
    PORT=8080
    restore_tag() { if [[ -n "$old" ]]; then set_tag "$old"; else sed -i '/^IMAGE_TAG=/d' .env; fi; }
    if ! pull; then log "ERROR: cannot pull $tag"; restore_tag; exit 1; fi
    # всё, кроме инстансов API, — поднять, если ещё не запущено
    mapfile -t others < <(compose config --services | grep -vxE 'api[0-9]+|migrate')
    compose up -d --no-recreate "${others[@]}"
    sleep 2
    log "running migrations"
    # миграции обратно совместимы (expand -> migrate -> contract), старые инстансы работают дальше
    if ! compose --profile tools run --rm -T migrate; then
      log "ERROR: migrations failed, nothing restarted"; restore_tag; exit 1
    fi
    log "applying grants for boutique_app"
    envv() { sed -n "s/^$1=//p" .env | tail -1; }
    if ! docker run --rm -i --network host -e PGPASSWORD="$(envv OWNER_DB_PASSWORD)" postgres:16.10-alpine \
        psql -q -h "$(envv DB_PRIVATE_IP)" -U boutique -d boutique -f - < grants.sql; then
      log "ERROR: grants failed, nothing restarted"; restore_tag; exit 1
    fi
    if ! rolling caddy /readyz api1 api2 api3; then
      if [[ -n "$old" ]]; then
        set_tag "$old"; log "rollback to $old"
        compose up -d --no-deps --force-recreate api1 api2 api3
      fi
      exit 1
    fi
    compose up -d --remove-orphans redis-exporter loki
    # изменения Caddyfile применяются без разрыва соединений
    compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile --force >/dev/null
    log "done: $tag"
    ;;

  edge)
    [[ -n "$tag" ]] || { echo "IMAGE_TAG required" >&2; exit 1; }
    old="$(prev_tag)"
    set_tag "$tag"
    PORT=3000
    "$root/scripts/render-targets.sh"
    restore_tag() { if [[ -n "$old" ]]; then set_tag "$old"; else sed -i '/^IMAGE_TAG=/d' .env; fi; }
    if ! pull; then log "ERROR: cannot pull $tag"; restore_tag; exit 1; fi
    mapfile -t others < <(compose config --services | grep -vxE 'web[0-9]+')
    compose up -d --no-recreate "${others[@]}"
    if ! rolling caddy /api/healthz web1 web2; then
      if [[ -n "$old" ]]; then
        set_tag "$old"; log "rollback to $old"
        compose up -d --no-deps --force-recreate web1 web2
      fi
      exit 1
    fi
    compose up -d --remove-orphans prometheus alertmanager blackbox
    compose exec -T caddy caddy reload --config /etc/caddy/Caddyfile --force >/dev/null
    # правила/цели Prometheus перечитываются без перезапуска
    compose exec -T prometheus wget -q -O /dev/null --post-data= http://127.0.0.1:9090/-/reload || true
    log "done: $tag"
    ;;
  *) echo "unknown role $role" >&2; exit 2 ;;
esac
