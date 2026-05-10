export type EventDetail = {
  label: string;
  value: string;
};

export type CooperationSection = {
  title: string;
  items: string[];
};

export type ReelsBrief = {
  format: string;
  deadline: string;
  requirements?: string[];
  briefText?: string;
  references?: string[];
};

export type StoriesBrief = {
  count?: string;
  briefText?: string;
  requirements?: string[];
};

export type Mentions = {
  title?: string;
  accounts: string[];
  notes?: string[];
};

export type Designer = {
  brand: string;
  designer?: string;
  handles?: string[];
};

export type DesignersSection = {
  title?: string;
  intro?: string;
  items: Designer[];
};

export type PartnerSection = {
  title: string;
  paragraphs: string[];
  handle?: string;
  imageUrl?: string;
  imageAlt?: string;
};

export type CampaignBrief = {
  token: string;
  brandName: string;
  campaignTitle: string;
  subtitle?: string;
  subtitleAsTagline?: boolean;
  context?: string;
  eventDetails?: EventDetail[];
  cooperationFormat?: string;
  fromBrand?: CooperationSection;
  fromCreator?: CooperationSection;
  reels?: ReelsBrief;
  stories?: StoriesBrief;
  mentions?: Mentions;
  designers?: DesignersSection;
  partner?: PartnerSection;
  aboutTitle?: string;
  aboutParagraphs?: string[];
  aboutNote?: string;
  aboutImageUrl?: string;
  aboutImageAlt?: string;
  inviteEventLabel?: string;
};
