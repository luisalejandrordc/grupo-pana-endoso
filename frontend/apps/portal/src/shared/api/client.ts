import {
  ApiClient,
  CSRF_HEADER,
  type ApiClientOptions,
  type Schemas,
} from "@endosoflow/api";
import type {
  CustomerDocumentType,
  CustomerProfile,
  CustomerSession,
  DocumentDownload,
  PortalSaleDetail,
  PortalSaleSummary,
} from "../types";

const json = (body: unknown) => ({
  method: "POST",
  body: JSON.stringify(body),
  skipRefresh: true,
});

/**
 * The portal's calls. It extends the shared HTTP core with the customers' own endpoints: their sign-in, their
 * session (a cookie with its own name and path, so it can never be mixed up with a staff session) and the
 * read-only `/portal` API.
 */
export class CustomerApiClient extends ApiClient {
  /** The core keeps its base address private, and renewing a session needs it. */
  private readonly root: string;

  constructor(options: ApiClientOptions) {
    super(options);
    this.root = options.baseUrl.replace(/\/$/, "");
  }

  async login(input: {
    documentType: CustomerDocumentType;
    documentNumber: string;
    password: string;
    rememberMe: boolean;
  }) {
    const session = await this.request<CustomerSession>(
      "/auth/customer/login",
      json(input),
    );
    this.setToken(session.accessToken);
    return session;
  }

  /** The core renews with the staff route; a customer's session lives on its own. */
  override async refreshSession<
    T extends { accessToken: string } = { accessToken: string },
  >(): Promise<T | null> {
    const response = await fetch(`${this.root}/auth/customer/refresh`, {
      method: "POST",
      credentials: "include",
      headers: CSRF_HEADER,
    });
    if (!response.ok) return null;
    const session = (await response.json()) as T;
    this.setToken(session.accessToken);
    return session;
  }

  override async logout() {
    try {
      await this.request<void>("/auth/customer/logout", {
        method: "POST",
        headers: CSRF_HEADER,
        skipRefresh: true,
      });
    } finally {
      this.setToken(null);
    }
  }

  forgotPassword(input: {
    documentType: CustomerDocumentType;
    documentNumber: string;
  }) {
    return this.request<{ status: "accepted" }>(
      "/auth/customer/forgot-password",
      json(input),
    );
  }

  /** Sets the first password from the activation email. The same endpoint serves staff and customers. */
  activateAccount(input: Schemas["TokenPasswordRequest"]) {
    return this.request<Schemas["ActivationResult"]>(
      "/auth/activate",
      json(input),
    );
  }

  /** Sets a new password from the reset email. It ends every session of the account. */
  resetPassword(input: Schemas["TokenPasswordRequest"]) {
    return this.request<void>("/auth/reset-password", json(input));
  }

  me() {
    return this.request<CustomerProfile>("/portal/me");
  }

  /** Changes the password. The server ends every session, this one included. */
  changePassword(input: { currentPassword: string; newPassword: string }) {
    return this.request<void>("/portal/me/password", {
      method: "PUT",
      body: JSON.stringify(input),
    });
  }

  listSales() {
    return this.request<{ items: PortalSaleSummary[] }>("/portal/sales");
  }

  getSale(id: string) {
    return this.request<PortalSaleDetail>(`/portal/sales/${id}`);
  }

  /** A short-lived signed address for one of the customer's files. */
  documentUrl(id: string) {
    return this.request<DocumentDownload>(
      `/portal/documents/${id}/download-url`,
    );
  }
}

export const api = new CustomerApiClient({
  baseUrl: import.meta.env.VITE_API_BASE_URL || "/api/v1",
});
