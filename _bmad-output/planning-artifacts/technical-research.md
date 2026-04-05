---
title: "Техническое исследование: UGCBoost"
type: technical-research
status: complete
created: "2026-04-04"
---

# Техническое исследование: UGCBoost

## Ключевые технические решения (TL;DR)

> Этот раздел -- сжатая сводка всего документа. Детали -- в соответствующих секциях.

- **Стек:** FastAPI (Python 3.12+) + SQLAlchemy 2.0 + asyncpg + PostgreSQL 16 + Redis | React + Vite для TMA | Next.js 15 + Shadcn/UI для Dashboard | aiogram 3.26 для Telegram Bot
- **LiveDune API (Business, $50/мес, 10K запросов):** Покрывает аналитику Instagram, TikTok, YouTube для модерации креаторов. **Критическая неизвестность:** возможность проверки аккаунтов, не подключённых к dashboard, -- не подтверждена, требует тестирования до начала разработки
- **TrustContract (TrustMe):** SMS-подпись за 90 секунд, полная юридическая сила по статье 152 ГК РК. REST API существует, но детали эндпоинтов и вебхуков не верифицированы -- необходим прямой контакт с TrustMe
- **Telegram Mini App:** Бесшовная аутентификация через initData (HMAC-SHA256), уведомления через Bot API (100% доставка). ~7.6 млн пользователей Telegram в Казахстане -- основной канал дистрибуции
- **Хостинг:** Hetzner Cloud (Финляндия) -- CX22 от ~4.49 EUR/мес, CX32 от ~8.49 EUR/мес (цены после повышения 01.04.2026). Docker Compose для MVP, Kubernetes при масштабировании
- **Аутентификация:** Три метода для трёх клиентов -- initData (TMA/креаторы), Email/OAuth Google (Dashboard/бренды), Email+TOTP 2FA (Admin). Единый JWT backend с refresh token rotation
- **Три главных технических риска:** (1) LiveDune API может не поддерживать проверку внешних аккаунтов -- блокирует автоматическую модерацию; (2) TrustContract API может не встраиваться в TMA WebView -- потребуется обходной UX; (3) Instagram/TikTok API требуют Business Verification и OAuth -- длительный процесс одобрения
- **ЭЦП и налоги:** НДС 16% (с 01.01.2026), режим самозанятого для креаторов -- 4% соцотчислений, 0% ИПН. UGCBoost как ТОО-посредник на ОУР позволяет брендам вычитать маркетинговые расходы
- **Верификация аккаунтов (MVP):** Challenge-based (временный пост с кодом) -- самый простой. OAuth -- во второй итерации
- **CI/CD:** GitHub Actions (lint + test + security audit + deploy). Staging из `develop`, Production из `main` с ручным одобрением
- **Мониторинг:** Prometheus + Grafana + structlog (JSON) + correlation ID. Loki для агрегации логов (Post-MVP)

---

## 1. LiveDune: API и интеграция

### 1.1 О платформе

