# BI dashboard

The dashboard is a read-only view over what the sales, tasks, payments and invoices modules already record.
Backend: `backend/internal/dashboard` (one endpoint per tab, `GET /api/v1/dashboard/{summary|process|market|finance|advisors|system}`).
Frontend: `frontend/apps/web/src/features/dashboard`, the **Panel** item of the menu.

## Rules every number follows

- **Flow vs snapshot.** Flow numbers follow the chosen period and use the date of the event itself (sale created,
  completed or cancelled; task completed; payment or invoice dated). Snapshot numbers describe today and ignore the
  period; the screens mark them **Hoy**. Both respect the sale filters.
- **Period.** Calendar dates in Lima time, both ends included. Default: the last 90 days. Maximum: three years.
- **By current state.** A sale cancelled and then reopened is not counted as cancelled.
- **Money is recorded, not accounting.** A value counts only once the task that confirms it is completed: task 1
  (quotation), task 3 (final commission), task 4 (money received), task 5 (money paid to Toyota), task 6 (invoices).
  Amounts travel as two-decimal strings. None of them is recognized revenue.
- **Roles.** An Advisor is always limited to their own sales on the server (the `advisorId` filter cannot widen it) and
  sees their own money. `advisors` is for Managers and Admins; `system` is for Admins only.

## Definitions

| Number                   | Definition                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------- |
| Closing rate             | completed ÷ (completed + cancelled), both by event date in the period                                                                       |
| Cycle time               | `completed_at − created_at` of the sales completed in the period (median and 90th percentile)                                               |
| Task work time           | `completed_at − started_at` of the tasks completed in the period (median)                                                                   |
| Task wait time           | `started_at − eligible_at`: time a task could start but nobody started it (median)                                                          |
| On-time rate             | completions in phase `early` or `in_approaching` ÷ completions in any SLA phase. Only tasks 2, 3 and 7–10 have an SLA                       |
| Overdue / approaching    | task in progress past its due date / past 80% of its time and not yet late                                                                  |
| Stalled                  | next task eligible for more than 48 hours and still pending                                                                                 |
| Reopened tasks           | `REOPENED` events on tasks in the period                                                                                                    |
| Temperature closing rate | among sales created in the period that are already resolved: completed ÷ (completed + cancelled). It lags: it changes while the rest finish |
| Commission (pipeline)    | final when task 3 is completed, provisional (initial) before                                                                                |
| Held in active sales     | received from customers minus paid to Toyota, on active sales                                                                               |

## Changing it

- A new number is a query in `repository.go`, a field in `types.go` and in `api/openapi.yaml`; run `npm run api:types`.
- Every query takes its sale conditions from `Filter.where`, so the filters and the Advisor scope mean the same everywhere.
- The connection pool is small (5), so a request runs at most 2 queries at a time (`maxParallel`).
