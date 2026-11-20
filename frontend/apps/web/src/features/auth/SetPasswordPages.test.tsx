import { createQueryClient } from "@endosoflow/api";
import { QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../../app/App";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const failure = (
  status: number,
  code: string,
  message: string,
  fields?: { field: string; code: string; message: string }[],
) => json({ error: { code, message, fields } }, status);

const session = {
  accessToken: "t",
  expiresAt: "2099-01-01T00:00:00Z",
  employee: {
    id: "e1",
    documentType: "DNI",
    documentNumber: "40123003",
    firstName: "Diego",
    lastName: "Ramírez",
    businessEmail: "diego@grupopana.pe",
    role: "sales_advisor",
    hasPhoto: false,
  },
};

type Call = {
  method: string;
  path: string;
  body: Record<string, string> | null;
  auth: string | null;
};
let calls: Call[];
let answers: Record<string, () => Response>;
let signedIn: boolean;

beforeEach(() => {
  calls = [];
  answers = {};
  signedIn = false;
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
        auth: new Headers(init?.headers).get("Authorization"),
      });
      const custom = answers[`${method} ${path}`];
      if (custom) return custom();
      if (path === "/auth/refresh")
        return signedIn
          ? json(session)
          : failure(401, "UNAUTHENTICATED", "Sin sesión.");
      if (path === "/auth/logout") return new Response(null, { status: 204 });
      if (path === "/sales")
        return json({ items: [], page: 1, pageSize: 12, total: 0 });
      if (path === "/advisors") return json({ items: [] });
      return failure(404, "NOT_FOUND", `Sin respuesta para ${method} ${path}`);
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function Where() {
  const { pathname, search } = useLocation();
  return <output data-testid="where">{pathname + search}</output>;
}

