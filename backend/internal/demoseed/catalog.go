package demoseed

// Тестовый каталог: магазины и товары, похожие на то, что реально продают бутики
// Ташкента. Фото — с Unsplash (Unsplash License: бесплатно, в том числе для
// коммерческого использования, без обязательной ссылки на автора; см. docs/DEMO_DATA.md).
// В поле Photo — идентификатор фото Unsplash (последние 11 символов адреса
// https://unsplash.com/photos/<описание>-<id>); скачивается при запуске seed.
//
// Все slug магазинов начинаются с SlugPrefix: по нему seed находит свои данные
// и удаляет их командой «api demo-data remove».

// SlugPrefix — признак тестового магазина.
const SlugPrefix = "demo-"

type Store struct {
	Slug        string
	Name        string
	Login       string
	Description string
	Address     string
	Phone       string
	Lat, Lng    float64
	Products    []Product
}

type Product struct {
	SKU         string
	Title       string
	Brand       string
	Category    string // slug категории: headwear, tops, bottoms, shoes
	Price       int64  // сум (в БД хранится в тийинах, ×100)
	Description string
	Sizes       []Size
	Set         bool   // «двойка»: верх и низ (или платье) на одном фото
	Photo       string // id фото на Unsplash
}

type Size struct {
	Size  string
	Stock int
}

var (
	jeansSizes   = []Size{{"28", 3}, {"30", 5}, {"32", 6}, {"34", 4}, {"36", 2}}
	menSizes     = []Size{{"S", 3}, {"M", 6}, {"L", 5}, {"XL", 3}, {"XXL", 1}}
	womenSizes   = []Size{{"XS", 2}, {"S", 5}, {"M", 5}, {"L", 3}}
	menShoes     = []Size{{"40", 2}, {"41", 4}, {"42", 5}, {"43", 4}, {"44", 2}, {"45", 1}}
	womenShoes   = []Size{{"36", 2}, {"37", 4}, {"38", 4}, {"39", 3}, {"40", 1}}
	unisexShoes  = []Size{{"38", 2}, {"39", 3}, {"40", 4}, {"41", 4}, {"42", 4}, {"43", 3}, {"44", 2}}
	oneSize      = []Size{{"ONE SIZE", 12}}
	suitSizes    = []Size{{"46", 2}, {"48", 4}, {"50", 4}, {"52", 3}, {"54", 1}}
	womenSuit    = []Size{{"40", 2}, {"42", 4}, {"44", 4}, {"46", 2}}
	lastPairOnly = []Size{{"S", 0}, {"M", 1}, {"L", 0}} // почти распродан: проверить «нет в наличии» по размерам
)

