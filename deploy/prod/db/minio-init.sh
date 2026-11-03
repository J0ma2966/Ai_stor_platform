#!/bin/sh
# Идемпотентная настройка MinIO (запускается при каждом деплое).
set -eu
mc alias set local http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
b="local/$S3_BUCKET"
mc mb -p "$b"
# приватный бакет; анонимно читается только pub/ (фото товаров, логотипы)
mc anonymous set none "$b"
mc anonymous set download "$b/pub"
# версии: 7 дней можно откатить случайное удаление/перезапись, потом удаляются насовсем
mc version enable "$b"
# в образе mc нет grep: проверяем вывод через case, иначе каждый деплой добавлял бы дубль правила
rules=$(mc ilm rule ls --json "$b" 2>/dev/null || true)
case "$rules" in
  *NoncurrentVersionExpiration*) ;;
  *) mc ilm rule add --noncurrent-expire-days 7 --expire-delete-marker "$b" ;;
esac

# пользователь приложения: только объекты своего бакета, без админ-прав
cat > /tmp/app-policy.json <<JSON
{"Version":"2012-10-17","Statement":[
 {"Effect":"Allow","Action":["s3:GetObject","s3:PutObject","s3:DeleteObject"],"Resource":["arn:aws:s3:::$S3_BUCKET/*"]},
 {"Effect":"Allow","Action":["s3:ListBucket","s3:GetBucketLocation"],"Resource":["arn:aws:s3:::$S3_BUCKET"]}]}
JSON
mc admin policy create local boutique-app /tmp/app-policy.json >/dev/null 2>&1 \
  || { mc admin policy remove local boutique-app >/dev/null 2>&1 || true; mc admin policy create local boutique-app /tmp/app-policy.json; }
mc admin user add local "$MINIO_APP_ACCESS_KEY" "$MINIO_APP_SECRET_KEY" >/dev/null
mc admin policy attach local boutique-app --user "$MINIO_APP_ACCESS_KEY" >/dev/null 2>&1 || true

# офсайт-бакет для зеркала должен существовать (и иметь versioning у провайдера)
mc alias set offsite "$MEDIA_MIRROR_ENDPOINT" "$BACKUP_S3_ACCESS_KEY" "$BACKUP_S3_SECRET_KEY" >/dev/null
# офсайт-бакеты (зеркало фото, WAL-G, pg_dump) должны существовать; создаём, если ключи позволяют
for b in "$MEDIA_MIRROR_URI" "${WALG_S3_PREFIX#s3://}" "${PGDUMP_S3_URI#s3://}"; do
  b="${b%%/*}"
  mc ls "offsite/$b" >/dev/null 2>&1 || mc mb -p "offsite/$b"
done
echo "minio-init: ok"
