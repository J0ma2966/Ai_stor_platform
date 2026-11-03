# syntax=docker/dockerfile:1.7
# PostgreSQL 16 + WAL-G (непрерывный архив WAL и базовые бэкапы для PITR).
# Тег образа считается из содержимого этой папки (см. .github/workflows/cd.yml),
# поэтому обычный деплой приложения НЕ перезапускает базу.
FROM postgres:16.10-bookworm
ARG WALG_VERSION=v3.0.7
ARG WALG_SHA256=76d51ed915165d45314bc947300b9d1776adb2d875d857f580a730fd6f66900e
ADD --checksum=sha256:${WALG_SHA256} \
    https://github.com/wal-g/wal-g/releases/download/${WALG_VERSION}/wal-g-pg-ubuntu-22.04-amd64.tar.gz /tmp/walg.tgz
RUN tar -xzf /tmp/walg.tgz -C /tmp && install -m 0755 /tmp/wal-g-pg-ubuntu-22.04-amd64 /usr/local/bin/wal-g \
 && rm -f /tmp/walg.tgz /tmp/wal-g-pg-ubuntu-22.04-amd64 && wal-g --version
COPY postgresql.conf pg_hba.conf /etc/postgresql/
COPY initdb/ /docker-entrypoint-initdb.d/
COPY boutique-entrypoint.sh walg-loop.sh /usr/local/bin/
ENTRYPOINT ["boutique-entrypoint.sh"]
CMD ["postgres", "-c", "config_file=/etc/postgresql/postgresql.conf", "-c", "hba_file=/etc/postgresql/pg_hba.conf"]
