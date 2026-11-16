# EndosoFlow — Employee Portal

React + TypeScript + Vite. It talks to the Go backend in `../backend` (the contract is `../backend/api/openapi.yaml`).

## Connect the current Go API

1. Start the Go backend, e.g. at `http://localhost:8080`.
2. In `.env`, set:

   ```dotenv
   VITE_DEMO_MODE=false
   VITE_API_BASE_URL=/api/v1
   VITE_API_PROXY_TARGET=http://localhost:8080
   ```

3. Restart Vite. The dev proxy forwards `/api` to the Go service, preserving the `/api/v1` path.

In live mode the access token stays in memory; the refresh cookie is handled by the browser as HttpOnly. Refresh/logout send `X-Requested-With: EndosoFlow`. The client retries a request once after a successful refresh. The backend must allow the same-origin proxy setup described in the API contract.

If the Go server listens on a different port, update `VITE_API_PROXY_TARGET`. For a deployment where the API is on a different origin, set `VITE_API_BASE_URL` to the appropriate API base and configure CORS/cookies to match the backend's security contract; same-origin proxying is the expected setup.

## Run it

```bash
# backend (in ../backend): a database with the migrations applied, then
ENV=local DATABASE_URL=... JWT_SECRET=<32+ characters> go run ./cmd/api
ENV=local DATABASE_URL=... go run ./cmd/seed        # optional: realistic data, see below

# frontend
npm install
npm run dev                                          # http://localhost:5173 (proxies /api and /media to :8080)
```

`go run ./cmd/seed` creates five employees (an Admin, a Manager, three Advisors), customers, a catalog with images and
nine sales at every stage, using the real services. It prints the document numbers; the password is `Demo-12345!`.
It refuses to run outside `ENV=local|test`. It replaces the old demo mode: what you see is the real application.

## Scripts

|                                     |                                                                   |
| ----------------------------------- | ----------------------------------------------------------------- |
| `npm run dev` / `build` / `preview` | develop, build and preview                                        |
| `npm run typecheck` / `npm test`    | types and unit tests                                              |
| `npm run api:types`                 | regenerate `src/shared/api/schema.d.ts` from the OpenAPI document |
| `npm run test:integration`          | the real API client against a running, seeded backend             |

## Structure

```
src/app/            routes, the shell (sidebar, top bar)
src/features/       sales (list, detail, task panel, new sale), customers, catalog, staff, profile, auth
src/shared/         API client + generated types, photos (usePhoto), ui (Avatar, Modal, Menu, Field), lib
src/styles/system.css   the design system: tokens, shell and the sales screens
```

Routes: `/sales`, `/sales/new`, `/sales/:id[/tasks|documents|notes|history][?task=N]`, `/customers`, `/catalog`,
`/staff` (Admin), `/profile`. A deployed build needs the host to answer every path with `index.html`.

### Integration test against a real backend

`customers.integration.test.ts` and `catalog.integration.test.ts` (in each feature folder) drive the real API client against a running backend with three employees (Admin, Manager, Advisor); the catalog one uploads real files and fetches them back, so the backend must serve its images. They are skipped unless `INTEGRATION_API` is set:

```bash
INTEGRATION_API=1 VITE_API_BASE_URL=http://localhost:8080/api/v1 INTEGRATION_PASSWORD=... \
INTEGRATION_ADMIN=DNI:<number> INTEGRATION_MANAGER=DNI:<number> INTEGRATION_ADVISOR=DNI:<number> \
npx vitest run src/features/customers/customers.integration.test.ts src/features/catalog/catalog.integration.test.ts
```

## Rules worth knowing

- **What a person may do comes from the server.** The sale and each task carry a `permissions` object; the screens only
  render it. There is no role logic in the frontend to keep in sync.
- **Task forms are checked against the contract.** `taskForms.contract.test.ts` compares every field a form sends with the
  backend's OpenAPI schemas, so a renamed field fails a test instead of failing a user.
- **Pictures are private.** `hasPhoto` says whether to ask; `Avatar` then fetches a 10-minute signed address and reuses it.

## Brand palette

The UI uses the EndosoFlow / GrupoPana red-and-black palette: logo red `#F11119`, darker red `#C90E16`, charcoal `#191919`, and neutral gray/white surfaces. The palette tokens are defined at the top of `src/styles.css` (`--brand-red`, `--brand-red-dark`, `--brand-red-soft`, and `--brand-red-muted`).
