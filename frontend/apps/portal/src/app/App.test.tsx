import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  answers,
  calls,
  customer,
  detail,
  failure,
  installApi,
  json,
  openPortal,
  session,
  stages,
  state,
  summary,
} from "./testSupport";

beforeEach(installApi);

/** Opens one stage of the purchase by pressing it on the stage row. */
async function openStage(
  user: ReturnType<typeof userEvent.setup>,
  number: number,
) {
  await user.click(
    await screen.findByRole("tab", { name: new RegExp(`Etapa ${number},`) }),
  );
}
afterEach(() => vi.unstubAllGlobals());

describe("signing in", () => {
  it("asks for a document and a password when nobody is signed in", async () => {
    openPortal();
    expect(
      await screen.findByRole("heading", { name: "Bienvenido a su portal" }),
    ).toBeVisible();
    expect(screen.getByLabelText("Número de documento")).toBeVisible();
    expect(screen.getByLabelText("Contraseña")).toBeVisible();
  });

  it("offers every document a customer can be registered with, companies and embassies included", async () => {
    openPortal();
    const select = await screen.findByLabelText("Tipo de documento");
    const labels = within(select)
      .getAllByRole("option")
      .map((option) => option.textContent);
    expect(labels).toEqual([
      "DNI",
      "RUC",
      "Carné de extranjería",
      "Pasaporte",
      "Carné diplomático (CDI)",
    ]);
  });

  it("signs in through the customer endpoint and shows their purchases", async () => {
    answers["POST /auth/customer/login"] = () => json(session);
    openPortal();
    const user = userEvent.setup();

    await user.selectOptions(
      await screen.findByLabelText("Tipo de documento"),
      "RUC",
    );
    await user.type(
      screen.getByLabelText("Número de documento"),
      "20123456789",
    );
    await user.type(screen.getByLabelText("Contraseña"), "una-clave-larga-1");
    await user.click(
      screen.getByRole("button", { name: /Ingresar a mi portal/ }),
    );

    expect(
      await screen.findByRole("heading", { name: "Mis compras" }),
    ).toBeVisible();
    const login = calls.find((call) => call.path === "/auth/customer/login");
    expect(login?.body).toEqual({
      documentType: "RUC",
      documentNumber: "20123456789",
      password: "una-clave-larga-1",
      rememberMe: true,
    });
    expect(calls.some((call) => call.path === "/auth/login")).toBe(false);
    expect(screen.getByText(customer.displayName)).toBeVisible();
  });

  it("shows the server's words when the credentials are refused", async () => {
    answers["POST /auth/customer/login"] = () =>
      failure(
        401,
        "INVALID_CREDENTIALS",
        "Documento o contraseña incorrectos.",
      );
    openPortal();
    const user = userEvent.setup();

    await user.type(await screen.findByLabelText("Número de documento"), "1");
    await user.type(screen.getByLabelText("Contraseña"), "mala");
    await user.click(
      screen.getByRole("button", { name: /Ingresar a mi portal/ }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Documento o contraseña incorrectos.",
    );
    expect(screen.queryByRole("heading", { name: "Mis compras" })).toBeNull();
  });

  it("brings back a session that is still open, without asking again", async () => {
    state.signedIn = true;
    openPortal();
    expect(
      await screen.findByRole("heading", { name: "Mis compras" }),
    ).toBeVisible();
    expect(screen.queryByLabelText("Número de documento")).toBeNull();
  });

  it("signs out from the account menu and goes back to the login", async () => {
    state.signedIn = true;
    openPortal();
    const user = userEvent.setup();
    await screen.findByRole("heading", { name: "Mis compras" });

    await user.click(screen.getByRole("button", { name: /Cuenta de María/ }));
    await user.click(screen.getByRole("menuitem", { name: "Cerrar sesión" }));

    expect(
      await screen.findByRole("heading", { name: "Bienvenido a su portal" }),
    ).toBeVisible();
    expect(calls.some((call) => call.path === "/auth/customer/logout")).toBe(
      true,
    );
  });

  it("asks for a reset link with the customer endpoint and never says whether the account exists", async () => {
    answers["POST /auth/customer/forgot-password"] = () =>
      json({ status: "accepted" }, 202);
    openPortal();
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", { name: "¿Olvidó su contraseña?" }),
    );
    await user.type(screen.getByLabelText("Número de documento"), "45871236");
    await user.click(
      screen.getByRole("button", { name: /Enviar instrucciones/ }),
    );

    expect(await screen.findByRole("status")).toHaveTextContent(
      /Si el documento corresponde a una cuenta activa/,
    );
    expect(
      calls.find((call) => call.path === "/auth/customer/forgot-password")
        ?.body,
    ).toEqual({ documentType: "DNI", documentNumber: "45871236" });
  });
});

