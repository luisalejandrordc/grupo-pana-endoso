import { api } from "../../../shared/api/client";
import { queryString } from "../../../shared/api/query";
import type { Schemas } from "../../../shared/api/schema";

export type ApiSale = Schemas["Sale"];
export type ApiSaleDetail = Schemas["SaleDetail"];
export type SaleList = Schemas["SaleList"];
export type SaleHistory = Schemas["SaleHistory"];
export type HistoryEntry = Schemas["SaleHistoryEntry"];
export type ApiTask = Schemas["Task"];
export type ApiTaskDetail = Schemas["TaskDetail"];
export type SaveTaskResult = Schemas["SaveTaskDataResult"];
export type ApiSlot = Schemas["DocumentSlot"];
export type ApiDocumentFile = Schemas["DocumentFile"];
export type DocumentDownload = Schemas["DocumentDownload"];
export type SelectableModel = Schemas["SelectableModel"];
export type Advisor = Schemas["Advisor"];
export type CreateSaleBody = Schemas["CreateSaleRequest"];
export type CancelCategory = Schemas["CancelSaleRequest"]["category"];
export type SaleStateValue = Schemas["SaleState"];
export type Temperature = Schemas["SaleTemperature"];

export interface SaleListParams {
  q?: string;
  state?: string;
  currentTask?: number;
  milestone?: number;
  advisorId?: string;
  customerType?: string;
  temperature?: string;
  modelId?: string;
  versionId?: string;
  page?: number;
  pageSize?: number;
  sort?: "createdAt" | "-createdAt" | "customerName" | "-customerName";
}

const json = (method: string, body?: unknown) => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const salesApi = {
  list: (params: SaleListParams) =>
    api.request<SaleList>(`/sales${queryString({ ...params })}`),
  get: (id: string) => api.request<ApiSaleDetail>(`/sales/${id}`),
  create: (body: CreateSaleBody) =>
    api.request<ApiSaleDetail>("/sales", json("POST", body)),
  updateNotes: (id: string, version: number, notes: string | null) =>
    api.request<ApiSaleDetail>(
      `/sales/${id}`,
      json("PATCH", { version, notes }),
    ),
  reassign: (id: string, advisorId: string) =>
    api.request<ApiSaleDetail>(
      `/sales/${id}/reassign`,
      json("POST", { advisorId }),
    ),
  cancel: (id: string, category: CancelCategory, text: string) =>
    api.request<ApiSaleDetail>(
      `/sales/${id}/cancel`,
      json("POST", { category, text: text || undefined }),
    ),
  reopen: (id: string, reason: string) =>
    api.request<ApiSaleDetail>(`/sales/${id}/reopen`, json("POST", { reason })),
  remove: (id: string, reason: string, confirmation: string) =>
    api.request<void>(`/sales/${id}`, json("DELETE", { reason, confirmation })),
  history: (id: string, page: number, pageSize: number) =>
    api.request<SaleHistory>(
      `/sales/${id}/history${queryString({ page, pageSize })}`,
    ),
};

export const tasksApi = {
  list: (saleId: string) =>
    api.request<{ items: ApiTask[] }>(`/sales/${saleId}/tasks`),
  get: (saleId: string, number: number) =>
    api.request<ApiTaskDetail>(`/sales/${saleId}/tasks/${number}`),
  saveData: (
    saleId: string,
    number: number,
    body: {
      version: number;
      data: Record<string, unknown>;
      reason?: string;
    },
  ) =>
    api.request<SaveTaskResult>(
      `/sales/${saleId}/tasks/${number}/data`,
      json("PATCH", body),
    ),
  saveNotes: (saleId: string, number: number, notes: string) =>
    api.request<ApiTaskDetail>(
      `/sales/${saleId}/tasks/${number}/notes`,
      json("PATCH", { notes }),
    ),
  start: (saleId: string, number: number) =>
    api.request<ApiTaskDetail>(`/sales/${saleId}/tasks/${number}/start`, {
      method: "POST",
    }),
  complete: (saleId: string, number: number) =>
    api.request<ApiTaskDetail>(`/sales/${saleId}/tasks/${number}/complete`, {
      method: "POST",
    }),
  restart: (saleId: string, number: number, reason: string) =>
    api.request<ApiTaskDetail>(
      `/sales/${saleId}/tasks/${number}/restart`,
      json("POST", { reason }),
    ),
  reopen: (saleId: string, number: number, reason: string) =>
    api.request<ApiTaskDetail>(
      `/sales/${saleId}/tasks/${number}/reopen`,
      json("POST", { reason }),
    ),
};

export const documentsApi = {
  list: (saleId: string) =>
    api.request<{ items: ApiSlot[] }>(`/sales/${saleId}/documents`),
  upload: (
    saleId: string,
    number: number,
    slot: string,
    file: File,
    reason?: string,
  ) => {
    const form = new FormData();
    form.append("file", file);
    if (reason) form.append("reason", reason);
    return api.request<ApiDocumentFile>(
      `/sales/${saleId}/tasks/${number}/documents/${slot}`,
      { method: "PUT", body: form },
    );
  },
  remove: (saleId: string, number: number, slot: string) =>
    api.request<void>(`/sales/${saleId}/tasks/${number}/documents/${slot}`, {
      method: "DELETE",
    }),
  downloadUrl: (documentId: string) =>
    api.request<DocumentDownload>(`/documents/${documentId}/download-url`),
};

export const pickersApi = {
  advisors: () => api.request<{ items: Advisor[] }>("/advisors"),
  selectableCatalog: () =>
    api.request<{ items: SelectableModel[] }>("/catalog/selectable"),
};
