#!/usr/bin/env bash
# Сквозная проверка прод-стека на ОДНОЙ машине (CI и локально, нужен root и Docker).
# Поднимает все роли (host, db primary, db replica, app, edge) из deploy/prod теми же
# скриптами, что и CD, с внутренним TLS Caddy и адресами «WireGuard» на dummy-интерфейсе,
# затем проверяет: HTTPS через Caddy, загрузку фото и подписанные ссылки, публичные
# картинки, сайт, метрики и алерты, логи в Loki, архив WAL и базовый бэкап WAL-G,
# потоковую реплику, PITR, pg_dump + учебное восстановление, деплой без простоя и откат.
#
#   REGISTRY=local/boutique TAG=smoke1 TAG2=smoke2 deploy/prod/tests/smoke.sh
#
# Образы должны быть собраны заранее: $REGISTRY/{api,web-production}:$TAG,
# $REGISTRY/postgres:smoke, $REGISTRY/backup:smoke. TAG2 — второй тег API для проверки
# обновления (может указывать на тот же образ). SKIP_BACKUP_IMAGE=1 пропускает pg_dump/drill.
set -euo pipefail
REGISTRY="${REGISTRY:-local/boutique}"; TAG="${TAG:-smoke1}"; TAG2="${TAG2:-smoke2}"
IP=10.77.0.1; IP2=10.77.0.2
D=boutique.test
src="$(cd "$(dirname "$0")/.." && pwd)"; src="${SMOKE_SRC:-$src}"
work="${SMOKE_WORK:-$(mktemp -d)}"
fail=0
pass() { printf '\e[32mPASS\e[0m %s\n' "$*"; }
bad()  { printf '\e[31mFAIL\e[0m %s\n' "$*"; fail=1; }
step() { printf '\n\e[1m== %s\e[0m\n' "$*"; }
check() { local name="$1"; shift; if "$@"; then pass "$name"; else bad "$name"; fi; }
retry() { local n="$1"; shift; for ((i = 0; i < n; i++)); do "$@" && return 0; sleep 2; done; return 1; }

cleanup() {
  [[ "${KEEP:-0}" == "1" ]] && { echo "KEEP=1: stack left running in $work"; return; }
  for d in edge app replica/db main/db host; do
    [[ -d "$work/$d" ]] || continue
    files=(-f compose.yml); [[ -f "$work/$d/compose.override.yml" ]] && files+=(-f compose.override.yml)
    (cd "$work/$d" && docker compose "${files[@]}" down -v --remove-orphans >/dev/null 2>&1 || true)
  done
  docker rm -f boutique-pitr >/dev/null 2>&1 || true
}
trap cleanup EXIT

step "network: dummy interface with WireGuard-like addresses"
command -v ip >/dev/null || ip() { docker run --rm --net host --cap-add NET_ADMIN busybox:1.37 ip "$@"; }
dev=boutique0
if ! ip link show $dev >/dev/null 2>&1 && ! { ip link add $dev type dummy && ip link set $dev up; } 2>/dev/null; then
  dev=lo   # ядро без модуля dummy: адреса на loopback тоже доступны из контейнеров
fi
ip addr add "$IP/32" dev $dev 2>/dev/null || true
ip addr add "$IP2/32" dev $dev 2>/dev/null || true

step "config: generate env with init-env.sh, switch to internal TLS and local offsite S3"
mkdir -p "$work"
cp -r "$src" "$work/main"; mkdir -p "$work/replica"; cp -r "$src" "$work/replica/prod"
ln -sfn "$work/main/app" "$work/app"; ln -sfn "$work/main/edge" "$work/edge"; ln -sfn "$work/main/host" "$work/host"
ln -sfn "$work/replica/prod/db" "$work/replica/db"
APP_IP=$IP DB_IP=$IP REPLICA_IP=$IP2 EDGE_IP=$IP REGISTRY=$REGISTRY \
  bash "$src/scripts/init-env.sh" "$work/env" "$D" ops@$D >/dev/null
