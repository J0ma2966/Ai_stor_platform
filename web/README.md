# web — фронтенд маркетплейса

Next.js 15 (App Router) + TypeScript, языки ru/uz через next-intl (`messages/*.json`, локаль в URL: `/ru/...`, `/uz/...`).

```bash
npm ci
NEXT_PUBLIC_API_URL=http://localhost:8080 NEXT_PUBLIC_MEDIA_ORIGIN=http://localhost:9000 npm run dev
npm test          # vitest
npm run lint      # проверка типов
npm run build
```

## Соглашения
- Все запросы к API только через `src/lib/api.ts`: access-токен в памяти (не localStorage), refresh по httpOnly cookie, на `401 token_invalid` один повтор после refresh, на `token_revoked`/`refresh_invalid` выход на экран входа.
- Ошибки показываются через `ErrorBox` с `request_id`; коды ошибок переводятся в `messages/*.json` (`errors.*`).
- Деньги приходят в тийинах: форматировать через `formatSum`.
- Пользовательский текст выводить только как текст (без `dangerouslySetInnerHTML`).
- CSP с nonce на каждый запрос: `src/middleware.ts` + `src/lib/csp.ts` (скрипты только с nonce и `'strict-dynamic'`), поэтому страницы рендерятся динамически. HSTS и прочие заголовки: `next.config.ts`.
- Своего service worker у сайта нет и не должно появляться без замены `src/lib/staleWorker.ts`: этот скрипт в `<head>` снимает любые регистрации на origin (чужой SW на localhost:3000 отдавал старые чанки, отсюда «reading 'call'» на F5).
- `NEXT_PUBLIC_MEDIA_ORIGIN` может быть списком через запятую (CDN картинок товаров + хранилище приватных фото).

## Экраны заказов и доставки
| Путь | Кто | Что |
|---|---|---|
| `/checkout?items=<id>:<qty>,...` | клиент | Оформление: адрес, подъезд/этаж/квартира/домофон, геолокация, фото места, комментарии к заказу и ко времени доставки |
| `/account/orders`, `/account/orders/{id}` | клиент | Мои заказы, шкала статусов, история |
| `/store/orders`, `/store/orders/{id}` | магазин | Список с фильтрами, окно доставки, «Скопировать данные доставки», смена статуса |
| `/login`, `/store/login` | — | Вход по SMS-коду / логину магазина |
