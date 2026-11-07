# Тестовые магазины и товары

Команда `api demo-data` заполняет платформу витриной для проверки: **4 магазина и 33 товара**
во всех зонах примерочной (головные уборы, верх, низ, обувь), из них 5 «двоек» (костюмы,
комплекты и платье — верх и низ на одном фото). Покупателей команда не создаёт.

Что получается:

- магазины сразу **активны**, у каждого есть адрес на карте Ташкента, телефон и обложка;
- у каждого магазина есть учётная запись для кабинета (логин ниже, пароль задаёте вы при запуске) —
  без неё магазином нельзя управлять: в системе один аккаунт на магазин;
- товары **опубликованы**, с размерами и остатками; у худи `DL-HD-BLK` остаток только у одного
  размера — удобно проверять «нет в наличии» по размерам;
- цены в сумах, похожие на цены бутиков Ташкента; часть товаров — модели известных брендов
  (Levi's 501, Nike), остальные — собственные марки магазинов.

## Как запустить

Фото скачиваются с Unsplash во время запуска, поэтому нужен интернет. Пароль для кабинетов
магазинов — минимум 10 символов.

**Windows (PowerShell)**, окружение уже поднято командой `.\scripts\dev.ps1 up`:

```powershell
.\scripts\dev.ps1 demo          # спросит пароль для кабинетов магазинов
.\scripts\dev.ps1 demo remove   # удалить всё тестовое
```

**Linux / macOS:**

```bash
DEMO_STORE_PASSWORD='придумайте-пароль' make demo
make demo-remove
```

**Сервер (staging / prod)** — по умолчанию команда отказывается работать при `APP_ENV=prod`,
чтобы тестовые товары случайно не попали к покупателям. Если это действительно нужно
(например, проверка до запуска), добавьте флаг `--allow-prod`:

```bash
docker compose exec -e DEMO_STORE_PASSWORD='...' api /api demo-data --allow-prod
docker compose exec api /api demo-data remove --allow-prod
```

После запуска войдите в кабинет магазина на `/store/login` с логином из таблицы ниже и вашим паролем.

## Повторный запуск и удаление

- Запускать можно сколько угодно раз: существующие магазины, аккаунты (и их пароли) и товары
  не меняются и не дублируются. Если какое-то фото не скачалось, товар остаётся **черновиком**,
  а команда в конце пишет его артикул — запустите её ещё раз, и фото докачается, товар опубликуется.
- `demo-data remove` удаляет магазины из этого списка (slug начинается с `demo-`) вместе с товарами,
  аккаунтами магазинов и файлами. Если по магазину уже есть **заказы**, его нельзя удалить
  (заказы — учётные документы): такой магазин архивируется, его товары снимаются с публикации.
- Данные описаны в `backend/internal/demoseed/catalog.go`: чтобы добавить товар, допишите его туда
  (фото — идентификатор с Unsplash, последние 11 символов адреса страницы фото).

## Фото и лицензия

Все фото — с [Unsplash](https://unsplash.com) по [лицензии Unsplash](https://unsplash.com/license):
бесплатно, в том числе в коммерческих целях, ссылка на автора не обязательна. Фото не изображают
конкретный товар магазина один в один — это тестовые данные. Перед запуском платформы для
покупателей тестовые магазины нужно удалить (`demo-data remove`).

## Магазины и товары

### Denim Lab — логин `demo-denim-lab`

Джинсы, футболки и рубашки на каждый день. Подгонка длины джинсов бесплатно.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| DL-501-IND | Джинсы прямого кроя 501 Original, индиго | Levi's | Низ | 890 000 | [C012HlPoiLs](https://unsplash.com/photos/C012HlPoiLs) |
| DL-TEE-WHT | Футболка базовая из хлопка, белая | Uniqlo | Верх | 149 000 | [8ACmRoleM24](https://unsplash.com/photos/8ACmRoleM24) |
| DL-SET-DNM | Джинсовый костюм: куртка и джинсы (двойка) | Denim Lab | Верх | 1 290 000 | [LdekusMFSsA](https://unsplash.com/photos/LdekusMFSsA) |
| DL-SHR-PLD | Рубашка в клетку из фланели | Denim Lab | Верх | 329 000 | [AE-9l0oedcg](https://unsplash.com/photos/AE-9l0oedcg) |
| DL-SNK-WHT | Кеды кожаные белые | Denim Lab | Обувь | 690 000 | [TByVYxFp7F8](https://unsplash.com/photos/TByVYxFp7F8) |
| DL-CAP-GRY | Бейсболка серая | Denim Lab | Головные уборы | 119 000 | [2loKxdi6Hmo](https://unsplash.com/photos/2loKxdi6Hmo) |
| DL-HD-BLK | Худи оверсайз чёрное | Denim Lab | Верх | 459 000 | [f5P1emDHupU](https://unsplash.com/photos/f5P1emDHupU) |

### Atlas Moda — логин `demo-atlas-moda`

Женская одежда: платья, костюмы, блузки и обувь. Новые поступления каждую неделю.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| AM-DR-GRN | Платье-сарафан без рукавов, зелёное (двойка) | Atlas Moda | Верх | 590 000 | [VyI0GBHSsJ8](https://unsplash.com/photos/VyI0GBHSsJ8) |
| AM-TOP-BLK | Топ с круглым вырезом, чёрный | Mango | Верх | 219 000 | [TMZHoaUprk8](https://unsplash.com/photos/TMZHoaUprk8) |
| AM-SET-PLT | Комплект: блузка и плиссированная юбка (двойка) | Atlas Moda | Верх | 749 000 | [rctpxLpXCjQ](https://unsplash.com/photos/rctpxLpXCjQ) |
| AM-SK-WHT | Юбка миди белая | Zara | Низ | 349 000 | [e5I1Q-MNtCA](https://unsplash.com/photos/e5I1Q-MNtCA) |
| AM-SUIT-WHT | Брючный костюм белый: жакет и брюки (двойка) | Atlas Moda | Верх | 1 590 000 | [C0rD3GUc85U](https://unsplash.com/photos/C0rD3GUc85U) |
| AM-TR-BEI | Тренч бежевый двубортный | Massimo Dutti | Верх | 1 390 000 | [O4uZbwuta8w](https://unsplash.com/photos/O4uZbwuta8w) |
| AM-SW-WHT | Свитер крупной вязки, молочный | Atlas Moda | Верх | 429 000 | [1Pj1Yrpca4A](https://unsplash.com/photos/1Pj1Yrpca4A) |
| AM-HL-WHT | Туфли на невысоком каблуке, белые | Atlas Moda | Обувь | 690 000 | [Mjz4YVIy69Y](https://unsplash.com/photos/Mjz4YVIy69Y) |
| AM-BT-BLK | Ботинки кожаные чёрные | Atlas Moda | Обувь | 890 000 | [_btfJLXnBoE](https://unsplash.com/photos/_btfJLXnBoE) |
| AM-HAT-STR | Соломенная шляпа с широкими полями | Atlas Moda | Головные уборы | 189 000 | [xtQGgvc1n3Y](https://unsplash.com/photos/xtQGgvc1n3Y) |

### Street Code — логин `demo-street-code`

Уличная мода: кроссовки, куртки, карго и аксессуары. Оригинальная продукция.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| SC-NK-RUN | Кроссовки беговые, красно-чёрные | Nike | Обувь | 1 450 000 | [kjq44My3RwU](https://unsplash.com/photos/kjq44My3RwU) |
| SC-NK-HI | Кроссовки высокие, красно-белые | Nike | Обувь | 1 690 000 | [5O94e9s_-ks](https://unsplash.com/photos/5O94e9s_-ks) |
| SC-SNK-WHT | Кроссовки белые низкие | Street Code | Обувь | 799 000 | [NUoPWImmjCU](https://unsplash.com/photos/NUoPWImmjCU) |
| SC-JK-LTH | Куртка кожаная косуха | Street Code | Верх | 1 890 000 | [611O0YD9UDs](https://unsplash.com/photos/611O0YD9UDs) |
| SC-CRG-KHK | Брюки карго, хаки | Street Code | Низ | 389 000 | [ZsUCBnNx5_0](https://unsplash.com/photos/ZsUCBnNx5_0) |
| SC-BN-GRY | Шапка-бини вязаная, серая | Street Code | Головные уборы | 99 000 | [y7NBxZuL-_w](https://unsplash.com/photos/y7NBxZuL-_w) |
| SC-BMB-BRN | Бомбер коричневый | Street Code | Верх | 1 090 000 | [Fg15LdqpWrs](https://unsplash.com/photos/Fg15LdqpWrs) |

### Gentleman Tailoring — логин `demo-gentleman`

Мужская классика: костюмы, рубашки, обувь из кожи. Подгонка по фигуре за 2 дня.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| GT-SUIT-BLK | Костюм классический чёрный: пиджак и брюки (двойка) | Gentleman Tailoring | Верх | 2 390 000 | [0-INtpAVg70](https://unsplash.com/photos/0-INtpAVg70) |
| GT-BLZ-BLK | Пиджак однобортный чёрный | Hugo Boss | Верх | 1 490 000 | [VsCBt8j-qg4](https://unsplash.com/photos/VsCBt8j-qg4) |
| GT-SHR-WHT | Рубашка классическая белая | Gentleman Tailoring | Верх | 349 000 | [e8sbQNYEwX8](https://unsplash.com/photos/e8sbQNYEwX8) |
| GT-SW-BRN | Свитер вязаный коричневый | Gentleman Tailoring | Верх | 529 000 | [IpJCzR5r7w8](https://unsplash.com/photos/IpJCzR5r7w8) |
| GT-CH-BEI | Брюки чинос бежевые | Gentleman Tailoring | Низ | 459 000 | [DjniaF8q-HI](https://unsplash.com/photos/DjniaF8q-HI) |
| GT-DRB-BRN | Туфли дерби коричневые | Gentleman Tailoring | Обувь | 990 000 | [ZzdIzsclJuI](https://unsplash.com/photos/ZzdIzsclJuI) |
| GT-OXF-BRN | Туфли оксфорды коричневые | Gentleman Tailoring | Обувь | 1 190 000 | [NfZiOJzZgcg](https://unsplash.com/photos/NfZiOJzZgcg) |
| GT-BT-BRN | Ботинки кожаные на шнуровке | Gentleman Tailoring | Обувь | 1 290 000 | [WlB8TsI_th0](https://unsplash.com/photos/WlB8TsI_th0) |
| GT-BN-GRY | Шапка вязаная, серый меланж | Gentleman Tailoring | Головные уборы | 149 000 | [NZ83iHvAFIk](https://unsplash.com/photos/NZ83iHvAFIk) |