LiveDune ([livedune.com](https://livedune.com/)) -- all-in-one платформа аналитики и управления социальными сетями. Компания ориентирована на SMM-агентства, маркетологов и бизнес в странах СНГ. Поддерживает оплату в тенге (KZT), что удобно для Казахстана.

**Поддерживаемые платформы:** Instagram, TikTok, YouTube, VKontakte, Facebook, LinkedIn, Pinterest, Odnoklassniki, Twitter/X, Telegram.

**Ключевые возможности для UGCBoost:**
- Проверка блогеров на накрутки (лайки, подписчики)
- Engagement Rate (ER) и Engagement Reach Rate (ERR) -- рассчитываются ежедневно в полночь по МСК на основе последних 20 публикаций
- Статистика аккаунтов: подписчики, просмотры, частота публикаций
- История подписчиков
- Получение постов аккаунта

Источники: [LiveDune](https://livedune.com/), [LiveDune Help Center](https://wiki.livedune.com/en/articles/9776017-engagement-tab)

### 1.2 API: доступность и эндпоинты

LiveDune предоставляет REST API для всех тарифных планов. Документация по методам API доступна по адресу [api.livedune.com/docs](https://api.livedune.com/docs/), а справочная статья -- на [wiki.livedune.com](https://wiki.livedune.com/ru/articles/11640100-api).

**Подтвержденные API-методы** (из документации Albato и wiki):
- Получение постов аккаунта (Get posts for an account)
- История подписчиков аккаунта (Subscriber history for an account)
- Stories аккаунта (только VK и Instagram)
- Кастомный API-запрос (Custom API request)

**Аутентификация:** API Token -- генерируется в личном кабинете LiveDune.

**Лимиты запросов по тарифам:**

| Тариф | Запросов/месяц | Цена |
|-------|---------------|------|
| Trial/Freemium | 100 | Бесплатно |
| Blogger | 2 000 | $29/мес (или ~500 руб) |
| Business | 10 000 | $50/мес (или ~2000 руб) |
| Agency | 100 000 | $200/мес |

Дополнительные пакеты запросов (не сгорают): от 1K до 1M запросов -- разовая покупка.

Источники: [LiveDune Pricing](https://livedune.com/pricing/), [LiveDune API Blog](https://livedune.com/ru/blog/livedune-api/), [LiveDune Wiki API](https://wiki.livedune.com/ru/articles/11640100-api)

### 1.3 Ограничения LiveDune API

**Критический момент:** Полная техническая документация API (api.livedune.com/docs) требует JavaScript-рендеринга и не индексируется публично. Детальный список всех эндпоинтов, форматов запросов/ответов и параметров невозможно верифицировать без доступа к личному кабинету LiveDune.

**Что НЕ подтверждено:**
- Возможность проверки произвольного аккаунта (не подключенного к dashboard) через API -- функция "Проверка блогера" доступна в UI, но наличие соответствующего API-эндпоинта не задокументировано публично
- Возможность получения метрик по конкретному посту через API (а не аккаунту в целом)
- Поддержка Threads в API

**Рекомендация:** Перед началом разработки необходимо зарегистрировать аккаунт LiveDune (минимум тариф Blogger) и протестировать API вручную. Критически важно выяснить, можно ли через API проверять метрики аккаунтов, не подключенных к dashboard, так как это ядро автоматической модерации креаторов.

### 1.4 Альтернативные аналитические API

Если LiveDune API не покрывает потребности, рассмотреть альтернативы:

**Modash API** ([modash.io/influencer-marketing-api](https://www.modash.io/influencer-marketing-api)):
- 380M+ профилей influencers (Instagram, TikTok, YouTube)
- Discovery API: поиск по фильтрам, демография аудитории, fake follower detection, engagement rate
- Raw API: live данные профилей, посты, подписчики
- REST API, аутентификация через access token
- **Цена:** Discovery API от $16 200/год, Raw API от $10 000/год -- дорого для MVP

**Phyllo** ([getphyllo.com](https://www.getphyllo.com/)):
- Unified API для данных креаторов с 20+ платформ
- Identity API для верификации владения аккаунтом
- Engagement API: лайки, просмотры, импрешены, шеры
- Работает по модели consent: креатор авторизует доступ к данным

**HypeAuditor:**
- Сильная аналитика аудитории и верификация подлинности
- **Цена:** от $399/месяц

Источники: [Modash API](https://www.modash.io/influencer-marketing-api), [Modash API Pricing](https://www.modash.io/influencer-marketing-api/pricing), [Phyllo](https://www.getphyllo.com/), [HypeAuditor vs Modash](https://www.modash.io/modash-vs-hypeauditor)

### 1.5 Рекомендация по аналитике

Для MVP: **LiveDune (тариф Business, $50/мес)** -- лучшее соотношение цены и покрытия для казахстанского рынка. 10 000 запросов/месяц достаточно для модерации 200-500 креаторов. Поддержка тенге -- бонус.

Fallback-стратегия: если API LiveDune не позволяет проверять произвольные аккаунты, использовать **Phyllo** для верификации + прямые API Instagram/TikTok для метрик.

---

## 2. TrustMe / TrustContract (ЭЦП в Казахстане)

### 2.1 О платформе

TrustMe ([trustme.kz](https://trustme.kz/)) -- казахстанская TrustTech-экосистема. Ключевой продукт -- **TrustContract** -- сервис онлайн-подписания договоров.

**Методы подписи:**
- SMS-код (OTP) -- основной, подпись за 90 секунд
- QR-код
- Face ID (биометрическая верификация через гос. базы данных)
- eGov Mobile (государственное приложение)
- Blockchain -- документы получают уникальный хэш

**Интеграции с системами:**
- Bitrix24, AmoCRM
- 1C:Предприятие
- Enbek.kz (автоматическая регистрация трудовых договоров)

**Масштаб:** 3000+ компаний, 1.5 млн+ пользователей.

Источники: [TrustMe](https://trustme.kz/), [TrustContract](https://trustme.kz/podpisat-dogovor-onlajn-v-kazahstane-servis-trustcontract-trustme/), [TrustMe LinkedIn](https://kz.linkedin.com/company/trust-me)

### 2.2 API TrustContract

**API существует** и задокументирован на Apiary: [trustmekz.docs.apiary.io](https://trustmekz.docs.apiary.io/). Это REST API.

**Подтвержденные возможности API:**
- Отправка документов на подпись из CRM/ERP
- Получение статуса подписания
- Массовая отправка (тысячи договоров через API)
- Автозаполнение данных по ИИН/БИН из государственных баз данных (ГБД ФЛ/ЮЛ)

**Что НЕ удалось верифицировать** (документация Apiary требует JS-рендеринга):
- Конкретные эндпоинты (URLs, HTTP-методы)
- Формат аутентификации (API key, OAuth, etc.)
- Webhook-колбэки для статуса подписания
- Возможность встраивания виджета подписи (iframe/SDK)
- Точный формат ответов

**Поддерживаемые документы:** PDF, Word.

**Рекомендация:** Связаться с TrustMe напрямую для получения полной документации API и обсуждения интеграционного флоу. Вероятный сценарий: через API отправляем договор с данными креатора --> TrustContract присылает ссылку на подпись --> креатор подписывает через SMS --> получаем webhook о завершении.

Источники: [API TrustContract Apiary](https://trustmekz.docs.apiary.io/), [TrustMe Contract](https://trustme.kz/contract/)

### 2.3 Юридическая сила

Документы, подписанные через TrustContract, имеют **полную юридическую силу** в Казахстане:
- **Статья 152 ГК РК:** сделка в письменной форме может быть совершена путем обмена электронными документами
- **Закон "Об электронном документе и ЭЦП":** SMS-подпись приравнивается к ЭЦП и собственноручной подписи
- Документы принимаются **судами и налоговыми органами**
- Подтверждено заключением Министерства цифрового развития, инноваций и аэрокосмической промышленности РК
- BI Group (крупнейший застройщик Казахстана) использует SMS-подписание для договоров купли-продажи недвижимости

**Механизм идентификации:** Закон РК "О связи" + БДИК (база данных идентификации клиентов) связывают IMEI устройства + SIM-карту + ИИН абонента, что однозначно идентифицирует подписанта.

Источник: [TrustMe - Законность](https://trustme.kz/zakonno_podpisyvat/)

### 2.4 Альтернативы TrustContract

**НУЦ РК + NCALayer:**
- Государственная система ЭЦП через НУЦ РК ([pki.gov.kz](https://pki.gov.kz/en/))
- NCALayer -- десктопное приложение для подписи в веб-приложениях
- JavaScript-клиент: [ncalayer-js-client](https://github.com/sigex-kz/ncalayer-js-client) (async API, WebSocket)
- **Минус:** требует установки NCALayer на устройстве пользователя -- непрактично для мобильных креаторов

**eGov Mobile:**
- Государственное мобильное приложение
- Не имеет публичного API для интеграции в сторонние сервисы

**Вывод:** TrustContract -- оптимальный выбор для UGCBoost. SMS-подпись не требует от креаторов установки дополнительного ПО. Поддержка API позволит автоматизировать процесс.

Источники: [NCALayer](https://pki.gov.kz/en/ncalayer-2/), [NCALayer JS Client](https://github.com/sigex-kz/ncalayer-js-client), [eGov ЭЦП](https://egov.kz/cms/en/services/pass_onlineecp)

---

## 3. Telegram Mini Apps (TMA)

### 3.1 Возможности платформы

Telegram Mini Apps -- веб-приложения внутри Telegram, работающие через WebView. По сути, стандартные веб-страницы (HTML/CSS/JS) с доступом к Telegram SDK.

**Поддерживаемые платформы:** iOS, Android, macOS, Windows, Linux.

**Способы запуска Mini App:**
1. Keyboard buttons (`web_app` type)
2. Inline buttons
3. Menu button
4. Main Mini App (из профиля бота)
5. Inline mode
6. Direct links: `https://t.me/botusername/appname?startapp=param`
7. Attachment menu

Источник: [Telegram Bot API - Web Apps](https://core.telegram.org/bots/webapps)

### 3.2 Аутентификация

**InitData** -- основной механизм идентификации пользователя:
- `Telegram.WebApp.initData` содержит query string с полями: `user` (id, first_name, username, language_code, is_premium), `query_id`, `auth_date`, `hash`
- Серверная валидация: HMAC-SHA256 с использованием bot token и константы "WebAppData"
- Альтернативная валидация через Ed25519 (`signature` field) -- без необходимости бот-токена
- Поле `auth_date` для проверки свежести данных

**Для UGCBoost:** Telegram автоматически аутентифицирует пользователя. Не нужен отдельный login flow. На бэкенде проверяем `initData` для идентификации.

### 3.3 Хранилище данных

| Тип | Лимит | Описание |
|-----|-------|----------|
| CloudStorage | 1024 items, 4096 chars/value | Облачное хранилище, синхронизируется между устройствами |
| DeviceStorage | 5 MB | Локальное хранилище (Bot API 9.0+), аналог localStorage |
| SecureStorage | 10 items | Шифрованное хранилище (iOS Keychain / Android Keystore) |

**Для UGCBoost:** Основные данные хранить на бэкенде. CloudStorage использовать для кэширования предпочтений UI. SecureStorage -- не актуально для нашего кейса.

### 3.4 Платежи

- `openInvoice(url, [callback])` -- открывает платежный интерфейс
- Событие `invoiceClosed` возвращает статус: paid, cancelled, failed, pending
- Поддержка Telegram Stars для подписок
- Интеграция со сторонними платежными провайдерами (Google Pay, Apple Pay из коробки)

**Для UGCBoost MVP:** Не актуально (Fashion Week -- только бартер). Для будущего: рассмотреть Telegram Stars или Kaspi Pay через внешнюю ссылку.

### 3.5 Дополнительные возможности (релевантные для UGCBoost)

- **Deep linking:** `https://t.me/botusername/appname?startapp=campaign_123` -- для прямых ссылок на кампании
- **Кнопки:** MainButton и SecondaryButton -- для основных действий (откликнуться на кампанию, сдать работу)
- **QR Scanner:** `showScanQrPopup()` -- потенциально для ивентов (Fashion Week)
- **Скачивание файлов:** `downloadFile()` -- для скачивания ТЗ/брифов
- **Share to Story:** `shareToStory()` -- потенциально для продвижения платформы
- **Геолокация:** `LocationManager` (Bot API 8.0+) -- для определения города креатора
- **Полноэкранный режим:** `requestFullscreen()` (Bot API 8.0+)
- **CSS-переменные:** `var(--tg-viewport-height)`, `var(--tg-viewport-stable-height)` -- для адаптивности
- **Safe Areas:** для корректного отображения на устройствах с вырезом

### 3.6 Ограничения TMA

- **Нет push-уведомлений от Mini App** -- уведомления только через Telegram Bot API (отдельные сообщения от бота)
- `openLink()` вызывается только из user interaction (клик)
- `readTextFromClipboard()` доступен только из attachment menu
- CloudStorage: максимум 1024 элементов
- DeviceStorage: 5 MB
- Mini App -- это всегда WebView, не нативное приложение
- **Нет фонового выполнения** -- приложение активно только когда открыто
- **Нет прямого доступа к файловой системе** -- загрузка файлов через стандартный `<input type="file">`
- **Вертикальный свайп** по умолчанию закрывает Mini App (можно отключить через `disableVerticalSwipes()`)

### 3.7 TMA vs PWA: компромиссы

| Критерий | Telegram Mini App | PWA |
|----------|-------------------|-----|
| Распространение | Внутри Telegram (1B+ MAU) | Любой браузер, SEO |
| Установка | Не требуется | Prompt на home screen |
| Аутентификация | Автоматическая (Telegram ID) | Своя система auth |
| Уведомления | Через Telegram Bot (100% delivery) | Push API (ненадежный) |
| Удержание | Высокое (чаты = привычка) | Низкое (забывают URL) |
| Вирусность | Шеринг внутри Telegram | Ссылки в любых каналах |
| SEO | Нет | Да |
| Сложность UX | Ограничена WebView | Полная веб-функциональность |

**Для UGCBoost:** TMA -- правильный выбор для креаторов. Аудитория уже в Telegram, аутентификация бесшовная, уведомления гарантированно доставляются. PWA рассмотреть для публичной витрины (лендинг, каталог кампаний).

Источники: [Telegram Mini Apps Docs](https://core.telegram.org/bots/webapps), [TMA 2026 Guide](https://magnetto.com/blog/everything-you-need-to-know-about-telegram-mini-apps), [TMA vs Native vs Web](https://freeblock.medium.com/telegram-mini-apps-vs-native-apps-vs-web-apps-2026-whats-best-for-your-product-1e72c12ebb1b)

### 3.8 Примеры успешных Mini Apps

- E-commerce fashion brand: 50K пользователей за 3 месяца, $0 на привлечение
- Маркетплейс: 50K пользователей за 2 месяца через вирусный шеринг
- Ивент-тикетинг: продажа билетов + QR-сканирование на входе
- Real estate: поиск, избранное, бронирование просмотров

Источник: [TMA Success Cases 2025](https://bazucompany.com/blog/case-studies-successful-telegram-mini-apps-in-2025/)

---

## 4. Рекомендации по технологическому стеку

### 4.1 Backend: FastAPI (Python)

**Рекомендация: FastAPI + SQLAlchemy 2.0 + asyncpg + PostgreSQL**

**Обоснование:**
- Tech lead (Alikhan) специализируется на бэкенде -- Python-экосистема наиболее продуктивна для быстрой разработки MVP
- FastAPI -- async-first, автоматическая OpenAPI-документация, встроенная валидация через Pydantic
- Производительность: FastAPI обходит NestJS/Express для database-heavy операций в бенчмарках
- Экосистема для интеграций: библиотеки для Telegram Bot API (`aiogram`), HTTP-клиенты (`httpx`), фоновые задачи (`celery`, `arq`), планировщик (`apscheduler`)
- SQLAlchemy 2.0: зрелый ORM с полной поддержкой async через asyncpg
- Миграции: Alembic

**Best practices (2025-2026):**
- `async_sessionmaker` для управления жизненным циклом сессий
- Connection pool: `pool_size=20`, `max_overflow=10` (для production)
- Не смешивать sync и async операции в async-эндпоинтах
- Prometheus для мониторинга (`prometheus-fastapi-instrumentator`)

**Альтернатива NestJS:** Хороший фреймворк, но overhead модульной архитектуры замедляет ранний MVP. Рассмотреть при масштабировании команды.

Источники: [FastAPI Best Practices](https://github.com/zhanymkanov/fastapi-best-practices), [FastAPI + SQLAlchemy 2.0 + asyncpg](https://leapcell.io/blog/building-high-performance-async-apis-with-fastapi-sqlalchemy-2-0-and-asyncpg), [FastAPI vs NestJS](https://medium.com/@kaushalsinh73/fastapi-vs-nestjs-which-one-wins-for-modern-apis-98d9f3242c6b)

### 4.2 База данных: PostgreSQL + Redis

**PostgreSQL:**
- Реляционные данные: пользователи, кампании, заявки, контракты
- JSONB для гибких полей (настройки кампании, метаданные)
- LISTEN/NOTIFY для real-time уведомлений о событиях
- Full-text search для поиска кампаний и креаторов
- Зрелая экосистема расширений (pg_cron для периодических задач)

**Redis:**
- Кэширование: результаты LiveDune API, сессии
- Rate limiting для API
- Очереди задач (если используем `arq` вместо Celery)
- Pub/Sub для real-time событий (если LISTEN/NOTIFY PostgreSQL недостаточно)

**Рекомендация по схеме данных:** Event Sourcing не нужен для MVP, но реализовать audit trail (history table) для ключевых действий: модерация креаторов, одобрение кампаний, сдача работ.

Источник: [PostgreSQL NOTIFY](https://medium.com/@sergey.dudik/a-deep-dive-into-postgres-notify-for-real-time-database-tracking-fdfb66dadf0e)

### 4.3 API Design: REST

**Рекомендация: REST (не GraphQL)**

Обоснование:
- Простота для MVP -- меньше boilerplate
- Четкие контракты для каждого endpoint (OpenAPI docs из FastAPI)
- Три потребителя API (TMA, Brand Dashboard, Admin) имеют разные, но предсказуемые потребности в данных -- REST с продуманными эндпоинтами справится
- GraphQL рассмотреть при масштабировании, если появятся сложные запросы с множественными связями

### 4.4 Frontend: Brand Dashboard + Admin

**Рекомендация: Next.js (React)**

- Более широкая экосистема дашборд-компонентов (Shadcn/UI, Radix, React Admin)
- TurboPack -- быстрая сборка (на 28% быстрее Vite/Nuxt в бенчмарках 2026)
- Лучше для API-rich приложений (на 23% меньше клиентского JS)
- Готовые дашборд-шаблоны и boilerplate для admin-панелей
- Server Components для оптимизации загрузки

**UI-библиотека:** Shadcn/UI + Tailwind CSS -- гибко, кастомизируемо, не привязывает к конкретному дизайну.

**Admin-панель:** Реализовать как часть основного Next.js приложения с RBAC (Role-Based Access Control), а не как отдельное приложение. Использовать middleware Next.js для авторизации.

Источники: [Next.js vs Nuxt 2026](https://nextjstemplates.com/blog/nextjs-vs-nuxt), [Best Backend Frameworks 2026](https://www.index.dev/blog/best-backend-frameworks-ranked)

### 4.5 Telegram Mini App Frontend

**Рекомендация: React + Vite + @telegram-apps/sdk-react**

- `@telegram-apps/sdk-react` v3.3.9 -- официальный SDK с React-хуками и компонентами
- React-шаблон: [Telegram-Mini-Apps/reactjs-template](https://github.com/Telegram-Mini-Apps/reactjs-template) -- Vite, TypeScript, tma.js
- Единая экосистема React для TMA и Brand Dashboard -- переиспользование компонентов, типов, утилит
- State management: Zustand (легковесный) или React Query (серверное состояние)

**UI-кит для TMA:** Telegram UI Kit (`@telegram-apps/telegram-ui`) или кастомные компоненты на Tailwind, стилизованные под Telegram (используя CSS-переменные темы Telegram).

Источники: [@telegram-apps/sdk-react NPM](https://www.npmjs.com/package/@telegram-apps/sdk-react), [React TMA Template](https://github.com/Telegram-Mini-Apps/reactjs-template)

### 4.6 Инфраструктура и хостинг

**Хостинг для production:**

Казахстан -- не регион присутствия крупных cloud-провайдеров (AWS, GCP, Azure). Варианты:

| Провайдер | Локация | Задержка до KZ | Цена | Комментарий |
|-----------|---------|----------------|------|-------------|
| Serverspace | Алматы | <5 мс | Умеренная | VPS в Казахстане, Intel Xeon Gold |
| THE.Hosting | Алматы | <5 мс | Умеренная | Дата-центр в Алматы |
| is*hosting | Алматы (Sairam DC) | <5 мс | Умеренная | Premium HI-END серверы |
| Hetzner | Финляндия / Нюрнберг | ~80-120 мс | Дешево | Лучшее соотношение цена/качество |
| AWS / GCP | Бахрейн / Мумбаи | ~100-150 мс | Дорого | Enterprise-grade, но далеко |

**Рекомендация для MVP:**
- **Primary:** Hetzner Cloud (Финляндия) -- отличное соотношение цена/производительность, достаточно для MVP
- **Alternative:** Локальный VPS в Алматы (Serverspace) -- если задержка критична для UX
- Docker Compose для MVP -- не нужен Kubernetes на старте
- CI/CD: GitHub Actions (бесплатно для open source; достаточно для private repos)

**Будущее масштабирование:** Kubernetes (k3s) или managed Kubernetes в Hetzner при переходе к production-scale.

Источники: [Serverspace KZ](https://serverspace.io/services/vps-server/vps-in-kazakhstan/), [Hetzner Cloud](https://www.hetzner.com/cloud), [Kazakhstan VPS](https://hostadvice.com/vps/kazakhstan/)

---

## 5. API социальных сетей

### 5.1 Instagram Graph API

**Версия:** v22.0 (2026)

**Требования:**
- Instagram Business или Creator аккаунт (personal -- нет доступа)
- Facebook App + Business Verification
- OAuth 2.0 через Facebook Login
- Разрешение `instagram_manage_insights`

**Доступные метрики (после обновлений марта 2025):**
- Views (новая метрика, заменила Impressions)
- Reach
- Likes, Comments, Shares, Saves
- Story views (заменила Story impressions)
- Follower demographics: возраст, пол, локация

**Deprecated метрики (v21+, январь 2025):**
- Media impressions
- Reel plays, Reel replays
- Story impressions

**Rate limits:** 200 вызовов/час на приложение.

**Instagram Basic Display API:** deprecated с 4 декабря 2024 -- доступ к personal аккаунтам больше невозможен.

**Для UGCBoost:** Instagram Graph API позволяет получить метрики постов (views, likes, comments, shares), но только для аккаунтов, авторизовавших приложение через OAuth. Нельзя получить данные произвольного аккаунта без его согласия. Это означает, что для автоматической проверки метрик креатора нужно либо просить его авторизовать приложение, либо использовать LiveDune/Modash.

Источники: [Instagram Graph API 2026](https://elfsight.com/blog/instagram-graph-api-complete-developer-guide-for-2026/), [Instagram Insights Updates](https://docs.supermetrics.com/docs/instagram-insights-updates), [Meta Developers](https://developers.facebook.com/docs/instagram-platform/api-reference/instagram-user/insights/)

### 5.2 TikTok API

**Коммерческий доступ (Login Kit + Display API):**
- OAuth 2.0
- `user.info.basic` (scope по умолчанию): имя, аватар
- `video.list`: список видео пользователя
- Отображение профиля и видео в приложении
- **Data Portability API:** пользователь может разрешить перенос своих данных

**Research API:**
- **Только для академических исследователей** из университетов США, ЕЭП, Великобритании, Швейцарии
- НЕ доступен для коммерческих приложений
- 1000 запросов/день, 100 записей/запрос

**Для UGCBoost:** TikTok Display API позволяет получить базовую информацию о пользователе и его видео через OAuth. Для метрик (views, likes) -- доступ ограничен. Наиболее практичный путь: LiveDune для аналитики TikTok или Phyllo через consent flow.

Источники: [TikTok Login Kit](https://developers.tiktok.com/doc/login-kit-overview), [TikTok Display API](https://developers.tiktok.com/doc/display-api-get-started), [TikTok Research API](https://developers.tiktok.com/products/research-api/)

### 5.3 Threads API

**Статус:** Запущен Meta 18 июня 2024, активно развивается.

**Доступные возможности (2025-2026):**
- Публикация постов
- Получение контента
- Управление разговорами
- Polls, location tagging
- Webhooks
- DM API (с июля 2025)
- Keyword search (500 запросов за 7 дней)
- Analytics
- С сентября 2025: доступ без привязки к Instagram аккаунту

**Для UGCBoost:** Threads API позволяет проверять посты и получать метрики. Относительно новый API, активно обновляется.

Источники: [Threads API Meta](https://developers.facebook.com/docs/threads), [Threads API Changelog](https://developers.facebook.com/docs/threads/changelog/)

### 5.4 Верификация владения аккаунтом

**Проблема:** Как подтвердить, что креатор действительно владеет указанным аккаунтом?

**Варианты:**
1. **OAuth-авторизация** (Instagram Graph API, TikTok Login Kit) -- наиболее надежный метод, пользователь авторизует доступ через официальный OAuth flow
2. **Phyllo Identity API** -- unified consent flow для верификации аккаунтов на 20+ платформах
3. **Challenge-based:** попросить креатора опубликовать временный пост/story с уникальным кодом -- менее надежно, но не требует OAuth
4. **DM-верификация:** отправить код в DM аккаунта -- если креатор его подтвердит, это подтверждение владения

**Рекомендация для MVP:** Вариант 3 (challenge-based) -- самый простой для запуска. OAuth реализовать во второй итерации для автоматического сбора метрик.

---

## 6. Риски интеграционной архитектуры

### 6.1 Зависимость от LiveDune

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| API не позволяет проверять произвольные аккаунты | Средняя | Высокое | Протестировать до начала разработки; fallback: Phyllo + прямые API |
| Изменение API без уведомления | Низкая | Среднее | Абстрактный слой-адаптер; мониторинг ответов API |
| Рост цен | Низкая | Низкое | Текущий тариф Business ($50/мес) -- приемлем для MVP |
| Прекращение работы сервиса | Очень низкая | Высокое | Fallback на Modash или Phyllo |

### 6.2 Зависимость от TrustContract

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| API недостаточно гибок для нашего flow | Средняя | Высокое | Связаться с TrustMe до разработки; протестировать API |
| Виджет подписи не встраивается в TMA | Средняя | Среднее | Открывать ссылку на подпись через `openLink()` в TMA |
| Сервис недоступен (downtime) | Низкая | Среднее | Graceful degradation: сохранять договор, подписать позже |

### 6.3 Ограничения Telegram Mini Apps

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| Производительность WebView на старых устройствах | Средняя | Среднее | Оптимизация bundle size, lazy loading, тестирование на low-end |
| Ограничение CloudStorage (1024 items) | Низкая | Низкое | Хранить данные на бэкенде, CloudStorage только для UI cache |
| Невозможность фоновой работы | Высокая | Среднее | Вся логика на бэкенде; уведомления через Telegram Bot |
| Изменение политики Telegram для Mini Apps | Низкая | Высокое | Следить за changelog; архитектура с возможностью перехода на PWA |

### 6.4 API социальных сетей

| Риск | Вероятность | Влияние | Митигация |
|------|-------------|---------|-----------|
| Instagram ужесточает доступ к API | Средняя | Высокое | Использовать LiveDune как прослойку; не зависеть от прямого API |
| TikTok Research API недоступен для коммерческого использования | Подтверждено | Высокое | Использовать Display API + LiveDune для метрик |
| Threads API нестабилен (молодой продукт) | Средняя | Низкое | Threads -- опциональный канал; приоритет Instagram и TikTok |

### 6.5 Защита персональных данных

Основной закон: **Закон РК No 94-V от 21 мая 2013 "О персональных данных и их защите"**.

**Ключевые требования:**
- Согласие субъекта на сбор и обработку персональных данных (ИИН, ФИО, адрес, соцсети)
- Обеспечение конфиденциальности данных ограниченного доступа
- Равенство прав субъектов, владельцев и операторов

**Отличия от GDPR:**
- Закон РК НЕ является полным аналогом GDPR
- Нет requirement для прозрачности и easily accessible form
- Нет right to be forgotten в том же объеме

**Рекомендация:** Включить в договор с креатором (через TrustContract) согласие на обработку персональных данных. Хранить ИИН, адреса и контактные данные в шифрованном виде. Минимизировать сбор данных -- собирать только необходимое.

Источники: [KZ Data Protection Law](https://adilet.zan.kz/eng/docs/Z1300000094), [KZ vs GDPR](https://hulr.org/spring-2024/on-aligning-kazakhstans-data-protection-legislation-with-gdpr-to-enhance-digital-trade)

---

## 7. Платежная инфраструктура (Post-MVP)

### 7.1 Kaspi Pay

**Kaspi** -- доминирующая платежная экосистема в Казахстане.

**Интеграция через ApiPay.kz** ([apipay.kz](https://apipay.kz)):
- REST API для создания инвойсов по номеру телефона
- Рекуррентные платежи, рефанды, вебхуки
- OpenAPI 3.0 спецификация
- Примеры кода: Python, Node.js, cURL
- Sandbox для тестирования
- ApiPay не берет процент от продаж

**Kaspi Merchant API:**
- Go-библиотека: [kaspi-merchant-api](https://github.com/abdymazhit/kaspi-merchant-api) -- реализация всех методов Kaspi Merchant API

Источники: [ApiPay.kz](https://apipay.kz/for-ai), [Kaspi Merchant API](https://github.com/abdymazhit/kaspi-merchant-api)

### 7.2 CloudPayments Kazakhstan

**CloudPayments** ([cloudpayments.kz](https://cloudpayments.kz/)) -- работает в Казахстане.

- Интернет-эквайринг: оплата банковскими картами
- Apple Pay, Google Pay на веб и в мобильных приложениях
- Рекуррентные платежи через token
- REST API: [developers.cloudpayments.com](https://developers.cloudpayments.com/portal/documentation/)
- Postman collection доступен
- Аутентификация через APP key

Источники: [CloudPayments KZ](https://cloudpayments.kz/), [CloudPayments Docs](https://developers.cloudpayments.com/portal/documentation/)

### 7.3 Stripe в Казахстане

**Stripe напрямую недоступен** для казахстанских компаний. Подключение возможно через посредников (Easy Payments и подобные), но сопряжено с рисками блокировки.

**Не рекомендуется для MVP.** Использовать Kaspi Pay + CloudPayments.

Источник: [Stripe KZ](https://easypayments.online/blog/podkluchenie-stripe-v-kazakhstane)

### 7.4 Выплаты креаторам

**Проблема:** Большинство креаторов -- физические лица без ИП/ТОО.

**Варианты:**
1. **Kaspi перевод** (P2P по номеру телефона) -- через ApiPay.kz API, самый простой для Казахстана
2. **Банковский перевод** -- через CloudPayments payout API
3. **UGCBoost как агент-посредник** -- выплаты через бухгалтерию компании (актуально для налогового комплаенса)

**Рекомендация:** Для Post-MVP -- Kaspi Pay для приема платежей от брендов + Kaspi переводы для выплат креаторам. Минимальный friction для казахстанских пользователей.

### 7.5 Валюта и налоги

- Основная валюта: KZT (казахстанский тенге)
- UGCBoost как ТОО на общеустановленном режиме (плательщик НДС **16%** -- с 1 января 2026 года ставка НДС в Казахстане повышена с 12% до 16% в рамках нового Налогового кодекса РК, подписанного Президентом 18 июля 2025)
- Порог обязательной постановки на учёт по НДС снижен с 20 000 МРП до 10 000 МРП (~43,25 млн тенге в 2026)
- Возможность вычета маркетинговых расходов для брендов при работе через UGCBoost
- Необходимо вести реестр договоров с креаторами для налоговой отчетности

Источники: [Изменения по НДС с 1 января 2026](https://pro1c.kz/news/zakonodatelstvo/izmeneniya-po-nds-s-1-yanvarya-2026-goda/), [Kazakhstan New Tax Code VAT Reform](https://www.vatupdate.com/2025/07/23/kazakhstans-new-tax-code-ushers-in-major-vat-reform-from-2026/), [EY Tax Alert KZ](https://www.ey.com/en_kz/technical/tax-alerts/kazakhstan-tax-legislation-update)

---

## 8. Архитектурные паттерны и аналоги

### 8.1 Анализ аналогичных платформ

**Billo** ([billo.app](https://billo.app/)):
- CreativeOps engine: AI + performance data от 150K+ объявлений
- Закрытый workflow: brief -> match -> produce -> launch -> measure -> iterate
- 5000+ проверенных креаторов
- Интеграции: Meta Ads Manager, TikTok Ads Manager

**Insense** ([insense.pro](https://insense.pro)):
- 68 500 influencers
- Нативные интеграции с Meta и TikTok Ads Manager
- Маркетплейс: креаторы откликаются на проекты

**Collabstr** ([collabstr.com](https://collabstr.com)):
- 200 000 influencers
- Модель marketplace: креаторы устанавливают свои цены

**Общие архитектурные паттерны:**
- Matching engine (бренды <-> креаторы) -- ядро любой UGC-платформы
- Campaign lifecycle management (создание -> модерация -> публикация -> исполнение -> аналитика)
- Content gallery с preview (автовоспроизведение видео, метрики)
- Rating/feedback system (обычно закрытый)

Источники: [Billo](https://billo.app/), [Insense](https://insense.pro/), [Collabstr](https://collabstr.com/)

### 8.2 Рекомендуемая архитектура для UGCBoost MVP

```text
                    +-----------------------+
                    |     Landing Page      |
                    |   (Next.js / Static)  |
                    +-----------+-----------+
                                |
                    +-----------v-----------+
                    |   Telegram Bot API    |
                    |   (aiogram / Python)  |
                    +-----------+-----------+
                                |
            +-------------------+-------------------+
            |                                       |
+-----------v-----------+             +-------------v-----------+
| Telegram Mini App     |             | Brand Dashboard / Admin |
| (React + Vite + TMA  |             | (Next.js + Shadcn/UI)   |
|  SDK)                 |             |                         |
+-----------+-----------+             +-------------+-----------+
            |                                       |
            +-------------------+-------------------+
                                |
                    +-----------v-----------+
                    |  FastAPI Backend      |
                    |  (Python 3.12+)      |
                    |  REST API            |
                    +-----------+-----------+
                                |
              +-----------------+-----------------+
              |                 |                 |
    +---------v---------+ +----v----+ +----------v----------+
    |    PostgreSQL      | |  Redis  | |  External APIs      |
    |  (Main DB)         | | (Cache) | |  - LiveDune         |
    |  - Users           | | - API   | |  - TrustContract    |
    |  - Campaigns       | |   cache | |  - Instagram API    |
    |  - Applications    | | - Rate  | |  - TikTok API       |
    |  - Contracts       | |   limit | |  - Threads API      |
    |  - Content         | +---------+ +---------------------+
    |    Submissions     |
    |  - Audit Log       |
    +--------------------+
```

**Ключевые архитектурные решения:**
1. **Monolith-first:** Единый FastAPI backend для всех клиентов. Микросервисы -- преждевременная оптимизация для MVP
2. **Adapter pattern** для внешних API: абстрактный интерфейс для аналитики (LiveDune -> Modash -> прямые API) -- легко заменить провайдера
3. **Background tasks:** Celery или arq для: отправки уведомлений, синхронизации метрик с LiveDune, напоминаний по дедлайнам
4. **Event-driven notifications:** PostgreSQL LISTEN/NOTIFY -> Backend -> Telegram Bot API -> User
5. **RBAC:** три роли (creator, brand, admin) с middleware проверки на уровне FastAPI

### 8.3 Notifications Architecture

Для напоминаний (2 недели, 1 неделя, 3 дня, 1 день до дедлайна):

```python
# Пример: планировщик напоминаний
# APScheduler + Telegram Bot API (aiogram)

from apscheduler.schedulers.asyncio import AsyncIOScheduler
from datetime import timedelta

REMINDER_OFFSETS = [
    timedelta(days=14),  # 2 недели
    timedelta(days=7),   # 1 неделя
    timedelta(days=3),   # 3 дня
    timedelta(days=1),   # 1 день
]

async def schedule_reminders(campaign_id: int, deadline: datetime):
    for offset in REMINDER_OFFSETS:
        remind_at = deadline - offset
        scheduler.add_job(
            send_campaign_reminder,
            trigger="date",
            run_date=remind_at,
            args=[campaign_id, offset.days],
        )

async def send_campaign_reminder(campaign_id: int, days_left: int):
    # Получить активных креаторов кампании
    # Отправить сообщение через Telegram Bot API
    await bot.send_message(
        chat_id=creator.telegram_id,
        text=f"Напоминание: до дедлайна кампании осталось {days_left} дн."
    )
```

Источник: [Telegram Bot API](https://core.telegram.org/bots/api), [APScheduler](https://apscheduler.readthedocs.io/)

---

## 9. Аутентификация и авторизация

### 9.1 Три клиента -- три метода аутентификации

UGCBoost обслуживает три различных клиента, каждый со своим методом аутентификации:

| Клиент | Метод аутентификации | Пользователи |
|--------|---------------------|--------------|
| Telegram Mini App | initData validation (HMAC-SHA256) | Креаторы |
| Brand Dashboard (Next.js) | Email/password + OAuth 2.0 (Google) | Бренды |
| Admin Panel (Next.js) | Email/password + 2FA (TOTP) | Администраторы |

#### 9.1.1 Telegram Mini App: initData validation

Telegram автоматически передаёт `initData` при открытии Mini App. Бэкенд валидирует подпись для идентификации пользователя.

**Алгоритм валидации:**
1. Извлечь `hash` из `initData` query string
2. Отсортировать оставшиеся параметры по ключу
3. Собрать строку `key=value`, разделённую `\n`
4. Создать HMAC-SHA256 с ключом `"WebAppData"` и значением bot token → получить `secret_key`
5. Создать HMAC-SHA256 с `secret_key` и data string → получить `computed_hash`
6. Сравнить `computed_hash` с полученным `hash`
7. Проверить `auth_date` (не старше 5 минут)

```python
import hashlib
import hmac
from urllib.parse import parse_qs
from fastapi import Request, HTTPException, Depends

BOT_TOKEN = "your-bot-token"

def validate_telegram_init_data(init_data: str) -> dict:
    """Валидация initData из Telegram Mini App."""
    parsed = dict(parse_qs(init_data, keep_blank_values=True))
    # Преобразовать списки в значения
    parsed = {k: v[0] for k, v in parsed.items()}

    received_hash = parsed.pop("hash", None)
    if not received_hash:
        raise HTTPException(status_code=401, detail="Missing hash")

    # Сортировка и формирование data check string
    data_check_string = "\n".join(
        f"{k}={v}" for k, v in sorted(parsed.items())
    )

    # HMAC-SHA256 валидация
    secret_key = hmac.new(
        b"WebAppData", BOT_TOKEN.encode(), hashlib.sha256
    ).digest()
    computed_hash = hmac.new(
        secret_key, data_check_string.encode(), hashlib.sha256
    ).hexdigest()

    if not hmac.compare_digest(computed_hash, received_hash):
        raise HTTPException(status_code=401, detail="Invalid signature")

    return parsed
```

**После валидации:** извлекаем `user.id` (Telegram ID), находим или создаём пользователя в БД, выдаём JWT access + refresh tokens для дальнейших запросов к API.

Источники: [Telegram InitData Docs](https://docs.telegram-mini-apps.com/platform/init-data), [telegram-init-data Python library](https://github.com/iCodeCraft/telegram-init-data), [TMA Authentication](https://crmchat.ai/blog/how-telegram-mini-apps-handle-user-authentication)

#### 9.1.2 Brand Dashboard: Email/Password + OAuth (Google)

**Email/Password:**
- Регистрация: email + пароль (минимум 8 символов, uppercase, lowercase, цифра)
- Хеширование паролей: `bcrypt` через библиотеку `passlib` (cost factor = 12)
- Email confirmation: отправка ссылки с временным токеном (срок жизни 24 часа)

**OAuth 2.0 (Google):**
- Используем `authlib` для OAuth flow
- Redirect flow: Brand Dashboard → Google OAuth → callback на FastAPI → создание/линковка аккаунта → redirect обратно с JWT
- Хранить `google_id` в таблице `User` для линковки

**Flow:**
1. Бренд нажимает "Войти через Google"
2. Frontend перенаправляет на Google OAuth consent screen
3. Google возвращает authorization code на callback URL
4. FastAPI обменивает code на access token, получает profile (email, name)
5. Создаём/находим пользователя, выдаём JWT

#### 9.1.3 Admin Panel: Email/Password + 2FA (TOTP)

- Те же email/password что и для брендов
- Дополнительный слой: **TOTP** (Time-based One-Time Password) через `pyotp`
- Совместим с Google Authenticator, Authy, 1Password
- При первом входе: QR-код для настройки authenticator app
- При каждом входе: после email/password → запрос 6-значного TOTP кода
- Backup codes (10 штук) для восстановления доступа

```python
import pyotp

# Генерация секрета для нового администратора
totp_secret = pyotp.random_base32()
# Генерация QR-кода URI
totp_uri = pyotp.TOTP(totp_secret).provisioning_uri(
    name=admin_email, issuer_name="UGCBoost Admin"
)

# Верификация TOTP при логине
def verify_totp(secret: str, code: str) -> bool:
    totp = pyotp.TOTP(secret)
    return totp.verify(code, valid_window=1)  # ±30 секунд
```

### 9.2 JWT-стратегия

**Структура токенов:**

| Параметр | Access Token | Refresh Token |
|----------|-------------|---------------|
| Срок жизни | 15 минут | 7 дней |
| Хранение (TMA) | In-memory (JavaScript variable) | CloudStorage |
| Хранение (Dashboard) | In-memory | HttpOnly Secure cookie |
| Payload | `user_id`, `role`, `exp`, `jti` | `user_id`, `token_family`, `exp`, `jti` |
| Алгоритм | HS256 (HMAC-SHA256) | HS256 |

**Refresh Token Rotation:**
- При каждом обновлении access token через refresh token → выдаётся **новый** refresh token
- Старый refresh token **инвалидируется** (одноразовое использование)
- Refresh tokens хранятся в Redis как whitelist (хешированные SHA-256)
- При обнаружении повторного использования старого refresh token → инвалидация всей `token_family` (все сессии пользователя)

```python
from datetime import datetime, timedelta
from jose import jwt

SECRET_KEY = "your-secret-key"
ALGORITHM = "HS256"
ACCESS_TOKEN_EXPIRE = timedelta(minutes=15)
REFRESH_TOKEN_EXPIRE = timedelta(days=7)

def create_access_token(user_id: int, role: str) -> str:
    payload = {
        "sub": str(user_id),
        "role": role,
        "exp": datetime.utcnow() + ACCESS_TOKEN_EXPIRE,
        "type": "access",
    }
    return jwt.encode(payload, SECRET_KEY, algorithm=ALGORITHM)

def create_refresh_token(user_id: int, token_family: str) -> str:
    payload = {
        "sub": str(user_id),
        "family": token_family,
        "exp": datetime.utcnow() + REFRESH_TOKEN_EXPIRE,
        "type": "refresh",
    }
    return jwt.encode(payload, SECRET_KEY, algorithm=ALGORITHM)
```

**Logout:** Удаление refresh token из Redis whitelist. Access token продолжит работать до истечения (15 мин) -- приемлемый компромисс для MVP. Для критичных операций -- проверка blacklist в Redis.

Источники: [FastAPI OAuth2 JWT](https://fastapi.tiangolo.com/tutorial/security/oauth2-jwt/), [JWT Refresh Token Rotation](https://choudharycodes.medium.com/title-securing-your-web-applications-with-jwt-authentication-and-refresh-token-rotation-63a9aa1a4b12), [FastAPI JWT Best Practices](https://medium.com/@jagan_reddy/jwt-in-fastapi-the-secure-way-refresh-tokens-explained-f7d2d17b1d17), [FastAPI Security Patterns PyCon 2026](https://us.pycon.org/2026/schedule/presentation/34/)

### 9.3 Session Management

- **Stateless auth** через JWT -- нет серверных сессий
- **Redis whitelist** для refresh tokens -- позволяет отзывать сессии
- **Multi-device:** каждое устройство получает свой refresh token с уникальной `token_family`
- **Принудительный logout:** удаление всех refresh tokens пользователя из Redis
- **Idle timeout:** refresh token не обновляется если пользователь неактивен > 7 дней

### 9.4 RBAC (Role-Based Access Control)

**Роли:**

| Роль | Описание | Создание аккаунта |
|------|----------|-------------------|
| `creator` | Креатор (UGC-автор) | Через Telegram Mini App |
| `brand` | Бренд/рекламодатель | Через Brand Dashboard (регистрация + верификация) |
| `admin` | Администратор платформы | Создаётся вручную через CLI/миграцию |

**Матрица разрешений:**

| Ресурс / Действие | Creator | Brand | Admin |
|--------------------|---------|-------|-------|
| Просмотр кампаний (опубликованных) | R | R | R |
| Создание кампании | - | C | C |
| Редактирование своей кампании | - | U | U |
| Удаление кампании | - | - | D |
| Подача заявки на кампанию | C | - | - |
| Просмотр своих заявок | R | - | R |
| Просмотр заявок на свою кампанию | - | R | R |
| Одобрение/отклонение заявки | - | U | U |
| Сдача работы (контента) | C | - | - |
| Просмотр сданных работ | R (свои) | R (своих кампаний) | R (все) |
| Модерация креаторов | - | - | CUD |
| Модерация кампаний | - | - | CUD |
| Просмотр аналитики платформы | - | - | R |
| Управление пользователями | - | - | CRUD |

**Реализация в FastAPI:**

```python
from enum import Enum
from fastapi import Depends, HTTPException

class Role(str, Enum):
    CREATOR = "creator"
    BRAND = "brand"
    ADMIN = "admin"

def require_role(*allowed_roles: Role):
    """Dependency для проверки роли пользователя."""
    async def role_checker(current_user=Depends(get_current_user)):
        if current_user.role not in allowed_roles:
            raise HTTPException(status_code=403, detail="Insufficient permissions")
        return current_user
    return role_checker

# Использование в эндпоинтах
@router.post("/campaigns")
async def create_campaign(
    data: CampaignCreate,
    user=Depends(require_role(Role.BRAND, Role.ADMIN)),
):
    ...

@router.post("/campaigns/{id}/apply")
async def apply_to_campaign(
    id: int,
    user=Depends(require_role(Role.CREATOR)),
):
    ...
```

### 9.5 Общая auth-логика для разных клиентов

Все три клиента обращаются к **одному FastAPI backend**. Разделение реализуется через:

1. **Единый `get_current_user` dependency** -- извлекает JWT из `Authorization: Bearer <token>` header, валидирует, возвращает объект пользователя
2. **Разные auth-эндпоинты** для каждого клиента:
   - `POST /api/auth/telegram` -- принимает initData, возвращает JWT
   - `POST /api/auth/login` -- email/password, возвращает JWT
   - `POST /api/auth/google` -- OAuth callback
   - `POST /api/auth/verify-2fa` -- TOTP верификация для админов
   - `POST /api/auth/refresh` -- обновление токенов (общий для всех)
3. **RBAC middleware** -- после аутентификации проверяет роль пользователя

### 9.6 Безопасность аутентификации

- **Token storage (TMA):** НЕ использовать localStorage (уязвим к XSS). Access token -- in-memory (JavaScript variable), refresh token -- CloudStorage (привязан к Telegram аккаунту)
- **Token storage (Dashboard):** Refresh token -- HttpOnly Secure SameSite=Strict cookie. Access token -- in-memory
- **CSRF:** SameSite=Strict cookie для refresh token. Для API endpoints -- не актуально (Bearer token в header)
- **XSS:** React экранирует вывод по умолчанию. CSP headers. Sanitization пользовательского ввода
- **Brute force:** Rate limiting на auth-эндпоинтах (5 попыток / минута на IP + email)
- **Password policy:** минимум 8 символов, bcrypt hash, проверка на утечки через haveibeenpwned API (опционально)

---

## 10. Схема базы данных (ключевые сущности)

### 10.1 Обзор сущностей и связей

```text
User (1) ──── (0..1) CreatorProfile
  │
  ├──── (0..1) BrandProfile
  │
  ├──── (N) Notification
  │
  └──── (N) AuditLog

BrandProfile (1) ──── (N) Campaign

Campaign (1) ──── (N) CampaignApplication
  │
  └──── (N) CampaignModeration

CampaignApplication (1) ──── (0..1) Contract
  │
  └──── (N) ContentSubmission

CreatorProfile (1) ──── (N) CampaignApplication
  │
  └──── (N) CreatorModeration

ContentSubmission (1) ──── (0..1) BrandFeedback
```

### 10.2 Список ключевых сущностей

| Сущность | Описание | Ключевые поля |
|----------|----------|---------------|
| **User** | Базовый аккаунт (все роли) | id, email, phone, role (ENUM), telegram_id, password_hash, totp_secret, google_id, is_active, created_at |
| **CreatorProfile** | Профиль креатора | user_id (FK), display_name, bio, city, instagram_handle, tiktok_handle, portfolio_urls (JSONB), iin_encrypted, address_encrypted, moderation_status |
| **BrandProfile** | Профиль бренда | user_id (FK), company_name, bin, industry, website, logo_url, is_verified |
| **Campaign** | Рекламная кампания | id, brand_id (FK), title, description, requirements (JSONB), content_type, budget_type (barter/paid), budget_amount, deadline, max_creators, status (ENUM), created_at |
| **CampaignApplication** | Заявка креатора на кампанию | id, campaign_id (FK), creator_id (FK), status (ENUM), cover_letter, applied_at, reviewed_at |
| **ContentSubmission** | Сданная работа | id, application_id (FK), content_url, content_type, caption, submitted_at, status (ENUM) |
| **Contract** | Договор (через TrustContract) | id, application_id (FK), document_url, trustcontract_id, signed_by_creator, signed_by_brand, signed_at |
| **Notification** | Уведомление пользователю | id, user_id (FK), type (ENUM), title, body, payload (JSONB), is_read, sent_via (telegram/email), created_at |
| **AuditLog** | Лог действий для аудита | id, user_id (FK), action, entity_type, entity_id, old_values (JSONB), new_values (JSONB), ip_address, created_at |
| **CreatorModeration** | Модерация креатора | id, creator_id (FK), moderator_id (FK), status (ENUM), rejection_reason, livedune_data (JSONB), moderated_at |
| **CampaignModeration** | Модерация кампании | id, campaign_id (FK), moderator_id (FK), status (ENUM), rejection_reason, moderated_at |
| **BrandFeedback** | Отзыв бренда о работе | id, submission_id (FK), rating (1-5), comment, created_at |

### 10.3 PostgreSQL-специфичные возможности

| Функция | Применение |
|---------|-----------|
| **ENUM types** | `user_role`, `campaign_status`, `application_status`, `submission_status`, `moderation_status` -- строгая типизация на уровне БД |
| **JSONB** | `campaign.requirements` (гибкие ТЗ), `creator_profile.portfolio_urls`, `notification.payload`, `audit_log.old_values/new_values` -- индексируемый JSON |
| **GIN Index** | Для JSONB-полей (поиск по `requirements`, фильтрация уведомлений) |
| **Partial Index** | `WHERE status = 'active'` на `campaign` -- ускорение поиска активных кампаний |
| **LISTEN/NOTIFY** | Real-time уведомления при изменении статусов заявок и контента |
| **pg_trgm** | Full-text поиск по имени креатора, названию кампании |
| **UUID** | Primary keys для публичных ресурсов (API IDs) -- безопаснее serial |

### 10.4 Ключевые индексы

```sql
-- Быстрый поиск пользователя по Telegram ID (для initData auth)
CREATE UNIQUE INDEX idx_user_telegram_id ON "user" (telegram_id) WHERE telegram_id IS NOT NULL;

-- Быстрый поиск пользователя по email (для email/password auth)
CREATE UNIQUE INDEX idx_user_email ON "user" (email) WHERE email IS NOT NULL;

-- Активные кампании (главный экран креатора)
CREATE INDEX idx_campaign_active ON campaign (deadline, created_at DESC)
    WHERE status = 'published';

-- Заявки креатора (история заявок)
CREATE INDEX idx_application_creator ON campaign_application (creator_id, applied_at DESC);

-- Заявки на кампанию (для бренда)
CREATE INDEX idx_application_campaign ON campaign_application (campaign_id, status);

-- Непрочитанные уведомления
CREATE INDEX idx_notification_unread ON notification (user_id, created_at DESC)
    WHERE is_read = false;

-- Аудит по сущности
CREATE INDEX idx_audit_entity ON audit_log (entity_type, entity_id, created_at DESC);

-- GIN индекс для JSONB поиска по требованиям кампании
CREATE INDEX idx_campaign_requirements ON campaign USING GIN (requirements);
```

### 10.5 Примеры SQLAlchemy моделей

```python
import uuid
from datetime import datetime
from enum import Enum as PyEnum
from sqlalchemy import (
    Column, String, Integer, Boolean, DateTime, ForeignKey,
    Text, Enum, Index, text
)
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy.orm import DeclarativeBase, relationship, Mapped, mapped_column


class Base(DeclarativeBase):
    pass


class UserRole(str, PyEnum):
    CREATOR = "creator"
    BRAND = "brand"
    ADMIN = "admin"


class CampaignStatus(str, PyEnum):
    DRAFT = "draft"
    PENDING_MODERATION = "pending_moderation"
    PUBLISHED = "published"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    CANCELLED = "cancelled"


class ApplicationStatus(str, PyEnum):
    PENDING = "pending"
    APPROVED = "approved"
    REJECTED = "rejected"
    WITHDRAWN = "withdrawn"


class User(Base):
    __tablename__ = "user"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    email: Mapped[str | None] = mapped_column(String(255), unique=True, nullable=True)
    phone: Mapped[str | None] = mapped_column(String(20), nullable=True)
    telegram_id: Mapped[int | None] = mapped_column(Integer, unique=True, nullable=True)
    password_hash: Mapped[str | None] = mapped_column(String(255), nullable=True)
    totp_secret: Mapped[str | None] = mapped_column(String(32), nullable=True)
    google_id: Mapped[str | None] = mapped_column(String(255), nullable=True)
    role: Mapped[UserRole] = mapped_column(
        Enum(UserRole, name="user_role"), nullable=False
    )
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    created_at: Mapped[datetime] = mapped_column(
        DateTime, server_default=text("NOW()")
    )

    creator_profile: Mapped["CreatorProfile"] = relationship(back_populates="user")
    brand_profile: Mapped["BrandProfile"] = relationship(back_populates="user")


class Campaign(Base):
    __tablename__ = "campaign"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    brand_id: Mapped[uuid.UUID] = mapped_column(
        ForeignKey("brand_profile.id"), nullable=False
    )
    title: Mapped[str] = mapped_column(String(255), nullable=False)
    description: Mapped[str] = mapped_column(Text, nullable=False)
    requirements: Mapped[dict] = mapped_column(JSONB, default=dict)
    budget_type: Mapped[str] = mapped_column(String(20), nullable=False)  # barter/paid
    budget_amount: Mapped[int | None] = mapped_column(Integer, nullable=True)
    deadline: Mapped[datetime] = mapped_column(DateTime, nullable=False)
    max_creators: Mapped[int] = mapped_column(Integer, default=10)
    status: Mapped[CampaignStatus] = mapped_column(
        Enum(CampaignStatus, name="campaign_status"),
        default=CampaignStatus.DRAFT,
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime, server_default=text("NOW()")
    )

    applications: Mapped[list["CampaignApplication"]] = relationship(
        back_populates="campaign"
    )

    __table_args__ = (
        Index("idx_campaign_active", "deadline", "created_at",
              postgresql_where=text("status = 'published'")),
    )


class CampaignApplication(Base):
    __tablename__ = "campaign_application"

    id: Mapped[uuid.UUID] = mapped_column(
        UUID(as_uuid=True), primary_key=True, default=uuid.uuid4
    )
    campaign_id: Mapped[uuid.UUID] = mapped_column(
        ForeignKey("campaign.id"), nullable=False
    )
    creator_id: Mapped[uuid.UUID] = mapped_column(
        ForeignKey("creator_profile.id"), nullable=False
    )
    status: Mapped[ApplicationStatus] = mapped_column(
        Enum(ApplicationStatus, name="application_status"),
        default=ApplicationStatus.PENDING,
    )
    cover_letter: Mapped[str | None] = mapped_column(Text, nullable=True)
    applied_at: Mapped[datetime] = mapped_column(
        DateTime, server_default=text("NOW()")
    )
    reviewed_at: Mapped[datetime | None] = mapped_column(DateTime, nullable=True)

    campaign: Mapped["Campaign"] = relationship(back_populates="applications")
```

Источники: [SQLAlchemy 2.0 Mapped Columns](https://docs.sqlalchemy.org/en/20/orm/mapped_sql_expr.html), [PostgreSQL JSONB](https://www.postgresql.org/docs/16/datatype-json.html), [PostgreSQL Partial Indexes](https://www.postgresql.org/docs/16/indexes-partial.html)

---

## 11. Безопасность (OWASP Top 10)

### 11.1 Обзор угроз и защиты

По данным OWASP 2025, уязвимости API составляют до 94% атак на веб-приложения. Ниже -- покрытие основных рисков в стеке UGCBoost.

### 11.2 Матрица угроз и контрмер

| OWASP-риск | Контрмера в UGCBoost | Реализация |
|------------|---------------------|------------|
| **A01: Broken Access Control** | RBAC middleware + owner checks | FastAPI dependencies: `require_role()`, проверка `user_id == resource.owner_id` |
| **A02: Cryptographic Failures** | Шифрование чувствительных данных | bcrypt для паролей, AES-256 для ИИН/адресов, TLS 1.3 |
| **A03: Injection** | Параметризованные запросы | SQLAlchemy ORM (никогда f-strings для SQL), Pydantic validation |
| **A04: Insecure Design** | Threat modeling, rate limiting | Лимиты на бизнес-операции (5 заявок/день, 3 кампании/день) |
| **A05: Security Misconfiguration** | Security headers, minimal exposure | CORS whitelist, отключение debug в production, minimal Docker images |
| **A06: Vulnerable Components** | Сканирование зависимостей | `pip-audit` + `safety` в CI/CD pipeline |
| **A07: Authentication Failures** | JWT + refresh rotation + 2FA | Описано в разделе 9 |
| **A08: Data Integrity Failures** | Подпись JWT, проверка initData | HMAC-SHA256 валидация, верификация webhook signatures |
| **A09: Logging & Monitoring** | Structured logging + audit trail | structlog + AuditLog table + Prometheus alerting |
| **A10: SSRF** | Валидация URL, whitelist | Проверка URL при загрузке контента, whitelist внешних API |

### 11.3 Input Validation (Pydantic)

FastAPI + Pydantic обеспечивают строгую валидацию на уровне API:

```python
from pydantic import BaseModel, Field, EmailStr, field_validator
import re

class CampaignCreate(BaseModel):
    title: str = Field(..., min_length=3, max_length=255)
    description: str = Field(..., min_length=10, max_length=5000)
    budget_type: Literal["barter", "paid"]
    budget_amount: int | None = Field(None, ge=0, le=10_000_000)
    max_creators: int = Field(default=10, ge=1, le=100)
    deadline: datetime

    @field_validator("title")
    @classmethod
    def sanitize_title(cls, v: str) -> str:
        # Удаление HTML-тегов и опасных символов
        return re.sub(r"<[^>]+>", "", v).strip()

class BrandRegistration(BaseModel):
    email: EmailStr
    password: str = Field(..., min_length=8, max_length=128)
    company_name: str = Field(..., min_length=2, max_length=255)
    bin: str = Field(..., pattern=r"^\d{12}$")  # БИН: 12 цифр
```

### 11.4 SQL Injection Prevention

- **SQLAlchemy ORM** использует параметризованные запросы по умолчанию
- **Запрет:** никогда не использовать `f"SELECT ... WHERE id = {user_input}"` или `.format()`
- **Raw SQL:** только через `text()` с bind parameters: `text("SELECT * FROM user WHERE id = :id").bindparams(id=user_id)`
- **Code review:** линтер `bandit` для обнаружения SQL injection в CI/CD

### 11.5 XSS Prevention

- **React** экранирует вывод по умолчанию (JSX → `React.createElement` → escaped HTML)
- **Запрет:** избегать `dangerouslySetInnerHTML` без санитизации
- **Content Security Policy (CSP)** header:
  ```text
  Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; connect-src 'self' https://api.ugcboost.kz
  ```
- **Для Telegram Mini App:** CSP адаптировать для WebView (разрешить Telegram SDK скрипты)

### 11.6 CSRF Protection

- **API endpoints (Bearer token):** CSRF не актуален -- токен передаётся в header, не в cookie
- **Refresh token cookie:** `SameSite=Strict` + `HttpOnly` + `Secure` flags
- **Next.js Dashboard:** CSRFToken middleware при использовании cookie-based auth

### 11.7 Rate Limiting

```python
# Стратегия rate limiting через Redis
RATE_LIMITS = {
    "auth/login": "5/minute",          # Защита от brute force
    "auth/telegram": "10/minute",      # initData validation
    "auth/refresh": "10/minute",       # Token refresh
    "campaigns/apply": "5/hour",       # Подача заявок
    "campaigns/create": "3/hour",      # Создание кампаний
    "content/submit": "10/hour",       # Сдача работ
    "default": "60/minute",            # Общие эндпоинты
}
```

Реализация через `slowapi` (обёртка над `limits`) или кастомный middleware с Redis:

```python
from slowapi import Limiter
from slowapi.util import get_remote_address

limiter = Limiter(key_func=get_remote_address, storage_uri="redis://redis:6379")

@router.post("/auth/login")
@limiter.limit("5/minute")
async def login(request: Request, ...):
    ...
```

### 11.8 Шифрование чувствительных данных

**Данные, требующие шифрования:**
- ИИН креатора -- AES-256-GCM (column-level encryption)
- Адрес креатора -- AES-256-GCM
- TOTP секреты администраторов -- AES-256-GCM

**Реализация:**
```python
from cryptography.fernet import Fernet

# Ключ шифрования -- из переменной окружения, НЕ в коде
ENCRYPTION_KEY = os.environ["ENCRYPTION_KEY"]
cipher = Fernet(ENCRYPTION_KEY)

def encrypt_iin(iin: str) -> str:
    return cipher.encrypt(iin.encode()).decode()

def decrypt_iin(encrypted_iin: str) -> str:
    return cipher.decrypt(encrypted_iin.encode()).decode()
```

**Важно:** Ключ шифрования хранится в переменной окружения, ротируется при компрометации. Backup ключей -- отдельно от backup базы данных.

### 11.9 API Security Headers

```python
from fastapi.middleware.cors import CORSMiddleware
from starlette.middleware import Middleware

# CORS -- строгий whitelist
app.add_middleware(
    CORSMiddleware,
    allow_origins=[
        "https://dashboard.ugcboost.kz",
        "https://admin.ugcboost.kz",
    ],
    allow_credentials=True,
    allow_methods=["GET", "POST", "PUT", "DELETE"],
    allow_headers=["Authorization", "Content-Type"],
)

# Security headers middleware
@app.middleware("http")
async def add_security_headers(request, call_next):
    response = await call_next(request)
    response.headers["X-Content-Type-Options"] = "nosniff"
    response.headers["X-Frame-Options"] = "DENY"
    response.headers["X-XSS-Protection"] = "1; mode=block"
    response.headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains"
    response.headers["Referrer-Policy"] = "strict-origin-when-cross-origin"
    return response
```

### 11.10 File Upload Security

Для загрузки контента (видео, фото) от креаторов:
- **Проверка MIME type** (magic bytes, не только расширение): разрешить `image/jpeg`, `image/png`, `video/mp4`
- **Ограничение размера:** 50 MB для видео, 10 MB для фото
- **Переименование файлов:** UUID-based имена, без сохранения оригинального имени
- **Хранение:** отдельная директория / S3-совместимое хранилище (Hetzner Object Storage)
- **Антивирусная проверка:** опционально через ClamAV для production

### 11.11 Сканирование зависимостей

```yaml
# GitHub Actions: проверка уязвимостей зависимостей
- name: Audit Python dependencies
  run: |
    pip install pip-audit safety
    pip-audit --strict
    safety check --full-report
```

Запускать при каждом PR и еженедельно по расписанию.

Источники: [OWASP Top 10 2025](https://juanrodriguezmonti.github.io/blog/owasp-top-10-2025/), [FastAPI OWASP Security](https://oneuptime.com/blog/post/2025-01-06-fastapi-owasp-security/view), [FastAPI Security Guide](https://www.shipsafer.app/blog/fastapi-security-guide), [OWASP API Top 10](https://www.pynt.io/learning-hub/owasp-top-10-guide/owasp-api-top-10)

---

## 12. Оценка серверных ресурсов для MVP

### 12.1 Ожидаемая нагрузка MVP

| Параметр | Оценка | Комментарий |
|----------|--------|-------------|
| Креаторы | 200-500 | Первые 3-6 месяцев |
| Бренды | 5-10 | Ручное привлечение |
| Активные кампании | 3-10 одновременно | |
| Запросов/день (API) | 1 000 - 3 000 | ~1-2 RPS в пиковые часы |
| Параллельных пользователей (пик) | 20-50 | Вечернее время, Алматы/Астана |
| Telegram Bot сообщений/день | 500-2 000 | Уведомления, напоминания |
| LiveDune API запросов/месяц | 1 000-3 000 | Модерация + периодический refresh |

### 12.2 Рекомендуемая конфигурация Hetzner Cloud

**Вариант 1: Минимальный MVP (один сервер)**

| Компонент | Спецификация | Цена (апрель 2026, после повышения) |
|-----------|-------------|--------------------------------------|
| Сервер | CX32: 4 vCPU, 8 GB RAM, 80 GB SSD | ~8,49 EUR/мес |
| Volumes | 20 GB дополнительно (для бэкапов) | ~1,10 EUR/мес |
| **Итого** | | **~9,59 EUR/мес (~4 700 KZT)** |

На одном сервере через Docker Compose: FastAPI + PostgreSQL + Redis + Nginx + Telegram Bot.

**Вариант 2: Рекомендуемый MVP (два сервера)**

| Компонент | Спецификация | Цена (апрель 2026, после повышения) |
|-----------|-------------|--------------------------------------|
| App server | CX22: 2 vCPU, 4 GB RAM, 40 GB SSD | ~4,49 EUR/мес |
| DB server | CX22: 2 vCPU, 4 GB RAM, 40 GB SSD | ~4,49 EUR/мес |
| Volumes | 40 GB (бэкапы БД) | ~2,20 EUR/мес |
| **Итого** | | **~11,18 EUR/мес (~5 500 KZT)** |

Разделение app и DB -- лучшая изоляция, независимое масштабирование.

**Примечание:** Цены Hetzner Cloud повышены на 25-33% с 1 апреля 2026 (CX22: 3,79 -> 4,49 EUR, CX32: 6,80 -> 8,49 EUR). Актуальные цены: [hetzner.com/cloud](https://www.hetzner.com/cloud).

### 12.3 Проекции размера базы данных

| Таблица | Записей за 6 мес | Примерный размер |
|---------|------------------|-----------------|
| User | 700 | < 1 MB |
| CreatorProfile | 500 | < 1 MB |
| Campaign | 50 | < 1 MB |
| CampaignApplication | 2 000 | < 5 MB |
| ContentSubmission | 500 | < 2 MB (метаданные, не файлы) |
| Notification | 20 000 | ~10 MB |
| AuditLog | 50 000 | ~30 MB |
| **Итого (данные)** | | **~50 MB** |
| Индексы | | ~20 MB |
| **Общий размер БД** | | **~70 MB** |

PostgreSQL с 40 GB SSD -- более чем достаточно на 2+ года.

### 12.4 Требования к Redis

| Данные в Redis | Примерный размер |
|---------------|-----------------|
| Refresh tokens (700 пользователей) | < 1 MB |
| API rate limiting counters | < 1 MB |
| LiveDune API cache | < 5 MB |
| Celery/arq task queue | < 1 MB |
| **Итого** | **< 10 MB** |

Redis с 256 MB -- более чем достаточно. По умолчанию Redis использует до `maxmemory` оперативной памяти сервера.

### 12.5 Когда масштабировать

| Сигнал | Действие |
|--------|----------|
| Среднее время ответа API > 500 мс | Профилирование запросов, добавление индексов, кэширование |
| CPU usage > 70% sustained | Апгрейд на CX32 / CPX22 |
| RAM usage > 80% | Апгрейд сервера или вынос Redis на отдельный сервер |
| 10 000+ активных пользователей | Рассмотреть managed PostgreSQL + отдельный Redis server |
| 50+ RPS sustained | Load balancer + 2 app-сервера |
| 100 000+ пользователей | Kubernetes (k3s) + managed DB |

Источники: [Hetzner Cloud Pricing](https://www.hetzner.com/cloud), [Hetzner New CX Plans](https://www.hetzner.com/news/new-cx-plans/), [Hetzner Price Adjustment April 2026](https://docs.hetzner.com/general/infrastructure-and-availability/price-adjustment/)

---

## 13. Архитектура Telegram-бота

### 13.1 aiogram 3.x: архитектура

aiogram 3.x (актуальная стабильная версия: **3.26.0**, март 2026) -- асинхронный фреймворк для Telegram Bot API на Python 3.10+. Основные компоненты:

**Dispatcher** -- центральный объект, принимает обновления от Telegram и маршрутизирует их.

**Router** -- модульная организация хэндлеров. Каждый Router -- логический модуль бота:

```python
from aiogram import Router

# Модульная структура бота
notifications_router = Router(name="notifications")
campaigns_router = Router(name="campaigns")
admin_router = Router(name="admin")
common_router = Router(name="common")

# Подключение к Dispatcher
dp = Dispatcher()
dp.include_routers(
    common_router,
    campaigns_router,
    notifications_router,
    admin_router,
)
```

**Middlewares** -- два уровня: outer (до фильтров) и inner (после фильтров, до хэндлера):

```python
from aiogram import BaseMiddleware
from typing import Any, Awaitable, Callable, Dict
from aiogram.types import TelegramObject

class DatabaseMiddleware(BaseMiddleware):
    """Middleware для инъекции DB-сессии в каждый хэндлер."""

    def __init__(self, session_factory):
        self.session_factory = session_factory

    async def __call__(
        self,
        handler: Callable[[TelegramObject, Dict[str, Any]], Awaitable[Any]],
        event: TelegramObject,
        data: Dict[str, Any],
    ) -> Any:
        async with self.session_factory() as session:
            data["db_session"] = session
            return await handler(event, data)

# Регистрация
dp.update.outer_middleware(DatabaseMiddleware(async_session_factory))
```

**Filters** -- декларативные фильтры для маршрутизации обновлений по условиям.

Источники: [aiogram Routers](https://mastergroosha.github.io/aiogram-3-guide/routers/), [aiogram Middlewares](https://docs.aiogram.dev/en/latest/dispatcher/middlewares.html), [aiogram Migration 2.x->3.x](https://docs.aiogram.dev/en/dev-3.x/migration_2_to_3.html)

### 13.2 Webhook vs Long Polling

| Критерий | Webhook | Long Polling |
|----------|---------|-------------|
| Механизм | Telegram отправляет HTTP POST на наш URL | Бот периодически запрашивает обновления |
| Требования | HTTPS URL, открытый порт | Нет особых требований |
| Задержка | Мгновенная доставка | ~1-2 секунды |
| Масштабируемость | Высокая (можно обрабатывать параллельно) | Ограничена одним процессом на токен |
| Надёжность | Telegram повторяет доставку при ошибке | Потеря обновлений при длительном даунтайме |
| Разработка | Сложнее (нужен HTTPS, настройка URL) | Проще (запуск одной командой) |
| Production | **Рекомендуется** | Для разработки/отладки |

**Рекомендация для UGCBoost:**
- **Development:** Long Polling (простота, не нужен публичный URL)
- **Production:** Webhook через интеграцию с FastAPI (aiohttp handler)

```python
# Production: Webhook через aiohttp, интегрированный с FastAPI
from aiogram.webhook.aiohttp_server import SimpleRequestHandler, setup_application
from aiohttp import web

WEBHOOK_URL = "https://api.ugcboost.kz/bot/webhook"
WEBHOOK_SECRET = "your-webhook-secret"

async def on_startup(bot: Bot):
    await bot.set_webhook(
        url=WEBHOOK_URL,
        secret_token=WEBHOOK_SECRET,
        allowed_updates=["message", "callback_query"],
    )

# В Docker Compose: FastAPI на :8000, aiohttp webhook handler на :8080
# Nginx проксирует /bot/webhook -> :8080
```

Источники: [aiogram Webhook Docs](https://docs.aiogram.dev/en/latest/dispatcher/webhook.html), [aiogram Long Polling](https://docs.aiogram.dev/en/latest/dispatcher/long_polling.html)

### 13.3 Структура команд бота для UGCBoost

| Команда | Описание | Доступ |
|---------|----------|--------|
| `/start` | Приветствие + открытие Mini App | Все |
| `/start campaign_{id}` | Deep link -- открытие конкретной кампании | Все |
| `/help` | Справка по боту | Все |
| `/profile` | Быстрый просмотр профиля креатора | Креаторы |
| `/campaigns` | Список активных кампаний (inline button → Mini App) | Креаторы |
| `/my_applications` | Мои заявки (inline button → Mini App) | Креаторы |
| `/support` | Связь с поддержкой | Все |

**Важно:** Основной UX -- через Mini App. Бот используется для:
1. Точки входа (`/start`, deep links)
2. Уведомлений (push-сообщения)
3. Быстрых команд для частых действий

### 13.4 Шаблоны уведомлений

```python
# Telegram поддерживает HTML и Markdown разметку в сообщениях.
# Рекомендация: использовать HTML (более предсказуемое экранирование).

TEMPLATES = {
    "application_approved": (
        "✅ <b>Заявка одобрена!</b>\n\n"
        "Кампания: <b>{campaign_title}</b>\n"
        "Бренд: {brand_name}\n"
        "Дедлайн: {deadline}\n\n"
        "Откройте приложение для подписания договора."
    ),
    "application_rejected": (
        "❌ <b>Заявка отклонена</b>\n\n"
        "Кампания: <b>{campaign_title}</b>\n"
        "Причина: {reason}\n\n"
        "Не расстраивайтесь — новые кампании появляются каждую неделю!"
    ),
    "deadline_reminder": (
        "⏰ <b>Напоминание о дедлайне</b>\n\n"
        "Кампания: <b>{campaign_title}</b>\n"
        "Осталось: <b>{days_left} дн.</b>\n"
        "Дедлайн: {deadline}\n\n"
        "Не забудьте сдать контент вовремя!"
    ),
    "new_campaign": (
        "🆕 <b>Новая кампания!</b>\n\n"
        "<b>{campaign_title}</b>\n"
        "{description_preview}\n\n"
        "Тип: {budget_type}\n"
        "Дедлайн: {deadline}"
    ),
    "content_feedback": (
        "💬 <b>Отзыв о вашей работе</b>\n\n"
        "Кампания: <b>{campaign_title}</b>\n"
        "Оценка: {'⭐' * rating}\n"
        "Комментарий: {comment}"
    ),
}

# Отправка с inline-кнопкой для перехода в Mini App
from aiogram.types import InlineKeyboardMarkup, InlineKeyboardButton

def campaign_keyboard(campaign_id: str) -> InlineKeyboardMarkup:
    return InlineKeyboardMarkup(inline_keyboard=[[
        InlineKeyboardButton(
            text="📱 Открыть в приложении",
            url=f"https://t.me/ugcboost_bot/app?startapp=campaign_{campaign_id}",
        )
    ]])
```

### 13.5 Rate Limits Telegram Bot API

| Лимит | Значение | Контекст |
|-------|----------|----------|
| Сообщений в секунду (broadcast) | 30 msg/sec (бесплатно) | Массовая рассылка разным пользователям |
| Платный broadcast | 1 000 msg/sec | Стоимость: 0.1 Stars/msg свыше 30/sec, минимум 10 000 Stars на балансе |
| Сообщений в группу | 20 msg/min | В одну группу |
| Сообщений в один чат | ~1 msg/sec | Личные чаты (рекомендация, не жёсткий лимит) |
| Ответ на callback_query | 30 секунд | После этого кнопка перестаёт быть кликабельной |

**Для UGCBoost MVP (200-500 креаторов):** 30 msg/sec бесплатного лимита более чем достаточно. Рассылка 500 уведомлений займёт ~17 секунд.

### 13.6 Обработка ошибок и retry-стратегия

```python
from aiogram.exceptions import TelegramRetryAfter, TelegramForbiddenError
import asyncio

async def safe_send_message(bot: Bot, chat_id: int, text: str, **kwargs) -> bool:
    """Отправка сообщения с обработкой ошибок и retry."""
    max_retries = 3
    for attempt in range(max_retries):
        try:
            await bot.send_message(chat_id=chat_id, text=text, **kwargs)
            return True
        except TelegramRetryAfter as e:
            # Rate limit -- ждём указанное время
            await asyncio.sleep(e.retry_after)
        except TelegramForbiddenError:
            # Пользователь заблокировал бота -- обновляем статус в БД
            await mark_user_bot_blocked(chat_id)
            return False
        except Exception as e:
            if attempt < max_retries - 1:
                await asyncio.sleep(2 ** attempt)  # Exponential backoff
            else:
                logger.error(f"Failed to send to {chat_id}: {e}")
                return False
    return False

async def broadcast_notification(
    bot: Bot, user_ids: list[int], text: str, **kwargs
):
    """Массовая рассылка с учётом rate limits."""
    for i, chat_id in enumerate(user_ids):
        await safe_send_message(bot, chat_id, text, **kwargs)
        # Не превышать 30 msg/sec
        if (i + 1) % 25 == 0:
            await asyncio.sleep(1)
```

Источники: [Telegram Bot FAQ Rate Limits](https://core.telegram.org/bots/faq), [Telegram Bot API](https://core.telegram.org/bots/api), [grammY Flood Limits](https://grammy.dev/advanced/flood)

---

## 14. Развёртывание и CI/CD

### 14.1 Структура Docker Compose

```yaml
# docker-compose.yml (production)
version: "3.9"

services:
  # --- Application ---
  api:
    build:
      context: ./backend
      dockerfile: Dockerfile
    ports:
      - "8000:8000"
    environment:
      - DATABASE_URL=postgresql+asyncpg://ugcboost:${DB_PASSWORD}@db:5432/ugcboost
      - REDIS_URL=redis://redis:6379/0
      - BOT_TOKEN=${BOT_TOKEN}
      - JWT_SECRET=${JWT_SECRET}
      - ENCRYPTION_KEY=${ENCRYPTION_KEY}
    depends_on:
      db:
        condition: service_healthy
      redis:
        condition: service_healthy
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8000/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  # --- Telegram Bot (webhook handler) ---
  bot:
    build:
      context: ./backend
      dockerfile: Dockerfile.bot
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgresql+asyncpg://ugcboost:${DB_PASSWORD}@db:5432/ugcboost
      - REDIS_URL=redis://redis:6379/0
      - BOT_TOKEN=${BOT_TOKEN}
      - WEBHOOK_SECRET=${WEBHOOK_SECRET}
    depends_on:
      - db
      - redis
    restart: unless-stopped

  # --- Background Worker (Celery/arq) ---
  worker:
    build:
      context: ./backend
      dockerfile: Dockerfile
    command: arq app.worker.WorkerSettings
    environment:
      - DATABASE_URL=postgresql+asyncpg://ugcboost:${DB_PASSWORD}@db:5432/ugcboost
      - REDIS_URL=redis://redis:6379/0
      - BOT_TOKEN=${BOT_TOKEN}
    depends_on:
      - db
      - redis
    restart: unless-stopped

  # --- Database ---
  db:
    image: postgres:16-alpine
    volumes:
      - postgres_data:/var/lib/postgresql/data
    environment:
      - POSTGRES_DB=ugcboost
      - POSTGRES_USER=ugcboost
      - POSTGRES_PASSWORD=${DB_PASSWORD}
    ports:
      - "127.0.0.1:5432:5432"  # Только localhost
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ugcboost"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  # --- Cache / Queue ---
  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${REDIS_PASSWORD} --maxmemory 256mb --maxmemory-policy allkeys-lru
    volumes:
      - redis_data:/data
    ports:
      - "127.0.0.1:6379:6379"
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"]
      interval: 10s
      timeout: 5s
      retries: 5
    restart: unless-stopped

  # --- Reverse Proxy ---
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - ./frontend/dist:/usr/share/nginx/html/tma:ro
    depends_on:
      - api
      - bot
    restart: unless-stopped

volumes:
  postgres_data:
  redis_data:
```

**Контейнеры (6 штук):** api, bot, worker, db, redis, nginx.

### 14.2 Управление переменными окружения

**Структура .env файлов:**
```text
.env.example          # Шаблон (в git)
.env                  # Production (НЕ в git, на сервере)
.env.staging          # Staging (НЕ в git, на сервере)
.env.development      # Разработка (НЕ в git, у разработчика)
```

**Секреты:**
| Переменная | Описание | Генерация |
|-----------|----------|-----------|
| `DB_PASSWORD` | Пароль PostgreSQL | `openssl rand -hex 32` |
| `REDIS_PASSWORD` | Пароль Redis | `openssl rand -hex 32` |
| `JWT_SECRET` | Секрет для подписи JWT | `openssl rand -hex 64` |
| `ENCRYPTION_KEY` | Ключ шифрования данных | `python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"` |
| `BOT_TOKEN` | Telegram Bot Token | Из @BotFather |
| `WEBHOOK_SECRET` | Секрет для webhook validation | `openssl rand -hex 32` |
| `LIVEDUNE_API_KEY` | API-ключ LiveDune | Из личного кабинета |

**Правило:** Файлы `.env*` (кроме `.env.example`) добавлены в `.gitignore`.

### 14.3 Staging vs Production

| Аспект | Staging | Production |
|--------|---------|-----------|
| URL | staging-api.ugcboost.kz | api.ugcboost.kz |
| Сервер | CX22 (Hetzner, ~4,49 EUR/мес) | CX32 (Hetzner, ~8,49 EUR/мес) |
| База данных | Отдельная PostgreSQL | Отдельная PostgreSQL |
| Telegram Bot | Отдельный бот (@ugcboost_stage_bot) | Основной бот (@ugcboost_bot) |
| LiveDune | Trial/Freemium тариф | Business тариф |
| TrustContract | Sandbox | Production |
| SSL | Let's Encrypt | Let's Encrypt |
| Deploy | Auto-deploy из ветки `develop` | Manual approve, deploy из `main` |

### 14.4 Миграции базы данных (Alembic)

```bash
# Структура
backend/
├── alembic/
│   ├── versions/           # Файлы миграций
│   ├── env.py             # Конфигурация Alembic
│   └── script.py.mako     # Шаблон миграции
├── alembic.ini
└── app/models/            # SQLAlchemy модели
```

**Workflow:**
```bash
# Создание миграции
alembic revision --autogenerate -m "add campaign table"

# Применение миграции (в Docker)
docker compose exec api alembic upgrade head

# Откат миграции
docker compose exec api alembic downgrade -1

# Просмотр текущей версии
docker compose exec api alembic current
```

**Правила:**
- Миграции хранятся в Git (версионированы)
- Autogenerate проверять вручную перед коммитом (может пропустить изменения)
- В CI/CD: `alembic upgrade head` запускается **до** старта нового контейнера API
- Деструктивные миграции (DROP COLUMN) -- отдельным PR с ревью

### 14.5 Стратегия бэкапов PostgreSQL

**Ежедневный бэкап через cron + pg_dump:**

```bash
#!/bin/bash
# /opt/ugcboost/scripts/backup.sh
BACKUP_DIR="/opt/ugcboost/backups"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RETAIN_DAYS=14

# Дамп через Docker
docker compose exec -T db pg_dump -U ugcboost -Fc ugcboost \
    > "${BACKUP_DIR}/ugcboost_${TIMESTAMP}.dump"

# Удаление старых бэкапов (старше 14 дней)
find ${BACKUP_DIR} -name "*.dump" -mtime +${RETAIN_DAYS} -delete

# Опционально: копирование на Hetzner Storage Box или S3
# rclone copy ${BACKUP_DIR}/ugcboost_${TIMESTAMP}.dump remote:backups/
```

**Расписание:**
| Бэкап | Частота | Хранение |
|-------|---------|----------|
| pg_dump (полный дамп) | Ежедневно, 03:00 UTC | 14 дней |
| Hetzner Snapshots (сервер) | Еженедельно | 4 снапшота |
| Volumes Backup | Ежедневно (Hetzner) | 7 дней |

**Восстановление:**
```bash
# Из pg_dump
docker compose exec -T db pg_restore -U ugcboost -d ugcboost --clean < backup.dump
```

**Для MVP:** pg_dump + cron достаточно. При росте БД > 1 GB -- рассмотреть pgBackRest для инкрементальных бэкапов.

Источники: [PostgreSQL Docker Backup](https://dev.to/piteradyson/postgresql-docker-backup-strategies-how-to-backup-postgresql-running-in-docker-containers-1bla), [Automated pg_dump](https://serversinc.io/blog/automated-postgresql-backups-in-docker-complete-guide-with-pg-dump/)

### 14.6 Zero-Downtime Deployment

Для MVP с Docker Compose -- упрощённый подход:

1. **Pull новый образ** на сервере
2. **Запустить миграции** (`alembic upgrade head`)
3. **Rolling restart:**
   ```bash
   # Обновляем app-контейнеры по одному (worker не обрабатывает HTTP)
   docker compose up -d --no-deps --build api
   # Ждём healthcheck
   sleep 10
   docker compose up -d --no-deps --build bot
   docker compose up -d --no-deps --build worker
   ```
4. **Nginx healthcheck** перенаправляет трафик на healthy контейнер

**Для production-grade zero-downtime:** Traefik вместо Nginx (автоматический blue-green routing) или Docker Swarm mode с `--update-delay`.

### 14.7 Стратегия логирования

**Библиотека:** `structlog` -- структурированные логи в JSON-формате.

**Конфигурация по окружению:**
- **Development:** Красивый human-readable вывод в консоль (цветной)
- **Production:** JSON-формат для машинной обработки

```python
import structlog

def configure_logging(environment: str):
    shared_processors = [
        structlog.stdlib.add_log_level,
        structlog.stdlib.add_logger_name,
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.StackInfoRenderer(),
    ]

    if environment == "production":
        structlog.configure(
            processors=[
                *shared_processors,
                structlog.processors.format_exc_info,
                structlog.processors.JSONRenderer(),
            ],
            logger_factory=structlog.stdlib.LoggerFactory(),
        )
    else:
        structlog.configure(
            processors=[
                *shared_processors,
                structlog.dev.ConsoleRenderer(colors=True),
            ],
            logger_factory=structlog.stdlib.LoggerFactory(),
        )

# Использование
logger = structlog.get_logger()

logger.info(
    "campaign_created",
    campaign_id=str(campaign.id),
    brand_id=str(brand.id),
    title=campaign.title,
)
# Production output:
# {"event": "campaign_created", "campaign_id": "...", "brand_id": "...",
#  "title": "...", "level": "info", "timestamp": "2026-04-04T12:00:00Z"}
```

**Correlation ID** -- для трассировки запросов:
```python
from asgi_correlation_id import CorrelationIdMiddleware

app.add_middleware(CorrelationIdMiddleware)
# Каждый HTTP-запрос получает уникальный X-Request-ID, который пробрасывается
# через все логи, связанные с этим запросом.
```

**Агрегация логов (Post-MVP):**
- **Вариант 1:** Loki + Grafana (open source, интеграция с Prometheus)
- **Вариант 2:** stdout в Docker → Promtail → Loki → Grafana
- **Вариант 3:** Managed: Betterstack Logs (бесплатный tier: 1 GB/мес)

Источники: [structlog + FastAPI](https://ouassim.tech/notes/setting-up-structured-logging-in-fastapi-with-structlog/), [Production-Grade FastAPI Logging](https://medium.com/@laxsuryavanshi.dev/production-grade-logging-for-fastapi-applications-a-complete-guide-f384d4b8f43b), [asgi-correlation-id](https://gist.github.com/nymous/f138c7f06062b7c43c060bf03759c29e)

### 14.8 CI/CD Pipeline (GitHub Actions)

```yaml
# .github/workflows/ci.yml
name: CI/CD

on:
  push:
    branches: [main, develop]
  pull_request:
    branches: [main]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install ruff mypy
      - run: ruff check backend/
      - run: mypy backend/app/

  test:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_DB: test_ugcboost
          POSTGRES_USER: test
          POSTGRES_PASSWORD: test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      redis:
        image: redis:7-alpine
        ports:
          - 6379:6379
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-python@v5
        with:
          python-version: "3.12"
      - run: pip install -r backend/requirements-dev.txt
      - run: pytest backend/ --cov=app --cov-report=xml
      - uses: codecov/codecov-action@v4

  security:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: pip install pip-audit safety bandit
      - run: pip-audit --strict -r backend/requirements.txt
      - run: bandit -r backend/app/ -ll

  deploy-staging:
    needs: [lint, test, security]
    if: github.ref == 'refs/heads/develop'
    runs-on: ubuntu-latest
    steps:
      - name: Deploy to staging
        run: |
          ssh deploy@staging.ugcboost.kz "cd /opt/ugcboost && git pull && docker compose up -d --build"

  deploy-production:
    needs: [lint, test, security]
    if: github.ref == 'refs/heads/main'
    runs-on: ubuntu-latest
    environment: production  # Requires manual approval
    steps:
      - name: Deploy to production
        run: |
          ssh deploy@api.ugcboost.kz "cd /opt/ugcboost && git pull && ./scripts/deploy.sh"
```

---

## 15. Стратегия тестирования

### 15.1 Подход и инструменты

| Уровень | Инструмент | Описание |
|---------|-----------|----------|
| Unit tests | pytest + pytest-asyncio | Тестирование бизнес-логики, сервисов, утилит |
| Integration tests | pytest + testcontainers | Тесты с реальной PostgreSQL и Redis в Docker |
| API tests | httpx.AsyncClient (FastAPI TestClient) | Тестирование эндпоинтов с полным request/response cycle |
| E2E tests | Playwright (Post-MVP) | Тестирование UI в Brand Dashboard |
| TMA E2E | Manual + Telegram Test Environment | Telegram Mini App тестирование в Test Environment |

### 15.2 Unit Testing

```python
# tests/unit/test_auth.py
import pytest
from app.auth.telegram import validate_telegram_init_data

def test_valid_init_data():
    """Тест валидации корректной initData."""
    init_data = "query_id=AAH...&user=%7B%22id%22%3A123%7D&auth_date=1700000000&hash=abc123..."
    result = validate_telegram_init_data(init_data, bot_token="test-token")
    assert result["user"]["id"] == 123

def test_invalid_hash():
    """Тест отклонения initData с невалидным hash."""
    with pytest.raises(HTTPException) as exc_info:
        validate_telegram_init_data("hash=invalid&auth_date=1700000000")
    assert exc_info.value.status_code == 401

# tests/unit/test_campaign_service.py
import pytest
from app.services.campaign import CampaignService

@pytest.mark.asyncio
async def test_create_campaign():
    """Тест создания кампании."""
    service = CampaignService(db_session=mock_session)
    campaign = await service.create(
        brand_id=uuid4(),
        title="Test Campaign",
        deadline=datetime.utcnow() + timedelta(days=30),
    )
    assert campaign.status == CampaignStatus.DRAFT
```

### 15.3 Integration Testing (testcontainers)

```python
# tests/conftest.py
import pytest
from testcontainers.postgres import PostgresContainer
from testcontainers.redis import RedisContainer
from httpx import AsyncClient, ASGITransport
from sqlalchemy.ext.asyncio import create_async_engine, async_sessionmaker

@pytest.fixture(scope="session")
def postgres_container():
    """PostgreSQL контейнер для всех тестов сессии."""
    with PostgresContainer("postgres:16-alpine") as pg:
        yield pg

@pytest.fixture(scope="session")
def redis_container():
    """Redis контейнер для всех тестов сессии."""
    with RedisContainer("redis:7-alpine") as redis:
        yield redis

@pytest.fixture
async def db_session(postgres_container):
    """Async DB сессия с транзакционной изоляцией."""
    url = postgres_container.get_connection_url().replace(
        "postgresql://", "postgresql+asyncpg://"
    )
    engine = create_async_engine(url)
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    async with session_factory() as session:
        yield session
        await session.rollback()

@pytest.fixture
async def client(db_session, redis_container):
    """HTTP-клиент для тестирования API."""
    app = create_app(db_session=db_session)
    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as ac:
        yield ac
```

### 15.4 API Testing

```python
# tests/api/test_campaigns.py
import pytest
from httpx import AsyncClient

@pytest.mark.asyncio
async def test_list_campaigns(client: AsyncClient, auth_headers: dict):
    """Тест получения списка кампаний."""
    response = await client.get("/api/campaigns", headers=auth_headers)
    assert response.status_code == 200
    data = response.json()
    assert "items" in data
    assert isinstance(data["items"], list)

@pytest.mark.asyncio
async def test_create_campaign_requires_brand_role(
    client: AsyncClient, creator_auth_headers: dict
):
    """Тест: креатор не может создавать кампании."""
    response = await client.post(
        "/api/campaigns",
        json={"title": "Test", "deadline": "2026-05-01T00:00:00Z"},
        headers=creator_auth_headers,
    )
    assert response.status_code == 403

@pytest.mark.asyncio
async def test_apply_to_campaign(
    client: AsyncClient, creator_auth_headers: dict, published_campaign_id: str
):
    """Тест подачи заявки на кампанию."""
    response = await client.post(
        f"/api/campaigns/{published_campaign_id}/apply",
        json={"cover_letter": "I would love to participate!"},
        headers=creator_auth_headers,
    )
    assert response.status_code == 201
    assert response.json()["status"] == "pending"
```

### 15.5 E2E Testing для Telegram Mini App

**Сложности:**
- Mini App работает в WebView Telegram -- нет стандартного браузерного окружения
- initData генерируется Telegram -- нельзя подделать в обычном браузере

**Стратегия:**
1. **Component tests:** тестировать React-компоненты TMA изолированно (Vitest + Testing Library)
2. **API mock:** мокировать `@telegram-apps/sdk-react` хуки для unit-тестов компонентов
3. **Telegram Test Environment:** Telegram предоставляет [Test Environment](https://core.telegram.org/bots/webapps#testing-mini-apps) для тестирования Mini Apps в режиме разработки
4. **Manual E2E:** чеклист ручного тестирования в Telegram Desktop/Mobile для каждого релиза

### 15.6 Целевое покрытие тестами

| Уровень | Покрытие (цель) | Приоритет |
|---------|----------------|-----------|
| Unit tests (бизнес-логика) | > 80% | Высокий |
| API tests (основные flows) | > 70% | Высокий |
| Integration tests (DB) | Ключевые queries | Средний |
| E2E (Dashboard) | Happy paths | Низкий (Post-MVP) |
| E2E (TMA) | Manual checklist | Низкий (Post-MVP) |

**Минимум для CI:** unit tests + API tests проходят перед мёржем в `main`.

Источники: [FastAPI Testing](https://testdriven.io/blog/fastapi-crud/), [Testcontainers Python](https://testcontainers.com/guides/getting-started-with-testcontainers-for-python/), [Testcontainers + FastAPI + asyncpg](https://github.com/lealre/fastapi-testcontainer-asyncpg), [Async FastAPI Testing](https://weirdsheeplabs.com/blog/fast-and-furious-async-testing-with-fastapi-and-pytest/)

---

## 16. Сводка технических решений

### Финальные рекомендации для MVP

| Компонент | Решение | Обоснование |
|-----------|---------|-------------|
| Backend | FastAPI + Python 3.12+ | Скорость разработки, async, экосистема |
| ORM | SQLAlchemy 2.0 + asyncpg | Зрелость, async, миграции через Alembic |
| Database | PostgreSQL 16 | Реляционные данные, JSONB, LISTEN/NOTIFY |
| Cache | Redis | API cache, rate limiting, queues |
| TMA Frontend | React + Vite + @telegram-apps/sdk-react | Единая экосистема с Dashboard |
| Dashboard | Next.js 15 + Shadcn/UI + Tailwind | Дашборды, SSR, широкая экосистема |
| Bot | aiogram 3.26+ | Лучший async Telegram Bot framework для Python |
| Аналитика соцсетей | LiveDune API (Business tier) | Цена, покрытие СНГ, оплата в тенге |
| ЭЦП | TrustContract API | Юридическая сила в РК, SMS-подпись, REST API |
| Хостинг | Hetzner Cloud (EU) | Цена/качество; локальный KZ VPS при необходимости |
| CI/CD | GitHub Actions | Бесплатно, интеграция с GitHub |
| Контейнеризация | Docker Compose | Достаточно для MVP |
| Мониторинг | Prometheus + Grafana | Стандарт отрасли, open source |

### Критические действия до начала разработки

1. **Зарегистрировать аккаунт LiveDune** (тариф Business) и протестировать API -- подтвердить возможность проверки произвольных аккаунтов
2. **Связаться с TrustMe** для получения полной документации API TrustContract и обсуждения интеграционного флоу
3. **Создать Facebook App** и пройти Business Verification для доступа к Instagram Graph API
4. **Создать TikTok App** на developers.tiktok.com и получить Login Kit
5. **Развернуть тестовый Telegram Bot** и Mini App для проверки UX-flow

### Неизвестные и требующие уточнения

- Точный список эндпоинтов LiveDune API и возможность проверки внешних аккаунтов
- Детали API TrustContract: эндпоинты, формат вебхуков, возможность встраивания виджета
- Поведение TrustContract при подписи из Telegram Mini App (открытие ссылки через `openLink()`)
- Стоимость тарифных планов TrustContract
- Возможность получения метрик TikTok-видео через Display API (или только через LiveDune)

---

## 17. Pre-Development Checklist

Детализированный чеклист задач, которые необходимо выполнить до начала разработки. Группировка по неделям -- рекомендуемая последовательность.

### Неделя 1: Критические проверки и API-доступ

- [ ] **Зарегистрировать аккаунт LiveDune (тариф Business, $50/мес) и протестировать API** — Alikhan, ~4 часа. **Блокер: CRITICAL.** Необходимо подтвердить: (1) возможность проверки аккаунтов, не подключённых к dashboard; (2) формат ответов API для ER, подписчиков, просмотров; (3) лимиты и throttling. Если API не позволяет проверку внешних аккаунтов -- немедленно переключиться на Phyllo или challenge-based верификацию.
- [ ] **Связаться с TrustMe для получения полной документации API TrustContract** — Aidana (первый контакт) + Alikhan (техническая оценка), ~2-3 часа. **Блокер: CRITICAL.** Запросить: (1) sandbox-доступ к API; (2) документацию эндпоинтов; (3) формат вебхуков о статусе подписания; (4) возможность встраивания виджета подписи (iframe/SDK); (5) тарифные планы.
- [ ] **Создать Telegram Bot через @BotFather** — Alikhan, ~1 час. **Блокер: CRITICAL.** Создать @ugcboost_bot (production) и @ugcboost_stage_bot (staging). Настроить Menu Button, описание, аватар. Проверить генерацию bot token.
- [ ] **Развернуть Telegram Mini App (hello world) и протестировать initData flow** — Alikhan, ~4 часа. **Блокер: CRITICAL.** Проверить: (1) initData передаётся корректно; (2) HMAC-SHA256 валидация работает на бэкенде; (3) deep links (`?startapp=param`) работают; (4) `openLink()` корректно открывает внешние URL (для TrustContract).

### Неделя 2: Платформенные регистрации и инфраструктура

- [ ] **Создать Facebook App и начать Business Verification** — Alikhan, ~2 часа (подача) + до 2 недель ожидания. **Блокер: IMPORTANT.** Необходимо для доступа к Instagram Graph API (v22.0). Требует: (1) Facebook Developer аккаунт; (2) Facebook Business Manager; (3) документы юрлица (выписка из БИН/ИИН). Процесс верификации может занять 1-2 недели.
- [ ] **Создать TikTok App на developers.tiktok.com** — Alikhan, ~2 часа (подача) + до 2 недель ожидания. **Блокер: IMPORTANT.** Зарегистрировать приложение, запросить Login Kit + Display API scope (`user.info.basic`, `video.list`). Пройти ревью приложения.
- [ ] **Арендовать Hetzner Cloud сервер и настроить базовую инфраструктуру** — Alikhan, ~4 часа. **Блокер: IMPORTANT.** Шаги: (1) CX22 (staging) в Финляндии; (2) настроить SSH, firewall (ufw), fail2ban; (3) установить Docker и Docker Compose; (4) настроить автоматические обновления безопасности (unattended-upgrades).
- [ ] **Настроить домен и SSL** — Alikhan, ~2 часа. **Блокер: IMPORTANT.** Зарегистрировать/настроить DNS-записи: `api.ugcboost.kz`, `staging-api.ugcboost.kz`, `dashboard.ugcboost.kz`. Настроить Let's Encrypt через certbot или Nginx proxy.
- [ ] **Протестировать API TrustContract в sandbox** — Alikhan, ~4 часа (зависит от получения доступа на Неделе 1). **Блокер: IMPORTANT.** Проверить: (1) отправка договора через API; (2) получение ссылки на подпись; (3) SMS-подпись; (4) webhook о завершении подписания; (5) открытие ссылки подписи из TMA через `openLink()`.

### Неделя 3: CI/CD, шаблоны и финальная подготовка

- [ ] **Настроить CI/CD pipeline (GitHub Actions)** — Alikhan, ~3 часа. **Блокер: IMPORTANT.** Создать `.github/workflows/ci.yml` с: lint (ruff + mypy), test (pytest), security (pip-audit + bandit), deploy staging (auto из develop), deploy production (manual approve из main).
- [ ] **Создать базовый Docker Compose и шаблон проекта** — Alikhan, ~4 часа. **Блокер: IMPORTANT.** Инициализировать: (1) FastAPI backend с health endpoint; (2) PostgreSQL + Redis контейнеры; (3) Alembic для миграций; (4) `.env.example` со всеми переменными; (5) Nginx reverse proxy конфиг.
- [ ] **Настроить мониторинг (базовый)** — Alikhan, ~2 часа. **Блокер: NICE-TO-HAVE.** Prometheus endpoint в FastAPI (`prometheus-fastapi-instrumentator`), базовые алерты: CPU > 80%, диск > 90%, API 5xx > 1%.
- [ ] **Настроить бэкапы PostgreSQL** — Alikhan, ~1 час. **Блокер: NICE-TO-HAVE.** Cron job для pg_dump (ежедневно, 03:00 UTC), ротация 14 дней. Проверить восстановление из бэкапа.
- [ ] **Составить юридический шаблон договора с креатором** — Aidana, ~4-6 часов. **Блокер: IMPORTANT.** Шаблон должен включать: (1) согласие на обработку персональных данных (ИИН, соцсети); (2) передачу прав на контент; (3) обязательства сторон; (4) конфиденциальность. Согласовать с юристом, подготовить для загрузки в TrustContract.

### Сводка по блокерам

| Уровень | Количество | Описание |
|---------|-----------|----------|
| **CRITICAL** | 4 | Блокируют начало разработки. Без них невозможно проектировать ключевые flow |
| **IMPORTANT** | 8 | Необходимы для первого спринта. Можно начать разработку параллельно, но нужны до MVP |
| **NICE-TO-HAVE** | 2 | Желательны, но не блокируют. Можно настроить позже |
