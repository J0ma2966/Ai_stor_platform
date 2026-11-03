#!/usr/bin/env bash
# Выполняется один раз при создании кластера (docker-entrypoint-initdb.d).
# Роли: boutique — владелец базы приложения (НЕ суперпользователь),
# replicator — потоковая репликация, monitor — postgres_exporter (pg_monitor).
set -euo pipefail
: "${APP_DB_PASSWORD:?}" "${REPLICATOR_PASSWORD:?}" "${MONITOR_PASSWORD:?}"
psql -v ON_ERROR_STOP=1 --username postgres --dbname postgres \
  -v app_pw="$APP_DB_PASSWORD" -v repl_pw="$REPLICATOR_PASSWORD" -v mon_pw="$MONITOR_PASSWORD" <<'SQL'
CREATE ROLE boutique LOGIN PASSWORD :'app_pw';
CREATE DATABASE boutique OWNER boutique;
CREATE ROLE replicator LOGIN REPLICATION PASSWORD :'repl_pw';
CREATE ROLE monitor LOGIN PASSWORD :'mon_pw' CONNECTION LIMIT 5;
GRANT pg_monitor TO monitor;
SELECT pg_create_physical_replication_slot('replica1');
SQL
psql -v ON_ERROR_STOP=1 --username postgres --dbname boutique <<'SQL'
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO boutique;
SQL
