#!/usr/bin/env bash
# Проверка установки на общий сервер (CI, запуск от root на чистой машине с Docker):
# рядом с уже работающим сайтом nginx ставится магазин командой ai-store install,
# затем проверяются сайт, API, загрузка фото и то, что «чужой» сайт не пострадал.
# Сертификаты Let's Encrypt заменяет поддельный certbot с локальным CA.
set -euo pipefail
D=shop.smoke.test
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
export AI_STORE_ROOT=/opt/ai-store SKIP_DNS_CHECK=1
CA=/tmp/ai-store-ca
fail=0
pass() { printf '\e[32mPASS\e[0m %s\n' "$*"; }
bad()  { printf '\e[31mFAIL\e[0m %s\n' "$*"; fail=1; }
step() { printf '\n\e[1m== %s\e[0m\n' "$*"; }
check() { local name="$1"; shift; if "$@"; then pass "$name"; else bad "$name"; fi; }
code() { curl -sS -m 15 --cacert $CA/ca.crt -o /dev/null -w '%{http_code}' "$@" || true; }

step "the neighbour: an nginx site that is already running on this server"
apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nginx >/dev/null
systemctl start nginx
echo neighbour-ok > /var/www/html/index.html
check "neighbour site answers before install" bash -c '[[ $(curl -s http://127.0.0.1/) == neighbour-ok ]]'

