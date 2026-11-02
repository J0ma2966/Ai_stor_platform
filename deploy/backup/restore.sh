#!/usr/bin/env bash
# Восстановление из бэкапа backup.sh.
#
#   restore.sh <файл.dump[.age] | s3://...> 
#
# Переменные:
#   TARGET_DATABASE_URL  куда восстанавливать (обязательно). База должна существовать.
#   AGE_IDENTITY         путь к приватному ключу age (для .age файлов)
#   CONFIRM_RESTORE=yes  обязательно: восстановление удаляет текущие данные в целевой базе
set -euo pipefail

src="${1:?usage: restore.sh <backup file or s3 uri>}"
: "${TARGET_DATABASE_URL:?TARGET_DATABASE_URL is required}"
if [[ "${CONFIRM_RESTORE:-}" != "yes" ]]; then
  echo "refusing to restore: set CONFIRM_RESTORE=yes (target data will be replaced)" >&2
  exit 2
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

if [[ "$src" == s3://* ]]; then
  aws s3 cp --only-show-errors "$src" "$work/"
  aws s3 cp --only-show-errors "$src.sha256" "$work/" || true
  src="$work/$(basename "$src")"
fi

if [[ -f "$src.sha256" ]]; then
  echo "$(cat "$src.sha256")  $src" | sha256sum -c --quiet - || { echo "checksum mismatch" >&2; exit 3; }
fi

dump="$src"
if [[ "$src" == *.age ]]; then
  : "${AGE_IDENTITY:?AGE_IDENTITY is required for encrypted backups}"
  dump="$work/restore.dump"
  age -d -i "$AGE_IDENTITY" -o "$dump" "$src"
fi

pg_restore --list "$dump" > /dev/null   # быстрая проверка целостности архива
pg_restore --clean --if-exists --no-owner --no-privileges --single-transaction --exit-on-error \
  --dbname="$TARGET_DATABASE_URL" "$dump"
echo "restore completed from $(basename "$1")"
