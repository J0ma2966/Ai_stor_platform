#!/usr/bin/env bash
# Выполняется один раз при создании кластера (docker-entrypoint-initdb.d).
# Роли: boutique — владелец базы и схемы, от него идут только миграции (НЕ суперпользователь);
# boutique_app — роль API: только SELECT/INSERT/UPDATE/DELETE, без DDL и без права
# отключать триггеры (журналы token_ledger и audit_log ей доступны только на чтение и вставку,
# права выдаёт deploy/prod/app/grants.sql после каждой миграции);
# replicator — потоковая репликация, monitor — postgres_exporter (pg_monitor).
set -euo pipefail
: "${OWNER_DB_PASSWORD:?}" "${APP_DB_PASSWORD:?}" "${REPLICATOR_PASSWORD:?}" "${MONITOR_PASSWORD:?}"
psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres \
  -v owner_pw="$OWNER_DB_PASSWORD" -v app_pw="$APP_DB_PASSWORD" -v repl_pw="$REPLICATOR_PASSWORD" -v mon_pw="$MONITOR_PASSWORD" <<'SQL'
CREATE ROLE boutique LOGIN PASSWORD :'owner_pw';
CREATE ROLE boutique_app LOGIN PASSWORD :'app_pw' CONNECTION LIMIT 300;
CREATE DATABASE boutique OWNER boutique;
REVOKE ALL ON DATABASE boutique FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE boutique TO boutique_app;
CREATE ROLE replicator LOGIN REPLICATION PASSWORD :'repl_pw';
CREATE ROLE monitor LOGIN PASSWORD :'mon_pw' CONNECTION LIMIT 5;
GRANT pg_monitor TO monitor;
SQL
# слот для реплики; один сервер (REPLICA_SLOT пустой): без слота, иначе он держал бы WAL на диске
if [[ -n "${REPLICA_SLOT:-}" ]]; then
  echo "SELECT pg_create_physical_replication_slot(:'slot');" |
    psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres -v slot="$REPLICA_SLOT"
fi
psql -v ON_ERROR_STOP=1 --username postgres --dbname boutique <<'SQL'
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO boutique;
SQL
