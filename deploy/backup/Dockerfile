# Сборка из корня репозитория: docker build -f deploy/backup/Dockerfile .
FROM postgres:16-alpine
RUN apk add --no-cache age aws-cli bash coreutils python3
COPY deploy/backup/backup.sh deploy/backup/restore.sh deploy/backup/verify-restore.sh deploy/backup/offsite-check.sh \
     deploy/backup/schedule.sh deploy/backup/restore-drill.sh /usr/local/bin/
COPY scripts/consistency-check.sql /usr/local/share/boutique/consistency-check.sql
ENV CONSISTENCY_SQL=/usr/local/share/boutique/consistency-check.sql
RUN mkdir -p /backups && chown postgres:postgres /backups
USER postgres
# Dev: бэкап каждые BACKUP_INTERVAL секунд. В prod команда задаётся в deploy/prod/db/compose.yml (schedule.sh).
CMD ["bash", "-c", "while true; do backup.sh || echo '{\"level\":\"error\",\"msg\":\"backup failed\"}'; sleep ${BACKUP_INTERVAL:-86400}; done"]
