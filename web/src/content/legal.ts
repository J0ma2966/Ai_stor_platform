// Тексты юридических страниц (ru/uz). Шаблон — перед запуском проверить у юриста
// и вписать реквизиты оператора. Обычный текст, без HTML: рендерится как текст.
export type LegalDoc = { title: string; updated: string; sections: { h: string; p: string[] }[] };
export type LegalKey = "terms" | "privacy" | "photos" | "about" | "contacts";

const UPDATED = "2026-11-02";

const ru: Record<LegalKey, LegalDoc> = {
  terms: {
    title: "Пользовательское соглашение",
    updated: UPDATED,
    sections: [
      { h: "1. Общие положения", p: ["Платформа Butik (далее — Платформа) — онлайн-маркетплейс, на котором бутики (далее — Магазины) размещают товары, а пользователи (далее — Клиенты) выбирают товары, пользуются AI-примеркой и оформляют заказы.", "Используя Платформу, вы принимаете условия настоящего соглашения. Если вы не согласны с условиями, не используйте Платформу."] },
      { h: "2. Регистрация", p: ["Клиент регистрируется по номеру мобильного телефона Узбекистана и подтверждает его кодом из SMS. При регистрации указывается пол — он нужен для выбора манекена в примерочной и может быть изменён в профиле.", "Клиент отвечает за сохранность доступа к своему номеру телефона. Учётные записи Магазинов создаёт только администрация Платформы."] },
      { h: "3. Товары и заказы", p: ["Продавцом товара является Магазин. Магазин отвечает за описание, цену, наличие и качество товара, а также за доставку заказа.", "При оформлении корзина делится на отдельные заказы по Магазинам. Каждый Магазин подтверждает или отклоняет свой заказ; статус заказа отображается в личном кабинете.", "Оплата заказа производится при получении, если иное не указано при оформлении. Вопросы возврата и обмена решаются с Магазином в соответствии с законодательством Республики Узбекистан о защите прав потребителей."] },
      { h: "4. AI-примерка и токены", p: ["AI-примерка создаёт изображение Клиента в выбранном образе по загруженным фотографиям. Результат носит иллюстративный характер: Платформа не гарантирует точную посадку и соответствие размера.", "Примерка оплачивается внутренними токенами. После регистрации Клиент однократно получает приветственные токены. Стоимость одной генерации отображается перед запуском.", "Если генерация не удалась по технической причине, списанные токены возвращаются автоматически. Токены не являются денежными средствами, не подлежат обмену на деньги и сгорают при удалении аккаунта."] },
      { h: "5. Правила поведения", p: ["Запрещается загружать фотографии других людей без их согласия, изображения несовершеннолетних, а также материалы, нарушающие закон или права третьих лиц.", "Запрещаются попытки получить доступ к чужим данным, нарушить работу Платформы или обойти ограничения. Администрация вправе заблокировать учётную запись за нарушение правил."] },
      { h: "6. Ответственность", p: ["Платформа предоставляет сервис «как есть» и прилагает разумные усилия для его бесперебойной работы. Платформа не несёт ответственности за действия Магазинов, но помогает урегулировать спор при обращении в поддержку."] },
      { h: "7. Изменения", p: ["Администрация может изменять соглашение. Новая редакция действует с момента публикации на этой странице."] },
    ],
  },
  privacy: {
    title: "Политика конфиденциальности",
    updated: UPDATED,
    sections: [
      { h: "1. Какие данные мы обрабатываем", p: ["Номер телефона, пол, язык интерфейса; по желанию — рост, вес и параметры тела.", "Фотографии для AI-примерки (спереди, сбоку, сзади) и результаты генераций.", "Данные заказа: имя и телефон получателя, адрес доставки, координаты точки и, по желанию, фото места доставки.", "Технические данные: IP-адрес, время запросов, идентификаторы сессии — для безопасности и диагностики."] },
      { h: "2. Зачем", p: ["Регистрация и вход, работа AI-примерки, оформление и доставка заказов, начисление и списание токенов, уведомления о статусах, защита от мошенничества, исполнение требований закона."] },
      { h: "3. Кто видит данные", p: ["Магазин видит только данные своих заказов, необходимые для доставки (имя, телефон, адрес, комментарии, фото места). Магазины не видят ваш профиль, параметры тела, фотографии для примерки и данные о других покупках.", "Сотрудники Платформы получают доступ к данным только для поддержки и модерации; каждое действие администратора записывается в журнал.", "Фотографии для примерки передаются сервису генерации изображений только для создания результата."] },
      { h: "4. Хранение и сроки", p: ["Фото места доставки удаляются через 30 дней после завершения заказа (доставлен, отклонён или отменён). Неиспользованные загрузки — через 24 часа.", "Уведомления хранятся 180 дней. Фотографии для примерки и результаты хранятся, пока вы их не удалите, или в срок, указанный в «Правилах обработки фотографий».", "Данные обрабатываются с учётом Закона Республики Узбекистан «О персональных данных»."] },
      { h: "5. Ваши права", p: ["Вы можете изменить данные профиля, удалить сохранённые фото и результаты, отозвать согласие и удалить аккаунт в разделе «Профиль». При удалении стираются телефон, параметры тела, фото, корзина, избранное и уведомления; оформленные заказы сохраняются для бухгалтерского и налогового учёта без привязки к профилю.", "По вопросам обработки данных пишите в поддержку (контакты — на странице «Контакты»)."] },
      { h: "6. Безопасность", p: ["Передача данных шифруется (HTTPS). Пароли магазинов хранятся в виде хэшей, коды входа — только в виде хэшей с ограниченным сроком действия. Фотографии хранятся в закрытом хранилище и выдаются по временным ссылкам. Метаданные (включая GPS) удаляются из загруженных фото."] },
    ],
  },
  photos: {
    title: "Правила обработки фотографий",
    updated: UPDATED,
    sections: [
      { h: "Что загружать", p: ["Три фотографии одного человека — себя — в полный рост: спереди, сбоку и сзади. Вертикальный кадр, хорошее освещение, без других людей; лучше простая облегающая одежда.", "Нельзя загружать фото других людей без их согласия и фото несовершеннолетних."] },
      { h: "Как мы их используем", p: ["Фото используются только для создания изображения примерки по вашему запросу. Мы не используем ваши фото для рекламы и не передаём их Магазинам.", "Перед загрузкой файл проверяется и пересохраняется: удаляются метаданные, в том числе геолокация."] },
      { h: "Хранение и удаление", p: ["Вы сами решаете, сохранять ли фото для повторных примерок. Сохранённые фото и результаты можно удалить в любой момент в разделе «Мои фото» и «История примерок». При удалении аккаунта они удаляются автоматически.", "Срок и место хранения биометрических изображений определяются с учётом требований законодательства Республики Узбекистан."] },
      { h: "Ограничения результата", p: ["Результат AI-примерки — иллюстрация. Он зависит от качества фото и изображений товаров и не гарантирует точной посадки и размера."] },
    ],
  },
  about: {
    title: "О платформе",
    updated: UPDATED,
    sections: [
      { h: "Butik", p: ["Butik объединяет каталоги бутиков в одном месте: один поиск, одна корзина и единый личный кабинет.", "Главная особенность — виртуальная примерочная: соберите образ из товаров разных магазинов и посмотрите на себя в нём по трём своим фотографиям."] },
      { h: "Магазинам", p: ["Бутики подключаются по приглашению администрации. Магазин сам управляет профилем и товарами, загружает каталог из Excel, видит статистику просмотров, добавлений в примерочную и заказов.", "Чтобы подключить бутик, свяжитесь с поддержкой."] },
    ],
  },
  contacts: {
    title: "Контакты",
    updated: UPDATED,
    sections: [{ h: "Поддержка", p: ["Ответим на вопросы о заказах, токенах, примерке и подключении магазина. Указывайте номер заказа или код ошибки, если он есть."] }],
  },
};

