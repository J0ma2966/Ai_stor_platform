#!/usr/bin/env bash
# Обёртка над стандартным docker-entrypoint.sh.
#   PG_ROLE=primary (по умолчанию) — обычный запуск, initdb при пустом каталоге.
#   PG_ROLE=replica — если каталог пуст, клонирует primary через pg_basebackup
#                     (слот replica1) и стартует как hot standby.
set -euo pipefail
role="${PG_ROLE:-primary}"
export PGDATA="${PGDATA:-/var/lib/postgresql/data}"

# Образ работает от пользователя postgres (не root): каталог данных должен
# принадлежать uid 999 (Ansible создаёт /srv/pgdata с нужным владельцем).
if [[ "$role" == "replica" && ! -s "$PGDATA/PG_VERSION" ]]; then
  : "${PRIMARY_HOST:?PRIMARY_HOST is required for replica}" "${REPLICATOR_PASSWORD:?}"
  mkdir -p "$PGDATA" && chmod 700 "$PGDATA"
  echo "replica: cloning from ${PRIMARY_HOST}:${PRIMARY_PORT:-5432}"
  until env PGPASSWORD="$REPLICATOR_PASSWORD" pg_isready -h "$PRIMARY_HOST" -p "${PRIMARY_PORT:-5432}" -U replicator -q; do
    echo "replica: waiting for primary"; sleep 3
  done
  env PGPASSWORD="$REPLICATOR_PASSWORD" pg_basebackup \
    -h "$PRIMARY_HOST" -p "${PRIMARY_PORT:-5432}" -U replicator -D "$PGDATA" \
    -X stream -S "${REPLICATION_SLOT:-replica1}" -R --checkpoint=fast --progress
  # application_name видно в pg_stat_replication на primary
  echo "primary_conninfo = 'host=${PRIMARY_HOST} port=${PRIMARY_PORT:-5432} user=replicator password=${REPLICATOR_PASSWORD} application_name=replica1'" \
    >> "$PGDATA/postgresql.auto.conf"
  echo "primary_slot_name = '${REPLICATION_SLOT:-replica1}'" >> "$PGDATA/postgresql.auto.conf"
fi

exec docker-entrypoint.sh "$@"
