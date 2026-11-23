# EndosoFlow — State Machines & Permissions Spec

## 1. How to read this document

This spec turns the confirmed business rules into precise state machines, SLA rules and a role × action permission matrix. It is the input to the Phase 1 requirements catalog and to data modeling (1B). No schema or code is implied.

**Two definitions used throughout**

- **Current task**: the single `in_progress` task of a sale; if none is `in_progress`, the next eligible task (the lowest-numbered task that is not completed).
- **Eligible task**: a `pending` task whose previous task is `completed` (Task 1 is eligible from sale creation). When a task became eligible is derived, not stored: it is the previous task’s completion time (Task 1: the sale’s creation time).

## 2. Task state machine

### 2.1 Valid shapes of a sale's tasks

Because progression is strictly linear, the 13 tasks of an `active` sale are always in exactly one of these shapes, where k is the number of completed tasks (0 to 13):

- Tasks 1..k are `completed`, task k+1 is `pending` and eligible (waiting to be started), tasks after that are `pending`.
- Tasks 1..k are `completed`, task k+1 is `in_progress`, tasks after that are `pending`.

No other combination is valid. This gives the backend and the database one simple invariant to enforce: **completed tasks form a prefix, at most one task is `in_progress`, and it is always task k+1.** Every transition below must preserve it, inside a single database transaction.

### 2.2 Transitions

| #   | Transition                                | Who                              | Preconditions                                                                                                                                                                                   | Effects                                                                                                                                                                                                                                                                                                         | Reason required       |
| --- | ----------------------------------------- | -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| T1  | `pending` → `in_progress` (start)         | Assigned Advisor, Manager, Admin | Sale is `active`; task is eligible; no task is `in_progress`; confirmation shown                                                                                                                | `started_at` = now; `due_date` = `started_at` + duration (only tasks with a duration); fields and uploads enabled; reminder state reset when this is a fresh start after a restart                                                                                                                              | No                    |
| T2  | `in_progress` → `completed` (complete)    | Assigned Advisor, Manager, Admin | All mandatory fields and documents valid; task-specific rules pass; confirmation shown                                                                                                          | `completed_at` = now; `completion_phase` computed from timestamps; the next task becomes eligible (its eligibility time is this completion time, derived); SLA reminders stop; milestone notification queued if the task triggers one; if this is Task 13, the sale becomes `completed` in the same transaction | No                    |
| T3  | `completed` → `in_progress` (reopen task) | Manager, Admin                   | Task is the highest-numbered completed task **and** no task is `in_progress`; if the sale is `completed`, only Task 13 can be reopened and the sale returns to `active` in the same transaction | Original `due_date` preserved; `completed_at` and `completion_phase` cleared (history stays in the audit log); reminder counters kept (cap not re-armed); fields editable again; the task's customer-visible data is hidden until it is completed again; the following task is no longer eligible               | Yes + audit           |
| T4  | `in_progress` → `pending` (restart)       | Manager, Admin                   | Task is the current `in_progress` task                                                                                                                                                          | Entered data and uploaded files are **kept, read-only**; `started_at` and `due_date` cleared; SLA timer discarded; reminder counters and phase reset; the task stays eligible; the next start (T1) calculates a fresh `due_date`                                                                                | Yes + audit           |
| T5  | Edit a completed task (no status change)  | Manager, Admin                   | Task is `completed`; financial source fields are editable only while **no downstream task is completed**                                                                                        | Fields or documents changed; append-only audit event with before/after summary                                                                                                                                                                                                                                  | Yes (Reason for Edit) |

Advisors can edit only the current `in_progress` task of their own assigned sales. Completed tasks are read-only for them.

### 2.3 Completion rules per task (summary)