step "fake certbot (local CA) and DNS"
mkdir -p $CA
cat > /usr/local/bin/certbot <<'F'
#!/usr/bin/env bash
# подделка для smoke-теста: выпускает сертификат локальным CA вместо Let's Encrypt
set -euo pipefail
CA=/tmp/ai-store-ca
[[ ${1:-} == plugins ]] && { echo "* nginx"; exit 0; }
name=""; sans=()
while [[ $# -gt 0 ]]; do case "$1" in --cert-name) name="$2"; shift 2 ;; -d) sans+=("DNS:$2"); shift 2 ;; *) shift ;; esac; done
[[ -f $CA/ca.key ]] || openssl req -x509 -newkey rsa:2048 -nodes -days 2 -subj "/CN=smoke-ca" -keyout $CA/ca.key -out $CA/ca.crt 2>/dev/null
live=/etc/letsencrypt/live/$name; mkdir -p "$live"
openssl req -newkey rsa:2048 -nodes -subj "/CN=$name" -keyout "$live/privkey.pem" -out $CA/req.csr 2>/dev/null
openssl x509 -req -in $CA/req.csr -CA $CA/ca.crt -CAkey $CA/ca.key -CAcreateserial -days 2 -out "$live/fullchain.pem" \
  -extfile <(printf 'subjectAltName=%s' "$(IFS=,; echo "${sans[*]}")") 2>/dev/null
cat /etc/ssl/certs/ca-certificates.crt $CA/ca.crt > $CA/bundle.crt
F
chmod +x /usr/local/bin/certbot
echo "127.0.0.1 $D api.$D media.$D s3.$D" >> /etc/hosts
# API должен доверять локальному CA: к S3 он ходит по https через nginx
cat > $CA/compose.extra.yml <<F
services:
  api:
    volumes: ["$CA/bundle.crt:/etc/ssl/certs/ca-certificates.crt:ro"]
F
export AI_STORE_COMPOSE_EXTRA=$CA/compose.extra.yml

step "install"
mkdir -p $AI_STORE_ROOT && cp -r "$repo" $AI_STORE_ROOT/src
bash $AI_STORE_ROOT/src/deploy/shared/ai-store install $D ops@$D
check "ai-store command installed" test -x /usr/local/bin/ai-store
echo ".env mode: $(stat -c '%a %U' /opt/ai-store/.env)"
check ".env is private" test "$(stat -c %a /opt/ai-store/.env)" = 600
check "neighbour site still answers" bash -c '[[ $(curl -s http://127.0.0.1/) == neighbour-ok ]]'
check "nothing listens on public addresses except nginx" bash -c "! ss -tlnH | awk '{print \$4}' | grep -E ':(3100|8180|9100)\$' | grep -v '^127\.0\.0\.1:'"

step "site, API, S3 and media through nginx"
check "http redirects to https" bash -c "curl -sI http://$D/ | grep -qi '^location: https://$D/'"
check "site /api/healthz 200" test "$(code https://$D/api/healthz)" = 200
check "site home 200" test "$(code https://$D/)" = 200
check "api /readyz 200" test "$(code https://api.$D/readyz)" = 200
check "api /metrics hidden" test "$(code https://api.$D/metrics)" = 404
check "api /debug hidden" test "$(code https://api.$D/debug/pprof/)" = 404
check "minio admin API hidden" test "$(code https://s3.$D/minio/admin/v3/info)" = 404
check "media root 404" test "$(code https://media.$D/)" = 404
check "media does not expose private keys" test "$(code https://media.$D/priv/x)" = 404

step "client login (SMS codes in the log), photo upload, presigned URL"
sed -i 's/^SMS_AUTH_ENABLED=.*/SMS_AUTH_ENABLED=true/' $AI_STORE_ROOT/.env
ai-store restart
phone="+998901112233"; API=https://api.$D
curl -sS -f --cacert $CA/ca.crt -o /dev/null -H 'Content-Type: application/json' -d "{\"phone\":\"$phone\"}" "$API/v1/auth/otp/request" || bad "otp request"
otp=""; for _ in $(seq 1 10); do
  otp=$(docker compose -p ai-store logs --no-log-prefix api 2>/dev/null | grep 'DEV OTP' | tail -1 | sed -n 's/.*"code":"\([0-9]*\)".*/\1/p'); [[ -n "$otp" ]] && break; sleep 1; done
tok=$(curl -sS --cacert $CA/ca.crt -H 'Content-Type: application/json' -d "{\"phone\":\"$phone\",\"code\":\"$otp\"}" "$API/v1/auth/otp/verify" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
check "client logged in" test -n "$tok"
python3 - /tmp/p.png <<'PY'
import sys,struct,zlib
w=h=8; raw=b''.join(b'\x00'+b'\xc8\x50\x50'*w for _ in range(h))
def ch(t,d): return struct.pack('>I',len(d))+t+d+struct.pack('>I',zlib.crc32(t+d)&0xffffffff)
open(sys.argv[1],'wb').write(b'\x89PNG\r\n\x1a\n'+ch(b'IHDR',struct.pack('>IIBBBBB',w,h,8,2,0,0,0))+ch(b'IDAT',zlib.compress(raw))+ch(b'IEND',b''))
PY
purl=""
for _ in $(seq 1 5); do
  up=$(curl -sS --cacert $CA/ca.crt -H "Authorization: Bearer $tok" -H 'Content-Type: image/png' --data-binary @/tmp/p.png "$API/v1/uploads?purpose=delivery" || true)
  purl=$(sed -n 's/.*"url":"\([^"]*\)".*/\1/p' <<<"$up" | sed 's/\\u0026/\&/g')
  [[ -n "$purl" ]] && break
  echo "upload response: ${up:0:300}"; sleep 3
done
[[ -n "$purl" ]] || docker compose -p ai-store logs --tail 40 api
check "upload returns presigned https://s3.$D URL" grep -q "^https://s3.$D/" <<<"$purl"
check "presigned URL downloads the photo" test "$(code "$purl")" = 200
check "private photo without signature is denied" test "$(code "${purl%%\?*}")" = 403
docker run --rm --network ai-store_default --entrypoint sh \
  -e U="$(grep ^MINIO_ROOT_USER= $AI_STORE_ROOT/.env | cut -d= -f2)" -e P="$(grep ^MINIO_ROOT_PASSWORD= $AI_STORE_ROOT/.env | cut -d= -f2)" \
  pgsty/mc@sha256:ce62c9053c69ce12a527a322345b5ad9f44b5f8a7b8189f82e516f2e3f30f1d8 \
  -c 'printf img > /tmp/x.jpg && mc alias set l http://minio:9000 "$U" "$P" >/dev/null && mc cp -q /tmp/x.jpg l/boutique-media/pub/smoke/x.jpg >/dev/null'
check "pub/ image 200 with immutable cache" bash -c "curl -sI --cacert $CA/ca.crt https://media.$D/pub/smoke/x.jpg | grep -qi 'cache-control: public, max-age=31536000, immutable'"
check "API sees the client address passed by nginx" bash -c "docker compose -p ai-store logs --no-log-prefix api | grep -q 'ip.:.127[.]0[.]0[.]1'"

step "admin, backup, status, repeated install"
printf 'Smoke-Admin-Pass-1\nSmoke-Admin-Pass-1\n' | ai-store create-admin smokeadmin | grep -q 'admin created' && pass "create-admin" || bad "create-admin"
ai-store backup
check "database dump created" bash -c 'ls /opt/ai-store/backups/db-*.dump >/dev/null'
check "media archive created" bash -c 'ls /opt/ai-store/backups/media-*.tar.gz >/dev/null'
ai-store status
bash $AI_STORE_ROOT/src/deploy/shared/ai-store install $D ops@$D >/dev/null && pass "repeated install is safe" || bad "repeated install"
check "passwords survive repeated install" test "$(code https://api.$D/readyz)" = 200

step "production mode: refused without mail, rolled back when the API cannot start, then switched"
if ai-store prod >/dev/null 2>&1; then bad "prod without SMTP must be refused"; else pass "prod without SMTP refused"; fi
check "still staging and healthy" bash -c "grep -q '^APP_ENV=staging' $AI_STORE_ROOT/.env && [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8180/readyz) == 200 ]]"
sed -i -e 's/^MAIL_PROVIDER=.*/MAIL_PROVIDER=smtp/' -e 's/^SMTP_HOST=.*/SMTP_HOST=smtp.smoke.test/' -e 's/^SMTP_USER=.*/SMTP_USER=u@smoke.test/' \
  -e 's/^SMTP_PASSWORD=.*/SMTP_PASSWORD=x/' -e 's/^MAIL_FROM=.*/MAIL_FROM=u@smoke.test/' $AI_STORE_ROOT/.env
# SMS-вход с SMS_PROVIDER=log в prod запрещён: API не стартует, режим должен вернуться
if READY_TRIES=10 ai-store prod >/dev/null 2>&1; then bad "prod with invalid config must fail"; else pass "prod with invalid config rolled back"; fi
check "back to staging and healthy after failed switch" bash -c "grep -q '^APP_ENV=staging' $AI_STORE_ROOT/.env && [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8180/readyz) == 200 ]]"
sed -i 's/^SMS_AUTH_ENABLED=.*/SMS_AUTH_ENABLED=false/' $AI_STORE_ROOT/.env
ai-store prod && pass "switched to prod" || bad "switch to prod"
check "prod mode saved and API ready" bash -c "grep -q '^APP_ENV=prod' $AI_STORE_ROOT/.env && [[ \$(curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8180/readyz) == 200 ]]"
check "site answers in prod" test "$(code https://$D/api/healthz)" = 200
sed -i -e 's/^BACKUP_TELEGRAM_BOT_TOKEN=.*/BACKUP_TELEGRAM_BOT_TOKEN=1:invalid/' -e 's/^BACKUP_TELEGRAM_CHAT_ID=.*/BACKUP_TELEGRAM_CHAT_ID=1/' $AI_STORE_ROOT/.env
ai-store backup 2>&1 | tee /tmp/backup.out
check "failed offsite upload is reported but the local backup is kept" grep -q 'ВНИМАНИЕ' /tmp/backup.out
check "no encrypted leftovers" bash -c '! ls /opt/ai-store/backups/*.enc >/dev/null 2>&1'

step "AI try-on: refused without a key, then switched on in prod"
fit() { curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:8180/v1/fitting/adds; }
check "try-on is off by default (503)" test "$(fit)" = 503
if ai-store tryon on --yes >/dev/null 2>&1; then bad "tryon on without OPENAI_API_KEY must be refused"; else pass "tryon on without key refused"; fi
sed -i 's/^OPENAI_API_KEY=.*/OPENAI_API_KEY=sk-smoke-not-a-real-key/' $AI_STORE_ROOT/.env
SKIP_OPENAI_CHECK=1 ai-store tryon on --yes && pass "tryon on" || bad "tryon on"
check "prod still on, try-on settings saved" bash -c "grep -q '^APP_ENV=prod' $AI_STORE_ROOT/.env && grep -q '^TRYON_PROVIDER=openai' $AI_STORE_ROOT/.env && grep -q '^BIOMETRIC_FOREIGN_APPROVED=true' $AI_STORE_ROOT/.env"
check "try-on API is on (not 503)" bash -c '[[ $(curl -s -o /dev/null -w "%{http_code}" -H "Content-Type: application/json" -d "{}" http://127.0.0.1:8180/v1/fitting/adds) != 503 ]]'
check "API log shows the in-process try-on worker" bash -c "docker compose -p ai-store logs --no-log-prefix api | grep -q 'tryon worker started'"
docker run --rm --network ai-store_default --entrypoint sh \
  -e U="$(grep ^MINIO_ROOT_USER= $AI_STORE_ROOT/.env | cut -d= -f2)" -e P="$(grep ^MINIO_ROOT_PASSWORD= $AI_STORE_ROOT/.env | cut -d= -f2)" \
  pgsty/mc@sha256:ce62c9053c69ce12a527a322345b5ad9f44b5f8a7b8189f82e516f2e3f30f1d8 \
  -c 'mc alias set l http://minio:9000 "$U" "$P" >/dev/null && mc ls l/boutique-biometric >/dev/null && mc anonymous get l/boutique-biometric' > /tmp/bio.out 2>&1 || true
cat /tmp/bio.out
check "separate biometric bucket exists and is private" grep -qi 'private\|none' /tmp/bio.out
check "biometric bucket is not reachable through media domain" test "$(code https://media.$D/boutique-biometric/x)" = 404
check "fitting page answers" test "$(code https://$D/fitting)" = 200

step "a broken nginx config is rolled back"
cp $AI_STORE_ROOT/src/deploy/shared/nginx/ai-store.conf.tmpl /tmp/tmpl.bak
echo 'this is not nginx syntax;' >> $AI_STORE_ROOT/src/deploy/shared/nginx/ai-store.conf.tmpl
if bash $AI_STORE_ROOT/src/deploy/shared/ai-store install $D ops@$D >/dev/null 2>&1; then bad "broken config must fail"; else pass "broken config rejected"; fi
cp /tmp/tmpl.bak $AI_STORE_ROOT/src/deploy/shared/nginx/ai-store.conf.tmpl
check "nginx config still valid" nginx -t
check "site still answers after rollback" test "$(code https://$D/api/healthz)" = 200
check "neighbour site still answers at the end" bash -c '[[ $(curl -s http://127.0.0.1/) == neighbour-ok ]]'

[[ $fail == 0 ]] && echo && echo "ALL OK" || { echo; echo "SMOKE FAILED"; docker compose -p ai-store ps; exit 1; }
