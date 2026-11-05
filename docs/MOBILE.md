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
