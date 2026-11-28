# @endosoflow/web

The staff site: sales and their 13-task process, customers, the vehicle catalog and team management. Used by advisors, managers and system administrators.

```bash
npm run dev:staff          # from the workspace root; http://localhost:5173
npm run test:integration -w @endosoflow/staff   # against a running backend (variables in package.json)
```

## Structure

```
src/
├─ app/                  The shell. Nothing here knows about a business rule.
│  ├─ App.tsx            Providers (toasts, session) and the login gate
│  ├─ routes.tsx         Lazy-loaded routes; the only place that wires a page to the session, the address and the toasts
│  ├─ guards/            RequireRole
│  ├─ layout/            AppShell, Sidebar, Topbar (narrow screens only: the menu button and the logo)
│  ├─ ErrorBoundary.tsx  A crash in one screen leaves the shell standing
│  └─ navigation.ts      Menu items and which roles see them
├─ features/             One folder per area of the product
│  ├─ auth/ sales/ customers/ catalog/ staff/ profile/
│  └─ each one: api.ts (HTTP) → queries.ts (TanStack Query) → *Page.tsx (screen) → smaller components
├─ shared/               Only what is specific to this site and used by several features
│  ├─ api/client.ts      EmployeeApiClient: the shared ApiClient core + the employee login and /me calls
│  ├─ api/lookups.ts     The DNI/RUC lookup (Codart through our server)
│  ├─ lookup/            useIdentityLookup + LookupNotice: filling a name from a DNI, shared by forms
│  ├─ lib/limits.ts      How long free text may be (the server's limits), shown as counters
│  ├─ photos/ lib/ types.ts ui/Avatar
└─ styles/               layout.css (the shell) and one file per feature
```

Design tokens, base styles, the shared components, hooks and formatters come from `@endosoflow/ui`; API types, the HTTP core and error helpers from `@endosoflow/api`.

## Rules of this site

1. **A feature never imports another feature.** What two features share moves to `shared/` (or to a package, if the portal needs it too).
2. **Pages receive their inputs as props** (`role`, `toast`) and are wired in `app/routes.tsx`. A test passes props; it does not have to fake a session.
3. **Data flows one way:** `api.ts` knows URLs, `queries.ts` knows cache keys and invalidation, the page knows neither.
4. **Types come from the contract.** Request and response types derive from `Schemas[...]` / `Operations[...]` in `@endosoflow/api`, never rewritten by hand.
5. **The server filters, sorts and pages.** The browser asks; it never re-filters a list.
6. **State that belongs to the address lives in the address:** filters, page, tab, and the list's search (`?q=`).

## Search, free text and lookups

- **Search sits in the list's own toolbar, first, beside the filters** (`SearchField` + `useSearchQuery`), on Ventas, Clientes, Catálogo and Colaboradores. It filters that list, so it lives with the controls that also filter it and next to the result count; a bar at the top of the page reads as a global search and this is not one. It is kept in the address (`?q=`), so a search can be shared and survives a refresh, and `/` focuses it. On a phone it takes its own full-width row. Where the list has tabs, the tabs are a row above (`.toolbar-tabs`) and search + filters follow.
- **Every free-text box shows its counter.** `Field` draws "123 / 2000" under any control that has a `maxLength` and a text value, warns from 90% and says so at the limit. The limits live in `shared/lib/limits.ts` and mirror the server's (notes 2000, cancellation detail 1000, descriptions 1000, reasons 500); a new text box gets a counter by being given `maxLength={LIMITS.…}`.
- **A name can be filled from a DNI** (`shared/lookup`): in the customer form, the new-employee form and task 2's signer. Same rules everywhere: only a DNI, names filled and locked with "Editar manualmente", a changed document clears a looked-up name, and any failure (unknown document, rate limit, spent allowance, outage) only says so and leaves the names to be typed. The source is recorded: `nameSource` for customers and employees, `signerNameSource` for task 2, which the server stores exactly as sent, so the form keeps it honest (`applyChange` in `taskForms.ts`).

## Tables

Use `<table className="table table-cards">`. On a phone each row becomes a card: the first cell is `td.cell-primary`, a cell marked `.cell-aside` goes to its right. Make the row's main text a real `<Link>` (or a `<button className="cell-title">`) so it works with a keyboard; an `onClick` on the `<tr>` is only the bigger mouse target. Mark a row that needs attention with `.is-overdue`.

## Testing

Tests describe behavior through roles and visible text, not class names, so a redesign does not break them. Two class names are part of the contract on purpose: `.modal-backdrop` and `.is-active` on the current menu link.

## Adding a feature

1. `features/<name>/api.ts`: URLs and types (from `Schemas`).
2. `features/<name>/queries.ts`: keys, `useQuery`/`useMutation`, invalidation.
3. `features/<name>/<Name>Page.tsx`: the screen, built from `@endosoflow/ui`.
4. A lazy route and a small route component in `app/routes.tsx`; an entry in `app/navigation.ts` if it needs a menu item.
5. A test next to the page.

## Notes on the team (`features/staff`)

- **Edit sends only what changed** plus the `version` it started from (`employeeForm.ts`, unit-tested). A personal contact is cleared by sending it empty. When an existing person is edited the name's source is not sent: the backend marks an edited name as manual. When a person is created it is sent (`manual`, or `looked_up` after a lookup).
- **A stale version** (`VERSION_CONFLICT`) does not lose the person's work silently: the form says someone else saved first and offers "Cargar datos actuales", which reloads the server's copy and saves against its version.
- **Administrative actions sit at the end of the record**, in the same dashed zone as the customer's, and only those that apply to the person's state: _reenviar activación_ (an active account that has not set its password), _desactivar_ / _reactivar_, and _eliminar_. Deactivate, reactivate and delete each ask for confirmation in a dialog opened over the record (`EmployeeDialogs.tsx`).
- **Deactivating** ends the person's sessions. The record stays open and shows the new state; whatever was typed in the form is kept and is saved against the server's new version.
- **Deleting** is permanent and requires a reason (kept in the audit trail). The server refuses it for anyone who ever signed in or acted: the dialog then says so and points to deactivating.
- **Nobody can lock themselves out:** the signed-in administrator's own record offers neither deactivating nor deleting, and its role field is locked. If they edit their own data the session is refreshed (`onSelfUpdated`) so the sidebar shows the new name.

## Public pages: the links in the emails

Two routes open without a session, and `app/App.tsx` serves them before the login gate (they never wait for, or depend on, a session):

| Route                       | Page                  | Call                        |
| --------------------------- | --------------------- | --------------------------- |
| `/activate-account?token=…` | `ActivateAccountPage` | `POST /auth/activate`       |
| `/reset-password?token=…`   | `ResetPasswordPage`   | `POST /auth/reset-password` |

Both are a `SetPasswordPage` (in `features/auth`) configured with its own words and call, and share `AuthLayout` with the login.

- **The token** is read from the address once, kept in memory and sent with the password. The server says nothing about a token until it is used, so a dead link (expired or already used) shows up as a `400` on submit: the page then says so and shows the way out (a reset link leads to the "forgot your password" step; an activation link tells the person to ask an administrator for a new one, from the team page's "Reenviar activación").
- **After success** the token is removed from the address, and the person is sent to the login. A reset ends every session of the account, so a browser that is signed in is signed out too.
- **Passwords** are chosen with `PasswordField` and the live `RuleList` from `@endosoflow/ui`; the rules (`shared/lib/passwordRules.ts`) are the same ones the profile page uses. The server has the last word and its complaint about the password is shown on the field.
- The sites need the SPA rewrite rule to open these links directly (see `docs/DEPLOY.md`).