minio_user=$(sed -n 's/^MINIO_ROOT_USER=//p' "$work/env/db.env")
minio_pass=$(sed -n 's/^MINIO_ROOT_PASSWORD=//p' "$work/env/db.env")
for f in db replica; do
  sed -i -e "s#^BACKUP_S3_ENDPOINT_URL=.*#BACKUP_S3_ENDPOINT_URL=http://$IP:9000#" \
         -e "s#^BACKUP_S3_ACCESS_KEY=.*#BACKUP_S3_ACCESS_KEY=$minio_user#" \
         -e "s#^BACKUP_S3_SECRET_KEY=.*#BACKUP_S3_SECRET_KEY=$minio_pass#" \
         -e "s#^PG_SHARED_BUFFERS=.*#PG_SHARED_BUFFERS=256MB#" -e "s#^PG_EFFECTIVE_CACHE_SIZE=.*#PG_EFFECTIVE_CACHE_SIZE=1GB#" \
         -e "s#^PG_MAINTENANCE_WORK_MEM=.*#PG_MAINTENANCE_WORK_MEM=128MB#" -e "/^PG_DATA_DIR=/d" -e "/^MINIO_DATA_DIR=/d" \
         "$work/env/$f.env"
  printf 'PG_IMAGE_TAG=smoke\nBACKUP_IMAGE_TAG=smoke\nMETRICS_HOST_DIR=%s/metrics-%s\n' "$work" "$f" >> "$work/env/$f.env"
done
echo "COMPOSE_PROJECT_NAME=boutique-db-replica" >> "$work/env/replica.env"
sed -i -e "s/^TLS_MODE=.*/TLS_MODE=internal/" -e "s/^APP_ENV=.*/APP_ENV=staging/" -e "s/^SMS_PROVIDER=.*/SMS_PROVIDER=log/" "$work/env/app.env"
sed -i -e "s/^TLS_MODE=.*/TLS_MODE=internal/" -e "s/^TELEGRAM_CHAT_ID=.*/TELEGRAM_CHAT_ID=-1001/" "$work/env/edge.env"
printf 'HTTP_PORT=8088\nHTTPS_PORT=8443\n' >> "$work/env/edge.env"
cp "$work/env/app.env" "$work/app/.env.new"
cp "$work/env/db.env" "$work/main/db/.env.new"
cp "$work/env/replica.env" "$work/replica/db/.env.new"
cp "$work/env/edge.env" "$work/edge/.env.new"
printf 'PRIVATE_IP=%s\nLOKI_PRIVATE_IP=%s\nHOST_NAME=smoke\nMETRICS_HOST_DIR=%s/metrics-db\n' "$IP" "$IP" "$work" > "$work/host/.env"
mkdir -p "$work/replica/db/secrets" "$work/edge/secrets" "$work/main/db/secrets"
cp "$work/env/age-identity.txt" "$work/replica/db/secrets/age-identity.txt"
cp "$work/env/age-identity.txt" "$work/main/db/secrets/age-identity.txt"
chmod 644 "$work/replica/db/secrets/age-identity.txt" "$work/main/db/secrets/age-identity.txt"
echo "123:fake" > "$work/edge/secrets/telegram-bot-token"
cp "$work/app/compose.staging.yml" "$work/app/compose.override.yml"
# в контейнерных CI-песочницах / не всегда shared-mount
grep -q ' / / .*shared:' /proc/self/mountinfo || cat > "$work/host/compose.override.yml" <<YML
services:
  node-exporter:
    volumes: ["/:/host:ro"]
YML
# blackbox и Grafana должны находить «публичные» имена на этой же машине
cat > "$work/edge/compose.override.yml" <<YML
services:
  blackbox:
    extra_hosts: ["api.$D:$IP", "s3.$D:$IP", "media.$D:$IP", "$D:$IP"]