const uz: Record<LegalKey, LegalDoc> = {
  terms: {
    title: "Foydalanuvchi shartnomasi",
    updated: UPDATED,
    sections: [
      { h: "1. Umumiy qoidalar", p: ["Butik platformasi (keyingi o‘rinlarda — Platforma) — butiklar (Do‘konlar) mahsulot joylaydigan, foydalanuvchilar (Mijozlar) esa mahsulot tanlaydigan, AI-kiyib ko‘rishdan foydalanadigan va buyurtma beradigan onlayn-marketpleys.", "Platformadan foydalanib, siz ushbu shartnoma shartlarini qabul qilasiz. Agar rozi bo‘lmasangiz, Platformadan foydalanmang."] },
      { h: "2. Ro‘yxatdan o‘tish", p: ["Mijoz O‘zbekiston mobil raqami orqali ro‘yxatdan o‘tadi va uni SMS-kod bilan tasdiqlaydi. Ro‘yxatdan o‘tishda jins ko‘rsatiladi — u kiyib ko‘rish xonasidagi manekenni tanlash uchun kerak va profilda o‘zgartirilishi mumkin.", "Mijoz o‘z telefon raqamiga kirish xavfsizligi uchun javobgar. Do‘kon hisoblarini faqat Platforma ma’muriyati yaratadi."] },
      { h: "3. Mahsulotlar va buyurtmalar", p: ["Mahsulot sotuvchisi — Do‘kon. Do‘kon mahsulot tavsifi, narxi, mavjudligi, sifati va buyurtmani yetkazib berish uchun javobgar.", "Rasmiylashtirishda savat Do‘konlar bo‘yicha alohida buyurtmalarga bo‘linadi. Har bir Do‘kon o‘z buyurtmasini tasdiqlaydi yoki rad etadi; holat shaxsiy kabinetda ko‘rinadi.", "Buyurtma uchun to‘lov, agar boshqacha ko‘rsatilmagan bo‘lsa, qabul qilishda amalga oshiriladi. Qaytarish va almashtirish masalalari O‘zbekiston Respublikasining iste’molchilar huquqlarini himoya qilish to‘g‘risidagi qonunchiligiga muvofiq Do‘kon bilan hal qilinadi."] },
      { h: "4. AI-kiyib ko‘rish va tokenlar", p: ["AI-kiyib ko‘rish yuklangan suratlar bo‘yicha Mijozning tanlangan obrazdagi tasvirini yaratadi. Natija illyustrativ xususiyatga ega: Platforma aniq yarashish va o‘lchamni kafolatlamaydi.", "Kiyib ko‘rish ichki tokenlar bilan to‘lanadi. Ro‘yxatdan o‘tgach, Mijoz bir marta xush kelibsiz tokenlarini oladi. Bitta generatsiya narxi ishga tushirishdan oldin ko‘rsatiladi.", "Generatsiya texnik sababga ko‘ra amalga oshmasa, yechilgan tokenlar avtomatik qaytariladi. Tokenlar pul mablag‘i emas, pulga almashtirilmaydi va akkaunt o‘chirilganda yo‘qoladi."] },
      { h: "5. Xulq-atvor qoidalari", p: ["Boshqa odamlarning suratlarini ularning roziligisiz, voyaga yetmaganlar tasvirlarini, shuningdek qonun yoki uchinchi shaxslar huquqlarini buzuvchi materiallarni yuklash taqiqlanadi.", "Boshqalarning ma’lumotlariga kirish, Platforma ishini buzish yoki cheklovlarni chetlab o‘tishga urinishlar taqiqlanadi. Ma’muriyat qoidalarni buzgan hisobni bloklash huquqiga ega."] },
      { h: "6. Javobgarlik", p: ["Platforma xizmatni «boricha» taqdim etadi va uning uzluksiz ishlashi uchun oqilona harakat qiladi. Platforma Do‘konlarning harakatlari uchun javobgar emas, lekin qo‘llab-quvvatlashga murojaat qilinganda nizoni hal qilishga yordam beradi."] },
      { h: "7. O‘zgarishlar", p: ["Ma’muriyat shartnomani o‘zgartirishi mumkin. Yangi tahrir ushbu sahifada e’lon qilingan paytdan kuchga kiradi."] },
    ],
  },
  privacy: {
    title: "Maxfiylik siyosati",
    updated: UPDATED,
    sections: [
      { h: "1. Qanday ma’lumotlarni qayta ishlaymiz", p: ["Telefon raqami, jins, interfeys tili; xohishga ko‘ra — bo‘y, vazn va tana o‘lchamlari.", "AI-kiyib ko‘rish uchun suratlar (old, yon, orqa tomondan) va generatsiya natijalari.", "Buyurtma ma’lumotlari: qabul qiluvchining ismi va telefoni, yetkazib berish manzili, nuqta koordinatalari va xohishga ko‘ra — yetkazib berish joyi surati.", "Texnik ma’lumotlar: IP-manzil, so‘rovlar vaqti, sessiya identifikatorlari — xavfsizlik va diagnostika uchun."] },
      { h: "2. Nima uchun", p: ["Ro‘yxatdan o‘tish va kirish, AI-kiyib ko‘rish ishi, buyurtmalarni rasmiylashtirish va yetkazish, tokenlarni hisoblash, holatlar haqida bildirishnomalar, firibgarlikdan himoya, qonun talablarini bajarish."] },
      { h: "3. Ma’lumotlarni kim ko‘radi", p: ["Do‘kon faqat o‘z buyurtmalarining yetkazib berish uchun zarur ma’lumotlarini ko‘radi (ism, telefon, manzil, izohlar, joy surati). Do‘konlar profilingizni, tana o‘lchamlaringizni, kiyib ko‘rish suratlaringizni va boshqa xaridlaringizni ko‘rmaydi.", "Platforma xodimlari ma’lumotlarga faqat qo‘llab-quvvatlash va moderatsiya uchun kiradi; administratorning har bir harakati jurnalga yoziladi.", "Kiyib ko‘rish suratlari tasvir generatsiyasi xizmatiga faqat natija yaratish uchun uzatiladi."] },
      { h: "4. Saqlash va muddatlar", p: ["Yetkazib berish joyi suratlari buyurtma yakunlangandan (yetkazilgan, rad etilgan yoki bekor qilingan) 30 kundan keyin o‘chiriladi. Foydalanilmagan yuklamalar — 24 soatdan keyin.", "Bildirishnomalar 180 kun saqlanadi. Kiyib ko‘rish suratlari va natijalari siz ularni o‘chirmaguningizcha yoki «Suratlarni qayta ishlash qoidalari»da ko‘rsatilgan muddatda saqlanadi.", "Ma’lumotlar O‘zbekiston Respublikasining «Shaxsga doir ma’lumotlar to‘g‘risida»gi Qonunini hisobga olgan holda qayta ishlanadi."] },
      { h: "5. Huquqlaringiz", p: ["«Profil» bo‘limida profil ma’lumotlarini o‘zgartirishingiz, saqlangan suratlar va natijalarni o‘chirishingiz, rozilikni qaytarib olishingiz va akkauntni o‘chirishingiz mumkin. O‘chirishda telefon, tana o‘lchamlari, suratlar, savat, saralanganlar va bildirishnomalar o‘chiriladi; berilgan buyurtmalar buxgalteriya va soliq hisobi uchun profilga bog‘lanmagan holda saqlanadi.", "Ma’lumotlarni qayta ishlash bo‘yicha savollar uchun qo‘llab-quvvatlashga yozing («Kontaktlar» sahifasi)."] },
      { h: "6. Xavfsizlik", p: ["Ma’lumotlar uzatilishi shifrlanadi (HTTPS). Do‘kon parollari xesh ko‘rinishida, kirish kodlari esa faqat cheklangan muddatli xesh ko‘rinishida saqlanadi. Suratlar yopiq omborda saqlanadi va vaqtinchalik havolalar orqali beriladi. Yuklangan suratlardan metama’lumotlar (shu jumladan GPS) o‘chiriladi."] },
    ],
  },
  photos: {
    title: "Suratlarni qayta ishlash qoidalari",
    updated: UPDATED,
    sections: [
      { h: "Nimani yuklash kerak", p: ["Bir odamning — o‘zingizning — to‘liq bo‘ydagi uchta surati: old, yon va orqa tomondan. Vertikal kadr, yaxshi yorug‘lik, boshqa odamlarsiz; oddiy, tanaga yopishib turadigan kiyim afzal.", "Boshqa odamlarning suratlarini ularning roziligisiz va voyaga yetmaganlarning suratlarini yuklash mumkin emas."] },
      { h: "Ulardan qanday foydalanamiz", p: ["Suratlar faqat sizning so‘rovingiz bo‘yicha kiyib ko‘rish tasvirini yaratish uchun ishlatiladi. Suratlaringizni reklama uchun ishlatmaymiz va Do‘konlarga bermaymiz.", "Yuklashdan oldin fayl tekshiriladi va qayta saqlanadi: metama’lumotlar, jumladan geolokatsiya o‘chiriladi."] },
      { h: "Saqlash va o‘chirish", p: ["Takroriy kiyib ko‘rish uchun suratlarni saqlash-saqlamaslikni o‘zingiz hal qilasiz. Saqlangan suratlar va natijalarni istalgan vaqtda «Suratlarim» va «Kiyib ko‘rishlar tarixi» bo‘limlarida o‘chirish mumkin. Akkaunt o‘chirilganda ular avtomatik o‘chiriladi.", "Biometrik tasvirlarni saqlash muddati va joyi O‘zbekiston Respublikasi qonunchiligi talablarini hisobga olgan holda belgilanadi."] },
      { h: "Natija cheklovlari", p: ["AI-kiyib ko‘rish natijasi — illyustratsiya. U suratlar va mahsulot tasvirlari sifatiga bog‘liq va aniq yarashish hamda o‘lchamni kafolatlamaydi."] },
    ],
  },
  about: {
    title: "Platforma haqida",
    updated: UPDATED,
    sections: [
      { h: "Butik", p: ["Butik butiklar kataloglarini bir joyga jamlaydi: bitta qidiruv, bitta savat va yagona shaxsiy kabinet.", "Asosiy xususiyat — virtual kiyib ko‘rish xonasi: turli do‘konlar mahsulotlaridan obraz yig‘ing va uchta suratingiz bo‘yicha o‘zingizni unda ko‘ring."] },
      { h: "Do‘konlar uchun", p: ["Butiklar ma’muriyat taklifi bilan ulanadi. Do‘kon profil va mahsulotlarni o‘zi boshqaradi, katalogni Excel’dan yuklaydi, ko‘rishlar, kiyib ko‘rish xonasiga qo‘shishlar va buyurtmalar statistikasini ko‘radi.", "Butikni ulash uchun qo‘llab-quvvatlash bilan bog‘laning."] },
    ],
  },
  contacts: {
    title: "Kontaktlar",
    updated: UPDATED,
    sections: [{ h: "Qo‘llab-quvvatlash", p: ["Buyurtmalar, tokenlar, kiyib ko‘rish va do‘konni ulash bo‘yicha savollarga javob beramiz. Agar bo‘lsa, buyurtma raqami yoki xato kodini ko‘rsating."] }],
  },
};

export function legalDoc(locale: string, key: LegalKey): LegalDoc {
  return (locale === "uz" ? uz : ru)[key];
}
export const LEGAL_KEYS: LegalKey[] = ["terms", "privacy", "photos"];
