# @endosoflow/portal

The customer portal: customers sign in and follow their sales and the progress of each one.

**Status: placeholder.** Every address shows a "Próximamente" page (`src/features/coming-soon`). The project around it is complete, so building the real portal is a matter of adding features, not setting things up.

```bash
npm run dev:portal         # from the workspace root; http://localhost:5174
npm run build:portal
```

## Structure (grows like the staff site)

```
src/
├─ main.tsx              Mounts the app: design system styles, then this site's
├─ app/App.tsx           Routes. Today: a catch-all to the holding page
├─ features/             One folder per area, as in apps/staff
│  └─ coming-soon/       The placeholder (delete it when the portal launches)
└─ styles/index.css      This site's own CSS; tokens and components come from @endosoflow/ui
```

## When you build it

What the staff site did that the portal can copy, in this order:

1. **Add `@endosoflow/api`** as a dependency and create `src/shared/api/client.ts` with `class CustomerApiClient extends ApiClient` and a `login()` for the customers' endpoint. The contract today only has `/auth/employee/login`; the backend needs to expose the customer one (customers have a `portalStatus`: `not_invited` / `invited` / `active`).
2. **Providers in `main.tsx`:** `QueryClientProvider` (use `createQueryClient()` from the API package) and, in `App.tsx`, `ToastProvider` and an `AuthProvider` modeled on `apps/staff/src/features/auth`.
3. **A shell** (header with the logo and the customer's name; a customer has few areas, so a top bar is likely enough) and `RequireAuth` for the routes behind the login.
4. **Features:** `my-sales/` (list), `sale-detail/` (the 13 tasks as a read-only timeline; the contract already marks which documents are `customerVisible`).
5. If the portal and the staff site start duplicating a component, move it to `@endosoflow/ui`.

The portal and the staff site are separate origins on purpose: a staff session never exists on the customers' site, and the other way round.
