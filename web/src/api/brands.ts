import { api } from "./client";

export interface Brand {
  id: string;
  name: string;
  logoUrl?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BrandListItem {
  id: string;
  name: string;
  logoUrl?: string;
  managerCount: number;
  createdAt: string;
  updatedAt: string;
}

export interface ManagerInfo {
  userId: string;
  email: string;
  assignedAt: string;
}

export interface BrandWithManagers extends Brand {
  managers: ManagerInfo[];
}

interface ListBrandsResponse {
  data: { brands: BrandListItem[] };
}

interface BrandResponse {
  data: Brand;
}

interface BrandDetailResponse {
  data: BrandWithManagers;
}

interface AssignManagerResponse {
  data: {
    userId: string;
    email: string;
    role: string;
    tempPassword?: string;
  };
}

interface MessageResponse {
  data: { message: string };
}

export function listBrands() {
  return api<ListBrandsResponse>("/brands");
}

export function getBrand(id: string) {
  return api<BrandDetailResponse>(`/brands/${id}`);
}

export function createBrand(name: string) {
  return api<BrandResponse>("/brands", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function updateBrand(id: string, name: string) {
  return api<BrandResponse>(`/brands/${id}`, {
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
  return api<AssignManagerResponse>(`/brands/${brandId}/managers`, {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export function removeManager(brandId: string, userId: string) {
  return api<MessageResponse>(`/brands/${brandId}/managers/${userId}`, {
    method: "DELETE",
  });
}
