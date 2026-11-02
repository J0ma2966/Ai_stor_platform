#!/usr/bin/env bash
# Еженедельная проверка НАСТОЯЩЕГО офсайт-бэкапа (а не свежего дампа):
#  1) самый новый файл в BACKUP_S3_URI не старше MAX_AGE_HOURS (26 ч) — иначе алерт;
#  2) он скачивается, расшифровывается, восстанавливается во временную базу;
#  3) на копии выполняется scripts/consistency-check.sql.
# Ненулевой код выхода = алерт (cron/CI должен уведомить).
#
#   BACKUP_S3_URI, ADMIN_DATABASE_URL, AGE_IDENTITY, [MAX_AGE_HOURS], [CONSISTENCY_SQL]
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
: "${BACKUP_S3_URI:?}" "${ADMIN_DATABASE_URL:?}" "${AGE_IDENTITY:?}"
max_age_h="${MAX_AGE_HOURS:-26}"

latest="$(aws s3 ls "${BACKUP_S3_URI%/}/" | awk '$4 ~ /^boutique_.*\.dump(\.age)?$/ {print $1" "$2" "$4}' | sort | tail -1)"
[[ -n "$latest" ]] || { echo "ALERT: no backups found in $BACKUP_S3_URI"; exit 1; }
name="$(awk '{print $3}' <<<"$latest")"
age_h=$(( ( $(date -u +%s) - $(date -u -d "$(awk '{print $1" "$2}' <<<"$latest")" +%s) ) / 3600 ))
echo "latest backup: $name (${age_h}h old)"
if (( age_h > max_age_h )); then echo "ALERT: newest backup is older than ${max_age_h}h"; exit 1; fi

tmpdb="offsite_check_$(date +%s)"
psql -q "$ADMIN_DATABASE_URL" -c "CREATE DATABASE $tmpdb"
trap 'psql -q "$ADMIN_DATABASE_URL" -c "DROP DATABASE IF EXISTS $tmpdb WITH (FORCE)"' EXIT
target="$(python3 -c 'import sys,urllib.parse as u; p=u.urlparse(sys.argv[1]); print(p._replace(path="/"+sys.argv[2]).geturl())' "$ADMIN_DATABASE_URL" "$tmpdb")"

TARGET_DATABASE_URL="$target" CONFIRM_RESTORE=yes "$here/restore.sh" "${BACKUP_S3_URI%/}/$name"
psql -q -v ON_ERROR_STOP=1 "$target" -f "${CONSISTENCY_SQL:-$here/../../scripts/consistency-check.sql}"
echo "offsite backup $name restored and consistent"
