---
title: "Product Brief Distillate: UGCBoost"
type: llm-distillate
source: "product-brief-ugcboost.md"
created: "2026-04-04"
purpose: "Token-efficient context for downstream PRD creation — captures all detail from discovery sessions that exceeds the executive brief"
---

# Product Brief Distillate: UGCBoost

## Market Validation

- Landing page ugcboost.kz was published with zero marketing/promotion — only organic search indexing
- Three companies contacted Aidana directly via WhatsApp (her phone number was on the landing):
  - **One Corporate** — holding company with travel agency
  - **Beauty by Sofia** — official distributor of multiple cosmetics brands
  - **First Media Group** — major marketing holding representing Kotex, Ferrero, Milka
- All three wanted to know how to place orders or connect with UGC creators — but the product didn't exist yet, so nothing came of it
- This validates that brands are actively searching for UGC solutions in Kazakhstan and finding none

## Competitive Landscape (Kazakhstan)

- **TusApp** — existing KZ platform, functions as a bulletin board, no quality moderation
- **TTfluence** — existing KZ platform, same bulletin board model
- **UGC app** (App Store: id6757330821) — another KZ entrant, same problems
- **Common weakness across all:** no entry barrier for creators, no moderation, no operational value-add for brands — brands still have to manually vet, contract, and track creators just like in Instagram DMs
- Multiple other similar apps exist in Google search results — all follow the same pattern

## Competitive Landscape (International)

- **Billo** — strong curation, reviews videos before delivery, ~$59/video, US/EU focus
- **Insense** — full-service UGC + paid social, Spark Ads integration, subscription from ~$400/mo
- **Trend.io** — hand-picks creators, reviews all content, ~$100+/video, premium positioning
- **Collabstr** — marketplace with portfolio review, creators set own rates, 15-20% platform fee
- **JoinBrands** — task-based model, light moderation, ~$35-100/video
- **None operate in CIS/Central Asia** — zero localized competition from international players
- Platforms with strongest quality curation (Billo, Trend) are the most successful — validates UGCBoost's moderation-first approach

## UGC Market Context

- UGC seeding = brands work on QUANTITY of content units, not individual influencer deals
- Typical UGC creator gets 3,000–5,000 views per reel — brands compensate with volume (50–500 creators per campaign)
- Two collaboration types: **barter** (brand sends product, creator posts) and **paid** (brand pays per reel, e.g. 20K–150K tenge)
- Brands currently find creators via: Instagram search, Threads posts where brands announce they're looking for UGC creators (creators leave "+" in comments), Google Forms, DMs
- The operational burden includes: vetting content quality, checking visual aesthetic and category fit, collecting physical addresses, sending products, managing contracts, tracking deliverables, chasing missing posts

## Creator Profile and Behavior

- Most UGC creators write "UGC creator" in their Instagram bio so brands can find them in search
- This backfires: audiences see the label and recognize content as paid/sponsored, reducing its native feel
- UGCBoost eliminates this need — creators are found through the platform, not public advertising
- Platform **recommends** (not requires) creators remove the "UGC creator" label; incentive: brands prefer organic-looking profiles, so unlabeled creators get selected more often
- This cannot be enforced — it's a recommendation with a clear business incentive, not a platform rule
- Creators are typically individuals without ИП/ТОО (no tax registration), which creates legal problems for brand accounting

## Creator Onboarding Flow (Detailed)

### Step 1: Application (Landing Page — Web)
- Creator fills out form on ugcboost.kz landing page
- Fields: name, surname, social media selection (Instagram, TikTok, Threads), links to accounts, content category/direction, city of residence, physical address (street, house — for product deliveries)
- Consent checkboxes: personal data processing, third-party data transfer, cross-border data transfer, platform terms and conditions
- On submission: confirmation screen ("Your application is under review") + prompt to open Telegram bot with explanation: "Please open our Telegram bot so we can notify you about results. We won't spam — only important updates."

### Step 2: Identity Verification
- Confirm that the person is the actual owner of the social media accounts they linked
- Prevents scam scenario: someone registers under a known blogger's name, takes barter products, never posts
- Verification method: TBD (needs technical design — could be OAuth, screenshot proof, or other)

### Step 3: Automated Moderation (LiveDune)
- Pull metrics from LiveDune API: follower count, average views on recent reels, engagement rate (ER), posting frequency, total number of publications
- Filter out obviously unqualified creators: too few posts (e.g., only 3 total), inactive accounts, very low engagement
- Purpose: reduce manual workload for Aidana by eliminating clearly unqualified applicants before human review

