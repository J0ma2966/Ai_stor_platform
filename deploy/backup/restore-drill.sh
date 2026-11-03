#!/usr/bin/env bash
# Еженедельная учебная тревога: поднимает ВРЕМЕННЫЙ PostgreSQL внутри контейнера
# (не трогает прод-базу), скачивает самый свежий офсайт-бэкап, восстанавливает его
# и проверяет инварианты (offsite-check.sh + consistency-check.sql).
#   BACKUP_S3_URI, AGE_IDENTITY (путь к приватному ключу age), [MAX_AGE_HOURS]
set -euo pipefail
work="$(mktemp -d)"
trap 'pg_ctl -D "$work/pg" -m immediate stop >/dev/null 2>&1 || true; rm -rf "$work"' EXIT
initdb -D "$work/pg" -U postgres --auth=trust >/dev/null
pg_ctl -D "$work/pg" -o "-p 55432 -k $work -c listen_addresses=''" -w start >/dev/null
export ADMIN_DATABASE_URL="postgresql://postgres@/postgres?host=$work&port=55432"
offsite-check.sh