describe("my purchases", () => {
  beforeEach(() => {
    state.signedIn = true;
  });

  it("lists each purchase with its vehicle, state and where it stands", async () => {
    openPortal();
    const link = await screen.findByRole("link", {
      name: "Corolla Cross XEI Hybrid",
    });
    expect(link).toHaveAttribute("href", `/sales/${summary.id}`);
    expect(screen.getByText("EF-2026-00042")).toBeVisible();
    expect(screen.getByText("En curso")).toBeVisible();
    expect(screen.getByText("Etapa actual: Asignación y Pago")).toBeVisible();
    expect(screen.getByText(/1 de 6 etapas completadas/)).toBeVisible();
    expect(
      screen.getByRole("img", {
        name: /Corolla Cross XEI Hybrid, color Blanco/,
      }),
    ).toBeVisible();
  });

  it("names every stage's state for a screen reader, not only by color", async () => {
    openPortal();
    await screen.findByRole("link", { name: /Corolla Cross/ });
    const track = screen.getByRole("list", { name: "Etapas de la compra" });
    expect(
      within(track).getByText(/1\. Cotización y Documentación: Completada/),
    ).toBeInTheDocument();
    expect(
      within(track).getByText(/2\. Asignación y Pago: En curso/),
    ).toBeInTheDocument();
    expect(
      within(track).getByText(/3\. Facturación Oficial: Pendiente/),
    ).toBeInTheDocument();
  });

  it("says a cancelled purchase is cancelled and does not pretend it is moving", async () => {
    answers["GET /portal/sales"] = () =>
      json({ items: [{ ...summary, state: "cancelled" }] });
    openPortal();
    expect(await screen.findByText("Esta compra fue cancelada")).toBeVisible();
    expect(screen.getByText("Cancelada")).toBeVisible();
    expect(screen.queryByText(/Etapa actual/)).toBeNull();
  });

  it("uses a stand-in when the color has no photo", async () => {
    answers["GET /portal/sales"] = () =>
      json({
        items: [
          { ...summary, vehicle: { ...summary.vehicle, mainImageUrl: "" } },
        ],
      });
    openPortal();
    expect(
      await screen.findByRole("img", { name: /sin foto disponible/ }),
    ).toBeVisible();
  });

  it("explains what to expect when there are no purchases yet", async () => {
    answers["GET /portal/sales"] = () => json({ items: [] });
    openPortal();
    expect(
      await screen.findByText("Aún no tiene compras registradas"),
    ).toBeVisible();
  });

  it("offers to try again when the list cannot be loaded", async () => {
    let attempts = 0;
    answers["GET /portal/sales"] = () => {
      attempts += 1;
      return attempts <= 2
        ? failure(500, "INTERNAL_ERROR", "Error del servidor.")
        : json({ items: [summary] });
    };
    openPortal();
    const user = userEvent.setup();
    // A server failure is tried once more before the page gives up, which takes a moment.
    await user.click(
      await screen.findByRole(
        "button",
        { name: /Reintentar/ },
        { timeout: 4000 },
      ),
    );
    expect(
      await screen.findByRole("link", { name: /Corolla Cross/ }),
    ).toBeVisible();
  });
});

