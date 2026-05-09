import { describe, it, expect } from "vitest";
import { ApiError } from "@/api/client";
import {
  isBatchInvalidDetails,
  parseSettled,
} from "./notifyResult";

const NAMES = { c1: "Иванова Анна", c2: "Петрова Анна" };

describe("parseSettled", () => {
  it("returns success kind with delivered count and undelivered names when data is present", () => {
    const result = parseSettled(
      { data: { undelivered: [{ creatorId: "c1", reason: "bot_blocked" }] } },
      null,
      2,
      NAMES,
    );
    expect(result).toEqual({
      kind: "success",
      undelivered: [{ creatorId: "c1", reason: "bot_blocked" }],
      deliveredCount: 1,
      undeliveredNames: NAMES,
    });
  });

  it("clamps deliveredCount to 0 when undelivered exceeds attempted (defensive)", () => {
    const result = parseSettled(
      {
        data: {
          undelivered: [
            { creatorId: "c1", reason: "unknown" },
            { creatorId: "c2", reason: "unknown" },
          ],
        },
      },
      null,
      1,
      NAMES,
    );
    expect(result.kind).toBe("success");
    expect(result.deliveredCount).toBe(0);
  });

  it("returns validation_error with parsed details on 422 CAMPAIGN_CREATOR_BATCH_INVALID", () => {
    const err = new ApiError(422, "CAMPAIGN_CREATOR_BATCH_INVALID", "x", [
      { creatorId: "c1", currentStatus: "invited" },
      { creatorId: "c2", currentStatus: "agreed" },
    ]);
    const result = parseSettled(undefined, err, 2, NAMES);
    expect(result).toEqual({
      kind: "validation_error",
      validationDetails: [
        { creatorId: "c1", currentStatus: "invited" },
        { creatorId: "c2", currentStatus: "agreed" },
      ],
      detailNames: NAMES,
    });
  });

  it("returns validation_error with empty details when 422 batch-invalid sends a malformed details payload", () => {
    const err = new ApiError(
      422,
      "CAMPAIGN_CREATOR_BATCH_INVALID",
      "x",
      "not-an-array",
    );
    const result = parseSettled(undefined, err, 1, NAMES);
    expect(result.kind).toBe("validation_error");
    expect(result.validationDetails).toEqual([]);
  });

  it("returns contract_template_required for 422 CONTRACT_TEMPLATE_REQUIRED", () => {
    const err = new ApiError(422, "CONTRACT_TEMPLATE_REQUIRED", "msg");
    const result = parseSettled(undefined, err, 1, NAMES);
    expect(result).toEqual({ kind: "contract_template_required" });
  });

  it("returns validation_unknown for 422 with a different code", () => {
    const err = new ApiError(422, "INVALID_BODY", "bad");
    const result = parseSettled(undefined, err, 1, NAMES);
    expect(result).toEqual({ kind: "validation_unknown" });
  });

  it("returns network_error for any other ApiError", () => {
    const err = new ApiError(500, "INTERNAL_ERROR");
    const result = parseSettled(undefined, err, 1, NAMES);
    expect(result).toEqual({ kind: "network_error" });
  });

  it("returns network_error when both data and error are absent", () => {
    const result = parseSettled(undefined, null, 1, NAMES);
    expect(result).toEqual({ kind: "network_error" });
  });

  it("treats non-ApiError throws as network_error (honest typing)", () => {
    const result = parseSettled(undefined, new TypeError("boom"), 1, NAMES);
    expect(result).toEqual({ kind: "network_error" });
  });
});

describe("isBatchInvalidDetails", () => {
  it("accepts a well-formed array", () => {
    expect(
      isBatchInvalidDetails([
        { creatorId: "c1", currentStatus: "invited" },
      ]),
    ).toBe(true);
  });

  it("rejects non-arrays", () => {
    expect(isBatchInvalidDetails(null)).toBe(false);
    expect(isBatchInvalidDetails(undefined)).toBe(false);
    expect(isBatchInvalidDetails({})).toBe(false);
    expect(isBatchInvalidDetails("string")).toBe(false);
  });

  it("rejects arrays whose items lack creatorId or currentStatus", () => {
    expect(isBatchInvalidDetails([{ creatorId: "c1" }])).toBe(false);
    expect(isBatchInvalidDetails([{ currentStatus: "invited" }])).toBe(false);
    expect(isBatchInvalidDetails([null])).toBe(false);
  });
});
