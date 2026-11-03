#!/usr/bin/env bash
# Переключение на реплику, если primary потерян. Запускать на сервере РЕПЛИКИ
# из каталога deploy/prod/db. Порядок (подробно — docs/RUNBOOK.md «Отказ сервера БД»):
#   1) убедиться, что старый primary выключен и не вернётся сам (иначе две «главные» базы);
#   2) failover.sh — реплика становится primary;
#   3) в секрете ENV_APP поменять DB_PRIVATE_IP на адрес реплики и перезапустить деплой app.
set -euo pipefail
compose() { docker compose -f compose.yml "$@"; }
state=$(compose exec -T postgres psql -U postgres -tAc "select pg_is_in_recovery()")
if [[ "$state" != "t" ]]; then echo "this server is already primary"; exit 0; fi
if [[ "${CONFIRM_FAILOVER:-}" != "yes" ]]; then
  echo "Old primary must be stopped. Re-run with CONFIRM_FAILOVER=yes" >&2; exit 2
fi
compose exec -T postgres psql -U postgres -tAc "select pg_promote(wait => true, wait_seconds => 60)"
compose exec -T postgres psql -U postgres -tAc "select case when pg_is_in_recovery() then 'still replica' else 'promoted: primary' end"
# этот сервер теперь primary: базовые бэкапы WAL-G
sed -i 's/^COMPOSE_PROFILES=.*/COMPOSE_PROFILES=replica,drill,walg/' .env
compose up -d walg
echo "next: update DB_PRIVATE_IP in ENV_APP and redeploy app"
