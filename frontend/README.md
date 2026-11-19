# EndosoFlow — frontend

Two sites and the packages they share, in one npm-workspaces repository.

```
frontend/
├─ apps/
│  ├─ staff/     Staff site: sales, customers, catalog, team.            → deployed on its own
│  └─ portal/    Customer portal: customers follow their sales.          → deployed on its own (placeholder for now)
├─ packages/
│  ├─ ui/        @endosoflow/ui   Design tokens, CSS, React components, hooks, formatters
│  └─ api/       @endosoflow/api  Types generated from the OpenAPI contract, HTTP client core, error helpers
├─ docs/         DEPLOY.md        How to deploy both sites on Render
├─ package.json  Workspace root: shared tooling and the scripts below
└─ tsconfig.base.json · eslint.config.js · vitest.setup.ts · .prettierignore
```

React 18 · TypeScript · Vite · TanStack Query · React Router · Vitest + Testing Library.

## Getting started

```bash
npm install            # once, at this folder: installs every app and package
npm run dev:staff      # http://localhost:5173
npm run dev:portal     # http://localhost:5174   (run each in its own terminal to have both)
```

Each app has a `.env.example`: copy it to `.env.local` in that app's folder if you need to point at another backend.

| Script (run at the root)                | What it does                                                                   |
| --------------------------------------- | ------------------------------------------------------------------------------ |
| `npm run check`                         | Typecheck + lint + format check + tests, for everything. Run before committing |
| `npm run build`                         | Builds both sites (`build:staff`, `build:portal` for one)                      |
| `npm test`                              | Tests of every workspace (`-w @endosoflow/ui` to run one)                      |
| `npm run typecheck` / `lint` / `format` | What they say; `lint:fix` fixes what it can                                    |
| `npm run api:types`                     | Regenerates the API types from `../backend/api/openapi.yaml`                   |

Run anything in one workspace with `-w`, for example `npm run test:watch -w @endosoflow/staff`.

## The rules of the workspace

```
 apps/staff ─┐
             ├──▶  @endosoflow/ui      (nothing)
 apps/portal ┘──▶  @endosoflow/api     (nothing)
```

1. **An app may use packages. A package never uses an app. Apps never use each other.** ESLint enforces it (`no-restricted-imports`), so a violation fails `npm run check`.
2. **The design system depends on nothing and the API package depends on nothing** of ours. They are leaves.
3. **Shared means used by both sites, or about to be.** A component only the staff site needs stays in `apps/staff` until the portal needs it; then it moves to `@endosoflow/ui`. Do not move code "just in case".
4. **What differs per site stays in the site:** its login and session, its routes, its screens, its shell (menu, topbar). The staff site signs in with a document and password against `/auth/employee/login`; the portal will have its own login. Both extend the same `ApiClient` core.
5. **Packages are consumed as source** (their `exports` point at `src/`), so there is no build step for them and a change shows up in both sites immediately. Import only from the package name (`@endosoflow/ui`), never from a deep path.

## Adding things

- **A shared component**: put it in `packages/ui/src/components/`, export it from `packages/ui/src/index.ts`, add its test next to it, and its CSS in `packages/ui/src/styles/components/`.
- **A feature in a site**: see `apps/staff/README.md` ("Adding a feature").
- **A new package** (say `@endosoflow/i18n`): create `packages/<name>/` with a `package.json` (`"exports": {".": "./src/index.ts"}`), a `tsconfig.json` that extends `../../tsconfig.base.json`, add it as a dependency (`"@endosoflow/<name>": "*"`) in the apps that use it, run `npm install`, and add a boundary rule in `eslint.config.js`.
- **A third site**: copy `apps/portal/` (it is the smallest complete app), rename it in `package.json`, give it its own port in `vite.config.ts`, and add `dev:*` / `build:*` scripts at the root.

## Deploying

Both sites deploy as separate static sites. Read **[docs/DEPLOY.md](docs/DEPLOY.md)** first: it explains the one decision that affects login (how the browser reaches the API on Render) and gives the exact settings.oth sites
