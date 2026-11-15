import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EmployeeProfile } from "../../shared/types";
import { ProfilePage } from "./ProfilePage";

afterEach(() => vi.unstubAllGlobals());

const user: EmployeeProfile = {
  id: "e1",
  documentType: "DNI",
  documentNumber: "40123003",
  firstName: "Diego",
  lastName: "Ramírez",
  businessEmail: "diego@grupopana.pe",
  role: "sales_advisor",
  hasPhoto: false,
};

function setup(profile: EmployeeProfile = user) {
  const calls: { method: string; path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname.replace(
        "/api/v1",
        "",
      );
      calls.push({
        method: (init?.method ?? "GET").toUpperCase(),
        path,
        body: init?.body,
      });
      if (path === "/me/photo" && init?.method === "DELETE")
        return new Response(null, { status: 204 });
      if (path === "/me/photo")
        return new Response(
          JSON.stringify({
            url: "https://f.test/p.png",
            expiresAt: "2099-01-01T00:00:00Z",
            mimeType: "image/png",
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      return new Response(
        JSON.stringify({
          url: "https://f.test/p.png",
          expiresAt: "2099-01-01T00:00:00Z",
          mimeType: "image/png",
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      );
    }),
  );
  const onPhotoChanged = vi.fn();
  const toast = vi.fn();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <ProfilePage
          user={profile}
          onSignOut={() => undefined}
          onPhotoChanged={onPhotoChanged}
          toast={toast}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { calls, onPhotoChanged, toast };
}

const file = (name: string, type: string, size = 100) =>
  new File([new Uint8Array(size)], name, { type });
const input = () =>
  document.querySelector<HTMLInputElement>('input[type="file"]')!;

describe("profile picture", () => {
  it("uploads the chosen picture as multipart and tells the app to reload", async () => {
    const u = userEvent.setup();
    const { calls, onPhotoChanged } = setup();
    await u.upload(input(), file("yo.png", "image/png"));
    await waitFor(() => expect(onPhotoChanged).toHaveBeenCalled());
    const put = calls.find(
      (c) => c.method === "PUT" && c.path === "/me/photo",
    )!;
    expect(put.body).toBeInstanceOf(FormData);
    expect((put.body as FormData).get("file")).toBeInstanceOf(File);
  });

  it("refuses a file that is not JPEG, PNG or WebP without sending it", async () => {
    const u = userEvent.setup({ applyAccept: false });
    const { calls, toast } = setup();
    await u.upload(input(), file("yo.gif", "image/gif"));
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Formato no admitido", tone: "error" }),
    );
    expect(calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("refuses a picture over 2 MB without sending it", async () => {
    const u = userEvent.setup();
    const { calls, toast } = setup();
    await u.upload(input(), file("big.png", "image/png", 2 * 1024 * 1024 + 1));
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "La foto es muy pesada" }),
    );
    expect(calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("offers to remove the picture only when there is one", async () => {
    setup();
    expect(
      screen.queryByRole("button", { name: "Quitar foto" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Subir foto/ }),
    ).toBeInTheDocument();
  });

  it("removes it", async () => {
    const u = userEvent.setup();
    const { calls, onPhotoChanged } = setup({ ...user, hasPhoto: true });
    await u.click(screen.getByRole("button", { name: "Quitar foto" }));
    await waitFor(() => expect(onPhotoChanged).toHaveBeenCalled());
    expect(
      calls.some((c) => c.method === "DELETE" && c.path === "/me/photo"),
    ).toBe(true);
  });
});
