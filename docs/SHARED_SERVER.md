# Установка на общий сервер (рядом с другим проектом)

Облегчённая установка для тестового запуска на сервере, где уже работает другой сайт
(сейчас это `89.125.62.98`, на нём «Платформа для заказов»). Боевая схема на отдельном
сервере описана в [DEPLOY.md](DEPLOY.md).

## Как это устроено

- Всё, что относится к магазину, лежит в одной папке `/opt/ai-store`:
  `src/` (код), `.env` (настройки и пароли), `data/` (база, фото), `backups/` (копии).
- Магазин работает в Docker и слушает только внутренние адреса `127.0.0.1:3100` (сайт),
  `:8180` (API) и `:9100` (хранилище фото). Порты 80, 443, 3000 и 5432 он не занимает.
- Запросы принимает nginx сервера. Установка добавляет ему один файл
  `/etc/nginx/sites-available/ai-store.conf` для адресов `shop.optolink.uz`,
  `api.`, `media.` и `s3.shop.optolink.uz`. Настройки других сайтов не меняются.
  Если nginx не принимает новый файл, установка отменяет его и останавливается.
- У магазина своя база PostgreSQL в контейнере. Базу сервера (порт 5432) он не использует.
- Память: контейнеры ограничены примерно 3,5 ГБ в сумме, обычно занято около 1 ГБ.
- Режим `APP_ENV=staging`: письма и SMS не отправляются, коды входа пишутся в журнал.

Вне папки создаются только: файл nginx и два файла `/etc/nginx/snippets/ai-store-*`,
`/etc/cron.d/ai-store`, команда `/usr/local/bin/ai-store`, сертификат
`/etc/letsencrypt/live/shop.optolink.uz`. Docker ставится из пакетов Ubuntu, если его нет.

## DNS

В панели домена `optolink.uz`: A-записи `shop` и `*.shop` на IP сервера.

## Установка

На сервере от root:

```bash
mkdir -p /opt/ai-store
git clone https://github.com/J0ma2966/Ai_stor_platform.git /opt/ai-store/src
bash /opt/ai-store/src/deploy/shared/ai-store install shop.optolink.uz admin@optolink.uz
ai-store create-admin admin
```

Первая сборка идёт 5–10 минут. В конце команда печатает адрес сайта. Повторный запуск
`install` безопасен: пароли и данные сохраняются.

## Команды

| Команда | Что делает |
|---|---|
| `ai-store status` | состояние сервисов, ответы сайта и API, занятое место |
| `ai-store update` | копия базы, новый код из GitHub, пересборка, перезапуск |
| `ai-store restart` | перезапуск после правки `/opt/ai-store/.env` |
| `ai-store logs api` | журнал (`api`, `web`, `postgres`, `minio`); выход Ctrl+C |
| `ai-store backup` | копия базы и фото в `/opt/ai-store/backups` (и так каждую ночь в 02:10, хранится 7 дней) |
| `ai-store create-admin <логин>` | создать администратора панели `/admin` |
| `ai-store add-swap` | добавить 2 ГБ подкачки, если для сборки не хватает памяти |

Код входа при регистрации в тестовом режиме виден в журнале: `ai-store logs api`.

## Восстановление из копии

```bash
docker compose -p ai-store exec -T postgres pg_restore -U boutique -d boutique --clean --if-exists < /opt/ai-store/backups/db-ДАТА.dump
```

## Удаление

```bash
cd /opt/ai-store && ENV_FILE=.env docker compose -p ai-store --env-file .env -f src/deploy/shared/compose.yml down
rm /etc/nginx/sites-enabled/ai-store.conf /etc/nginx/sites-available/ai-store.conf /etc/nginx/snippets/ai-store-*.conf /etc/cron.d/ai-store /usr/local/bin/ai-store
nginx -t && systemctl reload nginx
```

Папку `/opt/ai-store` с данными удалите отдельно, когда убедитесь, что копии не нужны.

## Ограничения

- Сервер находится не в Узбекистане. Для реальных покупателей нужен сервер в UZ
  (закон о персональных данных) и боевая схема из DEPLOY.md.
- Один экземпляр сайта и API, без реплики базы и без офсайт-копий. Копии лежат на том же сервере.
- API работает под владельцем базы (в боевой схеме у него урезанные права).
