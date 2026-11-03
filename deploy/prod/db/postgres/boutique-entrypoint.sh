#!/usr/bin/env bash
# Обёртка над стандартным docker-entrypoint.sh.
#   PG_ROLE=primary (по умолчанию) — обычный запуск, initdb при пустом каталоге.
#   PG_ROLE=replica — если каталог пуст, клонирует primary через pg_basebackup
#                     (слот replica1) и стартует как hot standby.
set -euo pipefail
role="${PG_ROLE:-primary}"
export PGDATA="${PGDATA:-/var/lib/postgresql/data}"

if [[ "$role" == "replica" && ! -s "$PGDATA/PG_VERSION" && "$(id -u)" == "0" ]]; then
  : "${PRIMARY_HOST:?PRIMARY_HOST is required for replica}" "${REPLICATOR_PASSWORD:?}"
  mkdir -p "$PGDATA" && chown postgres:postgres "$PGDATA" && chmod 700 "$PGDATA"
  echo "replica: cloning from ${PRIMARY_HOST}:${PRIMARY_PORT:-5432}"
  until gosu postgres env PGPASSWORD="$REPLICATOR_PASSWORD" pg_isready -h "$PRIMARY_HOST" -p "${PRIMARY_PORT:-5432}" -U replicator -q; do
    echo "replica: waiting for primary"; sleep 3
  done
  gosu postgres env PGPASSWORD="$REPLICATOR_PASSWORD" pg_basebackup \
    -h "$PRIMARY_HOST" -p "${PRIMARY_PORT:-5432}" -U replicator -D "$PGDATA" \
    -X stream -S "${REPLICATION_SLOT:-replica1}" -R --checkpoint=fast --progress
  # application_name видно в pg_stat_replication на primary
  echo "primary_conninfo = 'host=${PRIMARY_HOST} port=${PRIMARY_PORT:-5432} user=replicator password=${REPLICATOR_PASSWORD} application_name=replica1'" \
    | gosu postgres tee -a "$PGDATA/postgresql.auto.conf" >/dev/null
  echo "primary_slot_name = '${REPLICATION_SLOT:-replica1}'" | gosu postgres tee -a "$PGDATA/postgresql.auto.conf" >/dev/null
fi

exec docker-entrypoint.sh "$@"
