# 1I hardening: role regression test and demo data

Status: **written, not yet compiled or run by the author.** Run `make fmt vet test` first.

## Role regression test (`backend/cmd/api/roles_test.go`)

It runs the real router, the real authentication and the real services against the test database, with one
employee per role, and sends every route of the contract with every role.

| Check                                           | What it proves                                                                                                                                                                       |
| ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `TestEveryRouteInTheContractHasARoleRule`       | Needs no database. A new route in `openapi.yaml` fails the build until it is added to the matrix (or declared public), so no route ships unreviewed.                                 |
| `TestRoleMatrixOnTheRealStack`                  | For every route: no token or a garbage token is 401; a role that may call it is never 401 or 403 and never causes a 5xx on harmless input; a role that may not gets 403 `FORBIDDEN`. |
| `TestTokensAreCheckedAgainstTheDatabase`        | A token cannot claim a role the account does not have; a promotion or a deactivation takes effect on the next request; an unknown account is 401.                                    |
| `TestTamperedExpiredAndForeignTokensAreRefused` | A changed signature or payload, the `none` algorithm, an expired token, a token signed with another key and a non-Bearer scheme are all 401.                                         |

The matrix in the test is written from section 7 of the Architecture & API document, not read from the handlers,
so a handler that drifts from the documented rule fails. Rules that depend on the sale or the stage (an
advisor's scope, who can cancel at which task, who can reopen after Task 6) are covered by the flow tests.

Like the flow tests it needs `TEST_DATABASE_URL`; without it the database tests skip and only the contract
check runs.

Two routes in the architecture table are not implemented yet and are not in the contract: `GET /search` and
`GET /reference`, and the employee photo routes (`PUT /me/photo`, `GET /employees/{id}/photo`). When they are built they
need a rule in the matrix.

## Demo data (`backend/cmd/seed-demo`)

```
make bootstrap-admin ARGS="..."      # once, if the database has no System Admin
make seed-demo ARGS="-yes"
```

It loads, through the same services the application uses (so the rules, the audit trail and the files are real):

- a manager and three advisors, activated, with one shared password (`Demo-Password-2026!`, or `-password`);
- three models with versions, colors and a drawn picture for each color;
- twelve customers of every type (embassies, religious entities, international organizations and diplomats with
  a DNI, CE, passport or CDI);
- thirteen sales at every stage: just registered; Task 1 open with partial data; a task that is overdue; a quotation
  corrected by the manager; a task restarted once; sales waiting at Tasks 4, 7, 10 and 13; two completed (with a
  mix of on-time and late completions); and two cancelled (one by an advisor, one by a manager).

The clock is moved through each sale's story, so start, due and completion times are in the past and the phases
(early, within the limit, late) have something to show. Documents are valid PDFs and Excel workbooks with the
customer and sale in them.

It refuses to run when `ENV=production`, requires `-yes`, needs an active System Admin, and refuses a second
run (the demo accounts already exist). Random values come from `-seed`, so two databases get the same story.
To start again, recreate the database (`make db-reset`) and re-run.

| Account                         | Role    |
| ------------------------------- | ------- |
| `gerencia@demo.endosoflow.test` | Manager |
| `marco@demo.endosoflow.test`    | Advisor |
| `daniela@demo.endosoflow.test`  | Advisor |
| `luis@demo.endosoflow.test`     | Advisor |

## Stakeholder demo script (about 15 minutes)

1. **Advisor view** (Marco): the workspace with only his sales, the summary cards, a search by customer and a
   filter by current task. Open the overdue sale of the Emirati diplomat (Task 2, open for six days).
2. **A task from start to finish** on a sale that is ready: start Task 4, enter a deposit that differs from the
   expected amount (the form asks for the reason), attach the voucher, complete, and watch the milestone move.
3. **Documents**: upload a wrong file type (it is refused by content), replace a file, download it through the
   signed link.
4. **Manager view** (Carolina): all sales, filter by advisor, open a completed sale and read its history. Correct a
   completed task (it asks for a reason), try to correct a locked amount and read why it is locked, reopen the
   last task, restart a task.
5. **Cancel and reopen**: cancel a sale as an advisor (allowed through Task 3), try as an advisor at Task 4
   (refused), reassign a sale to another advisor and show that the first advisor no longer sees it.
6. **Admin view**: permanent deletion of a never-delivered sale with the typed confirmation, and the audit that
   survives it.

## Open items

- `seed-demo` does not delete anything. It is meant for a fresh database.
- The seed writes catalog pictures and documents to the configured stores: with R2 that means the real buckets.
