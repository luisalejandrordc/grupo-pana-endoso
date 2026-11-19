import { api } from "../../shared/api/client";
import { queryString, type Operations, type Schemas } from "@endosoflow/api";

/** A sale as the detail returns it: people, vehicle, milestones, the 13 task summaries and what the caller may do. */
export type ApiSale = Schemas["Sale"];
export type SaleListItem = Schemas["SaleListItem"];
export type SaleList = Schemas["SaleList"];
export type SaleHistory = Schemas["SaleHistory"];
export type HistoryEntry = Schemas["SaleHistoryEvent"];
export type ApiTask = Schemas["TaskSummary"];
export type ApiTaskDetail = Schemas["TaskDetail"];
export type TaskPermissions = Schemas["TaskPermissions"];
export type SalePermissions = Schemas["SalePermissions"];
export type TaskWarning = Schemas["TaskWarning"];
export type ApiSlot = Schemas["DocumentSlot"];
export type ApiTaskSlot = Schemas["TaskDocumentSlot"];
export type ApiDocumentFile = Schemas["DocumentFile"];
export type DocumentDownload = Schemas["DocumentDownload"];
export type SelectableModel = Schemas["SelectableModel"];
export type Advisor = Schemas["Advisor"];
export type CreateSaleBody = Schemas["CreateSaleRequest"];
export type CancelCategory = Schemas["CancellationCategory"];
export type SaleStateValue = Schemas["SaleState"];
export type Temperature = Schemas["SaleTemperature"];
export type TaskStatusValue = Schemas["TaskStatus"];

/** The query of GET /sales, straight from the generated contract so a new filter or sort appears here by itself. */
export type SaleListParams = NonNullable<
  Operations["listSales"]["parameters"]["query"]
>;

const json = (method: string, body?: unknown) => ({
  method,
  body: body === undefined ? undefined : JSON.stringify(body),
});

export const salesApi = {
  list: (params: SaleListParams) =>
    api.request<SaleList>(`/sales${queryString({ ...params })}`),
  get: (id: string) => api.request<ApiSale>(`/sales/${id}`),
  create: (body: CreateSaleBody) =>
    api.request<ApiSale>("/sales", json("POST", body)),
  updateNotes: (id: string, version: number, notes: string | null) =>
    api.request<ApiSale>(`/sales/${id}`, json("PATCH", { version, notes })),
  reassign: (id: string, advisorId: string, reason?: string) =>
    api.request<ApiSale>(
      `/sales/${id}/reassign`,
      json("POST", { advisorId, reason: reason || undefined }),
    ),
  cancel: (id: string, category: CancelCategory, reason: string) =>
    api.request<ApiSale>(
      `/sales/${id}/cancel`,
      json("POST", { category, reason }),
    ),
  reopen: (id: string, reason: string) =>
    api.request<ApiSale>(`/sales/${id}/reopen`, json("POST", { reason })),
  remove: (id: string, reason: string, confirm: string) =>
    api.request<void>(`/sales/${id}`, json("DELETE", { reason, confirm })),
  history: (id: string, page: number, pageSize: number) =>
    api.request<SaleHistory>(
      `/sales/${id}/history${queryString({ page, pageSize })}`,
    ),
};

export const tasksApi = {
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
    api.request<ApiTaskDetail>(
      `/sales/${saleId}/tasks/${number}/data`,
      json("PATCH", body),
    ),
  /** Notes carry no version: the last save wins and an open data form never goes stale because of them. */
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
