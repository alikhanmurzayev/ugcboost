// Prototype-local mirror of the relevant openapi schema shapes. Frozen here
// so the prototype has zero compile-time dependency on @/api/generated/schema
// (and so we don't drift if the real contract evolves).

export type SocialPlatform = "instagram" | "tiktok" | "threads";

export interface CreatorApplicationDetailCategory {
  code: string;
  name: string;
  sortOrder: number;
}

export interface CreatorApplicationDetailCity {
  code: string;
  name: string;
  sortOrder: number;
}

export interface CreatorApplicationDetailSocial {
  platform: SocialPlatform;
  handle: string;
}

export interface CreatorApplicationDetailConsent {
  consentType: "processing" | "third_party" | "cross_border" | "terms";
  acceptedAt: string;
  documentVersion?: string;
}

export interface TelegramLink {
  telegramUserId: number;
  telegramUsername?: string | null;
  telegramFirstName?: string | null;
}

export interface CreatorApplicationDetailData {
  id: string;
  lastName: string;
  firstName: string;
  middleName?: string | null;
  iin: string;
  birthDate: string;
  phone: string;
  city: CreatorApplicationDetailCity;
  address?: string | null;
  categoryOtherText?: string | null;
  status: "pending" | "approved" | "rejected" | "blocked";
  createdAt: string;
  updatedAt: string;
  categories: CreatorApplicationDetailCategory[];
  socials: CreatorApplicationDetailSocial[];
  consents: CreatorApplicationDetailConsent[];
  telegramLink?: TelegramLink | null;
}