### Step 4: Manual Moderation (Aidana)
- Aidana personally reviews each remaining application
- Checks: content quality, visual aesthetic, category fit (e.g., someone claiming "fashion" but posting casual home content = reject), consistency
- Can approve, reject, or reject with feedback (e.g., "recommend changing your category from fashion to lifestyle — your content fits better there")
- Approval/rejection reasons are stored in the database — this accumulates as training data for future AI-assisted moderation
- For Fashion Week MVP: all 200 creators go through manual review (manageable volume)
- Scaling plan: when volume exceeds capacity, Aidana has a person (currently handling UGC selection for Eurasian Fashion Week) who can be hired for moderation

### Step 5: Notification and Contract
- Approved creator receives Telegram notification: "Congratulations, your application is approved! You've been granted access to [category]. Please review and sign the service agreement."
- Contract signed via **TrustMe** (Kazakhstan e-signing service, legally binding, equivalent to wet signature, usable in court)
- Contract is a one-time service agreement (not per-campaign) covering:
  - Confidentiality: cannot share/screenshot brand details, campaign conditions, payment terms
  - Obligation to fulfill campaign requirements per the technical brief (ТЗ) when accepting a campaign
  - Liability: if creator takes barter product and doesn't deliver, they must reimburse product cost + platform costs + logistics costs
  - Content licensing: creator agrees that UGCBoost may grant brands the right to repurpose their content for paid ads (at UGCBoost's discretion)
- Contract duration: TBD (needs legal consultation — could be indefinite or fixed-term)
- After signing: full access to Telegram Mini App and available campaigns

### Step 6: Platform Access
- Creator enters the Telegram Mini App
- Sees available campaigns filtered by/relevant to their approved category
- Campaigns outside their category are visible but grayed out / blocked — cannot apply
- Creator **cannot change their category** freely — changing category triggers re-moderation (all access suspended until new category is reviewed and approved). This prevents gaming: e.g., entering as "lifestyle" then switching to "fashion" to access premium campaigns

## Campaign Flow (Detailed)

### Brand Creates Campaign
- Campaign types: **product** (physical / digital), **service**, or **event**
- Fields: brand name, campaign description, requirements/ТЗ (technical brief), content type (Reels, TikTok post, Threads post, or combination), tagging instructions (which accounts to tag, collab post requirements), barter/paid terms, target creator category, capacity limit (max creators), deadline
- For events: date, time, location, dress code, specific instructions
- Services: salon visits, consultations, procedures — creator must visit and create content about the experience

### Creator Applies
- Creator browses available campaigns, reads full ТЗ
- Must acknowledge they've read the brief before the "Apply" button becomes active — prevents spam applications
- Confirmation dialog: "Are you sure you want to apply for [Brand] x [Campaign Name]?" — reduces accidental clicks
- Creator can **withdraw their application** at any time before the brand approves them
- Application is treated as a **preference/wish**, not a guarantee — circumstances may result in different assignment

### Brand Reviews Applications (or Admin for Fashion Week MVP)
- Brand (or admin) sees list of pre-vetted creator applications for their campaign
- Can approve or reject individual creators
- **Cross-campaign visibility:** admin can see if a creator is already assigned to another active campaign — visual indicator/flag
- This prevents accidentally assigning the same creator to multiple campaigns (important when diversity of creators matters)
- However, this is **configurable per brand** — some brands may want the same creator on multiple campaigns

### Creator Limit Per Campaign
- For Eurasian Fashion Week: ideally **one campaign per creator** to maximize creator diversity across brand partners
- But this is flexible — if a popular campaign has too many applicants and an unpopular one has too few, creators may be asked to take a second campaign
- If a campaign fills up (e.g., 50 applicants for 20 spots), brand selects their preferred 20; remaining 30 are notified: "This campaign is full, please consider other campaigns"
- Some creators may not want certain campaigns (e.g., alcohol brands for personal/religious reasons) — their preferences are respected

### Work Submission
- After completing the campaign (e.g., attending event, creating content, publishing), creator submits links to published content
- Number of link fields is determined by campaign ТЗ: if requirement is 3 reels, 3 fields are shown by default
- Links must be unique — cannot submit the same URL multiple times
- "+" button to add extra links for bonus content (creator published more than required by their own initiative)
- Content can be across multiple platforms: Instagram Reels, TikTok videos, Threads posts — all supported
- Some campaigns may require collab posts (content appears on both creator's and brand's pages)

### Notifications and Reminders
- All notifications delivered via Telegram bot (not just in-app)
- Application status updates: submitted → under review → approved/rejected
- Deadline reminders: 2 weeks, 1 week, 3 days, 1 day before campaign deadline
- Possible: confirmation button in reminders (creator confirms they're still planning to deliver)

## Brand Dashboard Features (Full Product, Not MVP)

### Campaign Content Gallery
- Tile/grid view showing all published UGC content from a campaign
- Each tile: video thumbnail (auto-playing muted or paused on hover), post metrics (views, likes, comments) displayed in a format familiar from social networks
- Designed as a "wow moment" — brand manager opens campaign and instantly sees the full wall of content
- Desktop-optimized layout (brands use laptops/computers, not mobile)

### Creator Rating and Feedback System
- Brands rate each creator's work on a scale (e.g., 1–10)
- Can leave qualitative feedback per creator
- Ratings are **internal only** — not shown to creators — to avoid discouraging them over subjective brand expectations
- Example: brand expected viral results from a barter deal and is disappointed → low rating is subjective but still captured
- Data helps UGCBoost: identify top performers, track quality trends, build internal creator scoring
- Future potential: surface high-rated creators as "recommended" to other brands

### Campaign Analytics
- Aggregated metrics per campaign: total views, total engagement, total comments across all submitted content
- Metrics pulled and auto-updated via LiveDune API (views on reels grow over time — a reel can "blow up" weeks after posting)
- Update frequency: by request, or on schedule (e.g., every 3 days, once a week) — TBD
- For MVP: analytics collected manually or post-event, not automated

## Admin Panel (Detailed)

- Creator moderation queue: view applications, LiveDune metrics summary, approve/reject with reason
- Campaign management: create campaigns (under Eurasian Fashion Week for MVP), set parameters
- Cross-campaign creator tracking: see which creators are assigned where, prevent duplicate assignments when needed
- Contract status tracking
- Ban/blacklist functionality: for creators who violate terms (took product, didn't post, etc.)
- For MVP: admin = Aidana operating as Eurasian Fashion Week, creating all campaigns herself

## Technical Integrations

### LiveDune API
- Documentation: https://api.livedune.com/docs/
- Wiki/tariffs: https://wiki.livedune.com/ru/articles/11640100-api
- Supported social networks: Instagram (instagram_new), YouTube, Twitter, VK, OK, Telegram, Dzen — **TikTok NOT confirmed in API docs**
- Key endpoints: account analytics (followers, views, views_avg, likes, comments, er, er_views), posts list, individual post statistics, stories, videos, audience demographics (gender, age — Instagram, Threads, VK, OK only)
- Each creator's account must be added to LiveDune dashboard before metrics can be pulled
- **Tariff recommendation for MVP (200 creators):** Business plan (10,000 requests/month), with option to buy additional request packs
- Tariff pricing is in Russian rubles — need to verify if tenge pricing available
- **TikTok status: UNKNOWN** — needs direct inquiry to LiveDune. Their website has confusing info about TikTok analytics. Will contact them during business hours.
- For MVP: LiveDune used for moderation screening; automated analytics dashboard deferred to post-event

### TrustMe
- Kazakhstan e-signing service for legally binding contracts
- Signing via SMS verification — legally equivalent to wet signature in Kazakhstan
- Signed documents usable in court
- REST API available (details need to be requested from TrustMe directly)
- **No fallback acceptable** — simple checkbox/consent is not legally binding for a service agreement; proper e-signing is required for legal protection (liability, confidentiality, content licensing)

### Relog (Future)
- Logistics service for optimized delivery routing
- Collects all delivery addresses, builds efficient route for couriers
- Not in MVP — future integration for product delivery campaigns

## Eurasian Fashion Week Context (MVP Launch Event)

- **Dates:** May 13–14, 2026
- **Aidana's role:** Executive Director of Eurasian Fashion Week (EFW) — this is her day job alongside UGCBoost
- **Event structure:**
  - May 13: Main fashion show — Kazakhstani and international designers
  - May 14 morning: Kids Fashion Day — children's fashion designers
  - May 14 afternoon: Modest Fashion Day — Muslim/modest fashion designers
  - May 14 evening: Ethno designers show
  - After evening show: After Party (invitation-only for top creators)
- **Partners:** diverse brands — cosmetics, logistics companies, food/beverage, champagne/alcohol brands, coffee shops, automobile brands (e.g., Audi). Partners set up corners/stands at the venue or provide gifts.
- **Campaign structure for MVP:** Each partner integration = separate campaign. E.g.:
  - "EFW x [Auto Brand]" — attend show, visit auto brand's corner, shoot stylish fashion reel, tag brand in description
  - "EFW x [Food Brand]" — attend Kids Fashion Day, cover children's show, natively integrate food brand
  - "EFW x [Cosmetics Brand]" — attend and cover, collab post with EFW account
- **Target:** ~200 vetted creators, distributed across campaigns
- **Incentives for creators:** networking, fashion community, content for their portfolios, invitation to After Party for top performers, gifts for top 5 creators
- **All campaigns are barter-only** — no payments to creators, no payments from partners (free test to validate the platform)
- **Important constraint:** Aidana has other Fashion Week responsibilities beyond UGC, so she cannot take on additional brand campaigns during this period. Post-event focus shifts to commercial brand onboarding.

## Business Model Details

- **Not finalized** — three options under consideration:
  1. Commission per paid creator engagement
  2. Tiered subscription (campaign count + creator count per tier)
  3. Hybrid: subscription + commission
- **Content licensing as monetization lever:** creator contracts grant UGCBoost the right to control whether brands can use UGC as paid ad creatives. Basic tier: content lives only on creator's page. Premium tier: brand can repurpose for targeted ads (Spark Ads, Partnership Ads). UGCBoost decides — this is an upsell opportunity.
- **Agency/intermediary revenue:** UGCBoost as VAT-registered entity handles payments between brands and creators. Brands can claim marketing expenses as tax deductions (impossible when paying individual creators directly). UGCBoost takes a commission.
- **MVP (Fashion Week) is free** — no payment from partners, no payment to creators. Purpose: validate operations, build case study.
- **Post-MVP monetization plan:** approach brands from warm pipeline with proven results + ready creator pool

## Platform Strategy

- **UGC creators:** Telegram Mini App (MVP) → native mobile app (future, App Store / Google Play)
- **Brands:** web-only (employees work on computers, mobile not needed)
- **Admin:** web panel
- **Why Telegram Mini App for MVP:**
  - Telegram is dominant in Kazakhstan (~12–15M users)
  - Creators already use Telegram for communication
  - Lower friction than App Store download
  - Built-in notification system via bot
  - Cannot ship native app to App Store in time for May 13
- **Design principle:** MVP should not be over-fitted to Fashion Week — build with future multi-brand usage in mind from the start

## Rejected Ideas and Decisions

- **Screenshot-based analytics:** REJECTED — views grow over time (reel can go viral weeks later), screenshots are static; also creators can photoshop screenshots to inflate numbers. LiveDune API provides live, verifiable data.
- **Simple checkbox instead of TrustMe contract:** REJECTED — checkbox is not legally binding for service agreements in Kazakhstan. TrustMe e-signing has legal standing equivalent to wet signature, usable in court. Essential for liability protection.
- **Kaspi Shop integration for MVP:** DEFERRED — interesting but niche (only relevant for sellers with exclusive product cards). Most Kaspi cards have multiple sellers sharing one listing, making UGC attribution unclear. May revisit when Kaspi's reported "unique seller link" feature is confirmed.
- **Creator education/academy:** DEFERRED — good idea (convert rejected creators into future supply), but not priority for MVP or even Year 1. Will consider after platform is established.
- **AI-assisted moderation for MVP:** DEFERRED — manual moderation is sufficient for 200 creators. Approval/rejection reasons are being stored as training data. AI consideration starts when volume exceeds manual capacity.
- **Specific success metrics/KPIs for Fashion Week:** NOT DEFINED — team will assess success qualitatively based on operational execution and partner satisfaction, not pre-set numbers. Analytics from LiveDune will provide quantitative results post-event.

## Open Questions

- **TikTok analytics via LiveDune:** API docs don't list TikTok as supported social network. Needs direct inquiry to LiveDune. Critical because Fashion Week creators will post on both Instagram and TikTok.
- **TrustMe API specifics:** Need to request API access and documentation. Flow, rate limits, pricing unknown.
- **Contract duration:** Indefinite or fixed-term? Needs legal consultation.
- **Creator verification method:** How to confirm account ownership technically? OAuth? Verification post? Screenshot? Needs technical design.
- **Source tracking for creator recruitment:** Team is manually scouting UGC creators in Google Sheets and DMing them. Need tracking for where applications originate (UTM links? promo codes?). Not resolved — "will think about it."
- **LiveDune pricing in tenge:** Tariffs shown in Russian rubles. Need to confirm if Kazakhstan-specific pricing exists.
- **Category taxonomy:** Categories mentioned: fashion, beauty, lifestyle, parenting/mommy blog, food. Full category list not finalized.
- **Multi-category creators:** Can a creator be approved in multiple categories simultaneously? Not discussed.
