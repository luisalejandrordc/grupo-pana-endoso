# BI dashboard (Analítica)

Status: backend and frontend written; neither has been compiled or run yet. The API contract is in `backend/api/openapi.yaml` (tag _Analytics_); the
code is in `backend/internal/analytics`. Every figure is computed on demand from the operational tables, inside one
read-only `REPEATABLE READ` transaction per request, so the widgets of a tab always agree. There are no materialized
views and no cache; add them only if measurements ask for it.

## Endpoints and who may call them

| Endpoint                    | Roles                   | Notes                                                                      |
| --------------------------- | ----------------------- | -------------------------------------------------------------------------- |
| `GET /analytics/overview`   | Admin, Manager, Advisor | Snapshot, period KPIs, money, pipeline by milestone, trend, attention list |
| `GET /analytics/process`    | Admin, Manager, Advisor | Time, waiting, deadlines, load and rework per task                         |
| `GET /analytics/commercial` | Admin, Manager, Advisor | Vehicles, customer types, temperature, cancellations, money                |
| `GET /analytics/team`       | Admin, Manager          | One row per advisor                                                        |
| `GET /analytics/system`     | Admin                   | Email delivery, sign-ins, accounts, storage upkeep                         |

An Advisor is pinned to their own sales whatever they send in `advisorId`. Commission, Toyota payments, amount
mismatches and the price breakdown are `null` for an Advisor: the API never sends them.

## Definitions

- **Period**: `from`..`to` in Lima dates, inclusive; default the last 90 days; at most 731 days. The previous period is
  the equal-length range that ends the day before `from`. Buckets: day up to 31 days, week up to 180, month beyond.
- **Snapshot figures** (active, overdue, approaching, idle, pipeline value, pending collection, current load) describe
  now and ignore the period. The other filters (advisor, customer type, model, temperature) still apply.
- **Overdue**: the in-progress task is past its due date. **Approaching**: past 80% of its recommended duration.
  **Idle**: no task is in progress and the next one has been eligible for more than 24 hours. A sale has one current
  task, so the three never overlap.
- **Created / completed / cancelled**: `created_at`, `completed_at`, `cancelled_at` inside the period.
- **Close rate**: completed / (completed + cancelled) among the sales resolved in the period. Sales still in progress
  are not in the denominator.
- **Cycle time**: creation to completion, for sales completed in the period; median and P90.
- **Value of a sale**: the quoted final price (task 1) and, until it exists, the catalog reference price taken at
  creation. `estimatedCount` says how many sales used the fallback.
- **Collected / paid to Toyota**: deposited amounts of tasks 4 / 5 by payment date, on sales that are not cancelled.
- **Invoiced**: invoice amounts of task 6 by invoice date, on sales that are not cancelled.
- **Commission**: the final commission (task 3) of the sales completed in the period; the rate is that commission over
  the same sales' quoted price; drift is final minus initial commission.
- **Work time** of a task: `completed_at - started_at`. **Waiting time**: from becoming eligible (previous task's
  completion, or the sale's creation for task 1) to its start; negative waits caused by reopened earlier tasks are ignored.
- **Deadline compliance** covers only the tasks with a recommended duration (2, 3, 7, 8, 9, 10): early or approaching
  counts as on time.
- **Rework**: audit events `RESTARTED`, `REOPENED` and `CORRECTED` of task instances inside the period.
- **Commercial cohorts** group the sales _created_ in the period. Temperature is judged only on resolved sales.
- Sales are attributed to the advisor currently assigned, even if they were reassigned.
- All money is what the sales recorded, in USD. It is operational data, not accounting.

## Demo data

`SEED_HISTORY=1 ENV=local DATABASE_URL=... go run ./cmd/seed` (on a fresh database) also creates about fifty sales
spread over the last eleven months, with varied prices, outcomes and delays, so the trends have something to show.

## Frontend

`frontend/apps/web/src/features/analytics`, route `/analytics`, menu item _Analítica_ (all roles).

- Tabs by role: Resumen (an Advisor sees "Mi panel"), Proceso, Comercial, Equipo (Admin, Manager), Sistema (Admin).
- Each tab asks for its own endpoint and only while it is open; the page refreshes every minute and keeps the last
  figures on screen while a new filter loads.
- Filters live in the address (`period`, `from`, `to`, `advisor`, `customerType`, `model`, `temperature`, `tab`), so a
  view can be bookmarked or shared. Presets: last 30 days, last 90 days (the server default, no dates sent), this year,
  last year, custom.
- Only the two time series use Recharts (lazy-loaded with the route). Every other bar is plain HTML, so each figure is
  also text; each chart has a text summary and a "Ver los datos como tabla" toggle.
- `packages/api/src/schema.d.ts` was extended by hand for the analytics schemas; run `npm run api:types` to regenerate it
  from the contract.
- Needs `npm install` (adds `recharts`).
