# EndosoFlow — Frontend

React 18 + TypeScript + Vite. Server state with TanStack Query, routing with React Router, tests with Vitest and Testing Library.

```bash
npm install
npm run dev          # http://localhost:5173, proxies /api to the backend (see vite.config.ts)
npm run check        # typecheck + lint + format check + tests: run it before every commit
npm run build
```

| Script                      | What it does                                               |
| --------------------------- | ---------------------------------------------------------- |
| `npm run typecheck`         | `tsc --noEmit`, with `strict` and no unused code           |
| `npm run lint` / `lint:fix` | ESLint (TypeScript, React Hooks, React Refresh)            |
| `npm run format` / `:check` | Prettier                                                   |
| `npm test` / `test:watch`   | Vitest                                                     |
| `npm run api:types`         | Regenerates `shared/api/schema.d.ts` from the OpenAPI file |
| `npm run api:types:check`   | Fails if the generated types are out of date               |

## Structure

```
src/
├─ app/                  The shell. Nothing here knows about a business rule.
│  ├─ App.tsx            Providers (toasts, session) and the login gate
│  ├─ routes.tsx         Lazy-loaded routes; the only place that wires a page to the session, the address and the toasts
│  ├─ guards/            RequireRole
│  ├─ layout/            AppShell, Sidebar, Topbar
│  ├─ ErrorBoundary.tsx  A crash in one screen leaves the shell standing
│  └─ navigation.ts      Menu items, which roles see them, where the search box appears
├─ features/             One folder per area of the product
│  ├─ auth/ sales/ customers/ catalog/ staff/ profile/
│  └─ each one: api.ts (HTTP) → queries.ts (TanStack Query) → *Page.tsx (screen) → smaller components
├─ shared/
│  ├─ api/               client.ts (fetch, refresh, errors), generated schema.d.ts, errors.ts
│  ├─ ui/                The design system's components (see below)
│  ├─ hooks/             useSearchQuery, useMediaQuery, useDocumentTitle
│  ├─ lib/               Pure helpers (format, roles, debounce)
│  └─ types.ts
└─ styles/               See "Styles"
```

### Rules of the structure

1. **A feature never imports another feature.** What two features share moves to `shared/`.
2. **Pages receive their inputs as props** (`role`, `query`, `toast`) and are wired in `app/routes.tsx`. This keeps them easy to test: a test passes props, it does not have to fake a session.
3. **Data flows one way:** `api.ts` knows URLs, `queries.ts` knows cache keys and invalidation, the page knows neither.
4. **Types come from the contract.** Request and response types are derived from the generated `schema.d.ts` (`Schemas[...]`, `Operations[...]`), never rewritten by hand, so a change in the API shows up as a type error here.
5. **The server filters, sorts and pages.** The browser asks; it never re-filters a list.
6. **State that belongs to the address lives in the address:** filters, page, tab, and the top search (`?q=`). A link can be shared and the back button works.

## Styles

Plain CSS, in layers, loaded in this order by `styles/index.css`:

```
tokens.css      every color, size, shadow, radius, z-index and duration, as a CSS variable
base.css        reset, typography, focus ring, helpers (.sr-only, .muted, .mono)
layout.css      shell, sidebar, topbar, page, .stack, .page-head
components/     button, form, badge, avatar, table, tabs, feedback (skeleton, empty, callout, toast), overlay (modal, menu)
features/       the rules that only one area needs (login, sales, customers, catalog, profile)
```

- **Never write a color or a pixel size that a token already names.** To change the look, change `tokens.css`.
- **Color has a meaning.** Brand red = the main action and "needs attention" (an overdue task). Green = done / active. Blue = in progress. Amber = caution. Grey = neutral / inactive. A status is never shown by color alone: it always carries a word.
- Headings inherit their color, so a heading on a dark panel stays light.
- A component gets its class from its own layer; a feature file may place components but should not restyle them.

## The shared components (`shared/ui`)

| Component                        | Use it for                                                           |
| -------------------------------- | -------------------------------------------------------------------- |
| `PageHeader`, `BackLink`         | The title block every page opens with                                |
| `Badge` (`tone`, `plain`)        | A status or a label                                                  |
| `Tabs` (`underline`/`segmented`) | Sections of a page, or the filters of a list. Arrow keys move focus  |
| `Pagination`                     | The footer of any paged list                                         |
| `EmptyState`, `ErrorState`       | Nothing to show / a failed request (with "Reintentar")               |
| `Callout`                        | A message inside a page: error, warning, confirmation                |
| `SearchField`                    | A search box; `shortcut` makes `/` focus it                          |
| `Field`                          | A labelled control with hint or error, wired for screen readers      |
| `Modal`, `Menu`                  | Dialogs (focus trap, Escape) and action menus (arrow keys)           |
| `ToastProvider` + `useToast()`   | Notifications. `const { notify } = useToast()`; they stack and close |
| `Avatar`, `InitialsTile`         | A person / an organization                                           |

Buttons are a CSS contract: `.button` + `.button-primary | -secondary | -ghost | -danger` + optional `.button-sm | .button-wide`.

## Tables

Use `<table className="table table-cards">`. On a phone each row becomes a card: the first cell is `td.cell-primary`, a cell marked `.cell-aside` goes to its right. Make the row's main text a real `<Link>` (or a `<button className="cell-title">`) so it works with a keyboard; an `onClick` on the `<tr>` is only the bigger mouse target. Mark a row that needs attention with `.is-overdue`.

## Testing

Tests describe behavior through roles and visible text, not class names, so a redesign does not break them. Two class names are part of the contract on purpose: `.modal-backdrop` and `.is-active` on the current menu link.

Integration tests against a running backend are skipped by default; run them with `npm run test:integration` (see the variables in `package.json`).

## Adding a feature

1. `features/<name>/api.ts` — URLs and types (from `Schemas`).
2. `features/<name>/queries.ts` — keys, `useQuery`/`useMutation`, invalidation.
3. `features/<name>/<Name>Page.tsx` — the screen, built from `shared/ui`.
4. A lazy route and a small route component in `app/routes.tsx`; an entry in `app/navigation.ts` if it needs a menu item.
   5
