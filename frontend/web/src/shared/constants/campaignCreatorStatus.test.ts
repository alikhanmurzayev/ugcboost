import { describe, it, expect } from "vitest";
import {
  CAMPAIGN_CREATOR_STATUS,
  CAMPAIGN_CREATOR_GROUP_ORDER,
} from "./campaignCreatorStatus";

describe("CAMPAIGN_CREATOR_STATUS", () => {
  it("maps each label to the canonical status string", () => {
    expect(CAMPAIGN_CREATOR_STATUS.PLANNED).toBe("planned");
    expect(CAMPAIGN_CREATOR_STATUS.INVITED).toBe("invited");
    expect(CAMPAIGN_CREATOR_STATUS.DECLINED).toBe("declined");
    expect(CAMPAIGN_CREATOR_STATUS.AGREED).toBe("agreed");
  });
});

describe("CAMPAIGN_CREATOR_GROUP_ORDER", () => {
  it("orders statuses planned → invited → declined → agreed", () => {
    expect(CAMPAIGN_CREATOR_GROUP_ORDER).toEqual([
      "planned",
      "invited",
      "declined",
      "agreed",
    ]);
  });

  it("contains every status from CAMPAIGN_CREATOR_STATUS exactly once", () => {
    const values = Object.values(CAMPAIGN_CREATOR_STATUS);
    expect(CAMPAIGN_CREATOR_GROUP_ORDER).toHaveLength(values.length);
    expect(new Set(CAMPAIGN_CREATOR_GROUP_ORDER)).toEqual(new Set(values));
  });
});
