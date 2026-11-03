# Runbook: что делать, когда пришёл алерт

Алерты приходят в Telegram. Ссылка в алерте ведёт на раздел ниже. Общие правила:

- Все команды выполняются на сервере из каталога роли: `cd /opt/boutique/prod/<app|db|edge>` (на app: `docker compose -f compose.yml …`).
- Сначала посмотрите дашборд Grafana «Boutique — обзор», потом логи: Grafana → Explore → Loki, запрос `{service="api1"} |= "<request_id>"` или `{level="ERROR"}`.
- Пользователь сообщает `request_id` из сообщения об ошибке. Поиск по нему в Loki показывает весь запрос.
- Если вы что-то поменяли руками на сервере, повторите это в репозитории, иначе следующий деплой вернёт старое.

## Сервер недоступен
`HostDown`: Prometheus в Германии не достаёт сервер по WireGuard.
1. Проверьте, открывается ли сайт и `https://api.<домен>/readyz`. Если открываются, сломан только WireGuard: `ssh root@<сервер> wg show` (последний handshake должен быть меньше 2 минут назад), `systemctl restart wg-quick@wg0`.
2. Если сервер не отвечает по SSH, откройте панель хостинга: перезагрузка, консоль, состояние диска.
3. Если не отвечает сервер БД и он не поднимается за 15 минут, переходите к разделу [Отказ сервера БД](#отказ-сервера-бд).

## Сервис недоступен
`TargetDown` для api/postgres/redis/minio.
```bash
docker compose ps                      # что не в состоянии running
docker compose logs --tail 100 <сервис>
docker compose up -d <сервис>          # перезапуск одного сервиса
```
Для API хватает перезапуска одного инстанса, Caddy отправит запросы на другие. Если падают все три инстанса после релиза, [откатите релиз](#откат-релиза).

## Сайт или API недоступны
`EndpointDown`: внешняя проверка не получает 200.
1. `curl -sv https://api.<домен>/readyz`: если ответ 503 и `"db":"fail"` или `"redis":"fail"`, проблема в базе или Redis (см. разделы ниже).
2. Ошибка TLS: `docker compose logs caddy | grep -i acme`. Причины: DNS указывает не туда, закрыт порт 80, лимит Let's Encrypt. Caddy повторяет попытки сам.
3. Если сайт не отвечает, а API работает: `cd /opt/boutique/prod/edge && docker compose ps web1 web2`.

## Ошибки 5xx
`ApiHighErrorRate`: больше 2% ответов 5xx.
1. Loki: `{stack="boutique-app", level="ERROR"}`. Посмотрите, какой роут и какой текст ошибки.
2. Если ошибки начались сразу после релиза, [откатите релиз](#откат-релиза).
3. `unavailable` при загрузке фото: проверьте MinIO (`cd /opt/boutique/prod/db && docker compose ps minio`).
4. `statement timeout`: смотрите раздел [Медленно работает](#медленно-работает).

## Медленно работает
`ApiHighLatency`: p95 больше 500 мс.
1. Дашборд, панель «Задержка по роутам»: какой роут медленный.
2. Медленные запросы к базе (дольше 500 мс) пишет сам PostgreSQL: Loki `{service="postgres"} |= "duration"`. Топ запросов:
   ```bash
   cd /opt/boutique/prod/db
   docker compose exec -u postgres postgres psql -d boutique -c \
     "select calls, round(mean_exec_time) ms, left(query, 120) from pg_stat_statements order by total_exec_time desc limit 15"
   ```
3. Блокировки: `select pid, wait_event_type, now()-query_start, left(query,80) from pg_stat_activity where state <> 'idle' order by 3 desc;`
4. Профиль CPU живого API (с сервера app, по WireGuard): `curl -o cpu.pprof http://10.77.0.10:9101/debug/pprof/profile?seconds=30`, затем `go tool pprof cpu.pprof`.
5. Если CPU на app выше 85% всё время, увеличьте `API_CPUS` или добавьте инстанс (см. DEPLOY.md, «Масштабирование»).

## База данных недоступна
`PostgresDown`.
```bash
cd /opt/boutique/prod/db
docker compose ps postgres && docker compose logs --tail 200 postgres
df -h /srv/pgdata                       # закончилось место? см. «Заканчивается диск»
docker compose up -d postgres
```
Если контейнер не стартует из-за повреждения данных или сервер потерян, переходите к разделу [Отказ сервера БД](#отказ-сервера-бд).

## Отказ сервера БД
Primary потерян и в ближайшее время не вернётся. Реплика отстаёт максимум на секунды.
1. **Убедитесь, что старый primary выключен** (в панели хостинга) и сам не вернётся. Две главные базы одновременно означают потерю данных.
2. На сервере реплики:
   ```bash
   cd /opt/boutique/prod/db && CONFIRM_FAILOVER=yes ../scripts/failover.sh
   ```
3. В GitHub в секрете `ENV_APP` замените `DB_PRIVATE_IP` на адрес реплики (`10.77.0.21`), в переменных окружения поменяйте местами `DB_*` и `REPLICA_*`, затем Actions → CD → Run workflow → production.
4. Фото: MinIO работал на старом primary. Пока сервер не восстановлен, поднимите MinIO на новом primary и залейте фото из офсайт-зеркала:
   ```bash
   docker compose --profile primary up -d minio minio-init
   docker compose run --rm --entrypoint sh media-mirror -c \
     'mc alias set offsite "$MIRROR_ENDPOINT" "$BACKUP_S3_ACCESS_KEY" "$BACKUP_S3_SECRET_KEY" && mc mirror offsite/$DST $SRC'
   ```
   После этого поменяйте `COMPOSE_PROFILES=primary` в `ENV_DB` для нового primary.
5. Новую реплику создайте на новом сервере: при пустом каталоге данных она сама клонирует primary (`PG_ROLE=replica`, `PRIMARY_HOST=<новый primary>`). На новом primary создайте слот: `select pg_create_physical_replication_slot('replica1');`.

## PITR: восстановление на момент времени
Пример: в 14:59 кто-то удалил данные, и нужно вернуть базу на 14:58.
1. На сервере БД восстановите копию во временный экземпляр. Рабочая база не затрагивается:
   ```bash
   cd /opt/boutique/prod/db
   ../scripts/pitr-restore.sh "2026-11-02 14:58:00+05"
   ```
   Скрипт берёт последний базовый бэкап до этого времени, догоняет WAL до указанной секунды и запускает копию на `127.0.0.1:5433`. В копии архивирование WAL выключено.
2. Проверьте данные: `docker exec -it -u postgres boutique-pitr psql -d boutique`.
3. Вариант А, вернуть часть данных (обычно так и делают). Выгрузите нужные строки из копии и загрузите их в рабочую базу:
   ```bash
   docker exec -u postgres boutique-pitr pg_dump -d boutique -t <таблица> --data-only > rows.sql
   ```
4. Вариант Б, откатить всю базу (теряются все изменения после этого момента). Остановите API (`docker compose stop api1 api2 api3` на app), остановите рабочий postgres и в `ENV_DB` укажите `PG_DATA_DIR` на том `boutique-pitr-…` (или скопируйте его данные в `/srv/pgdata`), затем `deploy.sh db`, проверьте и запустите API. Реплику после этого пересоздайте: очистите её каталог данных.
5. Удалите временный экземпляр: `docker rm -f boutique-pitr && docker volume rm <том>`.

## Бэкапы
`WalArchiveFailing`, `BaseBackupTooOld`, `BackupJobFailed`, `PgDumpTooOld`, `RestoreDrillTooOld`.
```bash
cd /opt/boutique/prod/db
docker compose exec -u postgres postgres psql -c "select * from pg_stat_archiver"   # last_failed_wal, last_failed_time
docker compose logs --tail 100 postgres | grep -i archive
docker compose logs --tail 100 walg pgdump
docker compose exec walg wal-g backup-list                                       # есть ли свежие бэкапы
```
Частые причины: офсайт-S3 недоступен или закончилось место, истёк ключ доступа (обновите `BACKUP_S3_*` в `ENV_DB` и выкатите). Пока WAL не архивируется, он копится на диске базы. Если починить нельзя быстро, следите за местом (`df -h /srv/pgdata`).

Ручной запуск: базовый бэкап `docker compose exec walg wal-g backup-push /var/lib/postgresql/data`, дамп `docker compose exec pgdump backup.sh`, проверка восстановления (на реплике) `docker compose exec restore-drill restore-drill.sh`.

## Заканчивается диск
`DiskWillFillSoon`.
```bash
df -h; du -xsh /var/lib/docker/* /srv/* 2>/dev/null | sort -h | tail
docker system df
docker image prune -a --filter "until=168h"     # старые образы релизов
```
- На db: если растёт `pg_wal`, значит, не работает архивирование (см. «Бэкапы») или реплика долго недоступна (слот держит WAL до 50 ГБ, `max_slot_wal_keep_size`).
- На app: Loki хранит логи 30 дней (`deploy/prod/monitoring/loki.yml`, `retention_period`).
- Увеличить диск у хостинга обычно можно без остановки, затем `resize2fs`.

## Откат релиза
Быстрее всего: Actions → CD → Run workflow, в `ref` укажите предыдущий хороший коммит. Вручную на сервере:
```bash
cd /opt/boutique/prod && scripts/deploy.sh app <тег предыдущего коммита>   # тег = первые 12 символов SHA
```
Миграции обратно совместимы, поэтому старый код работает с новой схемой. Если миграция всё-таки сломала данные, используйте [PITR](#pitr-восстановление-на-момент-времени).

## Ротация секретов
- Пароль приложения к БД: `ALTER ROLE boutique PASSWORD '…'` на primary, затем новое значение `APP_DB_PASSWORD` в `ENV_APP` и `ENV_DB`, затем CD.
- `JWT_SECRET`: после смены все пользователи должны войти заново (access-токены живут 15 минут, refresh-токены отзываются).
- Ключи MinIO и Redis: поменяйте значения в `ENV_*`, затем CD. `minio-init` обновит пользователя приложения.
- Ключ age для дампов: создайте новый, обновите `AGE_RECIPIENT` и `AGE_IDENTITY`. Старый приватный ключ храните, пока живут старые дампы.
- Ключ шифрования WAL-G (`WALG_LIBSODIUM_KEY`) не меняйте, пока в архиве есть бэкапы, сделанные со старым ключом.

## Мониторинг молчит
Алерт `Watchdog` горит всегда и никуда не отправляется. Если в Grafana → Alerting его нет, значит, сломан сам мониторинг: `cd /opt/boutique/prod/edge && docker compose ps prometheus alertmanager`. Проверить доставку в Telegram: `docker compose logs alertmanager | grep -i telegram`. Частые причины: неверный chat id или бот не добавлен в группу.
