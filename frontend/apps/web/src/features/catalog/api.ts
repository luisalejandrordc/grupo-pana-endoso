import { api } from "../../shared/api/client";
import { queryString } from "../../shared/api/query";
import type { Schemas } from "../../shared/api/schema";

export type ApiModel = Schemas["VehicleModel"];
export type ApiModelSummary = Schemas["VehicleModelSummary"];
export type ApiVersion = Schemas["VehicleVersion"];
export type ApiColor = Schemas["VehicleColor"];
export type ApiImage = Schemas["CatalogImage"];
export type ModelList = Schemas["VehicleModelList"];
export type CreateModelData = Schemas["CreateVehicleModelData"];

export interface ModelListParams {
  q?: string;
  category?: string;
  status?: "all" | "active" | "archived";
  page?: number;
  pageSize?: number;
  sort?: "name" | "-name" | "createdAt" | "-createdAt";
}

const json = (method: string, body?: unknown) => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

/** A multipart body: the JSON under "data" and one file under `field`. */
export function multipart(data: unknown | null, field: string, file: File) {
  const form = new FormData();
  if (data !== null)
    form.append(
      "data",
      new Blob([JSON.stringify(data)], { type: "application/json" }),
    );
  form.append(field, file);
  return form;
}

export const catalogApi = {
  list: (params: ModelListParams) =>
    api.request<ModelList>(`/vehicle-models${queryString({ ...params })}`),
  get: (id: string) => api.request<ApiModel>(`/vehicle-models/${id}`),
  create: (data: CreateModelData, mainImage: File) =>
    api.request<ApiModel>("/vehicle-models", {
      method: "POST",
      body: multipart(data, "mainImage", mainImage),
    }),
  update: (id: string, body: Schemas["UpdateVehicleModelRequest"]) =>
    api.request<ApiModel>(`/vehicle-models/${id}`, json("PATCH", body)),
  archive: (id: string) =>
    api.request<ApiModel>(`/vehicle-models/${id}/archive`, { method: "POST" }),
  unarchive: (id: string) =>
    api.request<ApiModel>(`/vehicle-models/${id}/unarchive`, {
      method: "POST",
    }),
  remove: (id: string, reason: string) =>
    api.request<void>(`/vehicle-models/${id}${queryString({ reason })}`, {
      method: "DELETE",
    }),

  createVersion: (
    modelId: string,
    body: Schemas["CreateVehicleVersionRequest"],
  ) =>
    api.request<ApiVersion>(
      `/vehicle-models/${modelId}/versions`,
      json("POST", body),
    ),
  updateVersion: (id: string, body: Schemas["UpdateVehicleVersionRequest"]) =>
    api.request<ApiVersion>(`/vehicle-versions/${id}`, json("PATCH", body)),
  archiveVersion: (id: string) =>
    api.request<ApiVersion>(`/vehicle-versions/${id}/archive`, {
      method: "POST",
    }),
  unarchiveVersion: (id: string) =>
    api.request<ApiVersion>(`/vehicle-versions/${id}/unarchive`, {
      method: "POST",
    }),
  removeVersion: (id: string, reason: string) =>
    api.request<void>(`/vehicle-versions/${id}${queryString({ reason })}`, {
      method: "DELETE",
    }),

  createColor: (modelId: string, name: string, mainImage: File) =>
    api.request<ApiColor>(`/vehicle-models/${modelId}/colors`, {
      method: "POST",
      body: multipart({ name }, "mainImage", mainImage),
    }),
  updateColor: (id: string, body: Schemas["UpdateVehicleColorRequest"]) =>
    api.request<ApiColor>(`/vehicle-colors/${id}`, json("PATCH", body)),
  archiveColor: (id: string) =>
    api.request<ApiColor>(`/vehicle-colors/${id}/archive`, { method: "POST" }),
  unarchiveColor: (id: string) =>
    api.request<ApiColor>(`/vehicle-colors/${id}/unarchive`, {
      method: "POST",
    }),
  removeColor: (id: string, reason: string) =>
    api.request<void>(`/vehicle-colors/${id}${queryString({ reason })}`, {
      method: "DELETE",
    }),

  putImage: (colorId: string, viewpoint: string, file: File) =>
    api.request<ApiColor>(`/vehicle-colors/${colorId}/images/${viewpoint}`, {
      method: "PUT",
      body: multipart(null, "file", file),
    }),
  deleteImage: (colorId: string, viewpoint: string) =>
    api.request<ApiColor>(`/vehicle-colors/${colorId}/images/${viewpoint}`, {
      method: "DELETE",
    }),
};
