# @endosoflow/portal

The customer portal: customers sign in, follow each of their purchases through the six stages, see what staff recorded for them and download the documents meant for them. It is read-only: customers cannot edit their data, message anyone or upload files (the only thing they change is their own password).

```bash
npm run dev:portal         # from the workspace root; http://localhost:5174
npm run build:portal
```

## Structure

```
src/
├─ main.tsx              Providers: design system styles, TanStack Query, router
├─ app/                  The shell. Nothing here knows about a purchase
│  ├─ App.tsx            Toasts, session, the two public pages, and the login gate
│  ├─ routes.tsx         The screens behind the login
│  ├─ layout/PortalShell Top bar: brand, "Mis compras", "Mi cuenta", who is signed in, sign out
│  ├─ ErrorBoundary.tsx  A crash in one screen leaves the shell standing
│  └─ NotFoundPage.tsx
├─ features/
│  ├─ auth/              Login, forgotten password, activation and reset pages, the session
│  ├─ sales/             The list and the detail of a purchase (stages, recorded data, documents, advisor)
│  └─ account/           The customer's data (read-only) and the password change
├─ shared/
│  ├─ api/client.ts      CustomerApiClient: the shared ApiClient core + the customers' endpoints
│  ├─ lib/               format (dates, sizes, WhatsApp link), passwordRules, documentTypes
│  └─ types.ts           Everything derived from the contract (`Schemas[...]`)
└─ styles/               shell.css and one file per feature
```

## Routes

| Route                       | Screen               | Notes                                                                |
| --------------------------- | -------------------- | -------------------------------------------------------------------- |
| `/`                         | My purchases         | The target of "Ir a mi portal" in the emails                         |
| `/sales/:saleId`            | One purchase         | The target of the progress emails. `/compras/:saleId` redirects here |
| `/account`                  | My account           | Read-only data and the password change                               |
| `/activate-account?token=…` | Activate the account | Public. The link in the welcome email (`POST /auth/activate`)        |
| `/reset-password?token=…`   | Set a new password   | Public. The link in the reset email (`POST /auth/reset-password`)    |

The two public routes and the paths are what the backend writes into emails (`auth/links.go`, `notifications/render.go`): change them there and here together.

## Rules of this site

1. **The server decides what a customer sees; the browser only draws it.** The milestone states, the sections and the files arrive already filtered (completed tasks, customer-visible fields). Nothing here re-derives or hides data. A reopened task simply stops arriving, and the screen follows.
2. **The session is the customers' own.** `CustomerApiClient` renews and ends it on `/auth/customer/refresh` and `/auth/customer/logout` (the base client uses the staff routes). The cache is cleared when the session ends, so the next customer on a shared computer never sees the previous one's data.
3. **Progress is always re-read** (`staleTime: 0`): the list and the detail refetch when opened and when the window regains focus.
4. **A purchase that is someone else's looks exactly like one that does not exist** (404 `SALE_NOT_FOUND`), and so does the page.
5. **Calendar dates are never shifted.** A field of kind `date` is `YYYY-MM-DD` and is written with `formatDay`, not `new Date(...)` in the viewer's time zone. Moments (`createdAt`, `completedAt`, `uploadedAt`) are shown in Lima.
6. **Documents open through a short-lived signed address**, asked for on click (`/portal/documents/{id}/download-url`). The tab is opened first and filled in afterwards so browsers do not treat it as a pop-up.
7. **A cancelled purchase is shown as cancelled, without the reason**, and its stage that was "in progress" is shown as _Detenida_ instead of pulsing. Its documents stay available.
8. **Copy speaks to the customer as _usted_**, like the emails and the stage descriptions that come from the database.

## Testing

`npm test -w @endosoflow/portal`. `App.test.tsx` drives the whole portal through a fake of the API (`app/testSupport.tsx`): sign in and out, the list, the detail, downloads, cancelled and finished purchases, the account, and the links in the emails. Tests find things by role and visible text, not class names.

The portal and the staff site are separate origins on purpose: a staff session never exists on the customers' site, and the other way round.
