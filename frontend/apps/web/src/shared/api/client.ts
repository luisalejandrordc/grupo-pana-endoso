import { ApiClient } from "@endosoflow/api";
import type { DocumentType, EmployeeProfile, SessionResponse } from "../types";

export interface PhotoLink {
  url: string;
  expiresAt: string;
  mimeType: string;
}

/** The staff site's calls: the employee login and what belongs to the signed-in employee. */
export class EmployeeApiClient extends ApiClient {
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
    this.setToken(session.accessToken);
    return session;
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

export const api = new EmployeeApiClient({
  baseUrl: import.meta.env.VITE_API_BASE_URL || "/api/v1",
});
