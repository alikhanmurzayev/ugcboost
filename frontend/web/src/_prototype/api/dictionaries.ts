// Prototype dictionaries — static mock so /prototype forms (categories, cities)
// work without hitting the real /v1/dictionaries/{type} endpoint.
//
// Shape mirrors the real DictionaryEntry contract (code, name, sortOrder) so
// downstream code that expects those fields keeps working unchanged.
export interface DictionaryEntry {
  code: string;
  name: string;
  sortOrder: number;
}

export type DictionaryType = "categories" | "cities";

const MOCK_CATEGORIES: DictionaryEntry[] = [
  { code: "lifestyle", name: "Лайфстайл", sortOrder: 10 },
  { code: "fashion", name: "Мода", sortOrder: 20 },
  { code: "beauty", name: "Бьюти (макияж, уход)", sortOrder: 30 },
  { code: "fitness", name: "Фитнес / ЗОЖ", sortOrder: 40 },
  { code: "food", name: "Еда / Рестораны", sortOrder: 50 },
  { code: "travel", name: "Путешествия", sortOrder: 60 },
  { code: "tech", name: "Тех / Гаджеты", sortOrder: 70 },
  { code: "auto", name: "Авто", sortOrder: 80 },
  { code: "parenting", name: "Родительство", sortOrder: 90 },
  { code: "education", name: "Образование", sortOrder: 100 },
];

const MOCK_CITIES: DictionaryEntry[] = [
  { code: "almaty", name: "Алматы", sortOrder: 10 },
  { code: "astana", name: "Астана", sortOrder: 20 },
  { code: "shymkent", name: "Шымкент", sortOrder: 30 },
  { code: "atyrau", name: "Атырау", sortOrder: 40 },
  { code: "aktobe", name: "Актобе", sortOrder: 50 },
  { code: "karaganda", name: "Караганда", sortOrder: 60 },
  { code: "pavlodar", name: "Павлодар", sortOrder: 70 },
  { code: "ust-kamenogorsk", name: "Усть-Каменогорск", sortOrder: 80 },
];

const MOCK_LATENCY_MS = 150;

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), MOCK_LATENCY_MS));
}

export async function listDictionary(type: DictionaryType): Promise<DictionaryEntry[]> {
  if (type === "categories") return delay([...MOCK_CATEGORIES]);
  if (type === "cities") return delay([...MOCK_CITIES]);
  return delay([]);
}
