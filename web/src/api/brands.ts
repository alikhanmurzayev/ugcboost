import type { components } from "./generated/schema";
import { api } from "./client";

export type Brand = components["schemas"]["Brand"];
export type BrandListItem = components["schemas"]["BrandListItem"];
export type ManagerInfo = components["schemas"]["ManagerInfo"];

type ListBrandsResult = components["schemas"]["ListBrandsResult"];
type BrandResult = components["schemas"]["BrandResult"];
type GetBrandResult = components["schemas"]["GetBrandResult"];
type AssignManagerResult = components["schemas"]["AssignManagerResult"];
type MessageResponse = components["schemas"]["MessageResponse"];

export function listBrands() {
  return api<ListBrandsResult>("/brands");
}

export function getBrand(id: string) {
  return api<GetBrandResult>(`/brands/${id}`);
}

export function createBrand(name: string) {
  return api<BrandResult>("/brands", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function updateBrand(id: string, name: string) {
  return api<BrandResult>(`/brands/${id}`, {
    method: "PUT",
    body: JSON.stringify({ name }),
  });
}

export function deleteBrand(id: string) {
  return api<MessageResponse>(`/brands/${id}`, {
    method: "DELETE",
  });
}

export function assignManager(brandId: string, email: string) {
  return api<AssignManagerResult>(`/brands/${brandId}/managers`, {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export function removeManager(brandId: string, userId: string) {
  return api<MessageResponse>(`/brands/${brandId}/managers/${userId}`, {
    method: "DELETE",
  });
}
