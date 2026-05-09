import type { CampaignBrief } from "./types";

const qaraBurnToken =
  "bda815c1775ca0a8b3b2e0002aa37af093cb65d90538db48e2034c450b974b31";

export const campaigns: Record<string, CampaignBrief> = {
  [qaraBurnToken]: {
    token: qaraBurnToken,
    brandName: "BURN FAMILY × CODE7212 × Arai Bektursun",
    campaignTitle: "Показ коллекции\nQARA BURN",
    subtitle: "Коллаборация BURN FAMILY × CODE7212 × Arai Bektursun",
    context:
      "Показ на подиуме EURASIAN FASHION WEEK 2026 + доступ к показам других дизайнеров",
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
  },
};

// secretTokenFormat mirrors the backend's `^[A-Za-z0-9_-]{16,256}$`. Used
// here as a frontend short-circuit: any token that fails this regex cannot
// reach a real campaign on the backend, so we render NotFoundPage straight
// away instead of showing a generic brief + CTA. This shrinks the
// phishing-link UX surface.
const secretTokenFormat = /^[A-Za-z0-9_-]{16,256}$/;

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
