# Android-приложения

Два отдельных приложения на Kotlin + Jetpack Compose в `mobile/`:

| Модуль | applicationId | Для кого |
|---|---|---|
| `app-client` | `uz.boutique.client` | покупатели: каталог, карточка, отзывы, избранное, корзина, оформление, заказы, уведомления, профиль |
| `app-store` | `uz.boutique.store` | магазин: окно заказов и смена статусов, товары (фото, размеры, публикация), отзывы и ответы, статистика |

Общий код:

- `mobile/core` — чистый Kotlin (без Android SDK): API-клиент на OkHttp, модели, сессия с refresh-кукой и единым обновлением токена, правила и форматирование. Подключён как included build.
- `mobile/ui` — тема (дизайн B), компоненты, строки ru/uz.

Строки ru/uz хранятся в одной таблице `mobile/tools/strings.py`; после правки: `python3 mobile/tools/strings.py && python3 mobile/tools/strings.py --check`.

AI-примерки в приложениях нет (в проде выключена).

## Сборка и тесты

```bash
cd mobile/core && ../gradlew test           # ядро, без Android SDK
cd mobile && ./gradlew testDebugUnitTest lintDebug assembleDebug assembleRelease   # нужен Android SDK
```

CI: `.github/workflows/android.yml`; debug-APK обоих приложений лежат в артефакте `apk-debug` прогона.

Сквозной тест ядра против запущенного API (dev, `SMS_PROVIDER=log`, активный магазин):

```bash
cd mobile/core
BOUTIQUE_IT_API=http://localhost:8080 BOUTIQUE_IT_OTP_LOG=/path/to/api.log \
BOUTIQUE_IT_STORE_LOGIN=shop BOUTIQUE_IT_STORE_PASSWORD=... \
../gradlew test --tests '*LiveApiTest*'
```

## Адрес API

- debug: `http://10.0.2.2:8080` (localhost компьютера из эмулятора); в debug-сборке адрес меняется кнопкой «Сервер» на экране входа (и в профиле клиента).
- release: `boutique.apiUrl` в `mobile/gradle.properties` (сейчас заглушка `https://api.boutique.uz`), только HTTPS.

## Подпись release

Переменные окружения `BOUTIQUE_KEYSTORE`, `BOUTIQUE_KEYSTORE_PASSWORD`, `BOUTIQUE_KEY_ALIAS`, `BOUTIQUE_KEY_PASSWORD`. Без них release собирается неподписанным.

## Проверка с телефона на локальном сервере (Windows, Wi-Fi)

1. Узнайте IP компьютера в Wi-Fi: `ipconfig` → «IPv4-адрес», например `192.168.1.50`. Телефон должен быть в той же сети.
2. В корневом `.env`:
   ```
   DEV_BIND_ADDR=0.0.0.0          # API и фото доступны из локальной сети
   DEV_PUBLIC_HOST=192.168.1.50   # IP из шага 1 — по нему телефон грузит фото
   ```
   Для примерки через OpenAI там же: `TRYON_PROVIDER=openai`, `OPENAI_API_KEY=sk-...` (без них работает демо-провайдер `mock`).
3. `.\scripts\dev.ps1 up` (если уже запущено — `down`, затем `up`). При первом запуске Windows спросит доступ для Docker — разрешите для частных сетей. Если не спросил, в PowerShell от администратора:
   `New-NetFirewallRule -DisplayName "Boutique dev" -Direction Inbound -Protocol TCP -LocalPort 8080,9000 -Action Allow -Profile Private`
4. Проверка с телефона в браузере: `http://192.168.1.50:8080/healthz` должен ответить.
5. В debug-APK на экране входа нажмите «Адрес сервера» и введите `http://192.168.1.50:8080`.
6. Регистрация по SMS: код смотрите в `.\scripts\dev.ps1 otp`. Новый клиент получает 20 токенов — это одна примерка; добавить токены можно в админке сайта.
7. Для примерки нужны опубликованные товары с тремя фото: магазин создаётся в админке сайта (`.\scripts\dev.ps1 admin root`, затем `.\scripts\dev.ps1 web`), товары — в приложении магазина или кабинете на сайте.

Не держите `DEV_BIND_ADDR=0.0.0.0` в общественных сетях: dev-сервер без HTTPS.
