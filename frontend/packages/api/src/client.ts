/** The header the backend requires on cookie-backed calls (refresh, logout) to refuse cross-site forgeries. */
export const CSRF_HEADER = { "X-Requested-With": "EndosoFlow" } as const;

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

export type RequestOptions = RequestInit & { skipRefresh?: boolean };

export interface ApiClientOptions {
  /**
   * Where the API lives: "/api/v1" when a dev proxy or a rewrite rule makes it same-origin (which keeps
   * the refresh cookie first-party), or a full URL such as "https://api.example.com/api/v1".
   */
  baseUrl: string;
}

/**
 * The HTTP core every EndosoFlow site shares: it keeps the access token, sends it, and when the server
 * answers 401 it asks for a new one (once) and repeats the call. A site extends it with its own login and
 * its own endpoints: `class EmployeeApiClient extends ApiClient { login() {...} }`.
 */
export class ApiClient {
  private accessToken: string | null = null;
  private onUnauthorized?: () => void;
  private readonly baseUrl: string;

  constructor({ baseUrl }: ApiClientOptions) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
  }

  /** Called when the session cannot be renewed: the screen should go back to the login. */
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
    const response = await fetch(`${this.baseUrl}${path}`, {
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

  /** Trades the refresh cookie for a new access token; null when there is no valid session. */
  async refreshSession<
    T extends { accessToken: string } = { accessToken: string },
  >(): Promise<T | null> {
    const response = await fetch(`${this.baseUrl}/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: CSRF_HEADER,
    });
    if (!response.ok) return null;
    const session = (await response.json()) as T;
    this.accessToken = session.accessToken;
    return session;
  }

  async logout() {
    try {
      await this.request<void>("/auth/logout", {
        method: "POST",
        headers: CSRF_HEADER,
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
}