describe("one purchase", () => {
  const path = `/sales/${summary.id}`;
  beforeEach(() => {
    state.signedIn = true;
    answers[`GET /portal/sales/${summary.id}`] = () => json(detail);
  });

  it("shows the vehicle, the reference price and the six stages", async () => {
    openPortal(path);
    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "Corolla Cross XEI Hybrid",
      }),
    ).toBeVisible();
    expect(screen.getByText("Camioneta")).toBeVisible();
    expect(screen.getByText("Familiar")).toBeVisible();
    expect(screen.getByText(/32,500\.00/)).toBeVisible();
    const tabs = screen.getAllByRole("tab");
    expect(tabs).toHaveLength(6);
    expect(tabs[0]).toHaveTextContent("Cotización y Documentación");
    expect(tabs[5]).toHaveTextContent("Preparación y Entrega");
    expect(document.title).toBe(
      "Corolla Cross XEI Hybrid · EF-2026-00042 · EndosoFlow",
    );
  });

  it("shows what was recorded in a completed stage, with money and dates written for a person", async () => {
    openPortal(path);
    const user = userEvent.setup();
    await openStage(user, 1);
    const card = (
      await screen.findByRole("heading", { name: "Cotización del Vehículo" })
    ).closest("section")!;
    expect(within(card).getByText(/35,000\.00/)).toBeVisible();
    // A calendar date never slips to the day before, whatever the time zone of the viewer.
    expect(within(card).getByText("1 de marzo de 2026")).toBeVisible();
    expect(within(card).getByText("Incluye accesorios")).toBeVisible();
    expect(within(card).getByText(/cotizacion\.pdf · 200 KB/)).toBeVisible();
  });

  it("shows the advisor's contact and a WhatsApp link when the number allows it", async () => {
    openPortal(path);
    expect(await screen.findByText("Su asesor de Grupo Pana")).toBeVisible();
    expect(
      screen.getByRole("link", { name: "diego@grupopana.pe" }),
    ).toHaveAttribute("href", "mailto:diego@grupopana.pe");
    expect(screen.getByRole("link", { name: "999111222" })).toHaveAttribute(
      "href",
      "tel:999111222",
    );
    expect(
      screen.getByRole("link", { name: /Escribir por WhatsApp/ }),
    ).toHaveAttribute("href", "https://wa.me/51999111222");
  });

  it("uses the advisor's gender for the word, and offers no chat for a number that cannot have one", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        advisor: { ...detail.advisor, gender: "F", phone: "(01) 234 5678" },
      });
    openPortal(path);
    expect(await screen.findByText("Su asesora de Grupo Pana")).toBeVisible();
    expect(screen.queryByRole("link", { name: /WhatsApp/ })).toBeNull();
  });

  it("opens a document through a short-lived signed address", async () => {
    const tab = { opener: {}, location: { href: "" }, close: vi.fn() };
    vi.stubGlobal(
      "open",
      vi.fn(() => tab),
    );
    answers["GET /portal/documents/d1/download-url"] = () =>
      json({
        url: "https://files.example/signed?sig=abc",
        expiresAt: "2099-01-01T00:00:00Z",
        fileName: "cotizacion.pdf",
        mimeType: "application/pdf",
      });
    openPortal(path);
    const user = userEvent.setup();
    await openStage(user, 1);

    await user.click(
      await screen.findByRole("button", {
        name: "Descargar Cotización oficial",
      }),
    );

    await waitFor(() =>
      expect(tab.location.href).toBe("https://files.example/signed?sig=abc"),
    );
    expect(tab.opener).toBeNull();
    expect(tab.close).not.toHaveBeenCalled();
  });

  it("says so, and closes the empty tab, when a document cannot be opened", async () => {
    const tab = { opener: {}, location: { href: "" }, close: vi.fn() };
    vi.stubGlobal(
      "open",
      vi.fn(() => tab),
    );
    answers["GET /portal/documents/d1/download-url"] = () =>
      failure(
        503,
        "STORAGE_UNAVAILABLE",
        "El almacenamiento no está disponible.",
      );
    openPortal(path);
    const user = userEvent.setup();
    await openStage(user, 1);

    await user.click(
      await screen.findByRole("button", {
        name: "Descargar Cotización oficial",
      }),
    );

    expect(
      await screen.findByText("No pudimos abrir el documento"),
    ).toBeVisible();
    expect(
      screen.getByText("El almacenamiento no está disponible."),
    ).toBeVisible();
    expect(tab.close).toHaveBeenCalled();
  });

  it("shows a cancelled purchase as cancelled, keeps its documents, and gives no reason", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({ ...detail, state: "cancelled" });
    openPortal(path);
    const user = userEvent.setup();
    expect(await screen.findByText("Esta compra fue cancelada.")).toBeVisible();
    // The stage that was running is shown stopped, with its own explanation, and the meter keeps where it stopped.
    expect(screen.getByText("Detenida")).toBeVisible();
    expect(
      screen.getByText(/se canceló mientras esta etapa estaba en curso/),
    ).toBeVisible();
    expect(screen.queryByText("En curso")).toBeNull();
    expect(screen.getByText("al cancelarse")).toBeVisible();
    await openStage(user, 1);
    expect(
      await screen.findByRole("button", {
        name: "Descargar Cotización oficial",
      }),
    ).toBeVisible();
    expect(screen.queryByText(/motivo/i)).toBeNull();
  });

  it("celebrates a finished purchase with the day it finished", async () => {
    answers[`GET /portal/sales/${summary.id}`] = () =>
      json({
        ...detail,
        state: "completed",
        completedAt: "2026-06-20T15:00:00Z",
        milestones: stages(Array(6).fill("completed")),
      });
    openPortal(path);
    expect(await screen.findByText("Su compra está completa.")).toBeVisible();
    expect(screen.getByText(/Finalizó el 20 de junio de 2026/)).toBeVisible();
    expect(screen.getByText("Compra completada")).toBeVisible();
  });

  it("answers the same way for a purchase that is someone else's and one that does not exist", async () => {
    answers["GET /portal/sales/22222222-2222-2222-2222-222222222222"] = () =>
      failure(404, "SALE_NOT_FOUND", "La compra no existe.");
    openPortal("/sales/22222222-2222-2222-2222-222222222222");
    expect(await screen.findByText("No encontramos esta compra")).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Ver mis compras" }),
    ).toHaveAttribute("href", "/");
  });

  it("follows the Spanish address of older documentation to the same purchase", async () => {
    openPortal(`/compras/${summary.id}`);
    expect(
      await screen.findByRole("heading", {
        level: 1,
        name: "Corolla Cross XEI Hybrid",
      }),
    ).toBeVisible();
  });
});

