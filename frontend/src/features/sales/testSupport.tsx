import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { vi } from "vitest";
import type { ReactElement } from "react";
import { MemoryRouter } from "react-router-dom";
import type { ApiSale, ApiTaskDetail, TaskPermissions } from "./api";

export const SALE_ID = "0198aaaa-0000-7000-8000-000000000001";

export const noRights: TaskPermissions = {
  canStart: false,
  canComplete: false,
  canEditData: false,
  needsReason: false,
  canSaveNotes: false,
  canRestart: false,
  canReopen: false,
  canUploadDocuments: false,
  canRemoveDocuments: false,
};

export function task(over: Partial<ApiTaskDetail> = {}): ApiTaskDetail {
  return {
    number: 1,
    name: "Cotización del Vehículo",
    milestoneNumber: 1,
    status: "in_progress",
    version: 3,
    startedAt: "2026-03-02T10:00:00Z",
    completedAt: null,
    dueDate: "2026-03-05T10:00:00Z",
    completionPhase: null,
    recommendedDurationHours: 72,
    isEligible: true,
    data: {
      initialToyotaAmount: "30000.00",
      initialCommission: "2500.00",
      accessoryAmount: null,
      releaseProcessingAmount: null,
    },
    calculated: { quotedFinalPrice: "32500.00" },
    documents: [
      {
        slotCode: "quotation_pdf",
        label: "Cotización oficial (PDF)",
        allowedFormat: "pdf",
        required: true,
        customerVisible: true,
        file: null,
      },
    ],
    permissions: {
      ...noRights,
      canComplete: true,
      canEditData: true,
      canSaveNotes: true,
      canUploadDocuments: true,
      canRemoveDocuments: true,
    },
    warnings: [],
    notes: null,
    ...over,
  } as ApiTaskDetail;
}

export function sale(over: Partial<ApiSale> = {}): ApiSale {
  const tasks = Array.from({ length: 13 }, (_, i) => ({
    number: i + 1,
    name: `Tarea ${i + 1}`,
    milestoneNumber: Math.min(6, Math.floor(i / 2) + 1),
    status: i < 3 ? "completed" : i === 3 ? "in_progress" : "pending",
    startedAt: null,
    completedAt: null,
    dueDate: null,
    completionPhase: null,
    recommendedDurationHours: 24,
    isEligible: i === 3,
    version: 1,
  }));
  return {
    id: SALE_ID,
    code: "EF-2026-00001",
    state: "active",
    temperature: "warm",
    notes: null,
    version: 5,
    createdAt: "2026-03-02T10:00:00Z",
    updatedAt: "2026-03-02T10:00:00Z",
    completedAt: null,
    quotedFinalPrice: null,
    customer: {
      id: "c1",
      name: "Embajada de Japón",
      documentType: "RUC",
      documentNumber: "20123456789",
      customerType: "embajada",
    },
    vehicle: {
      model: "Hilux",
      version: "SR",
      color: "Blanco",
      referencePrice: "35000.00",
      imageUrl: null,
      series: null,
      vin: null,
      licensePlate: null,
    },
    advisor: {
      id: "adv-1",
      name: "Ana Rojas",
      email: "ana@grupopana.pe",
      phone: "+51 999",
      hasPhoto: false,
    },
    currentTask: {
      number: 4,
      name: "Tarea 4",
      status: "in_progress",
      milestoneNumber: 2,
      dueDate: null,
      isOverdue: false,
    },
    milestones: Array.from({ length: 6 }, (_, i) => ({
      number: i + 1,
      name: `Etapa ${i + 1}`,
      description: "",
      state: i < 1 ? "completed" : i === 1 ? "in_progress" : "pending",
    })),
    tasks,
    permissions: {
      canReassign: true,
      canCancel: true,
      canReopen: false,
      canDelete: false,
      canEditNotes: true,
    },
    cancellation: null,
    ...over,
  } as unknown as ApiSale;
}

export const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

export interface Request {
  method: string;
  path: string;
  body: unknown;
}

/** Stubs fetch with a router; every request is recorded. */
export function stubApi(
  routes: Record<string, (req: Request) => Response | Promise<Response>>,
) {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname.replace("/api/v1", "");
      const method = (init?.method ?? "GET").toUpperCase();
      let body: unknown = undefined;
      if (typeof init?.body === "string") body = JSON.parse(init.body);
      else if (init?.body instanceof FormData) body = init.body;
      const req = { method, path, body };
      requests.push(req);
      const handler = routes[`${method} ${path}`];
      if (handler) return handler(req);
      return json(
        {
          error: {
            code: "ROUTE_NOT_FOUND",
            message: `Sin ruta ${method} ${path}`,
          },
        },
        404,
      );
    }),
  );
  return requests;
}

export function withProviders(ui: ReactElement, path = "/") {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[path]}>{ui}</MemoryRouter>
    </QueryClientProvider>
  );
}
