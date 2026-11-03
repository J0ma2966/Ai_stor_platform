#!/usr/bin/env bash
# Сайдкар WAL-G: базовый бэкап раз в сутки в WALG_BACKUP_HOUR (UTC) + ротация.
# Пишет метрики для node_exporter (textfile collector) в $METRICS_DIR.
# Запускается в том же образе, что и PostgreSQL, с общим томом PGDATA (только чтение).
set -uo pipefail
hour="${WALG_BACKUP_HOUR:-21}"          # 21:00 UTC = 02:00 Ташкент
retain="${WALG_RETAIN_FULL:-7}"
metrics="${METRICS_DIR:-/metrics}/walg.prom"

log() { printf '{"ts":"%s","level":"%s","msg":"%s"}\n' "$(date -u +%FT%TZ)" "$1" "$2"; }
metric() {
  local tmp="$metrics.$$"
  {
    echo "# HELP boutique_walg_last_success_timestamp_seconds Last successful WAL-G base backup."
    echo "# TYPE boutique_walg_last_success_timestamp_seconds gauge"
    echo "boutique_walg_last_success_timestamp_seconds ${1}"
    echo "# HELP boutique_walg_last_run_ok 1 if the last base backup succeeded."
    echo "# TYPE boutique_walg_last_run_ok gauge"
    echo "boutique_walg_last_run_ok ${2}"
  } > "$tmp" && mv "$tmp" "$metrics"
}

run_backup() {
  log info "walg base backup started"
  if wal-g backup-push "$PGDATA" && wal-g delete retain FULL "$retain" --confirm; then
    last=$(date +%s); metric "$last" 1; log info "walg base backup done"
  else
    metric "${last:-0}" 0; log error "walg base backup failed"
  fi
}

last=0
# Первый базовый бэкап сразу, если в хранилище ещё нет ни одного (новая установка)
until pg_isready -q; do sleep 2; done
latest="$(wal-g backup-list --json 2>/dev/null | jq -r 'map(.time) | max // empty')"
if [[ -z "$latest" ]]; then run_backup; else
  last=$(date -u -d "$latest" +%s); metric "$last" 1
  log info "latest base backup: $latest"
fi
while true; do
  now=$(date -u +%s)
  next=$(date -u -d "today ${hour}:00" +%s)
  (( next <= now )) && next=$(date -u -d "tomorrow ${hour}:00" +%s)
  sleep $(( next - now ))
  run_backup
done
