# Contract coverage

Generated client coverage checked against the backend Section 43 snapshot and newer source routes. This is repository parity, not live deployment certification.

- Snapshot public Vendor operations: 711
- Generated operations: 744
- Newer source operations: 33
- Feature areas: 29
- Unresolved top-level request schemas: 0
- No Customer/internal/operator routes exposed by this project.

| Feature | Operations |
|---|---|
| access | 14 |
| auth | 7 |
| business | 35 |
| cloud_kitchen | 32 |
| crm | 122 |
| delivery | 24 |
| finance | 6 |
| food_catalogue | 61 |
| handover | 2 |
| inventory | 27 |
| onboarding | 21 |
| operational_cost | 7 |
| orchestration | 3 |
| orders | 25 |
| pastry | 45 |
| production | 54 |
| production_batches | 13 |
| production_cost | 5 |
| production_master | 20 |
| production_stock_policies | 2 |
| promotions | 7 |
| purchasing | 56 |
| readiness | 8 |
| recipe_cost | 5 |
| recipes | 14 |
| scheduled_commerce | 17 |
| shop_catalogue | 100 |
| support | 2 |
| taxonomy | 10 |

Response schemas remain incomplete in several backend documents. Those responses retain exact raw JSON through VendorResponse; no guessed response fields are supplied. Requiredness, state transitions, tenant scope and backend validation remain server authority.

Analytics public Vendor access, realtime subscription still need backend integration contracts. Compliance upload uses the onboarding-specific authorization flow; see ONBOARDING.md. Screen-level completeness is tracked separately in docs/frontend-coverage.md.
