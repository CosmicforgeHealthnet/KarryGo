# Fleet end-to-end activation report (staging)

Date: 2026-11-13. Branch: `staging`. Scope: make the Karryam Fleet web app
and `fleet-partner-service` usable end to end on staging: switch the modules
on, fix what breaks, and make staff management and payroll real.

## What was wrong

1. **Staff management was fake.** Every staff, role and permission page used
   hard-coded sample people. The invite form only pretended to send, and the
   API could not list invitations, show one staff member, or return contacts
   and roles.
2. **Almost every module was switched off on staging.** 173 of 178 feature
   flags default to off, and staging had only the core settings. Every module
   page returned "Resource not found".
3. **Workforce pages were blank even when enabled.** The API sent Go field
   names (`ID`, `StartAt`) while the pages read camelCase.
4. **Payroll and compensation forms could never save.** They sent date-only
   values the API rejects.
5. **No company could ever become ACTIVE.** Activation requires an active
   default branch, and nothing ever created one.
6. Many links went to pages that do not exist (trips, settlement detail, CRM
   contacts/locations, route progress, rescue, shift edit).
7. Ordinary lookups of unknown records returned 500 instead of 404.

## What changed

| Area | Change | Commit |
|---|---|---|
| Staff API | Directory with verified email/phone and roles; staff detail; invitation list, resend and cancel; camelCase roles and permissions | `d83e81e` |
| Activation | Every company gets an active "Head Office" branch; migration 000087 backfills existing companies | `b547b37`, `ce55724` |
| Staff web | Real directory, invite, detail (suspend/reactivate), roles, permission catalogue; Team section in the sidebar; invitation links (`?token=`) work | `5122bfb` |
| Errors | Unknown breakdowns, fuel records and parts return 404; verification status before submission and closure status with nothing open no longer 500 | `31fa43e` |
| Workforce | camelCase responses; leave and profile statuses match the database; Profiles list; tab bar for Profiles, Shifts, Leave, Qualifications, Training; shift edit route fixed | `1c247fc` |
| Payroll | Period and compensation-rule forms send RFC3339 dates; statements reachable | `1c247fc`, `dbcf5f6` |
| Links | Trips list and detail on the tracking API; settlement detail moved to its route; CRM contact and location create forms; dead progress/rescue links fixed | `dbcf5f6` |
| Staging | Module flags switched on via `deploy/platform/fleet-staging-modules.txt` and the `Fleet staging capabilities` workflow | see below |

## Modules switched on (staging)

These are tenant-owned modules whose data lives in Fleet: CRM, contracts,
quotations, service catalogue, SLAs; orders, shipments, packages, manifests,
recurring and private orders; dispatch planning, routes, control tower,
tracking, geofences, alerts; workforce, shifts, leave, training,
qualifications, performance; assets and maintenance, inspections, fuel, parts,
breakdowns; incidents, safety, quality, claims, escalations, compliance;
warehouses, inventory, fulfilment, dock and yard, terminal transfers, returns;
invoices, receivables, expenses, budgets, compensation, payroll,
subcontractor settlements; communications and notification rules; specialized
logistics categories; search, exports, imports, bulk imports; document
uploads and downloads.

Left off on purpose, because they move money or need a service or secret
staging does not have: Wallet allocations and withdrawals, finance posting
and integrations, private trip execution, webhooks, API clients, retention
execution, proof records, self-verified documents, and the medical-sensitive
and taxi-passenger categories. Payroll can be calculated and approved; it is
not posted to wallets.

## How this was verified

- Go: the full `fleet-partner-service` suite against PostgreSQL, plus new
  integration tests (staff directory and invitations, Head Office branch,
  migration 000087 replay).
- Browser: `scripts/staff-e2e.mjs` signs up, opens the staff directory,
  invites, resends, cancels, opens the owner's detail, and reads roles and
  permissions.
- Real service: the production binary was booted with every switched-on
  module and the worker enabled, signed in through the real OTP flow as an
  ACTIVE company, and every tenant read route was probed. Five server errors
  were found and fixed. Shift create/publish, payroll create/calculate and
  compensation-rule create were exercised.
- `next build` passes; changed pages type-check and lint clean.

## Still not working, and why

1. **Company approval does not complete automatically.** When an operator
   approves a company, Verification-Compliance records `VERIFIED`. Fleet's
   activation gate looks for `APPROVED` and never reads the decision back:
   nothing calls `TryActivate`. Activation also needs an ACTIVE company
   wallet, and staging has no Wallet service secret for Fleet. The
   eligibility policy additionally requires an approval request and
   human-verified documents. Until this pipeline is built, a submitted company
   stays UNDER_REVIEW. It can browse every module and manage staff, but
   cannot create operational records. This needs a decision on how
   operators approve companies; it was not changed here.
2. **Payroll is for providers only.** Office staff are not on payroll. Paying
   office staff is a new design, not a fix.
3. **No backend yet** for custom roles, editing staff, role bindings,
   approval policies, shift templates, training enrolments, reports,
   automation and recommendations (the spec's Section 24).
4. **Pages still on sample data** (no backend behind them): Wallet overview
   pages, parts of Compliance, Documents, Reports, Comms, Settings governance
   (sessions, policies), and some Private Ops screens.
