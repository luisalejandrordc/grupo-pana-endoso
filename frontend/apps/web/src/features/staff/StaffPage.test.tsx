import { createQueryClient } from "@endosoflow/api";
import { QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { EmployeeProfile } from "../../shared/types";
import type { Employee } from "./api";
import { StaffPage } from "./StaffPage";

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

const employee = (over: Partial<Employee> & { id: string }): Employee => ({
  documentType: "DNI",
  documentNumber: "40123000",
  firstName: "Nombre",
  lastName: "Apellido",
  nameSource: "manual",
  nationality: "Peruana",
  gender: "F",
  birthdate: "1992-04-10",
  hireDate: "2023-03-01",
  personalPhone: null,
  personalEmail: null,
  businessPhone: "+51 1 555 0100",
  businessEmail: "x@grupopana.pe",
  role: "sales_advisor",
  isActive: true,
  hasPhoto: false,
  activatedAt: "2023-03-02T10:00:00Z",
  version: 3,
  createdAt: "2023-03-01T10:00:00Z",
  updatedAt: "2023-03-01T10:00:00Z",
  ...over,
});

const diego = employee({
  id: "diego",
  firstName: "Diego",
  lastName: "Ramírez",
  role: "system_admin",
  businessEmail: "diego@grupopana.pe",
});
const ana = employee({
  id: "ana",
  firstName: "Ana",
  lastName: "Rojas",
  businessEmail: "ana@grupopana.pe",
  personalPhone: "+51 999 111 222",
});
const carlos = employee({
  id: "carlos",
  firstName: "Carlos",
  lastName: "Mendoza",
  businessEmail: "carlos@grupopana.pe",
  activatedAt: null,
});
const lucia = employee({
  id: "lucia",
  firstName: "Lucía",
  lastName: "Paredes",
  isActive: false,
  businessEmail: "lucia@grupopana.pe",
});

const admin: EmployeeProfile = {
  id: "diego",
  documentType: "DNI",
  documentNumber: "40123001",
  firstName: "Diego",
  lastName: "Ramírez",
  businessEmail: "diego@grupopana.pe",
  role: "system_admin",
  hasPhoto: false,
};

type Call = {
  method: string;
  path: string;
  params: URLSearchParams;
  body: Record<string, unknown> | null;
};
let calls: Call[] = [];
/** Answers for the calls a test cares about: key is "METHOD /path". */
let answers: Record<string, () => Response> = {};

beforeEach(() => {
  calls = [];
  answers = {};
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(String(input), "http://localhost");
      const path = url.pathname.replace("/api/v1", "");
      const method = init?.method ?? "GET";
      calls.push({
        method,
        path,
        params: url.searchParams,
        body: init?.body ? JSON.parse(String(init.body)) : null,
      });
      const custom = answers[`${method} ${path}`];
      if (custom) return custom();
      if (method === "GET" && path === "/employees") {
        const items = [diego, ana, carlos, lucia];
        return json({ items, page: 1, pageSize: 20, total: items.length });
      }
      return failure(404, "NOT_FOUND", `Sin respuesta para ${method} ${path}`);
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

const toast = vi.fn();
afterEach(() => toast.mockReset());

function setup(
  props: Partial<Parameters<typeof StaffPage>[0]> = {},
  user = admin,
) {
  const onSelfUpdated = vi.fn();
  render(
    <QueryClientProvider client={createQueryClient()}>
      <StaffPage
        user={user}
        toast={toast}
        onSelfUpdated={onSelfUpdated}
        {...props}
      />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), onSelfUpdated };
}

const calling = (method: string, path: string) =>
  calls.filter((c) => c.method === method && c.path === path);
const lastList = () => {
  const lists = calling("GET", "/employees");
  return lists[lists.length - 1];
};
const rowOf = async (name: string) =>
  (await screen.findByText(name)).closest("tr") as HTMLElement;

async function openMenu(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  await screen.findByText(name);
  await user.click(screen.getByRole("button", { name: `Acciones de ${name}` }));
  return screen.getByRole("menu");
}
const itemsOf = () =>
  screen.getAllByRole("menuitem").map((item) => item.textContent);

describe("StaffPage", () => {
  describe("the list", () => {
    it("shows each person's role, state and activation, and marks the signed-in administrator", async () => {
      setup();
      const ana = within(await rowOf("Ana Rojas"));
      expect(ana.getByText("Asesor")).toBeVisible();
      expect(ana.getByText("Activo")).toBeVisible();
      expect(ana.getByText("Activada")).toBeVisible();
      expect(
        within(await rowOf("Carlos Mendoza")).getByText("Pendiente"),
      ).toBeVisible();
      expect(
        within(await rowOf("Lucía Paredes")).getByText("Inactivo"),
      ).toBeVisible();
      expect(
        within(await rowOf("Diego Ramírez")).getByText("Tú"),
      ).toBeVisible();
      expect(within(await rowOf("Ana Rojas")).queryByText("Tú")).toBeNull();
    });

    it("asks the server for the search, the role, the state and the page instead of filtering itself", async () => {
      const { user } = setup();
      await screen.findByText("Ana Rojas");
      expect(lastList().params.get("page")).toBe("1");
      expect(lastList().params.get("pageSize")).toBe("20");

      await user.type(screen.getByRole("searchbox"), "ana");
      await waitFor(() => expect(lastList().params.get("q")).toBe("ana"));

      await user.selectOptions(screen.getByLabelText("Rol"), "sales_manager");
      await waitFor(() =>
        expect(lastList().params.get("role")).toBe("sales_manager"),
      );

      await user.selectOptions(screen.getByLabelText("Estado"), "inactive");
      await waitFor(() =>
        expect(lastList().params.get("status")).toBe("inactive"),
      );
    });

    it("explains an empty result, and a failure with a way to try again", async () => {
      answers["GET /employees"] = () =>
        json({ items: [], page: 1, pageSize: 20, total: 0 });
      const { user } = setup();
      expect(await screen.findByText("Aún no hay colaboradores")).toBeVisible();

      // A refusal (4xx) is final, so the error shows at once; a 5xx would be retried first.
      answers["GET /employees"] = () =>
        failure(400, "BAD_REQUEST", "La búsqueda no es válida.");
      await user.type(screen.getByRole("searchbox"), "x");
      expect(await screen.findByRole("alert")).toHaveTextContent(
        "La búsqueda no es válida.",
      );
      expect(screen.getByRole("button", { name: /Reintentar/ })).toBeVisible();
    });

    it("is closed to anyone who is not the system administrator, and asks the server for nothing", async () => {
      setup({}, { ...admin, role: "sales_manager" });
      expect(await screen.findByText("Acceso restringido")).toBeVisible();
      expect(calling("GET", "/employees")).toHaveLength(0);
    });
  });

  describe("the actions menu", () => {
    it("offers each person only what applies to them", async () => {
      const { user } = setup();

      await openMenu(user, "Ana Rojas");
      expect(itemsOf()).toEqual(["Editar", "Desactivar"]);
      await user.keyboard("{Escape}");

      await openMenu(user, "Carlos Mendoza");
      expect(itemsOf()).toEqual([
        "Editar",
        "Reenviar activación",
        "Desactivar",
      ]);
      await user.keyboard("{Escape}");

      await openMenu(user, "Lucía Paredes");
      expect(itemsOf()).toEqual(["Editar", "Reactivar"]);
    });

    it("never lets administrators deactivate themselves", async () => {
      const { user } = setup();
      await openMenu(user, "Diego Ramírez");
      expect(itemsOf()).toEqual(["Editar"]);
    });

    it("opens the editor from the name or from the row, but not from the menu button", async () => {
      const { user } = setup();
      await user.click(
        await screen.findByRole("button", { name: "Ana Rojas" }),
      );
      expect(
        screen.getByRole("dialog", { name: "Editar colaborador" }),
      ).toBeVisible();
      await user.click(screen.getByRole("button", { name: "Cancelar" }));

      await user.click(within(await rowOf("Ana Rojas")).getByText("Asesor"));
      expect(
        screen.getByRole("dialog", { name: "Editar colaborador" }),
      ).toBeVisible();
      await user.click(screen.getByRole("button", { name: "Cancelar" }));

      await openMenu(user, "Carlos Mendoza");
      expect(screen.queryByRole("dialog")).toBeNull();
    });
  });

  describe("editing", () => {
    async function openEditor(
      user: ReturnType<typeof userEvent.setup>,
      name = "Ana Rojas",
    ) {
      await user.click(await screen.findByRole("button", { name }));
      return screen.getByRole("dialog", { name: "Editar colaborador" });
    }

    it("starts with the person's data and nothing to save", async () => {
      const { user } = setup();
      const dialog = within(await openEditor(user));
      expect(dialog.getByLabelText("Nombres")).toHaveValue("Ana");
      expect(dialog.getByLabelText("Correo corporativo")).toHaveValue(
        "ana@grupopana.pe",
      );
      expect(dialog.getByLabelText("Teléfono personal (opcional)")).toHaveValue(
        "+51 999 111 222",
      );
      expect(dialog.getByLabelText("Correo personal (opcional)")).toHaveValue(
        "",
      );
      expect(
        dialog.getByRole("button", { name: "Guardar cambios" }),
      ).toBeDisabled();
    });

    it("saves only what changed, with the version it started from, then closes and refreshes the list", async () => {
      answers["PATCH /employees/ana"] = () =>
        json({ ...ana, businessPhone: "+51 1 555 0199", version: 4 });
      const { user, onSelfUpdated } = setup();
      const dialog = within(await openEditor(user));
      const listCalls = calling("GET", "/employees").length;

      const phone = dialog.getByLabelText("Teléfono corporativo");
      await user.clear(phone);
      await user.type(phone, "+51 1 555 0199");
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));

      await waitFor(() =>
        expect(calling("PATCH", "/employees/ana")).toHaveLength(1),
      );
      expect(calling("PATCH", "/employees/ana")[0].body).toEqual({
        version: 3,
        businessPhone: "+51 1 555 0199",
      });
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Colaborador actualizado",
          tone: "success",
        }),
      );
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      await waitFor(() =>
        expect(calling("GET", "/employees").length).toBeGreaterThan(listCalls),
      );
      expect(onSelfUpdated).not.toHaveBeenCalled();
    });

    it("clears a personal contact by sending it empty", async () => {
      answers["PATCH /employees/ana"] = () =>
        json({ ...ana, personalPhone: null, version: 4 });
      const { user } = setup();
      const dialog = within(await openEditor(user));
      await user.clear(dialog.getByLabelText("Teléfono personal (opcional)"));
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));
      await waitFor(() =>
        expect(calling("PATCH", "/employees/ana")).toHaveLength(1),
      );
      expect(calling("PATCH", "/employees/ana")[0].body).toEqual({
        version: 3,
        personalPhone: "",
      });
    });

    it("keeps administrators from changing their own role, and refreshes the session when they save their own data", async () => {
      answers["PATCH /employees/diego"] = () =>
        json({ ...diego, businessPhone: "+51 1 555 0111", version: 4 });
      const { user, onSelfUpdated } = setup();
      const dialog = within(await openEditor(user, "Diego Ramírez"));
      expect(dialog.getByLabelText("Rol asignado")).toBeDisabled();
      expect(
        dialog.getByText("No puedes cambiar tu propio rol."),
      ).toBeVisible();

      const phone = dialog.getByLabelText("Teléfono corporativo");
      await user.clear(phone);
      await user.type(phone, "+51 1 555 0111");
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));
      await waitFor(() => expect(onSelfUpdated).toHaveBeenCalledOnce());
    });

    it("puts the server's field problems next to the fields and stays open", async () => {
      answers["PATCH /employees/ana"] = () =>
        failure(422, "VALIDATION_FAILED", "Revisa los datos.", [
          {
            field: "businessEmail",
            code: "DUPLICATE",
            message: "Ya existe una cuenta con este correo.",
          },
        ]);
      const { user } = setup();
      const dialog = within(await openEditor(user));
      const email = dialog.getByLabelText("Correo corporativo");
      await user.clear(email);
      await user.type(email, "otro@grupopana.pe");
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));

      expect(
        await dialog.findByText("Ya existe una cuenta con este correo."),
      ).toBeVisible();
      expect(email).toHaveAccessibleDescription(
        "Ya existe una cuenta con este correo.",
      );
      // The banner and the field's own message are both announced.
      expect(
        dialog.getAllByRole("alert").map((alert) => alert.textContent),
      ).toEqual(
        expect.arrayContaining([expect.stringContaining("Revisa los datos.")]),
      );
      expect(toast).not.toHaveBeenCalled();
    });

    it("when someone else saved first, says so and offers the current data, then saves against the new version", async () => {
      answers["PATCH /employees/ana"] = () =>
        failure(409, "VERSION_CONFLICT", "El registro cambió.");
      answers["GET /employees/ana"] = () =>
        json({ ...ana, businessPhone: "+51 1 555 0777", version: 5 });
      const { user } = setup();
      const dialog = within(await openEditor(user));

      const first = dialog.getByLabelText("Nombres");
      await user.clear(first);
      await user.type(first, "Anita");
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));

      expect(await dialog.findByRole("alert")).toHaveTextContent(
        "Otra persona modificó a este colaborador",
      );
      expect(
        dialog.getByRole("button", { name: "Guardar cambios" }),
      ).toBeDisabled();

      await user.click(
        dialog.getByRole("button", { name: "Cargar datos actuales" }),
      );
      await waitFor(() =>
        expect(dialog.getByLabelText("Teléfono corporativo")).toHaveValue(
          "+51 1 555 0777",
        ),
      );
      expect(dialog.getByLabelText("Nombres")).toHaveValue("Ana");
      expect(dialog.queryByRole("alert")).toBeNull();

      answers["PATCH /employees/ana"] = () =>
        json({ ...ana, firstName: "Anita", version: 6 });
      await user.clear(dialog.getByLabelText("Nombres"));
      await user.type(dialog.getByLabelText("Nombres"), "Anita");
      await user.click(dialog.getByRole("button", { name: "Guardar cambios" }));
      await waitFor(() =>
        expect(calling("PATCH", "/employees/ana")).toHaveLength(2),
      );
      expect(calling("PATCH", "/employees/ana")[1].body).toEqual({
        version: 5,
        firstName: "Anita",
      });
    });
  });

  describe("deactivating", () => {
    it("asks first, and does nothing if the person backs out", async () => {
      const { user } = setup();
      await openMenu(user, "Ana Rojas");
      await user.click(screen.getByRole("menuitem", { name: "Desactivar" }));

      const dialog = within(
        screen.getByRole("dialog", { name: "Desactivar a Ana Rojas" }),
      );
      expect(
        dialog.getByText(/Se cerrarán sus sesiones activas/),
      ).toBeVisible();
      await user.click(dialog.getByRole("button", { name: "Cancelar" }));
      expect(screen.queryByRole("dialog")).toBeNull();
      expect(calling("POST", "/employees/ana/deactivate")).toHaveLength(0);
    });

    it("deactivates on confirmation, says so, and refreshes the list", async () => {
      answers["POST /employees/ana/deactivate"] = () =>
        json({ ...ana, isActive: false });
      const { user } = setup();
      await openMenu(user, "Ana Rojas");
      await user.click(screen.getByRole("menuitem", { name: "Desactivar" }));
      const listCalls = calling("GET", "/employees").length;
      await user.click(
        within(screen.getByRole("dialog")).getByRole("button", {
          name: "Desactivar",
        }),
      );

      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
      expect(calling("POST", "/employees/ana/deactivate")).toHaveLength(1);
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Colaborador desactivado",
          message: "Ana Rojas ya no puede iniciar sesión.",
        }),
      );
      await waitFor(() =>
        expect(calling("GET", "/employees").length).toBeGreaterThan(listCalls),
      );
    });

    it("when the server refuses, says why inside the dialog and stays open", async () => {
      answers["POST /employees/ana/deactivate"] = () =>
        failure(
          409,
          "CONFLICT",
          "No se puede desactivar al último administrador.",
        );
      const { user } = setup();
      await openMenu(user, "Ana Rojas");
      await user.click(screen.getByRole("menuitem", { name: "Desactivar" }));
      const dialog = within(screen.getByRole("dialog"));
      await user.click(dialog.getByRole("button", { name: "Desactivar" }));

      expect(await dialog.findByRole("alert")).toHaveTextContent(
        "No se puede desactivar al último administrador.",
      );
      expect(screen.getByRole("dialog")).toBeVisible();
      expect(toast).not.toHaveBeenCalled();
    });
  });

  describe("reactivating and resending", () => {
    it("reactivates an inactive person in one step", async () => {
      answers["POST /employees/lucia/reactivate"] = () =>
        json({ ...lucia, isActive: true });
      const { user } = setup();
      await openMenu(user, "Lucía Paredes");
      await user.click(screen.getByRole("menuitem", { name: "Reactivar" }));
      await waitFor(() =>
        expect(calling("POST", "/employees/lucia/reactivate")).toHaveLength(1),
      );
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Colaborador reactivado",
          tone: "success",
        }),
      );
    });

    it("resends the activation link and says where it went", async () => {
      answers["POST /employees/carlos/resend-activation"] = () =>
        json({ status: "accepted" }, 202);
      const { user } = setup();
      await openMenu(user, "Carlos Mendoza");
      await user.click(
        screen.getByRole("menuitem", { name: "Reenviar activación" }),
      );
      await waitFor(() =>
        expect(
          calling("POST", "/employees/carlos/resend-activation"),
        ).toHaveLength(1),
      );
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Enlace de activación enviado",
          message: "Se envió a carlos@grupopana.pe.",
        }),
      );
    });

    it("reports a refusal as an error notification", async () => {
      answers["POST /employees/carlos/resend-activation"] = () =>
        failure(409, "CONFLICT", "Esta cuenta ya fue activada.");
      const { user } = setup();
      await openMenu(user, "Carlos Mendoza");
      await user.click(
        screen.getByRole("menuitem", { name: "Reenviar activación" }),
      );
      await waitFor(() =>
        expect(toast).toHaveBeenCalledWith({
          title: "No se pudo reenviar la activación",
          message: "Esta cuenta ya fue activada.",
          tone: "error",
        }),
      );
    });
  });

  describe("creating", () => {
    it("sends the new person's data, trimmed, and says whether the link went out", async () => {
      answers["POST /employees"] = () =>
        json(
          {
            employee: employee({
              id: "nuevo",
              firstName: "Luis",
              lastName: "Quispe",
            }),
            activationLinkIssued: true,
          },
          201,
        );
      const { user } = setup();
      await user.click(
        await screen.findByRole("button", { name: /Nuevo colaborador/ }),
      );
      const dialog = within(
        screen.getByRole("dialog", { name: "Nuevo colaborador" }),
      );

      await user.type(
        dialog.getByLabelText("Número de documento"),
        " 45871236 ",
      );
      await user.type(dialog.getByLabelText("Nombres"), "Luis");
      await user.type(dialog.getByLabelText("Apellidos"), "Quispe");
      fireEvent.change(dialog.getByLabelText("Fecha de nacimiento"), {
        target: { value: "1990-01-01" },
      });
      fireEvent.change(dialog.getByLabelText("Fecha de contratación"), {
        target: { value: "2024-02-01" },
      });
      await user.type(
        dialog.getByLabelText("Teléfono corporativo"),
        "+51 1 555 0100",
      );
      await user.type(
        dialog.getByLabelText("Correo corporativo"),
        "luis@grupopana.pe",
      );
      await user.click(
        dialog.getByRole("button", { name: "Crear colaborador" }),
      );

      await waitFor(() =>
        expect(calling("POST", "/employees")).toHaveLength(1),
      );
      expect(calling("POST", "/employees")[0].body).toMatchObject({
        documentType: "DNI",
        documentNumber: "45871236",
        firstName: "Luis",
        lastName: "Quispe",
        birthdate: "1990-01-01",
        hireDate: "2024-02-01",
        businessEmail: "luis@grupopana.pe",
        role: "sales_advisor",
      });
      expect(calling("POST", "/employees")[0].body).not.toHaveProperty(
        "personalEmail",
      );
      expect(toast).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "Colaborador creado",
          message: "Se emitió el enlace de activación.",
        }),
      );
      await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    });
  });
});
