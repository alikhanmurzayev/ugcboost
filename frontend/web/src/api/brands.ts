import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type Brand = components["schemas"]["Brand"];
export type BrandListItem = components["schemas"]["BrandListItem"];
export type ManagerInfo = components["schemas"]["ManagerInfo"];

export async function listBrands() {
  const { data, error, response } = await client.GET("/brands");
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function getBrand(id: string) {
  const { data, error, response } = await client.GET("/brands/{brandId}", {
    params: { path: { brandId: id } },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function createBrand(name: string) {
  const { data, error, response } = await client.POST("/brands", {
    body: { name },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function updateBrand(id: string, name: string) {
  const { data, error, response } = await client.PUT("/brands/{brandId}", {
    params: { path: { brandId: id } },
    body: { name },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function deleteBrand(id: string) {
  const { data, error, response } = await client.DELETE("/brands/{brandId}", {
    params: { path: { brandId: id } },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function assignManager(brandId: string, email: string) {
  const { data, error, response } = await client.POST("/brands/{brandId}/managers", {
    params: { path: { brandId } },
    body: { email },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}

export async function removeManager(brandId: string, userId: string) {
  const { data, error, response } = await client.DELETE("/brands/{brandId}/managers/{userId}", {
    params: { path: { brandId, userId } },
  });
  if (error) throw new ApiError(response.status, error.error?.code ?? "INTERNAL_ERROR");
  return data;
}
