import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  json,
  sale,
  SALE_ID,
  stubApi,
  task,
  withProviders,
} from "./testSupport";
import { TaskPanel } from "./TaskPanel";

afterEach(() => vi.unstubAllGlobals());

const path = `/sales/${SALE_ID}/tasks/2`;
const toast = vi.fn();

const found = {
  documentType: "DNI",
  documentNumber: "45871236",
  source: "provider",
  firstName: "LUIS ALBERTO",
  lastName: "QUISPE HUAMÁN",
};

const signerTask = (data: Record<string, unknown> = {}, over = {}) =>
  task({
    number: 2,
    name: "Debida Diligencia del Cliente",
    milestoneNumber: 1,
    data: {
      signerDocumentType: "DNI",
      signerDocumentNumber: null,
      signerFirstName: null,
      signerLastName: null,
      signerNameSource: null,
      pepStatus: null,
      ddcRegime: null,
      ...data,
    },
    calculated: {},
    documents: [],
    ...over,
  });

function show(
  detail = signerTask(),
  extra: Record<string, () => Response> = {},
) {
  const requests = stubApi({
    [`GET ${path}`]: () => json(detail),
    ...extra,
  });
  render(withProviders(<TaskPanel sale={sale()} number={2} toast={toast} />));
  return requests;
}

const consult = () => screen.findByRole("button", { name: /Consultar RENIEC/ });

