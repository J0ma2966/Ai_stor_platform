# web — фронтенд маркетплейса

Next.js 15 (App Router) + TypeScript, языки ru/uz через next-intl (`messages/*.json`).

**Языки и адреса.** Русский — без префикса (`/catalog`), узбекский — под `/uz` (`/uz/catalog`); старые `/ru/...`
перенаправляются на адрес без префикса. Сегмента `[locale]` в `src/app` нет: `src/middleware.ts` переписывает
`/uz/...` в `/...` и передаёт язык заголовком `x-locale`. Переключатель в шапке меняет язык на месте, без новой
страницы: введённые данные, прокрутка и вход сохраняются (подробно — `src/i18n/switch.tsx`). Выбор запоминается
в cookie `NEXT_LOCALE`. В коде ссылки пишутся без префикса через `Link`/`useRouter`/`usePathname` из
`@/i18n/navigation`, префикс добавляется сам. Тексты — только в `messages/*.json`, ключи в ru и uz должны совпадать
(проверяет `src/__tests__/i18n.test.ts`).

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
- `NEXT_PUBLIC_MEDIA_ORIGIN` может быть списком через запятую (CDN картинок товаров + хранилище приватных фото).

## Экраны заказов и доставки
| Путь | Кто | Что |
|---|---|---|
| `/checkout?items=<id>:<qty>,...` | клиент | Оформление: адрес, подъезд/этаж/квартира/домофон, геолокация, фото места, комментарии к заказу и ко времени доставки |
| `/account/orders`, `/account/orders/{id}` | клиент | Мои заказы, шкала статусов, история |
| `/store/orders`, `/store/orders/{id}` | магазин | Список с фильтрами, окно доставки, «Скопировать данные доставки», смена статуса |
| `/login`, `/store/login` | — | Вход по SMS-коду / логину магазина |
