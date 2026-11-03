#!/usr/bin/env bash
# Простой планировщик для контейнера: запускает команду каждый день в HOUR:00 UTC
# (или раз в неделю, если задан WEEKDAY=1..7, 1 = понедельник) и пишет метрики
# для node_exporter (textfile collector): время последнего успеха и результат.
#
#   schedule.sh <имя_метрики> <команда> [аргументы...]
#   HOUR (UTC, по умолчанию 21 = 02:00 Ташкент), WEEKDAY, METRICS_DIR (/metrics), RUN_ON_START=1
set -uo pipefail
name="$1"; shift
hour="${HOUR:-21}"
dir="${METRICS_DIR:-/metrics}"
last=0

log() { printf '{"ts":"%s","level":"%s","job":"%s","msg":"%s"}\n' "$(date -u +%FT%TZ)" "$1" "$name" "$2"; }
metric() {
  [[ -d "$dir" && -w "$dir" ]] || return 0
  local f="$dir/${name}.prom" tmp="$dir/.${name}.prom.$$"
  {
    echo "# TYPE boutique_job_last_success_timestamp_seconds gauge"
    echo "boutique_job_last_success_timestamp_seconds{job_name=\"$name\"} $last"
    echo "# TYPE boutique_job_last_run_ok gauge"
    echo "boutique_job_last_run_ok{job_name=\"$name\"} $1"
  } > "$tmp" && mv "$tmp" "$f"
}
run() {
  log info "started"
  if "$@"; then last=$(date +%s); metric 1; log info "succeeded"
  else metric 0; log error "failed"; fi
}
next_run() {
  local now t; now=$(date -u +%s)
  t=$(date -u -d "today ${hour}:00" +%s)
  while (( t <= now )) || { [[ -n "${WEEKDAY:-}" ]] && [[ "$(date -u -d "@$t" +%u)" != "$WEEKDAY" ]]; }; do
    t=$(( t + 86400 ))
  done
  echo "$t"
}

[[ "${RUN_ON_START:-0}" == "1" ]] && run "$@"
while true; do
  t=$(next_run); log info "next run at $(date -u -d "@$t" +%FT%TZ)"
  sleep $(( t - $(date -u +%s) ))
  run "$@"
done
