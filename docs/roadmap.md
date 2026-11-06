# EndosoFlow — Roadmap

### Phase 0 — Discovery and platform validation

Define the business rules, answer the open questions, define the tech stack, resolve possible edge-cases, build the glossary and decision log, and deliver the State Machine & Permissions Spec. It also produces the MVP / V1 / future split.

- **MVP:** employee and customer sign-in with activation links, customers, vehicle catalog, sale creation, the 13-task workflow with documents, cancel and reopen, basic history, the customer portal with milestones and customer-visible documents, and the first two customer emails.
- **V1:** the remaining emails, SLA reminders and escalation, BI dashboard, raw exports, audit and access-log viewers, all catalog viewpoints, hardening.
- **Future:** PDF reports, backups and recovery automation, malware scanning, a custom domain, swapping Apps Script for Resend, an in-portal notification center, i18n.
- **Out of scope:** installments, other currencies, parallel tasks, multi-vehicle sales.

### Phase 1 — Employee side and core logic

| Sub-phase                            | What it delivers                                                                                                                                                                                                                                                                                                 |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **1A Requirements and UX**           | Requirements Catalog (functional and non-functional); Information Architecture & User Journeys                                                                                                                                                                                                                   |
| **1B Data modeling**                 | Final relational data model, ERD, constraints and indexes, schema-level invariants, rules checks                                                                                                                                                                                                                 |
| **1C Architecture and API contract** | Architecture & API Contract: modular monolith, pagination and filtering, RBAC design, concurrency and idempotency, project structure, conventions, API surface                                                                                                                                                   |
| **1D Foundation and authentication** | Repo, CI, environments and secrets, employee auth (hashing, JWT, refresh rotation and reuse detection, cookies and CSRF), access logs, password recovery with a minimal email service, RBAC middleware, rate limiting, audit writer, bootstrap admin. Auth and authorization tests; deployed with an empty shell |
| **1E Reference data modules**        | Employees, customers, Codart lookup and manual fallback, vehicle catalog with R2 images, provenance fields, CRUD per role matrix, with authorization tests                                                                                                                                                       |
| **1F Documents**                     | Private R2 storage, upload validation, metadata, replace and remove, signed downloads, storage cleanup job                                                                                                                                                                                                       |
| **1G Workflow engine**               | Sale creation, task state machine, per-task validation, auto-complete, cancel and reopen, reassignment, derived milestone view, audit events                                                                                                                                                                     |
| **1H Employee frontend**             | React app: login, sales workspace, sale detail with timline, task forms, admin screens                                                                                                                                                                                                                           |
| **1I Hardening and exit**            | History views, demo data, role regression tests, stakeholder demo                                                                                                                                                                                                                                                |

**Order of work for 1F, 1G and 1H (updated November 2026).** The documents table depends on sales and tasks, and completing a task depends on documents, so 1G is built in two slices around 1F: **1G-a** (sales and the task engine), then **1F**, then **1G-b** (real document check, edit of completed tasks, permanent delete). See `phase-1/1g-workflow-decisions.md`. The employee frontend (1H) was started early, alongside the API modules, to test them: customers, catalog and staff screens are connected to the real API; the sales workspace and task forms still use demo data until 1G-a is verified.

### Phase 2 — Customer side

| Sub-phase                      | What it delivers                                                                                                                                       |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **2A Customer authentication** | Separate login, activation, set-password, forgot-password, recovery, strict isolation, ID-guessing and cross-customer tests should fail safely         |
| **2B Customer visibility**     | Server-side allowlist of customer-visible fields and documents, milestone API, advisor contact, empty and error states, E2E tests                      |
| **2C Portal frontend**         | Sale list, six-milestone timeline, documents, avatars                                                                                                  |
| **2D Email notifications**     | Customer emails 1–10 and advisor emails 11–12, idempotency keys, claim-send-retry flow, failure handling, rollback and recompletion send no duplicates |
| **2E Hardening and pilot**     | Security review, pilot with a few customers                                                                                                            |

### Phase 3 — Scheduler, BI, and exports

| Sub-phase                        | What it delivers                                                                                                 |
| -------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| **3A SLA engine**                | Due dates, reminder state, completion phase. Boundary, pause and cap tests                                       |
| **3B Scheduler endpoints**       | Secret-protected hourly job, token cleanup, storage retries, rate limiting, overlap protection and fast response |
| **3C Escalation and indicators** | Level 1–3 emails, overdue badges, urgent filters                                                                 |
| **3D BI dashboard**              | KPI definitions first (numerator, denominator, period, inclusions), then role-scoped charts                      |
| **3E Exports and logs**          | Role-gated CSV and exports, audit and access-log viewers, export auditing, authorization tests                   |
| **3F Production readiness**      | Monitoring, runbook, security review, go-live checklist                                                          |
