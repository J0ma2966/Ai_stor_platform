# Бэкапы и восстановление

Два уровня защиты:

| Уровень | Чем | Как часто | Для чего |
|---|---|---|---|
| Логический бэкап | `deploy/backup/backup.sh` (pg_dump + age) | раз в сутки (сервис `backup` в compose или cron) | восстановить базу целиком или отдельные таблицы, перенести на другой сервер |
| Непрерывный архив WAL (PITR) | WAL-G | постоянно + базовый бэкап раз в сутки | откатиться на любую секунду (например, за минуту до ошибочного удаления) |

Оба уровня пишут в **отдельный** S3-бакет с versioning и object lock, в другом регионе/у другого провайдера, чем основная база.

## Шифрование

Бэкапы шифруются [age](https://age-encryption.org) публичным ключом. Сервер бэкапов знает только публичный ключ и не может прочитать бэкапы; приватный ключ хранится офлайн (менеджер паролей/сейф) у владельца.

```bash
age-keygen -o backup-key.txt         # приватный ключ: хранить вне серверов
age-keygen -y backup-key.txt         # публичный ключ → AGE_RECIPIENT
```

## Ручной бэкап

```bash
make backup                                     # в docker compose
# или напрямую:
DATABASE_URL=postgres://... AGE_RECIPIENT=age1... BACKUP_S3_URI=s3://boutique-backups/daily deploy/backup/backup.sh
```

Рядом с каждым файлом пишется `.sha256`. Локальные копии старше `RETENTION_DAYS` (14) удаляются; в S3 срок хранения задаётся правилом жизненного цикла бакета (рекомендуется 30 дней ежедневных + 12 месячных).

## Восстановление

```bash
createdb boutique_restored
TARGET_DATABASE_URL=postgres://.../boutique_restored AGE_IDENTITY=backup-key.txt CONFIRM_RESTORE=yes \
  deploy/backup/restore.sh s3://boutique-backups/daily/boutique_20261102T030000Z.dump.age
```

Скрипт проверяет контрольную сумму и целостность архива и восстанавливает в одной транзакции (при ошибке база остаётся как была). Без `CONFIRM_RESTORE=yes` не запускается.

Порядок при аварии: восстановить в новую базу → проверить (`/readyz`, количество заказов за последний день) → переключить `DATABASE_URL` → перезапустить API.

## Проверка, что бэкапы рабочие

`deploy/backup/verify-restore.sh` делает бэкап, восстанавливает его во временную базу, сверяет количество строк в ключевых таблицах и наличие защитных триггеров. Запускается в CI на каждый PR и должен запускаться по расписанию на prod-реплике (раз в неделю) с алертом при ошибке.

## PITR с WAL-G (prod)

В `postgresql.conf`:

```
wal_level = replica
archive_mode = on
archive_command = 'wal-g wal-push %p'
archive_timeout = 60
```

Переменные для WAL-G: `WALG_S3_PREFIX=s3://boutique-backups/wal`, `WALG_LIBSODIUM_KEY` (шифрование), `AWS_*`. Базовый бэкап по cron: `wal-g backup-push $PGDATA` раз в сутки, `wal-g delete retain FULL 7 --confirm` для ротации.

Восстановление на момент времени: `wal-g backup-fetch $PGDATA LATEST`, затем `restore_command = 'wal-g wal-fetch %f %p'` и `recovery_target_time = '2026-11-02 14:59:00+05'` в конфиге, создать `recovery.signal`, запустить PostgreSQL.

Цели: **RPO ≤ 1 минута** (потеря данных), **RTO ≤ 1 час** (время восстановления).
