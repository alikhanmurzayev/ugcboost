import type { CampaignBrief } from "./types";

const qaraBurnToken =
  "bda815c1775ca0a8b3b2e0002aa37af093cb65d90538db48e2034c450b974b31";

const ethnoFashionDayToken =
  "b75d0221430ef136ec88341595c9ce24503fa0dde9883935d69c1ad48f891a5b";

const ethnoFashionDayBarterToken =
  "7ce745a67f0c8a5567afc2b1423d4d2c38f8dd983086d66cabbc27391cbde233";

const efwGeneralToken =
  "c559487366dbc5829e85033cc6a70233af7bcb300b7584cc2d648237caa31afe";

const kidsFashionDayToken =
  "bbea37590e33e6635e4bdb3150c07e29b6df15fa246df2692785366f92659a5e";

export const campaigns: Record<string, CampaignBrief> = {
  [qaraBurnToken]: {
    token: qaraBurnToken,
    brandName: "BURN FAMILY × CODE7212 × Arai Bektursun",
    campaignTitle: "Показ коллекции\nQARA BURN",
    subtitle: "Коллаборация BURN FAMILY × CODE7212 × Arai Bektursun",
    context:
      "Показ на подиуме EURASIAN FASHION WEEK 2026 + доступ к показам других дизайнеров",
    inviteEventLabel: "EFW 13 мая в 18:00",
    eventDetails: [
      { label: "Дата", value: "13 мая" },
      { label: "Время", value: "18:00–22:00" },
      { label: "Адрес", value: "Koktobe Hall, ул. Омаровой 35а" },
      {
        label: "Вход",
        value:
          "Вход только по пригласительным билетам. Онлайн пригласительный билет на показ отправляется fashion-креатору после подписания соглашения о UGC-коллаборации в конце этой страницы",
      },
    ],
    cooperationFormat: "Бартер",
    fromBrand: {
      title: "От EURASIAN FASHION WEEK",
      items: [
        "Приглашение на показы 13 мая",
        "Мерч QARA BURN в подарок",
        "Приглашение на закрытое after-party Недели моды 14 мая в 22:00",
      ],
    },
    fromCreator: {
      title: "От креатора",
      items: [
        "Осветить показ коллекции QARA BURN — 1 Reels (обязательно)",
        "Дополнительный контент с Недели моды — по желанию",
      ],
    },
    reels: {
      format: "Сторителлинг / обзор коллекции / fashion-влог / разговорный",
      deadline: "До 15 апреля, 18:00 — отправить ссылку на опубликованный Reels в Telegram-бот",
      requirements: [
        "Показ коллекции QARA BURN на подиуме",
        "Крупные планы образов: фактуры, акценты, детали одежды",
        "Логотип BURN FAMILY / QARA BURN на экране во время показа",
        "Личная реакция на коллекцию: что понравилось, какие детали запомнились, какой вайб был у показа",
        "Себя на EURASIAN FASHION WEEK: образ, эмоции, личное присутствие на показе",
        "На фото-зоне бренда с инсталляцией угля и пламени",
        "Моменты, где видна атмосфера Недели моды: гости, свет, подиум, пресс-стена",
        "Мерч QARA BURN, который вы получите в подарок: можно показать в формате unboxing, try-on, детали ткани / принта / посадки",
      ],
    },
    mentions: {
      accounts: [
        "@efw.kz",
        "@burnfamily.kazakhstan",
        "@code7212",
        "@arai_bektursun",
      ],
      notes: [
        "Отправьте коллаб-пост на эти указанные аккаунты",
        "Отметки на Reels — как тег (tag) и упоминание всех аккаунтов текстом в описании Reels (caption)",
      ],
    },
    aboutParagraphs: [
      "**13 мая на подиуме** EURASIAN FASHION WEEK будет представлена коллекция **QARA BURN** — коллаборация **Burn Family** и молодого **карагандинского бренда CODE7212**, в названии которого зашифрован телефонный код Караганды. В основу коллекции лег **образ Караганды — города шахтеров и угля**. Не как топлива, а как символа внутренней силы, жара и энергии, которая рождается под давлением. Уголь плотный, темный, скрытый внутри земли, но именно в правильных условиях он раскрывает тепло и силу.",
      "Степь, уголь, юрта в дыму, оранжевые искры на черном фоне, кристалл угля как символ силы, закаленной давлением. Все это сложилось в коллекцию, где каждая деталь полна смысла: **кристалл угля на спине оверсайз-футболки**, **оранжевый треугольник** как отсылка к жару и свету, монохромная графика с единственным горящим акцентом. Получилось **черное на черном — с огнем внутри**.",
      "**Burn Family** пошли дальше и обратились к молодым талантам. Так дизайнер **Арай Бектурсын** создала образы специально для показа — без них на подиуме не было бы той самой эстетики и лоска, за которым приходят зрители.",
      "Караганда — это город, который буквально **вырос из шахт**. Не метафорически, а в самом прямом смысле: из горизонтов под землей, из труда, который не видно снаружи, из угля, который десятилетиями кормил целый регион.",
      "**CODE7212 — бренд из Караганды**, и это важно. Они работают с образом своего города не через ностальгию, а через настоящее уважение к материалу и истории. В их руках промышленный вайб становится **брутальным шиком**: шахтерские формы, металлические текстуры, тяжелые пропорции — все это превращается в эстетику, которую невозможно не показать на подиуме.",
      "Показ на EURASIAN FASHION WEEK станет **официальной презентацией QARA BURN** и откроет **старт продаж коллекции**.",
    ],
    aboutNote:
      "Текст ниже — справочная информация для сторителлинга. Повторять его полностью 1-в-1 необязательно, в основном это для понимания контекста.",
    aboutImageUrl: "/campaigns/qara-burn/qara-burn.jpeg",
    aboutImageAlt: "Коллекция QARA BURN",
  },
  [ethnoFashionDayToken]: {
    token: ethnoFashionDayToken,
    brandName: "EURASIAN FASHION WEEK 2026",
    campaignTitle: "ETHNO FASHION DAY",
    subtitle: "Интеграция UGC boost × TrustMe",
    subtitleAsTagline: true,
    context:
      "Специальный показ EURASIAN FASHION WEEK, посвященный этническим мотивам, культурному наследию и их современной интерпретации в моде",
    inviteEventLabel: "ETHNO FASHION DAY 14 мая в 18:00",
    eventDetails: [
      { label: "Дата", value: "14 мая" },
      { label: "Время", value: "18:00" },
      { label: "Адрес", value: "Koktobe Hall, ул. Омаровой 35а" },
      {
        label: "Вход",
        value:
          "Онлайн пригласительный билет отправляется fashion-креатору после подписания соглашения о сотрудничестве в конце этой страницы",
      },
    ],
    cooperationFormat: "Бартер",
    fromBrand: {
      title: "От EURASIAN FASHION WEEK",
      items: [
        "Приглашение на показы ETHNO FASHION DAY 14 мая в 18:00",
      ],
    },
    fromCreator: {
      title: "От креатора",
      items: [
        "Осветить ETHNO FASHION DAY с нативной интеграцией @ugc_boost и @trustme.kz — 1 Reels, от 3 Stories",
      ],
    },
    reels: {
      format:
        "Сторителлинг / обзор коллекции дизайнеров / влог / атмосфера Недели моды / стритстайл",
      deadline:
        "До 17 мая, 20:00 — отправить ссылку на опубликованный Reels в Telegram-бот",
      requirements: [
        "Показ коллекции дизайнеров на подиуме",
        "Крупные планы образов: фактуры, акценты, детали одежды",
        "Логотипы дизайнеров на экране во время показа коллекции (которые вам понравились)",
        "Личная реакция на коллекцию: что понравилось, какие детали запомнились, какой вайб был у показа",
        "Себя на EURASIAN FASHION WEEK: образ, эмоции, личное присутствие на показе",
        "Обзор зон партнеров и шоурум",
        "Моменты, где видна атмосфера Недели моды: гости, свет, подиум",
      ],
    },
    mentions: {
      accounts: ["@efw.kz", "@ugc_boost", "@trustme.kz"],
      notes: [
        "В контенте необходимо нативно упомянуть и отметить @ugc_boost и @trustme.kz: рассказать, что приглашение на показ EURASIAN FASHION WEEK вы получили через платформу UGC boost, а договор о сотрудничестве подписали онлайн через TrustMe.",
        "TrustMe — казахстанский IT-стартап, который развивает цифровую инфраструктуру для креативных индустрий, поддерживает локальное fashion-комьюнити и делает сотрудничество между брендами, событиями и креаторами более прозрачным и профессиональным.",
        "Отправьте коллаб-пост на: @efw.kz, @ugc_boost, @trustme.kz",
      ],
    },
    designers: {
      intro:
        "Можете отметить дизайнеров, чьи образы вам понравились больше всего, поделиться личными эмоциями, впечатлениями от коллекций и деталями, которые запомнились.",
      items: [
        { brand: "NURASEM", designer: "Нури Рыскулова", handles: ["@nurasem_kazakhstan"] },
        { brand: "EREN", designer: "Каламкас Сагындык", handles: ["@eren.label"] },
        { brand: "INNES", designer: "Айгерим Абен", handles: ["@aigerim_aben"] },
        { brand: "Lo Zarata", designer: "Искаханова Ляззат", handles: ["@ulttyk_kiim_lozarata"] },
        { brand: "REBIRTH", designer: "Мира Тастемирова", handles: ["@rebirth_concept_store"] },
        { brand: "MALIQUE", designer: "Малика Усупова", handles: ["@malika_ussupova"] },
        {
          brand: "Финалисты международного конкурса молодых дизайнеров «Жас-Өркен 2026» (университет АТУ)",
        },
      ],
    },
  },
  [ethnoFashionDayBarterToken]: {
    token: ethnoFashionDayBarterToken,
    brandName: "EURASIAN FASHION WEEK 2026",
    campaignTitle: "ETHNO FASHION DAY",
    context:
      "Специальный показ EURASIAN FASHION WEEK, посвященный этническим мотивам, культурному наследию и их современной интерпретации в моде",
    inviteEventLabel: "ETHNO FASHION DAY 14 мая в 18:00",
    eventDetails: [
      { label: "Дата", value: "14 мая" },
      { label: "Время", value: "18:00" },
      { label: "Адрес", value: "Koktobe Hall, ул. Омаровой 35а" },
      {
        label: "Вход",
        value:
          "Онлайн пригласительный билет отправляется fashion-креатору после подписания соглашения о сотрудничестве в конце этой страницы",
      },
    ],
    cooperationFormat: "Бартер",
    fromBrand: {
      title: "От EURASIAN FASHION WEEK",
      items: [
        "Приглашение на показы ETHNO FASHION DAY 14 мая в 18:00",
        "Подарок от партнёра Недели моды La mela — шампунь и кондиционер на основе натуральных компонентов (вручаем лично креатору на площадке)",
      ],
    },
    fromCreator: {
      title: "От креатора",
      items: [
        "Осветить ETHNO FASHION DAY — 1 Reels и Stories с интеграцией косметики La mela (подарок)",
        "Дополнительный контент с Недели моды — по желанию",
      ],
    },
    reels: {
      format:
        "Сторителлинг / обзор коллекции дизайнеров / влог / атмосфера Недели моды",
      deadline:
        "До 17 мая, 20:00 — отправить ссылку на опубликованный Reels в Telegram-бот",
      requirements: [
        "Показ коллекции дизайнеров на подиуме",
        "Крупные планы образов: фактуры, акценты, детали одежды",
        "Личная реакция на коллекцию: что понравилось, какие детали запомнились, какой вайб был у показа",
        "Себя на EURASIAN FASHION WEEK: стритстайл-образ, эмоции, личное присутствие на показе",
        "Атмосфера Недели моды",
        "Нативная интеграция уходовой косметики **La mela** (подарок)",
      ],
    },
    stories: {
      requirements: [
        "Атмосфера Недели моды @efw.kz",
        "Обзор продукции La mela (шампунь и кондиционер) с отметкой аккаунта @lamelacosmetics (можно дома)",
      ],
    },
    mentions: {
      title: "Отметки и коллаб-пост",
      accounts: ["@efw.kz", "@lamelacosmetics"],
    },
    partner: {
      title: "Информация о бренде La mela",
      paragraphs: [
        "La mela — казахстанский бренд профессионального ухода за волосами, созданный в Алматы и доведенный до совершенства в итальянских лабораториях. Формулы бренда La mela соединяют натуральные компоненты, современные технологии и ремесленные традиции профессионального ухода за волосами.",
        "Продукцию La mela можно нативно интегрировать в контент как продолжение fashion-впечатлений после показа: момент заботы о себе, восстановление после насыщенного дня, эстетичный ритуал ухода или паузу, в которой раскрывается идея внутренней гармонии и естественной красоты.",
      ],
      handle: "@lamelacosmetics",
      imageUrl: "/partners/la-mela/lamela.png",
      imageAlt: "Подарок La mela — шампунь и кондиционер",
    },
    designers: {
      title: "Дизайнеры ETHNO FASHION DAY",
      intro:
        "Можете отметить дизайнеров, чьи образы вам понравились больше всего, поделиться личными эмоциями, впечатлениями от коллекций и деталями, которые запомнились.",
      items: [
        { brand: "NURASEM", designer: "Нури Рыскулова", handles: ["@nurasem_kazakhstan"] },
        { brand: "EREN", designer: "Каламкас Сагындык", handles: ["@eren.label"] },
        { brand: "INNES", designer: "Айгерим Абен", handles: ["@aigerim_aben"] },
        { brand: "Lo Zarata", designer: "Искаханова Ляззат", handles: ["@ulttyk_kiim_lozarata"] },
        { brand: "REBIRTH", designer: "Мира Тастемирова", handles: ["@rebirth_concept_store"] },
        { brand: "MALIQUE", designer: "Малика Усупова", handles: ["@malika_ussupova"] },
        {
          brand: "Финалисты международного конкурса молодых дизайнеров «Жас-Өркен 2026» (университет АТУ)",
        },
      ],
    },
    aboutTitle: "Концепция 11-сезона EURASIAN FASHION WEEK 2026",
    aboutParagraphs: [
      "В этом году EFW пройдет под концепцией **«MURA. Silent Codes»**.",
      "**Наследие — это не прошлое.** Это живая энергия, которая продолжает жить в нас, даже если мы не всегда это осознаем. **MURA. Silent Codes** — это история о невидимых знаках, которые человек несет через поколения. Мы хотим показать, что наследие может звучать тихо, но именно оно формирует нашу идентичность.",
    ],
  },
  [efwGeneralToken]: {
    token: efwGeneralToken,
    brandName: "11-й сезон • EFW 2026",
    campaignTitle: "EURASIAN FASHION WEEK",
    subtitle: "Показы казахстанских и зарубежных дизайнеров",
    inviteEventLabel: "EURASIAN FASHION WEEK 13 мая в 18:00",
    eventDetails: [
      { label: "Дата", value: "13 мая" },
      { label: "Время", value: "18:00" },
      { label: "Адрес", value: "Koktobe Hall, ул. Омаровой 35а" },
      {
        label: "Вход",
        value:
          "Онлайн пригласительный билет отправляется fashion-креатору после подписания соглашения о сотрудничестве в конце этой страницы",
      },
    ],
    cooperationFormat: "Бартер",
    fromBrand: {
      title: "От EURASIAN FASHION WEEK",
      items: ["Приглашение на показы EURASIAN FASHION WEEK 13 мая в 18:00"],
    },
    fromCreator: {
      title: "От креатора",
      items: [
        "Осветить EURASIAN FASHION WEEK — 1 Reels, 4 Stories",
        "Дополнительный контент с Недели моды — по желанию",
      ],
    },
    reels: {
      format:
        "Сторителлинг / обзор коллекции дизайнеров / fashion-влог / атмосфера Недели моды",
      deadline:
        "До 16 мая, 20:00 — отправить ссылку на опубликованный Reels в Telegram-бот",
      requirements: [
        "Обзор коллекции дизайнеров на подиуме",
        "Крупные планы образов: фактуры, акценты, детали одежды",
        "Личная реакция на коллекцию: что понравилось, какие детали запомнились, какой вайб был у показа",
        "Логотипы дизайнеров на экране во время показа коллекции (которые вам понравились)",
        "Себя на EURASIAN FASHION WEEK: стритстайл-образ, эмоции, личное присутствие на показе",
        "Фойе: зоны партнеров Недели моды, шоурум, фото-зоны и атмосфера нетворкинга",
      ],
    },
    mentions: {
      accounts: ["@efw.kz"],
      notes: ["Отметьте @efw.kz и отправьте коллаб-пост на @efw.kz"],
    },
    designers: {
      intro:
        "Отметьте дизайнеров, чьи образы вам понравились больше всего, поделитесь личными эмоциями, впечатлениями от коллекций и деталями, которые запомнились.",
      items: [
        { brand: "MINAVARA", designer: "Ассоль Гамова", handles: ["@minavara_"] },
        { brand: "DJADI", designer: "Лейла Стяжкина", handles: ["@djadi.kz"] },
        { brand: "VERA SHASHERINA (Россия)", designer: "Вера Абрамова", handles: ["@verashasherina_brand"] },
        {
          brand: "BLACK BURN",
          handles: ["@burnfamily.kazakhstan", "@code7212", "@arai_bektursun"],
        },
        { brand: "GALIIA (Россия)", designer: "Елена Попова", handles: ["@galiia__27"] },
        { brand: "VEGAN TIGER (Южная Корея)", designer: "Yang Yoona", handles: ["@vegan_tiger"] },
        { brand: "SUN SERGIO", designer: "Сергей Шабунин", handles: ["@sunsergio_fashion"] },
        { brand: "DELICIA", designer: "Аяулым Абилова", handles: ["@delliciia"] },
        { brand: "RINA COLLECTION (Россия)", designer: "Катрина Иванюшкина", handles: ["@trina_official1"] },
        { brand: "HUSL SPORT", designer: "Исламбек Шарипов", handles: ["@sharipov_islambek", "@husl.sport"] },
        { brand: "TOMOYOV", designer: "Жандос Асен", handles: ["@_tomoyov"] },
      ],
    },
    aboutTitle: "Информационная справка",
    aboutParagraphs: [
      "13–14 мая в Алматы в Koktobe Hall состоится **11-й сезон** крупнейшей Недели моды прет-а-порте в Евразии — **EURASIAN FASHION WEEK**.",
      "В этом году EFW пройдет под концепцией **«MURA. Silent Codes»**.",
      "**Наследие — это не прошлое.** Это живая энергия, которая продолжает жить в нас, даже если мы не всегда это осознаем. **MURA. Silent Codes** — это история о невидимых знаках, которые человек несет через поколения. Мы хотим показать, что наследие может звучать тихо, но именно оно формирует нашу идентичность.",
      "Концепция **MURA. Silent Codes** раскрывается через три стадии — **«Вижу. Слышу. Говорю»**:\n\n**Вижу:** видеть, различать и принимать правду\n**Слышу:** слышать внутренний голос и силу предков\n**Говорю:** проявлять себя и говорить о важном через моду",
      "Гостей EURASIAN FASHION WEEK ждет уникальный **перформанс-открытие MURA. Silent Codes** от финалистов международного конкурса молодых дизайнеров **Жас Оркен** — коллекция **DISSONANCE.02**.",
    ],
  },
  [kidsFashionDayToken]: {
    token: kidsFashionDayToken,
    brandName: "Социально важный показ",
    campaignTitle: "KIDS FASHION DAY",
    context:
      "Приглашаем креаторов стать голосом и лицом социально важного проекта, поддержать его медийно и внести свой вклад в развитие инклюзивного общества в Казахстане.",
    inviteEventLabel: "KIDS FASHION DAY 14 мая в 12:00",
    eventDetails: [
      { label: "Дата", value: "14 мая" },
      { label: "Время", value: "12:00" },
      { label: "Адрес", value: "Koktobe Hall, ул. Омаровой 35а" },
      {
        label: "Вход",
        value:
          "Онлайн пригласительный билет отправляется UGC-креатору после подписания соглашения о сотрудничестве",
      },
    ],
    cooperationFormat: "Бартер",
    fromBrand: {
      title: "От EURASIAN FASHION WEEK",
      items: ["Приглашение на показы KIDS FASHION DAY 14 мая в 12:00"],
    },
    fromCreator: {
      title: "От креатора",
      items: [
        "Осветить KIDS FASHION DAY — 1 Reels, 3 Stories",
      ],
    },
    reels: {
      format: "Сторителлинг / обзор модного показа / влог",
      deadline:
        "До 17 мая, 20:00 — отправить ссылку на опубликованный Reels в Telegram-бот",
      briefText:
        "Снять контент с показа KIDS FASHION DAY на EURASIAN FASHION WEEK: показать бренды-участники, атмосферу детского показа, выходы детей-моделей и дизайнеров, особое внимание уделить социальной миссии проекта — инклюзии, равным возможностям и поддержке детей с особыми потребностями. В контенте можно отразить участие параспортсменов-боччистов Куаныша Аскарбекова и Итианы Шингисхановой, чьи истории легли в основу фильма «Ерекше», участие особенных детей в показе BENETTON, а также дебют юных дизайнеров Айлин Маулет с брендом AYLI и Айши Жандарбек с брендом AISHÉ. Нужно передать, что KIDS FASHION DAY объединяет моду, кино и социальную миссию, создавая пространство, где каждый ребенок уникален.",
    },
    mentions: {
      accounts: [
        "@efw.kz",
        "@kaz.benetton",
        "@kinopark_kz",
        "@tiger_films_kz",
        "@boccia_kazakhstan",
      ],
    },
    aboutTitle: "Информационная справка о показе",
    aboutParagraphs: [
      "Бренды-участники KIDS FASHION DAY:\n\n**BENETTON** @kaz.benetton\n**UKULIM** @ukilim_collection\n**BELLA SOPROMADZE BRAND** @bella_sopromadze_brand\n**INJUBABY** @inzhubaby_boutique\n**AISHÉ** @aishe_brand_\n**AYLI** @ailin_maulet",
      "**Kinopark** @kinopark_kz совместно с **Tiger Films** @tiger_films_kz реализовали кинопроект **«Ерекше»** при поддержке **Казахстанской федерации бочча** @boccia_kazakhstan.",
      "В рамках **KIDS FASHION DAY** на подиуме EURASIAN FASHION WEEK появятся действующие **параспортсмены-боччисты Куаныш Аскарбеков и Итиана Шингисханова**, чьи истории легли в основу картины. **«Ерекше»** — это история о **силе духа и равных возможностях**. Все средства от проката фильма направлены на **поддержку детей с инвалидностью**.",
      "А также в показе бренда **BENETTON** примут участие **особенные дети**, в том числе **дети с ДЦП и онкозаболеваниями**. Мы поддерживаем детей с особыми потребностями, помогаем им социализироваться, вдохновляем семьи и создаем пространство, где **каждый ребенок чувствует свою уникальность**.",
      "**KIDS FASHION DAY** подчеркивает важность построения **инклюзивного общества** — среды с **равными возможностями для всех**. Культура инклюзии строится не только на законодательной базе, но и на **искренней эмпатии** — способности слышать, понимать и принимать друг друга. Именно поэтому **кино и мода** становятся важными инструментами трансляции этих ценностей.",
      "Кроме того, впервые **юные талантливые дизайнеры** представят свои коллекции: **13-летняя Айлин Маулет** с брендом **AYLI** покажет **коллекцию вечерних нарядов**, а **12-летняя Айша Жандарбек** презентует **гламурно-деловую коллекцию** своего бренда **AISHÉ**.",
    ],
  },
};

