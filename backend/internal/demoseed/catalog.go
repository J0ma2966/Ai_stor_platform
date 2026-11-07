package demoseed

// Тестовый каталог: магазины и товары, похожие на то, что реально продают бутики
// Ташкента. Фото каждого товара лежит в папке photos/ под именем «<артикул в нижнем
// регистре>.jpg» и встроено в программу: интернет при запуске не нужен. Откуда
// взяты фото и под какой лицензией — photos/NOTICE.md.
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
	Set         bool // «двойка»: верх и низ (или платье) на одном фото
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
	womenJeans   = []Size{{"25", 2}, {"26", 4}, {"27", 5}, {"28", 4}, {"29", 2}}
	lastPairOnly = []Size{{"S", 0}, {"M", 1}, {"L", 0}} // почти распродан: проверить «нет в наличии» по размерам
)

// Catalog — 4 магазина, 37 товаров во всех зонах примерочной, 8 «двоек» (комплекты и платья).
var Catalog = []Store{
	{
		Slug: "demo-denim-lab", Name: "Denim Lab", Login: "demo-denim-lab",
		Description: "Джинсы, футболки и худи на каждый день. Подгонка длины джинсов бесплатно.",
		Address:     "Ташкент, Мирабадский район, ул. Тараса Шевченко, 21", Phone: "+998901110101",
		Lat: 41.296944, Lng: 69.276111,
		Products: []Product{
			{SKU: "DL-501-IND", Title: "Джинсы 501 Original прямого кроя, синие", Brand: "Levi's", Category: "bottoms", Price: 890_000,
				Description: "Классические джинсы прямого кроя из плотного денима, 100% хлопок. Посадка на талии, застёжка на пуговицы, лёгкие потёртости.",
				Sizes:       jeansSizes},
			{SKU: "DL-JNS-GRY", Title: "Джинсы slim fit, серые с потёртостями", Brand: "Denim Lab", Category: "bottoms", Price: 690_000,
				Description: "Зауженные джинсы из эластичного денима с эффектом выбеленности. Средняя посадка, пять карманов.",
				Sizes:       jeansSizes},
			{SKU: "DL-TEE-WHT", Title: "Футболка базовая из хлопка, белая", Brand: "Uniqlo", Category: "tops", Price: 149_000,
				Description: "Плотная футболка прямого силуэта с круглым вырезом. 100% хлопок, не просвечивает, держит форму после стирки.",
				Sizes:       menSizes},
			{SKU: "DL-TEE-BLK", Title: "Футболка базовая из хлопка, чёрная", Brand: "Uniqlo", Category: "tops", Price: 149_000,
				Description: "Плотная футболка прямого силуэта с круглым вырезом. 100% хлопок, цвет не вымывается после стирки.",
				Sizes:       menSizes},
			{SKU: "DL-SET-DNM", Title: "Джинсовый костюм: куртка и джинсы", Brand: "Denim Lab", Category: "tops", Price: 1_290_000, Set: true,
				Description: "Джинсовая куртка и прямые джинсы светлой варки из одного денима. Продаются комплектом.",
				Sizes:       womenSizes},
			{SKU: "DL-SNK-WHT", Title: "Кеды кожаные белые", Brand: "Denim Lab", Category: "shoes", Price: 690_000,
				Description: "Минималистичные кеды из гладкой кожи на плоской подошве. Подходят к джинсам и брюкам.",
				Sizes:       unisexShoes},
			{SKU: "DL-CAP-GRY", Title: "Бейсболка серая с прямым козырьком", Brand: "Denim Lab", Category: "headwear", Price: 119_000,
				Description: "Снепбек из плотного хлопка с вышитым логотипом и регулируемой застёжкой сзади.",
				Sizes:       oneSize},
			{SKU: "DL-HD-BLK", Title: "Худи чёрное с капюшоном", Brand: "Denim Lab", Category: "tops", Price: 459_000,
				Description: "Худи свободного кроя из футера с начёсом, капюшон на кулиске, карман-кенгуру.",
				Sizes:       lastPairOnly},
		},
	},
	{
		Slug: "demo-atlas-moda", Name: "Atlas Moda", Login: "demo-atlas-moda",
		Description: "Женская одежда: платья, комплекты, джинсы и обувь. Новые поступления каждую неделю.",
		Address:     "Ташкент, Юнусабадский район, ул. Амира Темура, 107", Phone: "+998901110202",
		Lat: 41.338056, Lng: 69.285833,
		Products: []Product{
			{SKU: "AM-SET-SKT", Title: "Комплект: кроп-топ и мини-юбка с орнаментом", Brand: "Atlas Moda", Category: "tops", Price: 649_000, Set: true,
				Description: "Белый кроп-топ без рукавов и расклешённая мини-юбка с этническим орнаментом на широком поясе. Продаются комплектом.",
				Sizes:       womenSizes},
			{SKU: "AM-SET-PLT", Title: "Комплект: блузка и юбка-клёш", Brand: "Atlas Moda", Category: "tops", Price: 749_000, Set: true,
				Description: "Свободная белая блузка с V-вырезом и юбка-клёш выше колена с серо-голубым принтом. Продаются комплектом.",
				Sizes:       womenSizes},
			{SKU: "AM-SET-SPT", Title: "Спортивный комплект: топ и шорты", Brand: "Atlas Moda", Category: "tops", Price: 389_000, Set: true,
				Description: "Топ-бра и шорты из эластичной ткани с блеском и контрастной розовой окантовкой. Для фитнеса и бега.",
				Sizes:       womenSizes},
			{SKU: "AM-DR-WHT", Title: "Сарафан на бретелях, молочный", Brand: "Atlas Moda", Category: "tops", Price: 590_000, Set: true,
				Description: "Лёгкое платье миди из вискозы с V-вырезом и завышенной талией, тонкие бретели.",
				Sizes:       womenSizes},
			{SKU: "AM-DR-BLK", Title: "Платье-комбинация мини, чёрное", Brand: "Mango", Category: "tops", Price: 459_000, Set: true,
				Description: "Короткое платье на тонких бретелях из струящейся ткани. Подходит и для вечера, и для пляжа.",
				Sizes:       womenSizes},
			{SKU: "AM-DR-FLR", Title: "Платье макси с цветочным принтом", Brand: "Atlas Moda", Category: "tops", Price: 690_000, Set: true,
				Description: "Длинное летнее платье из шифона с цветочным принтом, на тонких бретелях.",
				Sizes:       womenSizes},
			{SKU: "AM-TEE-CRL", Title: "Футболка с принтом, коралловая", Brand: "Atlas Moda", Category: "tops", Price: 179_000,
				Description: "Свободная хлопковая футболка с графичным принтом на груди.",
				Sizes:       womenSizes},
			{SKU: "AM-TOP-BLK", Title: "Футболка приталенная, чёрная", Brand: "Mango", Category: "tops", Price: 219_000,
				Description: "Базовая облегающая футболка из эластичного трикотажа. Хорошо сочетается с джинсами и юбками.",
				Sizes:       womenSizes},
			{SKU: "AM-JNS-SKN", Title: "Джинсы скинни с высокой посадкой", Brand: "Zara", Category: "bottoms", Price: 549_000,
				Description: "Облегающие джинсы из эластичного денима насыщенного синего цвета, подвороты по низу.",
				Sizes:       womenJeans},
			{SKU: "AM-HL-FLR", Title: "Туфли-лодочки с цветочным принтом и бантом", Brand: "Atlas Moda", Category: "shoes", Price: 690_000,
				Description: "Остроносые туфли на тонком каблуке 7 см, чёрный текстиль с цветочным принтом и объёмный бант.",
				Sizes:       womenShoes},
			{SKU: "AM-RUN-NVY", Title: "Кроссовки беговые женские, тёмно-синие", Brand: "Adidas", Category: "shoes", Price: 990_000,
				Description: "Лёгкие беговые кроссовки с дышащим трикотажным верхом и амортизирующей подошвой.",
				Sizes:       womenShoes},
			{SKU: "AM-BN-WHT", Title: "Шапка вязаная с помпоном, молочная", Brand: "Atlas Moda", Category: "headwear", Price: 159_000,
				Description: "Тёплая шапка крупной вязки с помпоном из искусственного меха.",
				Sizes:       oneSize},
		},
	},
	{
		Slug: "demo-street-code", Name: "Street Code", Login: "demo-street-code",
		Description: "Уличная мода: кроссовки, худи, куртки и шапки. Оригинальная продукция.",
		Address:     "Ташкент, Шайхантахурский район, ул. Навои, 30", Phone: "+998901110303",
		Lat: 41.318611, Lng: 69.255278,
		Products: []Product{
			{SKU: "SC-NK-HI", Title: "Кроссовки высокие, бело-красные", Brand: "Nike", Category: "shoes", Price: 1_690_000,
				Description: "Высокие кроссовки в баскетбольном стиле. Кожаный верх, резиновая подошва.",
				Sizes:       unisexShoes},
			{SKU: "SC-RUN-MLT", Title: "Кроссовки беговые, разноцветные", Brand: "Street Code", Category: "shoes", Price: 899_000,
				Description: "Беговые кроссовки с сетчатым верхом в бело-сине-красной расцветке и мягкой подошвой.",
				Sizes:       menShoes},
			{SKU: "SC-RUN-NVY", Title: "Кроссовки беговые, тёмно-синие", Brand: "Adidas", Category: "shoes", Price: 1_190_000,
				Description: "Лёгкие беговые кроссовки с дышащим верхом и амортизирующей подошвой.",
				Sizes:       menShoes},
			{SKU: "SC-JK-LTH", Title: "Куртка кожаная коричневая", Brand: "Street Code", Category: "tops", Price: 1_890_000,
				Description: "Куртка из натуральной кожи с отложным воротником, молнией и карманами на молнии.",
				Sizes:       menSizes},
			{SKU: "SC-ZIP-NVY", Title: "Толстовка на молнии с капюшоном, тёмно-синяя", Brand: "Street Code", Category: "tops", Price: 429_000,
				Description: "Толстовка из футера на молнии, капюшон на кулиске, боковые карманы.",
				Sizes:       menSizes},
			{SKU: "SC-HD-RED", Title: "Худи красное с капюшоном", Brand: "Street Code", Category: "tops", Price: 459_000,
				Description: "Худи из плотного хлопка с начёсом, карман-кенгуру и логотип на груди.",
				Sizes:       menSizes},
			{SKU: "SC-TEE-TIE", Title: "Футболка тай-дай, песочно-оранжевая", Brand: "Street Code", Category: "tops", Price: 199_000,
				Description: "Свободная футболка из хлопка, окрашенная вручную в технике тай-дай.",
				Sizes:       menSizes},
			{SKU: "SC-JOG-BLK", Title: "Джоггеры чёрные с манжетами", Brand: "Street Code", Category: "bottoms", Price: 389_000,
				Description: "Спортивные брюки из футера на резинке, манжеты по низу, боковые карманы.",
				Sizes:       menSizes},
			{SKU: "SC-CAP-RED", Title: "Бейсболка красная с прямым козырьком", Brand: "Street Code", Category: "headwear", Price: 119_000,
				Description: "Снепбек с вышитым логотипом, прямой козырёк, регулируемая застёжка.",
				Sizes:       oneSize},
			{SKU: "SC-BN-GRY", Title: "Шапка-бини вязаная, серая", Brand: "Street Code", Category: "headwear", Price: 99_000,
				Description: "Тёплая шапка с отворотом. Мягкая пряжа, растягивается по размеру.",
				Sizes:       oneSize},
			{SKU: "SC-BN-STR", Title: "Шапка в полоску с помпоном", Brand: "Street Code", Category: "headwear", Price: 129_000,
				Description: "Вязаная шапка в разноцветную полоску с большим помпоном.",
				Sizes:       oneSize},
		},
	},
	{
		Slug: "demo-gentleman", Name: "Gentleman Tailoring", Login: "demo-gentleman",
		Description: "Мужская классика и smart casual: костюмы, пиджаки, поло и трикотаж. Подгонка по фигуре за 2 дня.",
		Address:     "Ташкент, Яккасарайский район, ул. Бабура, 45", Phone: "+998901110404",
		Lat: 41.287222, Lng: 69.256944,
		Products: []Product{
			{SKU: "GT-SUIT-NVY", Title: "Костюм тёмно-синий: пиджак и брюки", Brand: "Gentleman Tailoring", Category: "tops", Price: 2_390_000, Set: true,
				Description: "Двойка из шерстяной костюмной ткани: однобортный пиджак на двух пуговицах и брюки прямого кроя.",
				Sizes:       suitSizes},
			{SKU: "GT-BLZ-GRY", Title: "Пиджак однобортный, графитовый", Brand: "Hugo Boss", Category: "tops", Price: 1_490_000,
				Description: "Пиджак приталенного кроя из шерсти. Две шлицы, внутренние карманы.",
				Sizes:       suitSizes},
			{SKU: "GT-HNL-BLU", Title: "Лонгслив-хенли, синий", Brand: "Gentleman Tailoring", Category: "tops", Price: 329_000,
				Description: "Трикотажный лонгслив с планкой на пуговицах и нагрудным карманом.",
				Sizes:       menSizes},
			{SKU: "GT-POLO-NVY", Title: "Поло из хлопка пике, тёмно-синее", Brand: "Gentleman Tailoring", Category: "tops", Price: 279_000,
				Description: "Поло с коротким рукавом из хлопка пике, воротник и планка на пуговицах.",
				Sizes:       menSizes},
			{SKU: "GT-TEE-GRY", Title: "Футболка из хлопка, серый меланж", Brand: "Gentleman Tailoring", Category: "tops", Price: 169_000,
				Description: "Футболка из мягкого хлопка с круглым вырезом.",
				Sizes:       menSizes},
			{SKU: "GT-BN-GRY", Title: "Шапка вязаная, серый меланж", Brand: "Gentleman Tailoring", Category: "headwear", Price: 149_000,
				Description: "Шапка из шерсти мериноса тонкой вязки, свободная посадка.",
				Sizes:       oneSize},
		},
	},
}

// Retired — артикулы прошлых версий каталога. Повторный запуск seed убирает такие
// товары из тестовых магазинов (товары, добавленные вручную, не трогает).
var Retired = []string{
	"DL-SHR-PLD",
	"AM-DR-GRN", "AM-SK-WHT", "AM-SUIT-WHT", "AM-TR-BEI", "AM-SW-WHT", "AM-HL-WHT", "AM-BT-BLK", "AM-HAT-STR",
	"SC-NK-RUN", "SC-SNK-WHT", "SC-CRG-KHK", "SC-BMB-BRN",
	"GT-SUIT-BLK", "GT-BLZ-BLK", "GT-SHR-WHT", "GT-SW-BRN", "GT-CH-BEI", "GT-DRB-BRN", "GT-OXF-BRN", "GT-BT-BRN",
}