describe("task 2: the signer's name from their DNI", () => {
  it("offers the lookup for a DNI only, and only once it has eight digits", async () => {
    show();
    const user = userEvent.setup();
    const button = await consult();
    expect(button).toBeDisabled();
    await user.type(screen.getByLabelText("Número de documento"), "45871236");
    expect(button).toBeEnabled();

    await user.selectOptions(
      screen.getByLabelText("Tipo de documento del firmante"),
      "PAS",
    );
    expect(
      screen.queryByRole("button", { name: /Consultar RENIEC/ }),
    ).toBeNull();
  });

  it("fills the names, locks them, and saves the signer with the name marked as looked up", async () => {
    const requests = show(signerTask(), {
      [`GET /lookups/identity`]: () => json(found),
      [`PATCH ${path}/data`]: () => json(signerTask({}, { version: 4 })),
    });
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Número de documento"),
      "45871236",
    );
    await user.click(await consult());

    expect(await screen.findByDisplayValue("LUIS ALBERTO")).toBeVisible();
    expect(screen.getByDisplayValue("QUISPE HUAMÁN")).toBeVisible();
    expect(screen.getByLabelText("Nombres del firmante")).toHaveAttribute(
      "readonly",
    );
    expect(screen.getByLabelText("Apellidos del firmante")).toHaveAttribute(
      "readonly",
    );
    expect(screen.getByText(/Nombre obtenido de RENIEC/)).toBeVisible();
    expect(
      screen.queryByRole("button", { name: /Consultar RENIEC/ }),
    ).toBeNull();
    const lookup = requests.find((r) => r.path === "/lookups/identity")!;
    expect(lookup.search).toContain("documentType=DNI");
    expect(lookup.search).toContain("documentNumber=45871236");

    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(requests.some((r) => r.method === "PATCH")).toBe(true),
    );
    expect(requests.find((r) => r.method === "PATCH")!.body).toEqual({
      version: 3,
      data: {
        signerDocumentNumber: "45871236",
        signerFirstName: "LUIS ALBERTO",
        signerLastName: "QUISPE HUAMÁN",
        signerNameSource: "looked_up",
      },
    });
  });

  it("saves names that were typed as manual", async () => {
    const requests = show(signerTask(), {
      [`PATCH ${path}/data`]: () => json(signerTask({}, { version: 4 })),
    });
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Nombres del firmante"),
      "Luis",
    );
    await user.type(screen.getByLabelText("Apellidos del firmante"), "Quispe");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(requests.some((r) => r.method === "PATCH")).toBe(true),
    );
    expect(requests.find((r) => r.method === "PATCH")!.body).toEqual({
      version: 3,
      data: {
        signerFirstName: "Luis",
        signerLastName: "Quispe",
        signerNameSource: "manual",
      },
    });
  });

  it("says why when the lookup fails, and leaves the names to be typed", async () => {
    show(signerTask(), {
      [`GET /lookups/identity`]: () =>
        json(
          {
            error: {
              code: "LOOKUP_NOT_FOUND",
              message: "No encontramos ese DNI.",
            },
          },
          404,
        ),
    });
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Número de documento"),
      "45871236",
    );
    await user.click(await consult());

    expect(
      await screen.findByText(
        /No encontramos ese DNI.*Puedes ingresar los datos manualmente/,
      ),
    ).toBeVisible();
    const names = screen.getByLabelText("Nombres del firmante");
    expect(names).not.toHaveAttribute("readonly");
    await user.type(names, "Luis");
    expect(names).toHaveValue("Luis");
  });

  it("clears looked-up names when the document changes, since they belonged to the old one", async () => {
    show(
      signerTask({
        signerDocumentNumber: "45871236",
        signerFirstName: "LUIS ALBERTO",
        signerLastName: "QUISPE HUAMÁN",
        signerNameSource: "looked_up",
      }),
    );
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Número de documento"), "0");
    expect(screen.getByLabelText("Nombres del firmante")).toHaveValue("");
    expect(screen.getByLabelText("Apellidos del firmante")).toHaveValue("");
    expect(screen.getByLabelText("Nombres del firmante")).not.toHaveAttribute(
      "readonly",
    );
  });

  it("opens a task saved with a looked-up name locked, with the way to edit it by hand", async () => {
    const requests = show(
      signerTask({
        signerDocumentNumber: "45871236",
        signerFirstName: "LUIS ALBERTO",
        signerLastName: "QUISPE HUAMÁN",
        signerNameSource: "looked_up",
      }),
      { [`PATCH ${path}/data`]: () => json(signerTask({}, { version: 4 })) },
    );
    const user = userEvent.setup();
    expect(
      await screen.findByLabelText("Nombres del firmante"),
    ).toHaveAttribute("readonly");
    // Nothing to save until something changes.
    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled();

    await user.click(
      screen.getByRole("button", { name: "Editar manualmente" }),
    );
    const names = screen.getByLabelText("Nombres del firmante");
    expect(names).not.toHaveAttribute("readonly");
    await user.clear(names);
    await user.type(names, "Luis Alberto");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(requests.some((r) => r.method === "PATCH")).toBe(true),
    );
    expect(requests.find((r) => r.method === "PATCH")!.body).toEqual({
      version: 3,
      data: { signerFirstName: "Luis Alberto", signerNameSource: "manual" },
    });
  });

  it("goes back to 'looked up' when a hand-edited name is put back as it was", async () => {
    show(
      signerTask({
        signerDocumentNumber: "45871236",
        signerFirstName: "LUIS ALBERTO",
        signerLastName: "QUISPE HUAMÁN",
        signerNameSource: "looked_up",
      }),
    );
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Editar manualmente" }),
    );
    const names = screen.getByLabelText("Nombres del firmante");
    await user.type(names, "X");
    await user.type(names, "{Backspace}");
    // Back to what the server holds: nothing changed, so there is nothing to save.
    expect(screen.getByRole("button", { name: "Guardar" })).toBeDisabled();
  });

  it("shows where a completed task's name came from, without offering to change it", async () => {
    show(
      signerTask(
        {
          signerDocumentNumber: "45871236",
          signerFirstName: "LUIS ALBERTO",
          signerLastName: "QUISPE HUAMÁN",
          signerNameSource: "looked_up",
        },
        { status: "completed", completedAt: "2026-03-04T10:00:00Z" },
      ),
    );
    expect(await screen.findByText(/Obtenido de RENIEC\./)).toBeVisible();
    const hint = screen.getByText(/Obtenido de RENIEC\./);
    expect(
      within(hint).queryByRole("button", { name: "Editar manualmente" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: /Consultar RENIEC/ }),
    ).toBeNull();
  });
});
