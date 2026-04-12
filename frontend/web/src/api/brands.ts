import type { components } from "./generated/schema";
import client, { ApiError } from "./client";

export type Brand = components["schemas"]["Brand"];
export type BrandListItem = components["schemas"]["BrandListItem"];
export type ManagerInfo = components["schemas"]["ManagerInfo"];

type ErrorBody = components["schemas"]["ErrorResponse"];

function throwErr(response: Response, error: unknown): never {
  const e = error as ErrorBody;
  throw new ApiError(response.status, e.error?.code ?? "INTERNAL_ERROR");
}

export async function listBrands() {
  const { data, error, response } = await client.GET("/brands");
  if (error) throwErr(response, error);
  return data;
}

export async function getBrand(id: string) {
  const { data, error, response } = await client.GET("/brands/{brandID}", {
    params: { path: { brandID: id } },
  });
  if (error) throwErr(response, error);
  return data;
}

export async function createBrand(name: string) {
  const { data, error, response } = await client.POST("/brands", {
    body: { name },
  });
  if (error) throwErr(response, error);
  return data;
}

export async function updateBrand(id: string, name: string) {
  const { data, error, response } = await client.PUT("/brands/{brandID}", {
    params: { path: { brandID: id } },
    body: { name },
  });
  if (error) throwErr(response, error);
  return data;
}

export async function deleteBrand(id: string) {
  const { data, error, response } = await client.DELETE("/brands/{brandID}", {
    params: { path: { brandID: id } },
  });
  if (error) throwErr(response, error);
  return data;
}

export async function assignManager(brandId: string, email: string) {
  const { data, error, response } = await client.POST("/brands/{brandID}/managers", {
    params: { path: { brandID: brandId } },
    body: { email },
  });
  if (error) throwErr(response, error);
  return data;
}

export async function removeManager(brandId: string, userId: string) {
  const { data, error, response } = await client.DELETE("/brands/{brandID}/managers/{userID}", {
    params: { path: { brandID: brandId, userID: userId } },
  });
  if (error) throwErr(response, error);
  return data;
}
