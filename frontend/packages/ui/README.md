# @endosoflow/ui

The EndosoFlow design system: one look for every site. It depends on nothing of ours (React, React Router and two fonts only).

```tsx
// main.tsx of an app: the design system first, then the app's own CSS
import "@endosoflow/ui/styles.css";
import "./styles/index.css";

import {
  Badge,
  EmptyState,
  PageHeader,
  Pagination,
  useToast,
} from "@endosoflow/ui";
```

## What is inside

```
src/
├─ index.ts              The public surface. Import only from "@endosoflow/ui"
├─ components/           React components (each with its test)
├─ hooks/                useDebounced, useDocumentTitle, useMediaQuery, useSearchQuery
├─ lib/format.ts         Dates, money and initials in es-PE / America/Lima
├─ brand.ts + assets/    logoLandscape, logoSquare (fingerprinted URLs)
└─ styles/               tokens.css → base.css → components/*.css, in that order (index.css)
```

| Component                                 | Use it for                                                                                                     |
| ----------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `PageHeader`, `BackLink`                  | The title block every page opens with                                                                          |
| `Badge` (`tone`, `plain`)                 | A status or a label                                                                                            |
| `Tabs` (`underline`/`segmented`)          | Sections of a page, or the filters of a list. Arrow keys move focus                                            |
| `Pagination`                              | The footer of any paged list                                                                                   |
| `EmptyState`, `ErrorState`                | Nothing to show / a failed request (with "Reintentar")                                                         |
| `Callout`                                 | A message inside a page: error, warning, confirmation                                                          |
| `SearchField`                             | A search box; `shortcut` makes `/` focus it                                                                    |
| `Field`                                   | A labelled control with hint or error, wired for screen readers                                                |
| `Modal`, `ConfirmDialog`                  | Dialogs (focus trap, Escape); "are you sure?" before something hard to undo (`tone="danger"`, `busy`, `error`) |
| `Menu` (`variant="icon"` for a table row) | Action menus: arrow keys, drawn on the page so a table never clips it, opens upward near the screen's bottom   |
| `ToastProvider` + `useToast()`            | Notifications. `const { notify } = useToast()`; they stack and close                                           |
| `InitialsTile`, `Icon`                    | An organization's initials; the icon set                                                                       |

Buttons are a CSS contract: `.button` + `.button-primary | -secondary | -ghost | -danger` + optional `.button-sm | .button-wide`.

## Styles

- **Never write a color or a pixel size that a token already names.** To change the look of every site, change `tokens.css`.
- **Color has a meaning.** Brand red = the main action and "needs attention". Green = done / active. Blue = in progress. Amber = caution. Grey = neutral / inactive. A status is never shown by color alone: it always carries a word.
- Headings inherit their color, so a heading on a dark panel stays light.
- Site-specific CSS (a shell, a login page) lives in the site, not here. A class belongs here when two sites would write the same rule.

## Peer dependencies

`react`, `react-dom` and `react-router-dom` come from the app (one copy per site). `PageHeader`'s `BackLink` and `useSearchQuery` use the router, so an app using them needs a `<BrowserRouter>`.
