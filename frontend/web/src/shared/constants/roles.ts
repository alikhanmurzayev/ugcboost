import type { components } from "@/api/generated/schema";

export type UserRole = components["schemas"]["User"]["role"];

export const Roles = {
  ADMIN: "admin" as const,
  BRAND_MANAGER: "brand_manager" as const,
};
