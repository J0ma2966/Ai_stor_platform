#!/usr/bin/env bash
# Генерирует цели Prometheus (edge/targets/*.yml) из edge/.env.
# Адреса — приватные адреса WireGuard серверов; REPLICA_PRIVATE_IP можно не задавать.
set -euo pipefail
dir="$(cd "$(dirname "$0")/.." && pwd)"
set -a; . "$dir/edge/.env"; set +a
: "${APP_PRIVATE_IP:?}" "${DB_PRIVATE_IP:?}" "${EDGE_PRIVATE_IP:?}" "${SITE_DOMAIN:?}" "${API_DOMAIN:?}" "${S3_DOMAIN:?}" "${MEDIA_DOMAIN:?}"
out="$dir/edge/targets"; mkdir -p "$out"

# host=имя:адрес для каждого сервера
hosts=("app:$APP_PRIVATE_IP" "db-primary:$DB_PRIVATE_IP" "edge:$EDGE_PRIVATE_IP")
[[ -n "${REPLICA_PRIVATE_IP:-}" ]] && hosts+=("db-replica:$REPLICA_PRIVATE_IP")

per_host() { # $1 порт -> список целей на всех серверах
  for h in "${hosts[@]}"; do
    printf -- '- targets: ["%s:%s"]\n  labels: {host: "%s"}\n' "${h#*:}" "$1" "${h%%:*}"
  done
}
per_host 9100 > "$out/node.yml"
per_host 8081 > "$out/cadvisor.yml"

{
  for p in 9101 9102 9103; do
    printf -- '- targets: ["%s:%s"]\n  labels: {host: "app", instance_name: "api%s"}\n' "$APP_PRIVATE_IP" "$p" "${p: -1}"
  done
} > "$out/api.yml"

{
  printf -- '- targets: ["%s:9187"]\n  labels: {host: "db-primary"}\n' "$DB_PRIVATE_IP"
  [[ -n "${REPLICA_PRIVATE_IP:-}" ]] && printf -- '- targets: ["%s:9187"]\n  labels: {host: "db-replica"}\n' "$REPLICA_PRIVATE_IP"
  true
} > "$out/postgres.yml"
printf -- '- targets: ["%s:9121"]\n  labels: {host: "app"}\n' "$APP_PRIVATE_IP" > "$out/redis.yml"
printf -- '- targets: ["%s:9000"]\n  labels: {host: "db-primary"}\n' "$DB_PRIVATE_IP" > "$out/minio.yml"

sfx=""; [[ "${TLS_MODE:-acme}" == "internal" ]] && sfx="_staging"
cat > "$out/blackbox.yml" <<YML
- targets:
    - https://${SITE_DOMAIN}/api/healthz
    - https://${API_DOMAIN}/readyz
    - https://${S3_DOMAIN}/minio/health/live
  labels: {module: http_2xx${sfx}, host: "external"}
- targets:
    - https://${MEDIA_DOMAIN}/
  labels: {module: https_any${sfx}, host: "external"}
YML
echo "targets written to $out"
