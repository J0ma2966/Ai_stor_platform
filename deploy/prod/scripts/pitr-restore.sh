#!/usr/bin/env bash
# Восстановление PostgreSQL на момент времени (PITR) из архива WAL-G во ВРЕМЕННЫЙ
# экземпляр рядом с рабочим (порт 127.0.0.1:5433). Рабочую базу не трогает.
# Запускать на сервере БД из каталога deploy/prod/db:
#
#   ../scripts/pitr-restore.sh "2026-11-02 14:59:00+05"     # время с часовым поясом
#   ../scripts/pitr-restore.sh latest                        # до последнего WAL в архиве
#
# Дальше: проверить данные (psql -h 127.0.0.1 -p 5433 -U postgres boutique),
# выгрузить нужное или переключиться целиком — docs/RUNBOOK.md, раздел «PITR».
set -euo pipefail
target="${1:?usage: pitr-restore.sh '<YYYY-MM-DD HH:MM:SS+TZ>'|latest}"
name="${PITR_NAME:-boutique-pitr}"
port="${PITR_PORT:-5433}"
vol="${name}-$(date -u +%Y%m%d%H%M%S)"
compose() { docker compose -f compose.yml "$@"; }

if docker ps -a --format '{{.Names}}' | grep -qx "$name"; then
  echo "container $name already exists: docker rm -f $name" >&2; exit 1
fi
docker volume create "$vol" >/dev/null
echo "restore volume: $vol"

# 1) последний базовый бэкап, сделанный ДО целевого времени
compose run --rm --no-deps -T --user root -e TARGET="$target" -e PGDATA=/restore \
  -v "$vol:/restore" --entrypoint bash postgres -euo pipefail -c '
    if [[ "$TARGET" == latest ]]; then b=LATEST; else
      t=$(date -u -d "$TARGET" +%s)
      b=$(wal-g backup-list --json | jq -r --argjson t "$t" \
            "map(select((.time | sub(\"\\\\.[0-9]+\"; \"\") | fromdateiso8601) <= \$t)) | sort_by(.time) | last | .backup_name // empty")
      [[ -n "$b" ]] || { echo "no base backup before $TARGET" >&2; exit 1; }
    fi
    echo "base backup: $b"
    wal-g backup-fetch /restore "$b"
    touch /restore/recovery.signal
    {
      echo "restore_command = '"'"'/usr/local/bin/wal-g wal-fetch %f %p'"'"'"
      [[ "$TARGET" == latest ]] || echo "recovery_target_time = '"'"'$TARGET'"'"'"
      echo "recovery_target_action = '"'"'promote'"'"'"
    } >> /restore/postgresql.auto.conf
    chown -R postgres:postgres /restore && chmod 700 /restore'

# 2) временный экземпляр: archive_mode=off — он НЕ должен писать в рабочий архив WAL
compose run -d --no-deps --name "$name" -e PGDATA=/restore -v "$vol:/restore" \
  -p "127.0.0.1:${port}:5432" postgres \
  postgres -c config_file=/etc/postgresql/postgresql.conf -c hba_file=/etc/postgresql/pg_hba.conf \
  -c archive_mode=off -c shared_buffers=1GB -c effective_cache_size=4GB >/dev/null

echo "replaying WAL..."
for _ in $(seq 1 600); do
  if docker exec "$name" psql -U postgres -tAc "select not pg_is_in_recovery()" 2>/dev/null | grep -q t; then
    echo "PITR done: container $name, port 127.0.0.1:$port, volume $vol"
    docker exec "$name" psql -U postgres -tAc "select 'last commit: ' || coalesce(pg_last_committed_xact()::text, 'n/a')" 2>/dev/null || true
    exit 0
  fi
  sleep 2
done
echo "recovery did not finish in 20 min: docker logs $name" >&2; exit 1
