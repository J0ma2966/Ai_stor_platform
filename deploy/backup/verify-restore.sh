#!/usr/bin/env bash
# Автоматическая проверка бэкапа: делает бэкап, восстанавливает его во временную
# базу и сравнивает количество строк в ключевых таблицах. Запускается в CI и по cron.
#
#   DATABASE_URL        исходная база
#   ADMIN_DATABASE_URL  подключение с правом CREATE DATABASE (обычно к базе postgres)
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
: "${DATABASE_URL:?}" "${ADMIN_DATABASE_URL:?}"

export BACKUP_DIR="$(mktemp -d)"
key="$BACKUP_DIR/key.txt"
age-keygen -o "$key" 2>/dev/null
export AGE_RECIPIENT="$(age-keygen -y "$key")"
file="$("$here/backup.sh" | tail -1)"

tmpdb="restore_check_$(date +%s)"
psql -q "$ADMIN_DATABASE_URL" -c "CREATE DATABASE $tmpdb"
trap 'psql -q "$ADMIN_DATABASE_URL" -c "DROP DATABASE IF EXISTS $tmpdb WITH (FORCE)"; rm -rf "$BACKUP_DIR"' EXIT
target="$(python3 -c 'import sys,urllib.parse as u; p=u.urlparse(sys.argv[1]); print(p._replace(path="/"+sys.argv[2]).geturl())' "$ADMIN_DATABASE_URL" "$tmpdb")"

TARGET_DATABASE_URL="$target" AGE_IDENTITY="$key" CONFIRM_RESTORE=yes "$here/restore.sh" "$file"

tables="users stores products orders order_deliveries token_ledger audit_log goose_db_version"
fail=0
for t in $tables; do
  a=$(psql -tAq "$DATABASE_URL" -c "SELECT count(*) FROM $t")
  b=$(psql -tAq "$target" -c "SELECT count(*) FROM $t")
  if [[ "$a" != "$b" ]]; then echo "MISMATCH $t: source=$a restored=$b"; fail=1; else echo "ok $t=$a"; fi
done
# append-only триггеры должны пережить восстановление
n=$(psql -tAq "$target" -c "SELECT count(*) FROM pg_trigger WHERE tgname IN ('token_ledger_append_only','token_ledger_no_truncate','audit_log_append_only','audit_log_no_truncate')")
if [[ "$n" != "4" ]]; then
  echo "append-only trigger missing after restore"; fail=1
fi
exit $fail
