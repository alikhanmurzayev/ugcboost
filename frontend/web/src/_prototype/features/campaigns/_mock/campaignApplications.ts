import type { CampaignApplication, CreatorReel } from "../types";

// Public sample videos used as mock UGC content while the real
// Instagram/TikTok ingest is not yet wired up.
const SAMPLE_VIDEOS = [
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/BigBuckBunny.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ElephantsDream.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerBlazes.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerEscapes.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerFun.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerJoyrides.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/ForBiggerMeltdowns.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/Sintel.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/SubaruOutbackOnStreetAndDirt.mp4",
  "https://commondatastorage.googleapis.com/gtv-videos-bucket/sample/TearsOfSteel.mp4",
];

// Generate 6 deterministic Reels per creator. Each gets a different sample
// video + a picsum thumbnail used as a `poster` while the video loads.
function reelsFor(creatorSlug: string): CreatorReel[] {
  let h = 0;
  for (let i = 0; i < creatorSlug.length; i++)
    h = (h * 31 + creatorSlug.charCodeAt(i)) >>> 0;
  return Array.from({ length: 6 }, (_, i) => {
    const seed = `${creatorSlug}-${i}`;
    const videoIdx = (h + i) % SAMPLE_VIDEOS.length;
    return {
      id: seed,
      thumbnailUrl: `https://picsum.photos/seed/${seed}/360/640`,
      videoUrl: SAMPLE_VIDEOS[videoIdx],
      viewCount: 2000 + ((i * 1731) % 25_000),
    };
  });
}

// Per-campaign applications. Same creator can appear in multiple campaigns.
export const MOCK_APPLICATIONS: CampaignApplication[] = [
  // ---- Campaign #1: "Челлендж «Что я купил за 5000 тенге в Fixprice»" ----
  {
    id: "app-001",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "approved",
    tzStatus: "accepted",
    tzSentAt: "2026-04-27T11:00:00Z",
    tzDecidedAt: "2026-04-27T13:42:00Z",
    appliedAt: "2026-04-26T12:30:00Z",
    decidedAt: "2026-04-27T10:00:00Z",
    creator: {
      id: "cr-001",
      firstName: "Айгерим",
      lastName: "Сатаева",
      age: 24,
      city: { code: "almaty", name: "Алматы" },
      categories: [
        { code: "beauty", name: "Бьюти (макияж, уход)" },
        { code: "lifestyle", name: "Лайфстайл" },
      ],
      socials: [
        { platform: "instagram", handle: "aigerim.sat" },
        { platform: "tiktok", handle: "aigerim_sat" },
      ],
      metrics: { followers: 12_400, avgViews: 18_500, er: 8.4 },
      recentReels: reelsFor("aigerim"),
    },
  },
  {
    id: "app-002",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "approved",
    tzStatus: "declined",
    tzSentAt: "2026-04-27T11:00:00Z",
    tzDecidedAt: "2026-04-27T18:05:00Z",
    appliedAt: "2026-04-26T15:10:00Z",
    decidedAt: "2026-04-27T10:00:00Z",
    creator: {
      id: "cr-002",
      firstName: "Тимур",
      lastName: "Болатов",
      age: 28,
      city: { code: "astana", name: "Астана" },
      categories: [{ code: "fashion", name: "Мода / Стиль" }],
      socials: [{ platform: "instagram", handle: "timur_bolatov" }],
      metrics: { followers: 6_200, avgViews: 9_300, er: 6.1 },
      recentReels: reelsFor("timur"),
    },
  },
  {
    id: "app-003",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "approved",
    tzStatus: "sent",
    tzSentAt: "2026-04-27T11:00:00Z",
    appliedAt: "2026-04-27T09:00:00Z",
    decidedAt: "2026-04-27T10:00:00Z",
    creator: {
      id: "cr-003",
      firstName: "Камила",
      lastName: "Ержанова",
      age: 21,
      city: { code: "almaty", name: "Алматы" },
      categories: [{ code: "lifestyle", name: "Лайфстайл" }],
      socials: [
        { platform: "instagram", handle: "kamilla.erj" },
        { platform: "tiktok", handle: "kamilla.erj" },
      ],
      metrics: { followers: 38_100, avgViews: 52_400, er: 11.2 },
      recentReels: reelsFor("kamilla"),
    },
  },
  {
    id: "app-004",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "new",
    tzStatus: "not_sent",
    appliedAt: "2026-04-27T11:45:00Z",
    creator: {
      id: "cr-004",
      firstName: "Жанна",
      lastName: "Касенова",
      age: 32,
      city: { code: "almaty", name: "Алматы" },
      categories: [{ code: "beauty", name: "Бьюти (макияж, уход)" }],
      socials: [{ platform: "instagram", handle: "jannabeauty" }],
      metrics: { followers: 4_800, avgViews: 6_100, er: 5.4 },
      recentReels: reelsFor("janna"),
    },
  },
  {
    id: "app-005",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "new",
    tzStatus: "not_sent",
    appliedAt: "2026-04-27T18:20:00Z",
    creator: {
      id: "cr-005",
      firstName: "Алина",
      lastName: "Турсунова",
      age: 19,
      city: { code: "astana", name: "Астана" },
      categories: [
        { code: "beauty", name: "Бьюти (макияж, уход)" },
        { code: "fashion", name: "Мода / Стиль" },
      ],
      socials: [
        { platform: "instagram", handle: "alina.tursun" },
        { platform: "tiktok", handle: "alinatursun" },
      ],
      metrics: { followers: 21_300, avgViews: 32_700, er: 9.8 },
      recentReels: reelsFor("alina"),
    },
  },
  {
    id: "app-006",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "new",
    tzStatus: "not_sent",
    appliedAt: "2026-04-28T08:00:00Z",
    creator: {
      id: "cr-006",
      firstName: "Мадина",
      lastName: "Сейтжанова",
      age: 26,
      city: { code: "almaty", name: "Алматы" },
      categories: [{ code: "fashion", name: "Мода / Стиль" }],
      socials: [{ platform: "instagram", handle: "madina.s" }],
      metrics: { followers: 9_700, avgViews: 12_200, er: 7.2 },
      recentReels: reelsFor("madina"),
    },
  },
  {
    id: "app-007",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "new",
    tzStatus: "not_sent",
    appliedAt: "2026-04-28T10:30:00Z",
    creator: {
      id: "cr-007",
      firstName: "Дильназ",
      lastName: "Кенжебаева",
      age: 23,
      city: { code: "shymkent", name: "Шымкент" },
      categories: [{ code: "lifestyle", name: "Лайфстайл" }],
      socials: [{ platform: "tiktok", handle: "dilnaz.k" }],
      metrics: { followers: 2_100, avgViews: 3_800, er: 4.6 },
      recentReels: reelsFor("dilnaz"),
    },
  },
  {
    id: "app-008",
    campaignId: "c1000000-0000-0000-0000-000000000001",
    status: "new",
    tzStatus: "not_sent",
    appliedAt: "2026-04-28T12:00:00Z",
    creator: {
      id: "cr-008",
      firstName: "Сабина",
      lastName: "Ахметова",
      age: 25,
      city: { code: "almaty", name: "Алматы" },
      categories: [
        { code: "beauty", name: "Бьюти (макияж, уход)" },
        { code: "lifestyle", name: "Лайфстайл" },
      ],
      socials: [
        { platform: "instagram", handle: "sabinaa" },
        { platform: "tiktok", handle: "sabinaa.ak" },
      ],
      metrics: { followers: 15_200, avgViews: 22_400, er: 9.1 },
      recentReels: reelsFor("sabina"),
    },
  },
];
