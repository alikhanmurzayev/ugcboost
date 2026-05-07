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
  requirements: string[];
  references?: string[];
};

export type Mentions = {
  accounts: string[];
  notes?: string[];
};

export type CampaignBrief = {
  token: string;
  brandName: string;
  campaignTitle: string;
  subtitle?: string;
  context?: string;
  eventDetails?: EventDetail[];
  cooperationFormat?: string;
  fromBrand?: CooperationSection;
  fromCreator?: CooperationSection;
  reels?: ReelsBrief;
  mentions?: Mentions;
  aboutParagraphs?: string[];
  aboutNote?: string;
};