YML
export REGISTRY SKIP_PULL=1

step "deploy: host agents, db primary, app, edge, replica (same scripts as CD)"
"$work/main/scripts/deploy.sh" host
"$work/main/scripts/deploy.sh" db
"$work/main/scripts/deploy.sh" app "$TAG"
"$work/main/scripts/deploy.sh" edge "$TAG"
"$work/replica/prod/scripts/deploy.sh" db
if [[ "${SKIP_BACKUP_IMAGE:-0}" == "1" ]]; then
  (cd "$work/main/db" && docker compose stop pgdump >/dev/null 2>&1 || true)
  (cd "$work/replica/db" && docker compose stop restore-drill >/dev/null 2>&1 || true)
fi

app() { (cd "$work/app" && docker compose -f compose.yml -f compose.override.yml "$@"); }
dbc() { (cd "$work/main/db" && docker compose "$@"); }
rep() { (cd "$work/replica/db" && docker compose "$@"); }
edge() { (cd "$work/edge" && docker compose -f compose.yml -f compose.override.yml "$@"); }
CURL=(curl -sk --noproxy "*" --max-time 10 --resolve "api.$D:443:$IP" --resolve "s3.$D:443:$IP" --resolve "media.$D:443:$IP" --resolve "$D:8443:$IP")
API="https://api.$D"

step "API through Caddy"
check "readyz 200 (db+redis)" retry 30 bash -c "$(printf '%q ' "${CURL[@]}") -f $API/readyz | grep -q '\"db\":\"ok\"'"
check "catalog 200" bash -c "$(printf '%q ' "${CURL[@]}") -f -o /dev/null $API/v1/products"
check "metrics/pprof not exposed publicly" bash -c "[[ \$($(printf '%q ' "${CURL[@]}") -o /dev/null -w '%{http_code}' $API/metrics) == 404 ]]"
check "HSTS/security headers from API" bash -c "$(printf '%q ' "${CURL[@]}") -sI $API/healthz | grep -qi 'x-content-type-options: nosniff'"
check "MinIO admin API blocked" bash -c "[[ \$($(printf '%q ' "${CURL[@]}") -o /dev/null -w '%{http_code}' https://s3.$D/minio/admin/v3/info) == 404 ]]"

step "database roles: API role cannot alter journals or schema"
apppw=$(sed -n 's/^APP_DB_PASSWORD=//p' "$work/env/app.env")
asapp() { docker run --rm --network host -e PGPASSWORD="$apppw" postgres:16.10-alpine psql -q -h "$IP" -U boutique_app -d boutique -v ON_ERROR_STOP=1 -c "$1" >/dev/null 2>&1; }
check "boutique_app can read catalog" asapp "select count(*) from products"
check "boutique_app cannot UPDATE token_ledger" bash -c "$(declare -f asapp); apppw=$apppw; IP=$IP; ! asapp 'update token_ledger set amount = amount'"
check "boutique_app cannot DELETE audit_log" bash -c "$(declare -f asapp); apppw=$apppw; IP=$IP; ! asapp 'delete from audit_log'"
check "boutique_app cannot disable triggers" bash -c "$(declare -f asapp); apppw=$apppw; IP=$IP; ! asapp 'alter table token_ledger disable trigger all'"
check "boutique_app cannot create tables" bash -c "$(declare -f asapp); apppw=$apppw; IP=$IP; ! asapp 'create table x(i int)'"

step "client: OTP login, private photo upload, presigned URL via s3 domain"
phone="+998901112233"
"${CURL[@]}" -f -o /dev/null -H 'Content-Type: application/json' -d "{\"phone\":\"$phone\"}" "$API/v1/auth/otp/request" || bad "otp request"
code=""; for _ in $(seq 1 10); do
  code=$(app logs --no-log-prefix api1 api2 api3 2>/dev/null | grep 'DEV OTP' | tail -1 | sed -n 's/.*"code":"\([0-9]*\)".*/\1/p'); [[ -n "$code" ]] && break; sleep 1; done