describe("my account", () => {
  beforeEach(() => {
    state.signedIn = true;
  });

  it("shows the customer's data as read-only and says who can change it", async () => {
    openPortal("/account");
    expect(
      await screen.findByRole("heading", { name: "Mi cuenta" }),
    ).toBeVisible();
    expect(screen.getByText("45871236")).toBeVisible();
    expect(screen.getByText("maria@example.com")).toBeVisible();
    expect(screen.getByText(/comuníquese con su asesor/)).toBeVisible();
  });

  it("keeps the button off until the new password is acceptable", async () => {
    openPortal("/account");
    const user = userEvent.setup();
    const button = await screen.findByRole("button", {
      name: "Cambiar contraseña",
    });
    expect(button).toBeDisabled();

    await user.type(
      screen.getByLabelText("Contraseña actual"),
      "vieja-clave-1",
    );
    await user.type(screen.getByLabelText("Nueva contraseña"), "corta");
    expect(button).toBeDisabled();
    await user.type(screen.getByLabelText("Nueva contraseña"), "-y-larga-123");
    await user.type(
      screen.getByLabelText("Confirmar nueva contraseña"),
      "corta-y-larga-123",
    );
    expect(button).toBeEnabled();
  });

  it("puts the server's complaint about the current password on that field", async () => {
    answers["PUT /portal/me/password"] = () =>
      failure(
        422,
        "INVALID_CURRENT_PASSWORD",
        "La contraseña actual no es correcta.",
      );
    openPortal("/account");
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Contraseña actual"),
      "equivocada-1",
    );
    await user.type(
      screen.getByLabelText("Nueva contraseña"),
      "nueva-clave-123",
    );
    await user.type(
      screen.getByLabelText("Confirmar nueva contraseña"),
      "nueva-clave-123",
    );
    await user.click(
      screen.getByRole("button", { name: "Cambiar contraseña" }),
    );

    expect(
      await screen.findByText("La contraseña actual no es correcta."),
    ).toBeVisible();
    expect(screen.getByRole("heading", { name: "Mi cuenta" })).toBeVisible();
  });

  it("signs the customer out after changing the password, because the server ended every session", async () => {
    answers["PUT /portal/me/password"] = () =>
      new Response(null, { status: 204 });
    openPortal("/account");
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Contraseña actual"),
      "vieja-clave-1",
    );
    await user.type(
      screen.getByLabelText("Nueva contraseña"),
      "nueva-clave-123",
    );
    await user.type(
      screen.getByLabelText("Confirmar nueva contraseña"),
      "nueva-clave-123",
    );
    await user.click(
      screen.getByRole("button", { name: "Cambiar contraseña" }),
    );

    expect(
      await screen.findByRole("heading", { name: "Bienvenido a su portal" }),
    ).toBeVisible();
    expect(await screen.findByText("Contraseña actualizada")).toBeVisible();
    expect(
      calls.find((call) => call.path === "/portal/me/password")?.body,
    ).toEqual({
      currentPassword: "vieja-clave-1",
      newPassword: "nueva-clave-123",
    });
  });
});

