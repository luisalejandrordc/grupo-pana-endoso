# EndosoFlow — Employee Portal

Provisional employee-side frontend for the EndosoFlow / Grupo Pana workflow. Built with React 18, Vite, and TypeScript. The UI is Spanish, desktop-first, and responsive for tablet and mobile.

## What is included

- Employee login and forgot-password flow.
- Role-aware workspace for Administrador, Gerencia de ventas, and Asesor de ventas.
- Sales workspace with search, filters, priorities, states, task visibility, and sample metrics.
- Four-step new-sale wizard: customer, vehicle, sale details, review.
- Sale detail with a 6-milestone progress strip and 13 task nodes; tabs for summary, tasks, documents, internal notes, and history.
- Demo task start/complete, sale cancellation/reopening, advisor reassignment, and local note/document changes.
- Customer directory, sample vehicle catalog, employee administration, profile, and password-change form.
- A live API client for endpoints present in the supplied OpenAPI contract: `GET /health` is documented (the UI is prepared for health checks), employee login, refresh, logout, forgot-password, `/me`, `/me/password`, and employee list/create routes.

## Important: current API boundary

The supplied OpenAPI document currently lists System, Authentication, and Staff paths. Sales, tasks, documents, customers, vehicle catalog, search, and reference endpoints appear in the architecture/requirements material but are not present in that OpenAPI file yet. This frontend **does not call invented routes** for those resources. The corresponding screens run against local sample data only while Demo Mode is on; with Demo Mode off, they display a clear integration placeholder. The employee staff list and create form use the documented API in live mode.

Demo changes live in React state and reset after a page reload. They do not touch the Go backend, send activation emails, or upload files to storage. File selection in Demo Mode is presentation-only.

## Quick start (for the presentation)

Requirements: Node.js 20+ and npm.

```bash
cd frontend
npm install
cp .env.example .env
npm run dev
```

Open the local URL printed by Vite. Leave `VITE_DEMO_MODE=true` for the presentation and click **Entrar en modo demostración**. Use the **Vista demo** selector in the top bar to preview the Admin, Gerencia, and Asesor role variants.

## Connect the current Go API

1. Start the Go backend, e.g. at `http://localhost:8080`.
2. In `.env.local`, set:

   ```dotenv
   VITE_DEMO_MODE=false
   VITE_API_BASE_URL=/api/v1
   VITE_API_PROXY_TARGET=http://localhost:8080
   ```

3. Restart Vite. The dev proxy forwards `/api` to the Go service, preserving the `/api/v1` path.
4. Sign in with the employee document type, number, and password already provisioned in the backend.

In live mode the access token stays in memory; the refresh cookie is handled by the browser as HttpOnly. Refresh/logout send `X-Requested-With: EndosoFlow`. The client retries a request once after a successful refresh. The backend must allow the same-origin proxy setup described in the API contract.

If the Go server listens on a different port, update `VITE_API_PROXY_TARGET`. For a deployment where the API is on a different origin, set `VITE_API_BASE_URL` to the appropriate API base and configure CORS/cookies to match the backend's security contract; same-origin proxying is the expected setup.

## API types and data fetching

- `src/shared/api/schema.d.ts` is generated from `backend/api/openapi.yaml`. Run `npm run api:types` whenever the contract changes and commit the result; `npm run api:types:check` fails if the file is stale. Use it through `Schemas` in `src/shared/api/schema.ts`, so a screen cannot drift from the real contract.
- Server data goes through TanStack Query (`@tanstack/react-query`). Each feature keeps its calls in `api.ts` and its hooks in `queries.ts`. Requests the server refused (4xx) are not retried.
- A screen runs in one of two modes: **live** (talks to the API) or **demo** (`VITE_DEMO_MODE`, sample data in memory). Customers and the vehicle catalog are live; the other screens still use demo data until their backend modules are wired.

## Validation

Environment variable VITE_DEMO_MODE must be set to true before running the tests.

```bash
npm run typecheck        # tsc --noEmit
npm test                 # Vitest: logic, screens against a fake API, app smoke tests
npm run build            # tsc -b && vite build
npm run api:types:check  # the generated types match the OpenAPI contract
```

### Integration test against a real backend

`customers.integration.test.ts` and `catalog.integration.test.ts` (in each feature folder) drive the real API client against a running backend with three employees (Admin, Manager, Advisor); the catalog one uploads real files and fetches them back, so the backend must serve its images. They are skipped unless `INTEGRATION_API` is set:

```bash
INTEGRATION_API=1 VITE_API_BASE_URL=http://localhost:8080/api/v1 INTEGRATION_PASSWORD=... \
INTEGRATION_ADMIN=DNI:<number> INTEGRATION_MANAGER=DNI:<number> INTEGRATION_ADVISOR=DNI:<number> \
npx vitest run src/features/customers/customers.integration.test.ts src/features/catalog/catalog.integration.test.ts
```

## Project structure

```text
src/
  main.tsx                 entry point
  app/                     shell, navigation, wiring
    App.tsx                composes auth, demo workspace, pages and dialogs
    layout/                Sidebar, Topbar, AppShell
    navigation.ts          View type
    useDemoWorkspace.ts    DEMO-ONLY in-memory data (sales, customers, catalog)
  features/                one folder per business area
    auth/                  LoginPage, useAuth
    sales/                 SalesWorkspace, SaleDetail (+ detail/ tabs), NewSaleWizard,
                           ConfirmModal, saleActions.ts (DEMO-ONLY rules), hooks
    customers/             LiveCustomersPage (API), DemoCustomersPage, form modal, dialogs, api/queries
    catalog/               LiveCatalogPage (API), DemoCatalogPage, model detail, create/version/color/image dialogs
    staff/  profile/
  shared/
    api/client.ts          typed API client, token refresh, error envelope
    demo/data.ts           presentation data and the 13 task definitions
    lib/                   format helpers, roles
    ui/                    Icon, Modal, Field, Brand, MetricCard, ApiGap, toast
    config.ts              demoMode flag
    types.ts               domain-facing frontend types
  styles/                  design system split by area; index.css fixes the cascade order
  test/                    Vitest setup
```

Rules of thumb while the project grows:

- A feature never imports from another feature's internals; shared code goes to `shared/`.
- Anything marked **DEMO-ONLY** (`useDemoWorkspace.ts`, `saleActions.ts`, `shared/demo/`) is scaffolding. When a resource's routes appear in `backend/api/openapi.yaml`, move that resource to the real API and delete its demo code. The backend owns every business rule.
- `styles/index.css` imports the files in a fixed order because later files override earlier ones; keep new sections before `responsive.css`.

## Backend integration checklist

- [x] `POST /auth/employee/login`, `/auth/refresh`, `/auth/logout`
- [x] `POST /auth/forgot-password`
- [x] `GET /me` client method; session profile is also returned by login/refresh
- [x] `PUT /me/password`
- [x] `GET /employees` and `POST /employees`
- [ ] Wire sales/customer/catalog/task/document/history data once their routes are added to `api/openapi.yaml`
- [ ] Replace sample task names and milestone labels with `/reference` response values
- [ ] Replace demo image URLs with current catalog image URLs / signed storage URLs
- [ ] Add upload and optimistic-lock handling when the task/document endpoints are available

## Brand palette

The UI uses the EndosoFlow / GrupoPana red-and-black palette: logo red `#F11119`, darker red `#C90E16`, charcoal `#191919`, and neutral gray/white surfaces. The palette tokens are defined at the top of `src/styles.css` (`--brand-red`, `--brand-red-dark`, `--brand-red-soft`, and `--brand-red-muted`).
