import type { DocumentType, EmployeeProfile, SessionResponse } from "../types";

const API_BASE = (import.meta.env.VITE_API_BASE_URL || "/api/v1").replace(
  /\/$/,
  "",
);

export class ApiError extends Error {
  code: string;
  status: number;
  fields?: { field: string; code: string; message: string }[];
  requestId?: string;

  constructor(
    message: string,
    code: string,
    status: number,
    fields?: ApiError["fields"],
    requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.fields = fields;
    this.requestId = requestId;
  }
}

type RequestOptions = RequestInit & { skipRefresh?: boolean };

export interface PhotoLink {
  url: string;
  expiresAt: string;
  mimeType: string;
}

export class ApiClient {
  private accessToken: string | null = null;
  private onUnauthorized?: () => void;

  setUnauthorizedHandler(handler: () => void) {
    this.onUnauthorized = handler;
  }

  setToken(token: string | null) {
    this.accessToken = token;
  }

  getToken() {
    return this.accessToken;
  }

  private async parseResponse<T>(response: Response): Promise<T> {
    if (response.status === 204) return undefined as T;
    const payload = await response.json().catch(() => ({}));
    if (!response.ok) {
      const error = payload?.error;
      throw new ApiError(
        error?.message || "Ocurrió un error. Inténtalo nuevamente.",
        error?.code || "REQUEST_FAILED",
        response.status,
        error?.fields,
        error?.requestId,
      );
    }
    return payload as T;
  }

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const { skipRefresh, ...fetchOptions } = options;
    const headers = new Headers(fetchOptions.headers || {});
    if (
      fetchOptions.body &&
      !(fetchOptions.body instanceof FormData) &&
      !headers.has("Content-Type")
    ) {
      headers.set("Content-Type", "application/json");
    }
    if (this.accessToken)
      headers.set("Authorization", `Bearer ${this.accessToken}`);
    const response = await fetch(`${API_BASE}${path}`, {
      ...fetchOptions,
      headers,
      credentials: "include",
    });

    if (
      response.status === 401 &&
      !skipRefresh &&
      this.accessToken &&
      !path.startsWith("/auth/")
    ) {
      const refreshed = await this.refreshSession();
      if (refreshed)
        return this.request<T>(path, { ...options, skipRefresh: true });
      this.accessToken = null;
      this.onUnauthorized?.();
    }
    return this.parseResponse<T>(response);
  }

  async login(input: {
    documentType: DocumentType;
    documentNumber: string;
    password: string;
    rememberMe: boolean;
  }) {
    const session = await this.request<SessionResponse>(
      "/auth/employee/login",
      {
        method: "POST",
        body: JSON.stringify(input),
        skipRefresh: true,
      },
    );
    this.accessToken = session.accessToken;
    return session;
  }

  async refreshSession(): Promise<SessionResponse | null> {
    const response = await fetch(`${API_BASE}/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: { "X-Requested-With": "EndosoFlow" },
    });
    if (!response.ok) return null;
    const session = (await response.json()) as SessionResponse;
    this.accessToken = session.accessToken;
    return session;
  }

  async logout() {
    try {
      await this.request<void>("/auth/logout", {
        method: "POST",
        headers: { "X-Requested-With": "EndosoFlow" },
      });
    } finally {
      this.accessToken = null;
    }
  }

  health() {
    return this.request<{
      status: "ok" | "degraded";
      database: "ok" | "error";
    }>("/health", { skipRefresh: true });
  }

  me() {
    return this.request<EmployeeProfile>("/me");
  }

  /** A short-lived address for an employee's picture (404 PHOTO_NOT_FOUND when there is none). */
  employeePhoto(id: string) {
    return this.request<PhotoLink>(`/employees/${id}/photo`);
  }

  /** Sets or replaces the signed-in employee's picture. */
  setMyPhoto(file: File) {
    const form = new FormData();
    form.append("file", file);
    return this.request<PhotoLink>("/me/photo", { method: "PUT", body: form });
  }

  removeMyPhoto() {
    return this.request<void>("/me/photo", { method: "DELETE" });
  }

  forgotPassword(input: {
    documentType: DocumentType;
    documentNumber: string;
  }) {
    return this.request<{ status: "accepted" }>("/auth/forgot-password", {
      method: "POST",
      body: JSON.stringify(input),
      skipRefresh: true,
    });
  }

  changePassword(input: { currentPassword: string; newPassword: string }) {
    return this.request<void>("/me/password", {
      method: "PUT",
      body: JSON.stringify(input),
    });
  }
}

export const api = new ApiClient();