- Every field and document is mandatory unless marked optional in the master prompt (Task 13's Surquillo transfer date; Task 2's _Vigencia de Poder_ is required only when the snapshotted `customer_type` is not _Diplomático_; the Task 4 Mismatch Reason is required only when amounts differ).
- Task 1: quoted final price = initial Toyota amount + initial commission + accessory amount + release processing amount. All four amounts are strictly positive.
- Task 3: final commission = Task 1 quoted price − final Toyota amount − Task 1 accessory amount − Task 1 release processing amount. It must be strictly positive; the final Toyota amount must also be strictly positive. Series and VIN must be unique among `active` and `completed` sales.
- Task 4: deposited amount must equal Task 1's quoted price exactly, otherwise a Mismatch Reason is required (exact match, single deposit, USD only). Bank + transaction number unique within the Task 4 space.
- Task 5: deposited amount must equal Task 3's final Toyota amount, otherwise a Mismatch Reason is required (same mechanism). Bank + transaction number unique within the Task 5 space (separate from Task 4).
- Task 6: four invoices, each with number (`SERIES-CORRELATIVE`, Proposal pattern: 4 alphanumeric characters, a hyphen, 1 to 8 digits), date, amount and PDF. Each amount must match its expected value exactly or carry a Mismatch Reason. Invoice numbers have two uniqueness namespaces that also cover cancelled sales: Toyota invoices among themselves, and commission, accessories and release-processing invoices across those three kinds.
- Task 8: DAM number unique; customs office from the controlled list (Marítima del Callao, Chancay); control channel Verde / Naranja / Rojo.
- Task 10: license plate unique among `active` and `completed` sales.
- Dates: impossible values are hard-blocked; out-of-order dates produce a warning only.
- No tax (IGV) logic anywhere; amounts are compared exactly.

### 2.4 Financial source fields

These fields feed calculations or checks in later tasks. Once any later task is completed they are locked, and the Manager or Admin must roll back in reverse order (T3 repeatedly) before correcting them:

- Task 1: the four amounts and the quoted final price.
- Task 3: the final Toyota amount and the final commission.

All other fields of a completed task (including documents) can be edited with a Reason for Edit. Task 4 and Task 5 deposited amounts are also locked once Task 6 is completed.

## 3. Sale state machine

States: `active`, `completed`, `cancelled`. Sale-level data is immutable after creation except notes and the assigned advisor.

