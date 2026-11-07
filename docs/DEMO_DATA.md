# Тестовые магазины и товары

Команда `api demo-data` заполняет платформу витриной для проверки: **4 магазина и 37 товаров**
во всех зонах примерочной (головные уборы, верх, низ, обувь), из них 8 «двоек» (костюмы,
комплекты и платья — верх и низ на одном фото). Покупателей команда не создаёт.

Что получается:

- магазины сразу **активны**, у каждого есть адрес на карте Ташкента, телефон и обложка;
- у каждого магазина есть учётная запись для кабинета (логин ниже, пароль задаёте вы при запуске) —
  без неё магазином нельзя управлять: в системе один аккаунт на магазин;
- товары **опубликованы**, с настоящими фото, размерами и остатками; у худи `DL-HD-BLK` остаток
  только у одного размера — удобно проверять «нет в наличии» по размерам;
- цены в сумах, похожие на цены бутиков Ташкента; часть товаров — модели известных брендов
  (Levi's, Nike, Adidas), остальные — собственные марки магазинов.

## Как запустить

Фото лежат в репозитории (`backend/internal/demoseed/photos/`) и встроены в программу:
интернет команде не нужен. После `git pull` пересоберите контейнер API (`.\scripts\dev.ps1 up`
делает это сам), иначе в нём останется старая версия команды без фото.
Пароль для кабинетов магазинов — минимум 10 символов.

**Windows (PowerShell):**

```powershell
.\scripts\dev.ps1 up            # поднять окружение (пересобирает API)
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

- Запускать можно сколько угодно раз: ничего не дублируется. Повторный запуск приводит тестовые
  магазины к каталогу:
  - ставит фото товарам без фото и публикует их (черновики от первой версии команды);
  - **заменяет картинку, если она отличается от фото в папке** — так нарисованные картинки
    прошлой версии команды становятся настоящими фото; старые файлы удаляются;
  - обновляет названия, описания, цены, размеры и остатки товаров и описания магазинов;
  - удаляет товары прошлых версий каталога (список `Retired` в `catalog.go`).
- Повторный запуск не меняет пароли учётных записей магазинов, не публикует товары, снятые
  с публикации вручную, и не трогает товары, которые вы добавили в тестовый магазин сами.
  Но фото и описания товаров из каталога он вернёт к исходным, если вы их меняли.
- `demo-data remove` удаляет магазины из этого списка (slug начинается с `demo-`) вместе с товарами,
  аккаунтами магазинов и файлами. Если по магазину уже есть **заказы**, его нельзя удалить
  (заказы — учётные документы): такой магазин архивируется, его товары снимаются с публикации.
- Данные описаны в `backend/internal/demoseed/catalog.go`. Чтобы добавить товар, допишите его туда
  и положите фото в `backend/internal/demoseed/photos/<артикул в нижнем регистре>.jpg`
  (портрет 3:4, не меньше 380 px по ширине, лучше от 768 px) с источником в `photos/NOTICE.md`;
  тест `TestCatalogIsValid` проверит, что фото есть и подходит. Убранный из каталога артикул допишите в `Retired`.

## Фото и лицензия

Фото взяты из демо-данных открытых интернет-магазинов [Bagisto](https://github.com/bagisto/bagisto)
и [Sylius](https://github.com/Sylius/Sylius) (лицензия MIT), [Saleor](https://github.com/saleor/saleor)
и [Solidus](https://github.com/solidusio/solidus) (BSD-3-Clause). Эти лицензии разрешают копировать
и изменять файлы при сохранении уведомления об авторских правах; источник каждого файла и тексты
лицензий — в [photos/NOTICE.md](../backend/internal/demoseed/photos/NOTICE.md). Фото Bagisto и
Sylius — сгенерированные изображения; логотипы на некоторых фото принадлежат их владельцам.
Это тестовые данные: перед запуском платформы для покупателей тестовые магазины нужно удалить
(`demo-data remove`).

Для примерки подходят все фото: на каждом хорошо видна вещь своей зоны. На фото «двоек» видны
и верх, и низ; на фото обуви — пара целиком.

## Магазины и товары

### Denim Lab — логин `demo-denim-lab`

Джинсы, футболки и худи на каждый день. Подгонка длины джинсов бесплатно.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| DL-501-IND | Джинсы 501 Original прямого кроя, синие | Levi's | Низ | 890 000 | [dl-501-ind.jpg](../backend/internal/demoseed/photos/dl-501-ind.jpg) |
| DL-JNS-GRY | Джинсы slim fit, серые с потёртостями | Denim Lab | Низ | 690 000 | [dl-jns-gry.jpg](../backend/internal/demoseed/photos/dl-jns-gry.jpg) |
| DL-TEE-WHT | Футболка базовая из хлопка, белая | Uniqlo | Верх | 149 000 | [dl-tee-wht.jpg](../backend/internal/demoseed/photos/dl-tee-wht.jpg) |
| DL-TEE-BLK | Футболка базовая из хлопка, чёрная | Uniqlo | Верх | 149 000 | [dl-tee-blk.jpg](../backend/internal/demoseed/photos/dl-tee-blk.jpg) |
| DL-SET-DNM | Джинсовый костюм: куртка и джинсы (двойка) | Denim Lab | Верх | 1 290 000 | [dl-set-dnm.jpg](../backend/internal/demoseed/photos/dl-set-dnm.jpg) |
| DL-SNK-WHT | Кеды кожаные белые | Denim Lab | Обувь | 690 000 | [dl-snk-wht.jpg](../backend/internal/demoseed/photos/dl-snk-wht.jpg) |
| DL-CAP-GRY | Бейсболка серая с прямым козырьком | Denim Lab | Головные уборы | 119 000 | [dl-cap-gry.jpg](../backend/internal/demoseed/photos/dl-cap-gry.jpg) |
| DL-HD-BLK | Худи чёрное с капюшоном | Denim Lab | Верх | 459 000 | [dl-hd-blk.jpg](../backend/internal/demoseed/photos/dl-hd-blk.jpg) |

### Atlas Moda — логин `demo-atlas-moda`

Женская одежда: платья, комплекты, джинсы и обувь. Новые поступления каждую неделю.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| AM-SET-SKT | Комплект: кроп-топ и мини-юбка с орнаментом (двойка) | Atlas Moda | Верх | 649 000 | [am-set-skt.jpg](../backend/internal/demoseed/photos/am-set-skt.jpg) |
| AM-SET-PLT | Комплект: блузка и юбка-клёш (двойка) | Atlas Moda | Верх | 749 000 | [am-set-plt.jpg](../backend/internal/demoseed/photos/am-set-plt.jpg) |
| AM-SET-SPT | Спортивный комплект: топ и шорты (двойка) | Atlas Moda | Верх | 389 000 | [am-set-spt.jpg](../backend/internal/demoseed/photos/am-set-spt.jpg) |
| AM-DR-WHT | Сарафан на бретелях, молочный (двойка) | Atlas Moda | Верх | 590 000 | [am-dr-wht.jpg](../backend/internal/demoseed/photos/am-dr-wht.jpg) |
| AM-DR-BLK | Платье-комбинация мини, чёрное (двойка) | Mango | Верх | 459 000 | [am-dr-blk.jpg](../backend/internal/demoseed/photos/am-dr-blk.jpg) |
| AM-DR-FLR | Платье макси с цветочным принтом (двойка) | Atlas Moda | Верх | 690 000 | [am-dr-flr.jpg](../backend/internal/demoseed/photos/am-dr-flr.jpg) |
| AM-TEE-CRL | Футболка с принтом, коралловая | Atlas Moda | Верх | 179 000 | [am-tee-crl.jpg](../backend/internal/demoseed/photos/am-tee-crl.jpg) |
| AM-TOP-BLK | Футболка приталенная, чёрная | Mango | Верх | 219 000 | [am-top-blk.jpg](../backend/internal/demoseed/photos/am-top-blk.jpg) |
| AM-JNS-SKN | Джинсы скинни с высокой посадкой | Zara | Низ | 549 000 | [am-jns-skn.jpg](../backend/internal/demoseed/photos/am-jns-skn.jpg) |
| AM-HL-FLR | Туфли-лодочки с цветочным принтом и бантом | Atlas Moda | Обувь | 690 000 | [am-hl-flr.jpg](../backend/internal/demoseed/photos/am-hl-flr.jpg) |
| AM-RUN-NVY | Кроссовки беговые женские, тёмно-синие | Adidas | Обувь | 990 000 | [am-run-nvy.jpg](../backend/internal/demoseed/photos/am-run-nvy.jpg) |
| AM-BN-WHT | Шапка вязаная с помпоном, молочная | Atlas Moda | Головные уборы | 159 000 | [am-bn-wht.jpg](../backend/internal/demoseed/photos/am-bn-wht.jpg) |

### Street Code — логин `demo-street-code`

Уличная мода: кроссовки, худи, куртки и шапки. Оригинальная продукция.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| SC-NK-HI | Кроссовки высокие, бело-красные | Nike | Обувь | 1 690 000 | [sc-nk-hi.jpg](../backend/internal/demoseed/photos/sc-nk-hi.jpg) |
| SC-RUN-MLT | Кроссовки беговые, разноцветные | Street Code | Обувь | 899 000 | [sc-run-mlt.jpg](../backend/internal/demoseed/photos/sc-run-mlt.jpg) |
| SC-RUN-NVY | Кроссовки беговые, тёмно-синие | Adidas | Обувь | 1 190 000 | [sc-run-nvy.jpg](../backend/internal/demoseed/photos/sc-run-nvy.jpg) |
| SC-JK-LTH | Куртка кожаная коричневая | Street Code | Верх | 1 890 000 | [sc-jk-lth.jpg](../backend/internal/demoseed/photos/sc-jk-lth.jpg) |
| SC-ZIP-NVY | Толстовка на молнии с капюшоном, тёмно-синяя | Street Code | Верх | 429 000 | [sc-zip-nvy.jpg](../backend/internal/demoseed/photos/sc-zip-nvy.jpg) |
| SC-HD-RED | Худи красное с капюшоном | Street Code | Верх | 459 000 | [sc-hd-red.jpg](../backend/internal/demoseed/photos/sc-hd-red.jpg) |
| SC-TEE-TIE | Футболка тай-дай, песочно-оранжевая | Street Code | Верх | 199 000 | [sc-tee-tie.jpg](../backend/internal/demoseed/photos/sc-tee-tie.jpg) |
| SC-JOG-BLK | Джоггеры чёрные с манжетами | Street Code | Низ | 389 000 | [sc-jog-blk.jpg](../backend/internal/demoseed/photos/sc-jog-blk.jpg) |
| SC-CAP-RED | Бейсболка красная с прямым козырьком | Street Code | Головные уборы | 119 000 | [sc-cap-red.jpg](../backend/internal/demoseed/photos/sc-cap-red.jpg) |
| SC-BN-GRY | Шапка-бини вязаная, серая | Street Code | Головные уборы | 99 000 | [sc-bn-gry.jpg](../backend/internal/demoseed/photos/sc-bn-gry.jpg) |
| SC-BN-STR | Шапка в полоску с помпоном | Street Code | Головные уборы | 129 000 | [sc-bn-str.jpg](../backend/internal/demoseed/photos/sc-bn-str.jpg) |

### Gentleman Tailoring — логин `demo-gentleman`

Мужская классика и smart casual: костюмы, пиджаки, поло и трикотаж. Подгонка по фигуре за 2 дня.

| Артикул | Товар | Бренд | Зона | Цена, сум | Фото |
|---|---|---|---|---|---|
| GT-SUIT-NVY | Костюм тёмно-синий: пиджак и брюки (двойка) | Gentleman Tailoring | Верх | 2 390 000 | [gt-suit-nvy.jpg](../backend/internal/demoseed/photos/gt-suit-nvy.jpg) |
| GT-BLZ-GRY | Пиджак однобортный, графитовый | Hugo Boss | Верх | 1 490 000 | [gt-blz-gry.jpg](../backend/internal/demoseed/photos/gt-blz-gry.jpg) |
| GT-HNL-BLU | Лонгслив-хенли, синий | Gentleman Tailoring | Верх | 329 000 | [gt-hnl-blu.jpg](../backend/internal/demoseed/photos/gt-hnl-blu.jpg) |
| GT-POLO-NVY | Поло из хлопка пике, тёмно-синее | Gentleman Tailoring | Верх | 279 000 | [gt-polo-nvy.jpg](../backend/internal/demoseed/photos/gt-polo-nvy.jpg) |
| GT-TEE-GRY | Футболка из хлопка, серый меланж | Gentleman Tailoring | Верх | 169 000 | [gt-tee-gry.jpg](../backend/internal/demoseed/photos/gt-tee-gry.jpg) |
| GT-BN-GRY | Шапка вязаная, серый меланж | Gentleman Tailoring | Головные уборы | 149 000 | [gt-bn-gry.jpg](../backend/internal/demoseed/photos/gt-bn-gry.jpg) |