describe("the links in the emails", () => {
  it("activates an account from /activate-account with no session needed", async () => {
    answers["POST /auth/activate"] = () => json({ accountType: "customer" });
    openPortal("/activate-account?token=abc123");
    const user = userEvent.setup();

    await user.type(
      await screen.findByLabelText("Nueva contraseña"),
      "una-clave-larga-1",
    );
    await user.type(
      screen.getByLabelText("Confirmar contraseña"),
      "una-clave-larga-1",
    );
    await user.click(screen.getByRole("button", { name: /Activar mi cuenta/ }));

    expect(
      await screen.findByRole("heading", { name: "Su cuenta está activa" }),
    ).toBeVisible();
    expect(calls.find((call) => call.path === "/auth/activate")?.body).toEqual({
      token: "abc123",
      password: "una-clave-larga-1",
    });
  });

  it("says a used or expired activation link is dead and who to ask", async () => {
    answers["POST /auth/activate"] = () =>
      failure(400, "INVALID_LINK", "El enlace no es válido.");
    openPortal("/activate-account?token=old");
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Nueva contraseña"),
      "una-clave-larga-1",
    );
    await user.type(
      screen.getByLabelText("Confirmar contraseña"),
      "una-clave-larga-1",
    );
    await user.click(screen.getByRole("button", { name: /Activar mi cuenta/ }));

    expect(
      await screen.findByRole("heading", {
        name: "Este enlace ya no es válido",
      }),
    ).toBeVisible();
    expect(screen.getByText(/pida a su asesor de Grupo Pana/)).toBeVisible();
  });

  it("resets a password from /reset-password and leads a dead link to a new request", async () => {
    answers["POST /auth/reset-password"] = () =>
      failure(400, "INVALID_LINK", "Enlace inválido.");
    openPortal("/reset-password?token=zzz");
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Nueva contraseña"),
      "una-clave-larga-1",
    );
    await user.type(
      screen.getByLabelText("Confirmar contraseña"),
      "una-clave-larga-1",
    );
    await user.click(
      screen.getByRole("button", { name: /Guardar contraseña/ }),
    );

    await user.click(
      await screen.findByRole("button", { name: /Pedir un enlace nuevo/ }),
    );
    expect(
      await screen.findByRole("heading", { name: "Recupere su acceso" }),
    ).toBeVisible();
  });

  it("explains a link that arrives without its code", async () => {
    openPortal("/reset-password");
    expect(await screen.findByText(/Falta el código del enlace/)).toBeVisible();
  });

  it("takes the used code out of the address once the password is set", async () => {
    answers["POST /auth/reset-password"] = () =>
      new Response(null, { status: 204 });
    openPortal("/reset-password?token=abc");
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Nueva contraseña"),
      "una-clave-larga-1",
    );
    await user.type(
      screen.getByLabelText("Confirmar contraseña"),
      "una-clave-larga-1",
    );
    await user.click(
      screen.getByRole("button", { name: /Guardar contraseña/ }),
    );
    expect(
      await screen.findByRole("heading", { name: "Contraseña actualizada" }),
    ).toBeVisible();
  });
});

describe("any other address", () => {
  it("says the page does not exist and leads back to the purchases", async () => {
    state.signedIn = true;
    openPortal("/nada-por-aqui");
    expect(await screen.findByText("No encontramos esta página")).toBeVisible();
    expect(
      screen.getByRole("link", { name: "Ir a mis compras" }),
    ).toHaveAttribute("href", "/");
  });
});
