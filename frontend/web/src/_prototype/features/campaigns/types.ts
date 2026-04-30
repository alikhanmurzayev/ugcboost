export const CampaignTypes = {
  SERVICE: "service",
  FOOD: "food",
  PRODUCT: "product",
  EVENT: "event",
} as const;

export type CampaignType =
  (typeof CampaignTypes)[keyof typeof CampaignTypes];

export const CampaignStatuses = {
  DRAFT: "draft",
  PENDING_MODERATION: "pending_moderation",
  REJECTED: "rejected",
  ACTIVE: "active",
  COMPLETED: "completed",
} as const;

export type CampaignStatus =
  (typeof CampaignStatuses)[keyof typeof CampaignStatuses];

export type SocialPlatform = "instagram" | "tiktok" | "threads";

export type ApplicationStatus =
  | "new"
  | "approved"
  | "rejected"
  | "uncertain";

// TZ acceptance phase, only relevant for approved applications.
// "not_sent"  — TZ hasn't been sent to creator yet
// "sent"      — TZ sent, awaiting creator's response
// "accepted"  — creator accepted the TZ (firm commitment)
// "declined"  — creator declined; brand can replace via the replacement modal
// "replaced"  — declined creator who has been swapped out for a replacement;
//               kept in the data set for history but hidden from the active list
export type TzStatus =
  | "not_sent"
  | "sent"
  | "accepted"
  | "declined"
  | "replaced";

export interface ApplicantSocial {
  platform: SocialPlatform;
  handle: string;
}

export interface CreatorReel {
  id: string;
  thumbnailUrl: string;
  videoUrl?: string;
  permalink?: string;
  viewCount?: number;
}

export interface CampaignApplication {
  id: string;
  campaignId: string;
  status: ApplicationStatus;
  // TZ acceptance phase. Defaults to "not_sent". Becomes meaningful once the
  // brand presses "Подписать ТЗ" — at that point all approved applications
  // flip to "sent".
  tzStatus: TzStatus;
  tzSentAt?: string;
  tzDecidedAt?: string;
  appliedAt: string; // ISO
  decidedAt?: string;
  creator: {
    id: string;
    firstName: string;
    lastName: string;
    avatarUrl?: string;
    age: number;
    city: { code: string; name: string };
    categories: { code: string; name: string }[];
    socials: ApplicantSocial[];
    metrics: {
      followers: number;
      avgViews: number;
      // ER (Engagement Rate by Views) in %, e.g. 8.4
      er: number;
    };
    // Last 6 publications shown as a portfolio strip.
    recentReels: CreatorReel[];
  };
}
export type ContentFormat = "reels" | "stories" | "post";
export type Gender = "male" | "female";
export type Language = "kz" | "ru";

export interface Reference {
  url: string;
  description: string;
}

export interface Attachment {
  id: string;
  name: string;
  url: string;
  contentType: string; // MIME, e.g. "image/png", "application/pdf"
  sizeBytes: number;
}

export interface VisitDetails {
  city: string;
  address: string;
  // empty array means "any time"; otherwise concrete dt slots
  slots: { date: string; time: string }[];
}

export type EventParking = "paid" | "free" | "none";
export type EventTransferType = "personal" | "group";

export interface EventTransfer {
  type: EventTransferType;
  // For "group" transfer — pickup point and schedule are required.
  pickup?: string;
  schedule?: string;
}

export interface EventDetails {
  country: string;
  city: string;
  date: string; // ISO date YYYY-MM-DD
  timeFrom: string; // HH:mm
  timeTo: string; // HH:mm
  address: string;
  dressCode?: string;
  // Only meaningful when the creator is required on-site (requiresVisit).
  parking?: EventParking;
  entryInstructions?: string;
  transfer?: EventTransfer;
}

export type PaymentType =
  | "barter"
  | "fixed"
  | "barter_fixed"
  | "creator_proposal";

export interface Campaign {
  id: string;
  brandId: string;
  brandName: string; // denormalized for list display
  type: CampaignType;
  status: CampaignStatus;

  // Core brief
  title: string;
  description: string;
  // Structured content requirements (replaces old free-form `contentRequirements`).
  hashtags?: string;
  mentionsInCaption?: string;
  mentionsInPublication?: string;
  adDisclaimer?: string;
  adDisclaimerPlacement?: "caption" | "publication" | "both";
  references: Reference[];

  // Brand assets — logos, posters, mood-boards. Shown to creators in TMA.
  attachments: Attachment[];

  // Audience filters
  creatorsCount: number;
  // When true, brand accepts creators from any category — `categories` is ignored.
  anyCategories: boolean;
  categories: { code: string; name: string }[];
  minFollowers: number;
  minAvgViews: number;
  ageMin?: number;
  ageMax?: number;
  // When true, brand accepts creators from any city — `cities` is ignored.
  anyCities: boolean;
  cities: { code: string; name: string }[];
  genders: Gender[];
  languages: Language[];

  // Content requirements
  socialPlatform: SocialPlatform; // exactly one
  contentFormats: ContentFormat[]; // only used when socialPlatform === "instagram"
  // Per-format counts (e.g., {reels: 1, stories: 2}). For non-Instagram platforms
  // a single key "post" is used as a generic "publication" counter.
  postsByFormat: Partial<Record<ContentFormat, number>>;
  publishDeadline: string; // ISO — meaning depends on `publishDeadlineMode`
  // "until" = creator can publish any day up to the date;
  // "exact" = all creators publish on this exact date.
  publishDeadlineMode: "until" | "exact";

  // Payment
  paymentType: PaymentType;
  paymentAmount?: number; // KZT — present when type is "fixed" or "barter_fixed"
  barterDescription?: string; // text — present when type is "barter" or "barter_fixed"
  barterValue?: number; // KZT — declared value of barter goods
  barterAttachments: Attachment[]; // photos of the barter goods (always present, may be empty)

  // Where the creator publishes the content:
  // "creator" — on creator's profile (Instagram/TikTok/Threads);
  // "brand_only" — creator hands the asset to the brand, doesn't publish.
  // For Threads always implicitly "creator" — collab-post and brand-only aren't supported.
  publicationMode: "creator" | "brand_only";
  // Optional collab-post on Instagram (creator + brand co-authored publication).
  // Only meaningful when socialPlatform === "instagram" && publicationMode === "creator".
  isCollabPost: boolean;
  collabBrandHandles: string[];
  // Cross-posting policy. "creator_choice" — creator decides;
  // otherwise the creator must duplicate the publication into another platform.
  crossposting: "creator_choice" | "to_instagram" | "to_tiktok";

  // Approval flags
  requiresScriptApproval: boolean; // sketch/script before shooting
  requiresMaterialApproval: boolean; // final material before publish

  // Logistics
  requiresVisit: boolean;
  visitDetails?: VisitDetails;

  // Type-specific
  eventDetails?: EventDetails; // for "event" type
  halal?: boolean; // mandatory for "food", optional for "service" with visit

  // Moderation
  rejectionComment?: string; // when status === "rejected"

  // Timestamps
  createdAt: string;
  updatedAt: string;
  publishedAt?: string;
}
