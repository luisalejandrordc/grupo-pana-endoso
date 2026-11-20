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

function setup(
  answer: () => Response = () => new Response(null, { status: 204 }),
) {
  const calls: {
    method: string;
    path: string;
    body: Record<string, string> | null;
  }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname.replace(
        "/api/v1",
        "",
      );
      calls.push({
        method: init?.method ?? "GET",
        path,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      });
      return path === "/me/password"
        ? answer()
        : new Response(null, { status: 404 });
    }),
  );
  const toast = vi.fn();
  const onSignOut = vi.fn();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter>
        <ProfilePage
          user={user}
          onSignOut={onSignOut}
          onPhotoChanged={vi.fn()}
          toast={toast}
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return { calls, toast, onSignOut, user: userEvent.setup() };
}

const passwordCalls = (calls: { path: string }[]) =>
  calls.filter((c) => c.path === "/me/password");

describe("ProfilePage: changing the password", () => {
  it("has a labelled field, with its own show button, for each of the three passwords", () => {
    setup();
    for (const label of [
      "Contraseña actual",
      "Nueva contraseña",
      "Confirmar nueva contraseña",
    ]) {
      expect(screen.getByLabelText(label)).toHaveAttribute("type", "password");
      expect(
        screen.getByRole("button", { name: `Mostrar ${label.toLowerCase()}` }),
      ).toBeVisible();
    }
  });

  it("refuses a short password or a mismatch before asking the server", async () => {
    const { calls, toast, user: ui } = setup();
    await ui.type(screen.getByLabelText("Contraseña actual"), "la-actual-123");
    await ui.type(screen.getByLabelText("Nueva contraseña"), "corta");
    await ui.type(screen.getByLabelText("Confirmar nueva contraseña"), "corta");
    // The page checks the length itself (and says so), instead of leaving it to the browser.
    await ui.click(
      screen.getByRole("button", { name: /Actualizar contraseña/ }),
    );
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Contraseña muy corta", tone: "error" }),
    );
    expect(passwordCalls(calls)).toHaveLength(0);
  });

  it("changes the password and signs out, as it says it will", async () => {
    const { calls, toast, onSignOut, user: ui } = setup();
    await ui.type(screen.getByLabelText("Contraseña actual"), "la-actual-123");
    await ui.type(screen.getByLabelText("Nueva contraseña"), "una-clave-nueva");
    await ui.type(
      screen.getByLabelText("Confirmar nueva contraseña"),
      "una-clave-nueva",
    );
    expect(screen.getByText(/Las contraseñas coinciden/)).toHaveTextContent(
      "(cumplido)",
    );
    await ui.click(
      screen.getByRole("button", { name: /Actualizar contraseña/ }),
    );

    await waitFor(() => expect(passwordCalls(calls)).toHaveLength(1));
    expect(passwordCalls(calls)[0]).toMatchObject({
      method: "PUT",
      body: {
        currentPassword: "la-actual-123",
        newPassword: "una-clave-nueva",
      },
    });
    expect(toast).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Contraseña actualizada" }),
    );
    expect(onSignOut).toHaveBeenCalledOnce();
  });
});
