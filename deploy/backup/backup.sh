#!/usr/bin/env bash
# Логический бэкап PostgreSQL: pg_dump (custom format) -> шифрование age -> локально и/или S3.
#
# Переменные:
#   DATABASE_URL       строка подключения (обязательно)
#   BACKUP_DIR         куда класть файлы (по умолчанию /backups)
#   AGE_RECIPIENT      публичный ключ age (age1...). Если задан — бэкап шифруется,
#                      и сервер бэкапов НЕ может его расшифровать (приватный ключ хранится отдельно).
#   BACKUP_S3_URI      s3://bucket/path — если задан, файл копируется в S3 (aws cli)
#   RETENTION_DAYS     сколько дней хранить локальные копии (по умолчанию 14)
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
mkdir -p "$BACKUP_DIR"
umask 077

ts="$(date -u +%Y%m%dT%H%M%SZ)"
name="boutique_${ts}.dump"
tmp="$BACKUP_DIR/.${name}.partial"
trap 'rm -f "$tmp"' EXIT

log() { printf '{"ts":"%s","level":"%s","msg":"%s"}\n' "$(date -u +%FT%TZ)" "$1" "$2"; }

log info "backup started"
if [[ -n "${AGE_RECIPIENT:-}" ]]; then
  name="${name}.age"
  pg_dump --format=custom --compress=6 --no-owner --no-privileges "$DATABASE_URL" | age -r "$AGE_RECIPIENT" -o "$tmp"
else
  log warn "AGE_RECIPIENT not set: backup is NOT encrypted (allowed only in dev)"
  pg_dump --format=custom --compress=6 --no-owner --no-privileges --file="$tmp" "$DATABASE_URL"
fi

final="$BACKUP_DIR/$name"
mv "$tmp" "$final"
sha256sum "$final" | awk '{print $1}' > "$final.sha256"
size=$(stat -c %s "$final")
log info "backup written file=$name bytes=$size"

if [[ -n "${BACKUP_S3_URI:-}" ]]; then
  aws s3 cp --only-show-errors "$final" "${BACKUP_S3_URI%/}/$name"
  aws s3 cp --only-show-errors "$final.sha256" "${BACKUP_S3_URI%/}/$name.sha256"
  log info "backup uploaded to s3"
fi

find "$BACKUP_DIR" -maxdepth 1 -name 'boutique_*' -mtime +"$RETENTION_DAYS" -delete
echo "$final"