| #   | Transition             | Who                                                                  | Preconditions                                                                                                                             | Effects                                                                                                                                                                                                                                  | Reason               |
| --- | ---------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------- |
| S1  | create → `active`      | Advisor (auto-assigned to self), Manager, Admin (choose the advisor) | Active customer; active model / version / color, and the color belongs to the version's model                                             | Customer and vehicle snapshots, `customer_type` snapshot, reference-price snapshot; sale temperature set by the creator (immutable); 13 task instances created `pending`; Task 1 is eligible from creation; Notification 2 queued; audit | No                   |
| S2  | `active` → `completed` | Automatic                                                            | Task 13 completed                                                                                                                         | Audit event marked as automatic, actor = whoever completed Task 13; Notification 9 queued                                                                                                                                                | No                   |
| S3  | `completed` → `active` | Manager, Admin                                                       | Reopening Task 13 (T3)                                                                                                                    | Done in the same transaction as T3; mandatory audit                                                                                                                                                                                      | Yes                  |
| S4  | `active` → `cancelled` | See cancellation rule below                                          | Sale is `active` (completed sales cannot be cancelled; revert Task 13 first)                                                              | Tasks frozen and read-only; reminders stop; `cancelled_at`, `cancelled_by`, reason category and text stored as first-class sale fields; series, VIN and plate become reusable; transaction and invoice numbers stay reserved; audit      | Yes: category + text |
| S5  | `cancelled` → `active` | See reopen rule below                                                | Identifier conflict check (series, VIN, plate) against `active` and `completed` sales; the partial unique indexes are the real guard      | SLA resumes with remaining time (section 5.4); audit                                                                                                                                                                                     | Yes                  |
| S6  | Reassign advisor       | Manager, Admin                                                       | Sale is active; target is an active Sales Advisor (only active sales can be reassigned; completed and cancelled sales are not reassigned) | Previous advisor loses access and the new one gains it immediately after commit; Notification 10 queued (new advisor's business contact); audit                                                                                          | No                   |
| S7  | Permanent delete       | Admin                                                                | Test or corrupted sale that never reached delivery (Task 13 never completed); typed confirmation                                          | Related files removed from storage; audit rows survive; audit event written                                                                                                                                                              | Yes                  |

**Cancellation rule.** Let _c_ be the number of the current task.

- Assigned Advisor may cancel while _c_ is 1 to 3.
- Manager may cancel while _c_ is 1 to 6.
- Admin may cancel any `active` sale, and is the only one who can cancel once Task 6 is completed.

**Reopen rule.** Manager or Admin may reopen a sale that was cancelled while _c_ was 1 to 6. Only an Admin may reopen a sale that was cancelled after Task 6 was completed. Advisors can never reopen.

**Customer view of cancellation.** The customer sees the sale as _Cancelada_ without the reason, and no email is sent in MVP/V1.

## 4. Milestones (derived, never stored)

| Milestone                     | Tasks |
| ----------------------------- | ----- |
| 1. Cotización y Documentación | 1–2   |
| 2. Asignación y Pago          | 3–4   |
| 3. Facturación Oficial        | 5–6   |
| 4. Trámites Aduaneros         | 7–8   |
| 5. Liberación y Placas        | 9–10  |
| 6. Preparación y Entrega      | 11–13 |

A milestone is `completed` when all its tasks are completed; `in_progress` when it contains the current task and is not completed; `pending` otherwise. When all 13 tasks are completed, all six milestones are `completed` and there is no current task. The overall sale state (`active` / `completed` / `cancelled`) is always shown separately; for a cancelled sale the milestones show the state frozen at cancellation. One shared derivation (a database view or query) serves the customer portal, employee filters and BI.

**Customer visibility.** Customers see only fields and documents marked customer-visible, and only from **completed** tasks. When a task is reopened or restarted its data disappears from the portal and the milestone moves back honestly. Sale creation data (vehicle, advisor business email and phone, creation date) is visible from the start.

## 5. SLA, reminders and escalation

### 5.1 Durations

| Task                        | Duration | Reminder interval |
| --------------------------- | -------- | ----------------- |
| 2 Debida Diligencia         | 3 days   | 12 hours          |
| 3 Liquidación de Unidad     | 12 days  | 24 hours          |
| 7 Documentos Endosados      | 10 days  | 24 hours          |
| 8 Trámite Aduanero          | 12 days  | 24 hours          |
| 9 Trámite de Liberación     | 30 days  | 48 hours          |
| 10 Inmatriculación y Placas | 15 days  | 24 hours          |

All other tasks have no duration, no SLA and `completion_phase` = `not_applicable` (excluded from SLA averages). Durations are elapsed calendar time; weekends do not pause the clock. A new duration outside these ranges requires an explicit policy.

### 5.2 Timeline of one SLA interval

1. T1 start: `due_date` = `started_at` + duration.
2. At 80% of the duration: one **Approaching** warning to the assigned Advisor.
3. After `due_date`, at the first hourly evaluation: **overdue reminder 1**.
4. Then one reminder per interval (section 5.1) up to a cap of **6** overdue reminders.
5. After the cap: no more emails, UI highlight only.

### 5.3 Escalation by reminder count

| Reminder    | To                  | Copied                          |
| ----------- | ------------------- | ------------------------------- |
| 80% warning | Assigned Advisor    | none                            |
| 1           | Assigned Advisor    | none                            |
| 2–3         | All active Managers | Advisor                         |
| 4–6         | All active Admins   | Advisor and all active Managers |

If a level has no active recipient, the attempt is logged as failed with that reason and the next level is not skipped. No overdue state ever unlocks or reassigns a task.

### 5.4 What resets and what resumes

| Event                        | `due_date`                                                                               | Reminder counters and phase |
| ---------------------------- | ---------------------------------------------------------------------------------------- | --------------------------- |
| Reopen a completed task (T3) | Preserved                                                                                | Kept (cap not re-armed)     |
| Restart (T4, then T1)        | Recalculated from the new `started_at`                                                   | Reset                       |
| Sale cancelled               | Reminders stop immediately                                                               | Frozen                      |
| Cancelled sale reopened (S5) | Shifted forward by the time spent cancelled (equivalent to resuming with remaining time) | Kept                        |

### 5.5 `completion_phase` (computed from timestamps, never from reminders actually sent)

Let D = duration and _elapsed_ = `completed_at` − `started_at`.

- `early`: elapsed < 80% of D.
- `in_approaching`: 80% of D ≤ elapsed ≤ D.
- `overdue`: D < elapsed < D + one reminder interval.
- `recurring_overdue`: elapsed ≥ D + one reminder interval.
- `not_applicable`: task has no duration.

It is recalculated every time the task is completed, using the preserved `due_date` after a reopen.

### 5.6 Scheduler behavior

- Hourly trigger, authenticated with a secret, rate-limited, and answering quickly.
- Evaluation is stateless and idempotent: it claims a task's reminder in the database before sending, so overlapping runs never double-send.
- A reminder is due when `last_reminder_sent_at` + interval is reached, with a few minutes of tolerance so scheduler jitter cannot skip a cycle.
- Reminders to deactivated employees are skipped.

**Quiet hours.** SLA reminder and escalation emails are sent only between 08:00 and 20:00 America/Lima. A reminder that falls due outside that window is sent at the first run inside it. Customer notifications are not restricted by this window.

## 6. System automatic notifications (idempotent per sale + event)

| #   | Trigger                                                                                                                                                                    | Recipient |
| --- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| 1   | Customer account created (by any actor), with a single-use activation link                                                                                                 | Customer  |
| 2   | Sale created                                                                                                                                                               | Customer  |
| 3   | Task 1 completed                                                                                                                                                           | Customer  |
| 4   | Task 3 completed                                                                                                                                                           | Customer  |
| 5   | Task 6 completed                                                                                                                                                           | Customer  |
| 6   | Task 7 completed                                                                                                                                                           | Customer  |
| 7   | Task 10 completed                                                                                                                                                          | Customer  |
| 8   | Task 11 completed                                                                                                                                                          | Customer  |
| 9   | Task 13 completed                                                                                                                                                          | Customer  |
| 10  | Advisor on the sale changed (new idempotency key per change)                                                                                                               | Customer  |
| 11  | Advisor becomes assigned to a sale by a Manager or Admin, either when the sale is created or through reassignment. Not sent when an advisor creates a sale for themselves. | Advisor   |
| 12  | Advisor unassigned from a sale due to reassignment. Sent to the previously assigned advisor.                                                                               | Advisor   |

A successfully sent milestone notification is never re-sent when a task is reverted and completed again. Failed attempts may be retried under a defined policy. Employees get activation links by email at their business address.

**How account links are delivered.** Only the hash of a link token is stored, so a plaintext link cannot be rebuilt later. Creating an employee, resending an activation, asking for a password reset and creating a customer therefore queue a notification that carries no token (in the same transaction as the change that caused it). The dispatcher mints the link at the moment it sends, in the same transaction as the send, cancelling any open link of that account first. The link's validity (72 h for activation, 1 h for reset) counts from the send, a retry always carries a fresh link, and a link whose email can never be delivered is revoked. The dispatcher is woken as soon as the queuing transaction commits, so these emails normally leave within a second; the periodic run (`MAIL_DISPATCH_INTERVAL`) remains the safety net and the retry clock.

## 7. Role × action permission matrix

"Own" means sales where the user is the **current** `assigned_advisor_id`. Access never derives from `created_by`. Every cell is enforced on the backend.

### 7.1 Staff, catalog and customers

| Action                                                | Admin                                                                      | Manager                                 | Advisor                                                                                       | Customer                |
| ----------------------------------------------------- | -------------------------------------------------------------------------- | --------------------------------------- | --------------------------------------------------------------------------------------------- | ----------------------- |
| List / view staff                                     | All                                                                        | Active Advisors only (for reassignment) | No                                                                                            | No                      |
| Create staff, assign role, reset or resend activation | Yes                                                                        | No                                      | No                                                                                            | No                      |
| Deactivate staff                                      | Yes (not the last active Admin; blocked while an advisor has active sales) | No                                      | No                                                                                            | No                      |
| Delete staff permanently                              | Exceptional, only if no history                                            | No                                      | No                                                                                            | No                      |
| View catalog                                          | Yes                                                                        | Yes                                     | Yes (browse and select active entries)                                                        | Own vehicle only        |
| Create / edit models, versions, colors, images        | Yes                                                                        | Yes                                     | No                                                                                            | No                      |
| Archive catalog entries                               | Yes                                                                        | Yes                                     | No                                                                                            | No                      |
| Delete catalog entries permanently                    | Exceptional                                                                | No                                      | No                                                                                            | No                      |
| Create customer                                       | Yes                                                                        | Yes                                     | Yes                                                                                           | No                      |
| View / edit customer                                  | Any                                                                        | Any                                     | Customers with at least one of their sales, plus customers they created that have no sale yet | Own profile (read-only) |
| Document lookup of an existing customer               | Full                                                                       | Full                                    | Legal name only when not yet linked                                                           | No                      |
| Activate / deactivate customer, resend activation     | Yes                                                                        | Yes                                     | No                                                                                            | No                      |
| Delete customer permanently                           | Exceptional, only without sales                                            | No                                      | No                                                                                            | No                      |
| Codart name lookup                                    | Yes                                                                        | Yes                                     | Yes (rate-limited, quota-guarded, logged)                                                     | No                      |

### 7.2 Sales, tasks and documents

| Action                                   | Admin                                         | Manager                         | Advisor                    | Customer                                                      |
| ---------------------------------------- | --------------------------------------------- | ------------------------------- | -------------------------- | ------------------------------------------------------------- |
| View sales                               | All                                           | All                             | Own                        | Own, customer-visible data only                               |
| Create sale                              | Yes, picks advisor                            | Yes, picks advisor              | Yes, auto-assigned to self | No                                                            |
| Edit sale notes                          | Yes                                           | Yes                             | Own                        | No                                                            |
| Change customer, vehicle, temperature    | Nobody (immutable)                            | Nobody                          | Nobody                     | No                                                            |
| Reassign advisor                         | Yes                                           | Yes                             | No                         | No                                                            |
| Cancel sale                              | Any `active` sale                             | While _c_ ≤ 6                   | Own, while _c_ ≤ 3         | No                                                            |
| Reopen cancelled sale                    | Yes                                           | Only if cancelled while _c_ ≤ 6 | No                         | No                                                            |
| Delete sale permanently                  | Test / corrupted, never delivered             | No                              | No                         | No                                                            |
| Start / complete / edit the current task | Yes                                           | Yes                             | Own                        | No                                                            |
| Add task notes                           | Yes                                           | Yes                             | Own                        | No                                                            |
| Restart (`in_progress` → `pending`)      | Yes                                           | Yes                             | No                         | No                                                            |
| Reopen completed task                    | Yes                                           | Yes                             | No                         | No                                                            |
| Edit completed task                      | Yes (reason; financial fields locked per 2.4) | Yes (same)                      | No                         | No                                                            |
| Upload document                          | Yes                                           | Yes                             | Own, current task          | No                                                            |
| Replace / remove document                | Yes (reason)                                  | Yes (reason)                    | No                         | No                                                            |
| Download document                        | All                                           | All                             | Own sales                  | Own sales, customer-visible documents of completed tasks only |

### 7.3 Analytics, history, exports and system

| Action                          | Admin                                        | Manager                                                 | Advisor                      | Customer |
| ------------------------------- | -------------------------------------------- | ------------------------------------------------------- | ---------------------------- | -------- |
| BI dashboard                    | Global, operational and system               | Global business and operational                         | Own pipeline and performance | No       |
| Record history / audit timeline | All, including employee and security events  | All business history (sales, tasks, customers, catalog) | Own sales                    | No       |
| Access logs                     | Yes                                          | No                                                      | No                           | No       |
| Raw exports                     | Full system, including audit and access logs | Customers, sales, task audit logs, cancellation logs    | None                         | No       |
| Change own password / logout    | Yes                                          | Yes                                                     | Yes                          | Yes      |
| Banks and customs-office lists  | Developer-managed only (no admin UI)         | No                                                      | No                           | No       |
| Scheduler endpoints             | Secret-authenticated only                    | No                                                      | No                           | No       |

Every export, permanent deletion, role change, reassignment, cancellation, reopening and document change writes an audit event. Access logs stay separate from the business audit log.
