import { createQueryClient } from "@endosoflow/api";
import { QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { vi } from "vitest";
import { api } from "../shared/api/client";
import type {
  CustomerProfile,
  PortalMilestone,
  PortalSaleDetail,
  PortalSaleSummary,
} from "../shared/types";
import App from "./App";

export const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

export const failure = (
  status: number,
  code: string,
  message: string,
  fields?: { field: string; code: string; message: string }[],
) => json({ error: { code, message, fields } }, status);

export const customer: CustomerProfile = {
  id: "c1",
  displayName: "María Fernanda Quispe",
  documentType: "DNI",
  documentNumber: "45871236",
  email: "maria@example.com",
  phone: "987654321",
};

export const session = {
  accessToken: "token-1",
  expiresAt: "2099-01-01T00:00:00Z",
  customer,
};

export const stages = (states: PortalMilestone["state"][]): PortalMilestone[] =>
  states.map((state, index) => ({
    number: index + 1,
    name: [
      "Cotización y Documentación",
      "Asignación y Pago",
      "Facturación Oficial",
      "Trámites Aduaneros",
      "Liberación y Placas",
      "Preparación y Entrega",
    ][index],
    description: `Descripción de la etapa ${index + 1}.`,
    state,
  }));

export const summary: PortalSaleSummary = {
  id: "11111111-1111-1111-1111-111111111111",
  code: "EF-2026-00042",
  state: "active",
  createdAt: "2026-03-10T15:00:00Z",
  vehicle: {
    model: "Corolla Cross",
    version: "XEI Hybrid",
    color: "Blanco Perlado",
    mainImageUrl: "https://img.example/main.jpg",
  },
  milestones: stages([
    "completed",
    "in_progress",
    "pending",
    "pending",
    "pending",
    "pending",
  ]),
};

export const detail: PortalSaleDetail = {
  id: summary.id,
  code: summary.code,
  state: "active",
  createdAt: summary.createdAt,
  completedAt: null,
  vehicle: {
    model: "Corolla Cross",
    version: "XEI Hybrid",
    color: "Blanco Perlado",
    referencePrice: "32500.00",
    categories: ["camioneta", "familiar"],
    mainImageUrl: "https://img.example/main.jpg",
    // Deliberately not in walking order: the portal must still put the main view first.
    images: [
      { viewpoint: "interior", url: "https://img.example/interior.jpg" },
      { viewpoint: "exterior_front", url: "https://img.example/front.jpg" },
      { viewpoint: "main_view", url: "https://img.example/main.jpg" },
    ],
  },
  advisor: {
    name: "Diego Ramírez",
    email: "diego@grupopana.pe",
    phone: "999111222",
    gender: "M",
  },
  milestones: summary.milestones,
  sections: [
    {
      taskNumber: 1,
      milestoneNumber: 1,
      title: "Cotización del Vehículo",
      completedAt: "2026-03-12T15:00:00Z",
      fields: [
        {
          key: "quotedFinalPrice",
          label: "Precio final cotizado",
          kind: "money",
          value: "35000.00",
        },
        {
          key: "quoteDate",
          label: "Fecha de la cotización",
          kind: "date",
          value: "2026-03-01",
        },
        {
          key: "note",
          label: "Observación",
          kind: "text",
          value: "Incluye accesorios",
        },
      ],
      documents: [
        {
          id: "d1",
          taskNumber: 1,
          label: "Cotización oficial",
          fileName: "cotizacion.pdf",
          sizeBytes: 204800,
          uploadedAt: "2026-03-12T15:00:00Z",
        },
      ],
    },
  ],
};

export type Answers = Record<string, () => Response>;

export const calls: { method: string; path: string; body: unknown }[] = [];
export const answers: Answers = {};
export const state = { signedIn: false };

/** Replaces `fetch` with a small fake of the API: what is not in `answers` is a 404. */
export function installApi() {
  calls.length = 0;
  for (const key of Object.keys(answers)) delete answers[key];
  state.signedIn = false;
  api.setToken(null);
  api.setUnauthorizedHandler(() => undefined);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname.replace(
        "/api/v1",
        "",
      );
      const method = init?.method ?? "GET";
      calls.push({
        method,
        path,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      });
      const custom = answers[`${method} ${path}`];
      if (custom) return custom();
      if (path === "/auth/customer/refresh")
        return state.signedIn
          ? json(session)
          : failure(401, "SESSION_INVALID", "Sin sesión.");
      if (path === "/auth/customer/logout")
        return new Response(null, { status: 204 });
      if (path === "/portal/sales") return json({ items: [summary] });
      return failure(
        404,
        "ROUTE_NOT_FOUND",
        `Sin respuesta para ${method} ${path}`,
      );
    }),
  );
}

export function openPortal(path = "/") {
  return render(
    <QueryClientProvider client={createQueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