function open(path: string) {
  render(
    <QueryClientProvider client={createQueryClient()}>
      <MemoryRouter initialEntries={[path]}>
        <App />
        <Where />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

const calling = (method: string, path: string) =>
  calls.filter((c) => c.method === method && c.path === path);
const newPassword = () => screen.getByLabelText("Nueva contraseña");
const confirmation = () => screen.getByLabelText("Confirmar contraseña");

async function fill(
  user: ReturnType<typeof userEvent.setup>,
  value = "una-clave-larga",
) {
  await user.type(newPassword(), value);
  await user.type(confirmation(), value);
}

describe.each([
  {
    name: "activating an account",
    path: "/activate-account",
    endpoint: "/auth/activate",
    heading: "Activa tu cuenta",
    submit: "Activar cuenta",
    doneHeading: "Tu cuenta está activa",
    title: "Activar cuenta · EndosoFlow",
    retry: "Ir a iniciar sesión",
  },
  {
    name: "resetting a password",
    path: "/reset-password",
    endpoint: "/auth/reset-password",
    heading: "Crea una nueva contraseña",
    submit: "Guardar contraseña",
    doneHeading: "Contraseña actualizada",
    title: "Restablecer contraseña · EndosoFlow",
    retry: "Pedir un enlace nuevo",
  },
])(
  "$name",
  ({ path, endpoint, heading, submit, doneHeading, title, retry }) => {
    it("opens from the email link with no session, and shows the form instead of the login", async () => {
      open(`${path}?token=abc-123`);
      expect(
        await screen.findByRole("heading", { name: heading }),
      ).toBeVisible();
      expect(
        screen.queryByRole("heading", { name: /Qué bueno tenerte/ }),
      ).toBeNull();
      expect(newPassword()).toHaveAttribute("type", "password");
      expect(document.title).toBe(title);
    });

    it("asks for nothing until the password is long enough and typed twice the same", async () => {
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      const button = screen.getByRole("button", { name: submit });
      expect(button).toBeDisabled();
      expect(screen.getByText(/Al menos 10 caracteres/)).toHaveTextContent(
        "(pendiente)",
      );

      await user.type(newPassword(), "una-clave-larga");
      await user.type(confirmation(), "otra-distinta");
      expect(screen.getByText(/Al menos 10 caracteres/)).toHaveTextContent(
        "(cumplido)",
      );
      expect(screen.getByText(/Las contraseñas coinciden/)).toHaveTextContent(
        "(pendiente)",
      );
      expect(button).toBeDisabled();

      await user.clear(confirmation());
      await user.type(confirmation(), "una-clave-larga");
      expect(screen.getByText(/Las contraseñas coinciden/)).toHaveTextContent(
        "(cumplido)",
      );
      expect(button).toBeEnabled();
    });

    it("sends the link's token with the password, then says it is done and drops the token from the address", async () => {
      answers[`POST ${endpoint}`] = () => new Response(null, { status: 204 });
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      await fill(user);
      await user.click(screen.getByRole("button", { name: submit }));

      expect(
        await screen.findByRole("heading", { name: doneHeading }),
      ).toBeVisible();
      expect(calling("POST", endpoint)).toHaveLength(1);
      expect(calling("POST", endpoint)[0].body).toEqual({
        token: "abc-123",
        password: "una-clave-larga",
      });
      // a public call: it carries no session
      expect(calling("POST", endpoint)[0].auth).toBeNull();
      expect(screen.getByTestId("where")).toHaveTextContent(
        new RegExp(`^${path}$`),
      );
      expect(document.body).not.toHaveTextContent("una-clave-larga");
      expect(document.body).not.toHaveTextContent("abc-123");
    });

    it("leads to the login once it is done", async () => {
      answers[`POST ${endpoint}`] = () => new Response(null, { status: 204 });
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      await fill(user);
      await user.click(screen.getByRole("button", { name: submit }));
      await user.click(
        await screen.findByRole("button", { name: /Iniciar sesión/ }),
      );
      expect(
        await screen.findByRole("heading", { name: /Qué bueno tenerte/ }),
      ).toBeVisible();
      expect(screen.getByTestId("where")).toHaveTextContent(/^\/$/);
    });

    it("explains a link that expired or was already used, and offers the way out", async () => {
      answers[`POST ${endpoint}`] = () =>
        failure(400, "INVALID_TOKEN", "El enlace no es válido.");
      const user = open(`${path}?token=viejo`);
      await screen.findByRole("heading", { name: heading });
      await fill(user);
      await user.click(screen.getByRole("button", { name: submit }));

      expect(
        await screen.findByRole("heading", {
          name: "Este enlace ya no es válido",
        }),
      ).toBeVisible();
      expect(screen.queryByLabelText("Nueva contraseña")).toBeNull();
      await user.click(screen.getByRole("button", { name: retry }));
      expect(
        await screen.findByRole("heading", {
          name:
            path === "/reset-password"
              ? "Recupera tu acceso"
              : /Qué bueno tenerte/,
        }),
      ).toBeVisible();
    });

    it("without a token in the link, says it is incomplete and sends nothing", async () => {
      open(path);
      expect(
        await screen.findByRole("heading", {
          name: "Este enlace ya no es válido",
        }),
      ).toBeVisible();
      expect(screen.getByText(/Falta el código del enlace/)).toBeVisible();
      expect(screen.queryByLabelText("Nueva contraseña")).toBeNull();
      expect(calling("POST", endpoint)).toHaveLength(0);
    });

    it("puts the server's complaint about the password on the field and lets the person try again", async () => {
      answers[`POST ${endpoint}`] = () =>
        failure(
          422,
          "VALIDATION_FAILED",
          "La contraseña no cumple los requisitos.",
          [
            {
              field: "password",
              code: "TOO_COMMON",
              message: "Es una contraseña demasiado común.",
            },
          ],
        );
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      await fill(user, "1234567890");
      await user.click(screen.getByRole("button", { name: submit }));

      expect(
        await screen.findByText("Es una contraseña demasiado común."),
      ).toBeVisible();
      expect(newPassword()).toHaveAccessibleDescription(
        "Es una contraseña demasiado común.",
      );
      expect(screen.getByRole("button", { name: submit })).toBeEnabled();
      expect(screen.getByLabelText("Nueva contraseña")).toHaveValue(
        "1234567890",
      );
    });

    it("when the server asks to slow down, says so and keeps the form", async () => {
      answers[`POST ${endpoint}`] = () =>
        failure(429, "RATE_LIMITED", "Demasiados intentos. Espera un momento.");
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      await fill(user);
      await user.click(screen.getByRole("button", { name: submit }));
      expect(
        await screen.findByText("Demasiados intentos. Espera un momento."),
      ).toBeVisible();
      expect(screen.getByRole("button", { name: submit })).toBeEnabled();
    });

    it("lets each password be shown to check what was typed", async () => {
      const user = open(`${path}?token=abc-123`);
      await screen.findByRole("heading", { name: heading });
      await user.type(newPassword(), "clave");
      await user.click(
        screen.getByRole("button", { name: "Mostrar nueva contraseña" }),
      );
      expect(newPassword()).toHaveAttribute("type", "text");
      expect(confirmation()).toHaveAttribute("type", "password");
      expect(
        screen.getByRole("button", { name: "Mostrar confirmar contraseña" }),
      ).toBeVisible();
    });
  },
);

describe("a browser that is already signed in", () => {
  it("is signed out by a reset, because the server ended every session of the account", async () => {
    signedIn = true;
    answers["POST /auth/reset-password"] = () =>
      new Response(null, { status: 204 });
    const user = open("/reset-password?token=abc-123");
    await screen.findByRole("heading", { name: "Crea una nueva contraseña" });
    await fill(user);
    await user.click(
      screen.getByRole("button", { name: "Guardar contraseña" }),
    );
    await screen.findByRole("heading", { name: "Contraseña actualizada" });
    expect(calling("POST", "/auth/logout")).toHaveLength(1);

    await user.click(screen.getByRole("button", { name: /Iniciar sesión/ }));
    expect(
      await screen.findByRole("heading", { name: /Qué bueno tenerte/ }),
    ).toBeVisible();
  });

  it("is left signed in by an activation, which ends no session", async () => {
    signedIn = true;
    answers["POST /auth/activate"] = () => new Response(null, { status: 204 });
    const user = open("/activate-account?token=abc-123");
    await screen.findByRole("heading", { name: "Activa tu cuenta" });
    await fill(user);
    await user.click(screen.getByRole("button", { name: "Activar cuenta" }));
    await screen.findByRole("heading", { name: "Tu cuenta está activa" });
    await waitFor(() =>
      expect(calling("POST", "/auth/activate")).toHaveLength(1),
    );
    expect(calling("POST", "/auth/logout")).toHaveLength(0);
  });
});