// secretTokenFormat mirrors the backend's `^[A-Za-z0-9_-]{16,256}$`. Used
// here as a frontend short-circuit: any token that fails this regex cannot
// reach a real campaign on the backend, so we render NotFoundPage straight
// away instead of showing a generic brief + CTA. This shrinks the
// phishing-link UX surface.
export const secretTokenFormat = /^[A-Za-z0-9_-]{16,256}$/;

// genericBrief is the fallback for tokens we don't have a hand-crafted
// brief for yet — backend has the real campaign + invitation, but the brief
// content (about, reels, mentions) lives only in this static map until we
// move it into the API. Keeps the decision flow usable for any live
// campaign without rebuilding the bundle every time admin makes one.
function genericBrief(token: string): CampaignBrief {
  return {
    token,
    brandName: "UGCBoost",
    campaignTitle: "Приглашение в кампанию",
    subtitle: "Подтвердите участие или откажитесь",
    fromBrand: {
      title: "Что вас ждёт",
      items: [
        "Условия и детали кампании отправит админ в Telegram",
        "Здесь вы фиксируете решение — согласие или отказ",
      ],
    },
  };
}

export function getCampaignByToken(token: string): CampaignBrief | undefined {
  if (!secretTokenFormat.test(token)) return undefined;
  return campaigns[token] ?? genericBrief(token);
}