// Catalog — 4 магазина, 33 товара во всех зонах примерочной, 5 «двоек».
var Catalog = []Store{
	{
		Slug: "demo-denim-lab", Name: "Denim Lab", Login: "demo-denim-lab",
		Description: "Джинсы, футболки и рубашки на каждый день. Подгонка длины джинсов бесплатно.",
		Address:     "Ташкент, Мирабадский район, ул. Тараса Шевченко, 21", Phone: "+998901110101",
		Lat: 41.296944, Lng: 69.276111,
		Products: []Product{
			{SKU: "DL-501-IND", Title: "Джинсы прямого кроя 501 Original, индиго", Brand: "Levi's", Category: "bottoms", Price: 890_000,
				Description: "Классические джинсы прямого кроя из плотного денима 100% хлопок. Посадка на талии, застёжка на пуговицы. Цвет — тёмный индиго.",
				Sizes:       jeansSizes, Photo: "C012HlPoiLs"},
			{SKU: "DL-TEE-WHT", Title: "Футболка базовая из хлопка, белая", Brand: "Uniqlo", Category: "tops", Price: 149_000,
				Description: "Плотная футболка прямого силуэта с круглым вырезом. 100% хлопок, не просвечивает, держит форму после стирки.",
				Sizes:       menSizes, Photo: "8ACmRoleM24"},
			{SKU: "DL-SET-DNM", Title: "Джинсовый костюм: куртка и джинсы", Brand: "Denim Lab", Category: "tops", Price: 1_290_000, Set: true,
				Description: "Комплект из джинсовой куртки-рубашки и прямых джинсов одного оттенка. Продаётся вместе.",
				Sizes:       menSizes, Photo: "LdekusMFSsA"},
			{SKU: "DL-SHR-PLD", Title: "Рубашка в клетку из фланели", Brand: "Denim Lab", Category: "tops", Price: 329_000,
				Description: "Мягкая фланелевая рубашка в сине-коричневую клетку. Свободный крой, можно носить навыпуск или как лёгкую куртку.",
				Sizes:       menSizes, Photo: "AE-9l0oedcg"},
			{SKU: "DL-SNK-WHT", Title: "Кеды кожаные белые", Brand: "Denim Lab", Category: "shoes", Price: 690_000,
				Description: "Минималистичные кеды из гладкой кожи на плоской подошве. Подходят к джинсам и брюкам.",
				Sizes:       unisexShoes, Photo: "TByVYxFp7F8"},
			{SKU: "DL-CAP-GRY", Title: "Бейсболка серая", Brand: "Denim Lab", Category: "headwear", Price: 119_000,
				Description: "Хлопковая бейсболка с изогнутым козырьком и регулируемым ремешком.",
				Sizes:       oneSize, Photo: "2loKxdi6Hmo"},
			{SKU: "DL-HD-BLK", Title: "Худи оверсайз чёрное", Brand: "Denim Lab", Category: "tops", Price: 459_000,
				Description: "Худи свободного кроя из футера с начёсом, капюшон на кулиске, карман-кенгуру.",
				Sizes:       lastPairOnly, Photo: "f5P1emDHupU"},
		},
	},
	{
		Slug: "demo-atlas-moda", Name: "Atlas Moda", Login: "demo-atlas-moda",
		Description: "Женская одежда: платья, костюмы, блузки и обувь. Новые поступления каждую неделю.",
		Address:     "Ташкент, Юнусабадский район, ул. Амира Темура, 107", Phone: "+998901110202",
		Lat: 41.338056, Lng: 69.285833,
		Products: []Product{
			{SKU: "AM-DR-GRN", Title: "Платье-сарафан без рукавов, зелёное", Brand: "Atlas Moda", Category: "tops", Price: 590_000, Set: true,
				Description: "Лёгкое платье из вискозы длины миди, без рукавов, с пуговицами по переду. Платье в примерочной занимает и верх, и низ.",
				Sizes:       womenSizes, Photo: "VyI0GBHSsJ8"},
			{SKU: "AM-TOP-BLK", Title: "Топ с круглым вырезом, чёрный", Brand: "Mango", Category: "tops", Price: 219_000,
				Description: "Базовый облегающий топ из эластичного трикотажа. Хорошо сочетается с юбками и брюками.",
				Sizes:       womenSizes, Photo: "TMZHoaUprk8"},
			{SKU: "AM-SET-PLT", Title: "Комплект: блузка и плиссированная юбка", Brand: "Atlas Moda", Category: "tops", Price: 749_000, Set: true,
				Description: "Белая блузка на пуговицах и плиссированная юбка миди. Продаются комплектом.",
				Sizes:       womenSizes, Photo: "rctpxLpXCjQ"},
			{SKU: "AM-SK-WHT", Title: "Юбка миди белая", Brand: "Zara", Category: "bottoms", Price: 349_000,
				Description: "Юбка А-силуэта длины миди на широком поясе. Плотная ткань, есть подклад.",
				Sizes:       womenSizes, Photo: "e5I1Q-MNtCA"},
			{SKU: "AM-SUIT-WHT", Title: "Брючный костюм белый: жакет и брюки", Brand: "Atlas Moda", Category: "tops", Price: 1_590_000, Set: true,
				Description: "Однобортный жакет свободного кроя и прямые брюки со стрелками. Костюмная ткань с добавлением вискозы.",
				Sizes:       womenSuit, Photo: "C0rD3GUc85U"},
			{SKU: "AM-TR-BEI", Title: "Тренч бежевый двубортный", Brand: "Massimo Dutti", Category: "tops", Price: 1_390_000,
				Description: "Классический тренч длины миди с поясом, погонами и двубортной застёжкой. Водоотталкивающая ткань.",
				Sizes:       womenSizes, Photo: "O4uZbwuta8w"},
			{SKU: "AM-SW-WHT", Title: "Свитер крупной вязки, молочный", Brand: "Atlas Moda", Category: "tops", Price: 429_000,
				Description: "Тёплый свитер крупной вязки свободного силуэта. Шерсть с акрилом, не колется.",
				Sizes:       womenSizes, Photo: "1Pj1Yrpca4A"},
			{SKU: "AM-HL-WHT", Title: "Туфли на невысоком каблуке, белые", Brand: "Atlas Moda", Category: "shoes", Price: 690_000,
				Description: "Лодочки с острым носом на каблуке 4 см. Натуральная кожа, мягкая стелька.",
				Sizes:       womenShoes, Photo: "Mjz4YVIy69Y"},
			{SKU: "AM-BT-BLK", Title: "Ботинки кожаные чёрные", Brand: "Atlas Moda", Category: "shoes", Price: 890_000,
				Description: "Ботинки из гладкой кожи на устойчивом каблуке, боковая молния.",
				Sizes:       womenShoes, Photo: "_btfJLXnBoE"},
			{SKU: "AM-HAT-STR", Title: "Соломенная шляпа с широкими полями", Brand: "Atlas Moda", Category: "headwear", Price: 189_000,
				Description: "Лёгкая шляпа из натуральной соломы для лета и отпуска.",
				Sizes:       oneSize, Photo: "xtQGgvc1n3Y"},
		},
	},
	{
		Slug: "demo-street-code", Name: "Street Code", Login: "demo-street-code",
		Description: "Уличная мода: кроссовки, куртки, карго и аксессуары. Оригинальная продукция.",
		Address:     "Ташкент, Шайхантахурский район, ул. Навои, 30", Phone: "+998901110303",
		Lat: 41.318611, Lng: 69.255278,
		Products: []Product{
			{SKU: "SC-NK-RUN", Title: "Кроссовки беговые, красно-чёрные", Brand: "Nike", Category: "shoes", Price: 1_450_000,
				Description: "Лёгкие беговые кроссовки с дышащим сетчатым верхом и амортизирующей подошвой.",
				Sizes:       menShoes, Photo: "kjq44My3RwU"},
			{SKU: "SC-NK-HI", Title: "Кроссовки высокие, красно-белые", Brand: "Nike", Category: "shoes", Price: 1_690_000,
				Description: "Высокие кроссовки в баскетбольном стиле. Кожаный верх, резиновая подошва.",
				Sizes:       unisexShoes, Photo: "5O94e9s_-ks"},
			{SKU: "SC-SNK-WHT", Title: "Кроссовки белые низкие", Brand: "Street Code", Category: "shoes", Price: 799_000,
				Description: "Белые кроссовки на каждый день: кожаный верх, перфорация, мягкая стелька.",
				Sizes:       unisexShoes, Photo: "NUoPWImmjCU"},
			{SKU: "SC-JK-LTH", Title: "Куртка кожаная косуха", Brand: "Street Code", Category: "tops", Price: 1_890_000,
				Description: "Косуха из натуральной кожи с косой молнией и металлической фурнитурой.",
				Sizes:       menSizes, Photo: "611O0YD9UDs"},
			{SKU: "SC-CRG-KHK", Title: "Брюки карго, хаки", Brand: "Street Code", Category: "bottoms", Price: 389_000,
				Description: "Свободные брюки карго из хлопкового твила с объёмными карманами.",
				Sizes:       jeansSizes, Photo: "ZsUCBnNx5_0"},
			{SKU: "SC-BN-GRY", Title: "Шапка-бини вязаная, серая", Brand: "Street Code", Category: "headwear", Price: 99_000,
				Description: "Тёплая шапка с отворотом. Мягкая пряжа, растягивается по размеру.",
				Sizes:       oneSize, Photo: "y7NBxZuL-_w"},
			{SKU: "SC-BMB-BRN", Title: "Бомбер коричневый", Brand: "Street Code", Category: "tops", Price: 1_090_000,
				Description: "Бомбер на трикотажных манжетах, лёгкий утеплитель. Для межсезонья.",
				Sizes:       menSizes, Photo: "Fg15LdqpWrs"},
		},
	},
	{
		Slug: "demo-gentleman", Name: "Gentleman Tailoring", Login: "demo-gentleman",
		Description: "Мужская классика: костюмы, рубашки, обувь из кожи. Подгонка по фигуре за 2 дня.",
		Address:     "Ташкент, Яккасарайский район, ул. Бабура, 45", Phone: "+998901110404",
		Lat: 41.287222, Lng: 69.256944,
		Products: []Product{
			{SKU: "GT-SUIT-BLK", Title: "Костюм классический чёрный: пиджак и брюки", Brand: "Gentleman Tailoring", Category: "tops", Price: 2_390_000, Set: true,
				Description: "Двойка из шерстяной костюмной ткани: однобортный пиджак на двух пуговицах и брюки прямого кроя.",
				Sizes:       suitSizes, Photo: "0-INtpAVg70"},
			{SKU: "GT-BLZ-BLK", Title: "Пиджак однобортный чёрный", Brand: "Hugo Boss", Category: "tops", Price: 1_490_000,
				Description: "Пиджак приталенного кроя из шерсти. Два шлицы, внутренние карманы.",
				Sizes:       suitSizes, Photo: "VsCBt8j-qg4"},
			{SKU: "GT-SHR-WHT", Title: "Рубашка классическая белая", Brand: "Gentleman Tailoring", Category: "tops", Price: 349_000,
				Description: "Рубашка из хлопкового поплина, воротник кент, манжеты на пуговицах. Легко гладится.",
				Sizes:       menSizes, Photo: "e8sbQNYEwX8"},
			{SKU: "GT-SW-BRN", Title: "Свитер вязаный коричневый", Brand: "Gentleman Tailoring", Category: "tops", Price: 529_000,
				Description: "Свитер из мериносовой шерсти с круглым вырезом. Тёплый и тонкий — под пиджак.",
				Sizes:       menSizes, Photo: "IpJCzR5r7w8"},
			{SKU: "GT-CH-BEI", Title: "Брюки чинос бежевые", Brand: "Gentleman Tailoring", Category: "bottoms", Price: 459_000,
				Description: "Чинос зауженного кроя из хлопка с эластаном. К рубашке и к пиджаку.",
				Sizes:       jeansSizes, Photo: "DjniaF8q-HI"},
			{SKU: "GT-DRB-BRN", Title: "Туфли дерби коричневые", Brand: "Gentleman Tailoring", Category: "shoes", Price: 990_000,
				Description: "Дерби из гладкой кожи ручной окраски, кожаная подошва.",
				Sizes:       menShoes, Photo: "ZzdIzsclJuI"},
			{SKU: "GT-OXF-BRN", Title: "Туфли оксфорды коричневые", Brand: "Gentleman Tailoring", Category: "shoes", Price: 1_190_000,
				Description: "Классические оксфорды с закрытой шнуровкой. В комплекте коробка и пыльник.",
				Sizes:       menShoes, Photo: "NfZiOJzZgcg"},
			{SKU: "GT-BT-BRN", Title: "Ботинки кожаные на шнуровке", Brand: "Gentleman Tailoring", Category: "shoes", Price: 1_290_000,
				Description: "Ботинки из вощёной кожи на рифлёной подошве. Для осени и зимы.",
				Sizes:       menShoes, Photo: "WlB8TsI_th0"},
			{SKU: "GT-BN-GRY", Title: "Шапка вязаная, серый меланж", Brand: "Gentleman Tailoring", Category: "headwear", Price: 149_000,
				Description: "Шапка из шерсти мериноса тонкой вязки.",
				Sizes:       oneSize, Photo: "NZ83iHvAFIk"},
		},
	},
}