tok=$("${CURL[@]}" -H 'Content-Type: application/json' -d "{\"phone\":\"$phone\",\"code\":\"$code\"}" "$API/v1/auth/otp/verify" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
check "client logged in" test -n "$tok"
python3 - "$work/p.jpg" <<'PY'
import sys,struct,zlib
# минимальный валидный PNG 8x8 (API перекодирует его в JPEG)
w=h=8; raw=b''.join(b'\x00'+b'\xc8\x50\x50'*w for _ in range(h))
def ch(t,d): return struct.pack('>I',len(d))+t+d+struct.pack('>I',zlib.crc32(t+d)&0xffffffff)
open(sys.argv[1],'wb').write(b'\x89PNG\r\n\x1a\n'+ch(b'IHDR',struct.pack('>IIBBBBB',w,h,8,2,0,0,0))+ch(b'IDAT',zlib.compress(raw))+ch(b'IEND',b''))
PY
# Первая загрузка после старта может попасть на прогрев MinIO/DNS: до 5 попыток, ответ печатаем.
purl=""
for _ in $(seq 1 5); do
  up=$("${CURL[@]}" -H "Authorization: Bearer $tok" -H 'Content-Type: image/png' --data-binary @"$work/p.jpg" "$API/v1/uploads?purpose=delivery" || true)
  purl=$(sed -n 's/.*"url":"\([^"]*\)".*/\1/p' <<<"$up" | sed 's/\\u0026/\&/g')
  [[ -n "$purl" ]] && break
  echo "upload response: ${up:0:300}"; sleep 3
done
[[ -n "$purl" ]] || app logs --tail 30 api1 api2 api3 caddy 2>&1 | tail -60
check "upload returns presigned https://s3.$D URL" grep -q "^https://s3.$D/" <<<"$purl"
check "presigned URL downloads the photo" bash -c "[[ \$($(printf '%q ' "${CURL[@]}") -o /dev/null -w '%{http_code}' '$purl') == 200 ]]"
unsigned="${purl%%\?*}"
check "private photo without signature is denied" bash -c "[[ \$($(printf '%q ' "${CURL[@]}") -o /dev/null -w '%{http_code}' '$unsigned') == 403 ]]"

step "public product images via media domain"
dbc exec -T minio sh -c "printf img > /tmp/x.jpg && mc alias set l http://127.0.0.1:9000 \"\$MINIO_ROOT_USER\" \"\$MINIO_ROOT_PASSWORD\" >/dev/null && mc cp -q /tmp/x.jpg l/boutique-media/pub/smoke/x.jpg >/dev/null"
check "pub/ image 200 with immutable cache" bash -c "$(printf '%q ' "${CURL[@]}") -sI https://media.$D/pub/smoke/x.jpg | grep -qi 'cache-control: public, max-age=31536000, immutable'"
check "media domain does not expose private keys" bash -c "[[ \$($(printf '%q ' "${CURL[@]}") -o /dev/null -w '%{http_code}' https://media.$D/priv/x) == 404 ]]"

step "site (Next.js) through edge Caddy"
check "site /api/healthz 200" retry 15 bash -c "$(printf '%q ' "${CURL[@]}") -f -o /dev/null https://$D:8443/api/healthz"
check "site /ru/login 200 with nonce CSP" bash -c "$(printf '%q ' "${CURL[@]}") -sI https://$D:8443/ru/login | grep -qi \"content-security-policy: .*nonce-\""

step "monitoring: scrape targets, alert rules, logs"
prom() { edge exec -T prometheus wget -qO- "http://127.0.0.1:9090/api/v1/$1"; }
check "all scrape targets up" retry 30 bash -c "$(declare -f prom edge); work=$work; prom 'targets?state=active' | python3 -c 'import sys,json; t=json.load(sys.stdin)[\"data\"][\"activeTargets\"]; d=[x[\"labels\"][\"job\"]+\" \"+x[\"scrapeUrl\"] for x in t if x[\"health\"]!=\"up\" and x[\"labels\"][\"job\"]!=\"blackbox\" and not (x[\"labels\"][\"host\"]==\"db-replica\" and x[\"labels\"][\"job\"] in (\"node\",\"cadvisor\"))]; print(\"down:\",d) if d else None; sys.exit(1 if d else 0)'"
check "blackbox probes of API/S3 succeed" retry 20 bash -c "$(declare -f prom edge); work=$work; prom 'query?query=min(probe_success%7Binstance!~%22.*$D/api.*%22%7D)' | grep -q '\"1\"\]'"
check "alert rules loaded (25)" bash -c "$(declare -f prom edge); work=$work; [[ \$(prom rules | grep -o '\"type\":\"alerting\"' | wc -l) -ge 25 ]]"
check "Watchdog alert reaches Alertmanager" retry 30 bash -c "$(declare -f edge); work=$work; edge exec -T alertmanager wget -qO- http://127.0.0.1:9093/api/v2/alerts | grep -q Watchdog"
check "api logs arrive in Loki" retry 30 bash -c "curl -sf --noproxy '*' -G http://$IP:3100/loki/api/v1/query_range --data-urlencode 'query={service=\"api1\"}' --data-urlencode limit=1 | grep -q request_id"
gpass=$(sed -n 's/^GRAFANA_ADMIN_PASSWORD=//p' "$work/env/app.env")
gq() { curl -sf --noproxy '*' -u "admin:$gpass" "http://$IP:3000$1"; }
check "Grafana (UZ, WireGuard only) reaches Prometheus in DE" retry 20 bash -c "$(declare -f gq); gpass=$gpass; IP=$IP; gq /api/datasources/uid/prometheus/health | grep -q OK"
check "Grafana reaches Loki locally" retry 20 bash -c "$(declare -f gq); gpass=$gpass; IP=$IP; gq /api/datasources/uid/loki/health | grep -qi ok"
check "Grafana not published by edge Caddy" bash -c "! grep -q 'reverse_proxy grafana' $work/edge/Caddyfile"

step "backups: WAL archive, base backup, replica"
psqlp() { dbc exec -T -u postgres postgres psql -U postgres -d boutique -tAc "$1"; }
psqlp "select pg_switch_wal()" >/dev/null
check "WAL archived to offsite S3" retry 30 bash -c "$(declare -f psqlp dbc); work=$work; [[ \$(psqlp 'select archived_count from pg_stat_archiver') -gt 0 ]]"
check "archive has no failures" bash -c "$(declare -f psqlp dbc); work=$work; [[ \$(psqlp 'select failed_count from pg_stat_archiver') == 0 ]]"
check "first WAL-G base backup made automatically" retry 60 bash -c "$(declare -f dbc); work=$work; dbc exec -T walg wal-g backup-list 2>/dev/null | grep -q base_"
check "base backup metric written" retry 10 grep -q 'boutique_walg_last_run_ok 1' "$work/metrics-db/walg.prom"
check "replica streaming" retry 60 bash -c "$(declare -f psqlp dbc); work=$work; psqlp \"select state from pg_stat_replication where application_name='replica1'\" | grep -q streaming"
psqlp "create table if not exists smoke_marker(id int primary key, note text); insert into smoke_marker values (1, 'before') on conflict do nothing; alter table smoke_marker owner to boutique" >/dev/null
check "replica sees new rows" retry 20 bash -c "$(declare -f rep); work=$work; rep exec -T -u postgres postgres psql -U postgres -d boutique -tAc 'select note from smoke_marker where id=1' | grep -q before"
ro_check() { ! rep exec -T -u postgres postgres psql -d boutique -c "insert into smoke_marker values (2, 'x')" >/dev/null 2>&1; }
check "replica is read-only" ro_check

step "PITR: restore to a moment before an accidental delete"
sleep 2; t=$(date -u '+%Y-%m-%d %H:%M:%S+00'); sleep 2
psqlp "delete from smoke_marker" >/dev/null
psqlp "select pg_switch_wal()" >/dev/null; sleep 5
(cd "$work/main/db" && "$work/main/scripts/pitr-restore.sh" "$t") && pass "pitr-restore.sh finished" || bad "pitr-restore.sh"
check "restored copy has the deleted row" bash -c "docker exec -u postgres boutique-pitr psql -U postgres -d boutique -tAc 'select note from smoke_marker where id=1' | grep -q before"
check "production DB still without the row (untouched)" bash -c "$(declare -f psqlp dbc); work=$work; [[ \$(psqlp 'select count(*) from smoke_marker') == 0 ]]"
check "restored copy does not archive into prod WAL archive" bash -c "docker exec -u postgres boutique-pitr psql -U postgres -tAc 'show archive_mode' | grep -q off"
docker rm -f boutique-pitr >/dev/null

if [[ "${SKIP_BACKUP_IMAGE:-0}" != "1" ]]; then
  step "logical backup (pg_dump + age) and weekly restore drill"
  check "pg_dump uploaded to offsite" bash -c "$(declare -f dbc); work=$work; dbc exec -T pgdump backup.sh >/dev/null"
  check "restore drill restores latest offsite dump and passes invariants" bash -c "$(declare -f rep); work=$work; rep exec -T restore-drill restore-drill.sh"
fi

step "zero-downtime rolling deploy of API ($TAG -> $TAG2) and automatic rollback"
docker tag "$REGISTRY/api:$TAG" "$REGISTRY/api:$TAG2"
errs="$work/errs"; : > "$errs"
( end=$((SECONDS + 600)); while (( SECONDS < end )) && [[ ! -f "$work/stop" ]]; do
    c=$("${CURL[@]}" -o /dev/null -w '%{http_code}' "$API/v1/products" || true); [[ "$c" == 200 ]] || echo "$c" >> "$errs"; sleep 0.1
  done ) & load=$!
ROLL_PAUSE=3 "$work/main/scripts/deploy.sh" app "$TAG2" && pass "deploy.sh app $TAG2" || bad "deploy.sh app $TAG2"
touch "$work/stop"; wait "$load" || true
check "no failed requests during rolling deploy" test ! -s "$errs"
check "all API instances on $TAG2" bash -c "$(declare -f app); work=$work; [[ \$(app ps --format '{{.Image}}' api1 api2 api3 | grep -c ':$TAG2') == 3 ]]"
docker tag busybox:1.37 "$REGISTRY/api:broken" 2>/dev/null || docker tag "$REGISTRY/backup:smoke" "$REGISTRY/api:broken"
if ROLL_PAUSE=1 "$work/main/scripts/deploy.sh" app broken >/dev/null 2>&1; then bad "broken release must fail"; else pass "broken release refused"; fi
check "rolled back to $TAG2 and serving" bash -c "$(declare -f app); work=$work; grep -q '^IMAGE_TAG=$TAG2' $work/app/.env && $(printf '%q ' "${CURL[@]}") -f -o /dev/null $API/readyz"

step "failover: promote replica"
CONFIRM_FAILOVER=yes bash -c "cd $work/replica/db && $work/replica/prod/scripts/failover.sh" >/dev/null && pass "failover.sh" || bad "failover.sh"
check "replica is primary now and writable" bash -c "$(declare -f rep); work=$work; rep exec -T -u postgres postgres psql -U postgres -d boutique -tAc 'insert into smoke_marker values (3, \$\$after\$\$) returning id' | grep -q 3"

echo
if (( fail )); then echo "SMOKE FAILED"; exit 1; fi
echo "SMOKE OK"
