# @endosoflow/staff

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
│  ├─ layout/            AppShell, Sidebar, Topbar
│  ├─ ErrorBoundary.tsx  A crash in one screen leaves the shell standing
│  └─ navigation.ts      Menu items, which roles see them, where the search box appears
├─ features/             One folder per area of the product
│  ├─ auth/ sales/ customers/ catalog/ staff/ profile/
│  └─ each one: api.ts (HTTP) → queries.ts (TanStack Query) → *Page.tsx (screen) → smaller components
├─ shared/               Only what is specific to this site and used by several features
│  ├─ api/client.ts      EmployeeApiClient: the shared ApiClient core + the employee login and /me calls
│  ├─ photos/ lib/ types.ts ui/Avatar
└─ styles/               layout.css (the shell) and one file per feature
```

Design tokens, base styles, the shared components, hooks and formatters come from `@endosoflow/ui`; API types, the HTTP core and error helpers from `@endosoflow/api`.

## Rules of this site

1. **A feature never imports another feature.** What two features share moves to `shared/` (or to a package, if the portal needs it too).
2. **Pages receive their inputs as props** (`role`, `query`, `toast`) and are wired in `app/routes.tsx`. A test passes props; it does not have to fake a session.
3. **Data flows one way:** `api.ts` knows URLs, `queries.ts` knows cache keys and invalidation, the page knows neither.
4. **Types come from the contract.** Request and response types derive from `Schemas[...]` / `Operations[...]` in `@endosoflow/api`, never rewritten by hand.
5. **The server filters, sorts and pages.** The browser asks; it never re-filters a list.
6. **State that belongs to the address lives in the address:** filters, page, tab, and the top search (`?q=`).

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

- **Edit sends only what changed** plus the `version` it started from (`employeeForm.ts`, unit-tested). A personal contact is cleared by sending it empty. The name's source is never sent: the backend marks an edited name as manual.
- **A stale version** (`VERSION_CONFLICT`) does not lose the person's work silently: the form says someone else saved first and offers "Cargar datos actuales", which reloads the server's copy and saves against its version.
- **What the menu offers depends on the person's state:** edit always; resend activation only for an active account that has not set a password; deactivate only for an active person other than yourself; reactivate only for an inactive one.
- **Deactivating asks first** (it ends the person's sessions); reactivating and resending do not.
- An administrator who edits their own record refreshes the session (`onSelfUpdated`), so the sidebar shows the new name; and cannot change their own role from this screen.
- The contract also has `DELETE /employees/{id}` for an employee who never signed in or acted. It is deliberately not in the UI: deactivating is the normal way to remove access.

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
