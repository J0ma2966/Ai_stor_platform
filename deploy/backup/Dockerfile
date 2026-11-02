FROM postgres:16-alpine
RUN apk add --no-cache age aws-cli bash coreutils python3
COPY backup.sh restore.sh verify-restore.sh /usr/local/bin/
# Простой планировщик: бэкап каждые BACKUP_INTERVAL секунд (по умолчанию сутки).
CMD ["bash", "-c", "while true; do backup.sh || echo '{\"level\":\"error\",\"msg\":\"backup failed\"}'; sleep ${BACKUP_INTERVAL:-86400}; done"]
