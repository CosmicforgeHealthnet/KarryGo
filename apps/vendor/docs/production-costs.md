# Production and operational costs

More -> Production costs opens batch costs, cost policies, saved history and highest-cost materials. Batch details also link directly to that batch's costs. These screens use the existing neutral light/dark surfaces, teal actions, animated full-screen forms, confirmation dialogs, skeleton reads and action-specific pending indicators.

## Public backend flow

The mounted Section 16D surface is five production-cost and seven operational-cost routes. It provides valuation policy management, immutable calculations and reads. It does not provide general expense-entry, expense allocation, accounting or payment commands. No such fields or features were added.

Policies choose an actual ingredient or packaging material, either business-wide or for an actual branch. The three backend methods are latest accepted purchasing cost, weighted average accepted purchasing cost and manual configured cost. Weighted receipt limits stay within 0..200; zero uses the backend default of ten. Money is submitted in integer minor units. Manual currency, unit and reason are preserved. Edit preserves the original material and branch, which the backend does not update. Archive retains historical evidence. Changes use the actual policy version in If-Match and a durable idempotency key.

Choose an actual production batch from the server or open its detail link. Expected and actual calculations are bodyless POST commands: expected reads frozen planned quantities; actual reads recorded consumption plus waste. The server persists each calculation as new immutable evidence. A saved expected snapshot is not automatically recalculated after a policy edit. The comparison reads saved expected/actual snapshots, including missing or incomplete information. Missing amounts display Unavailable; an incomplete comparison never invents a variance. Component breakdowns read the snapshot evidence endpoint. History uses offset/limit pages; highest-cost materials use the server's recent-snapshot read model rather than fabricated revenue analytics.

Cost commands are journaled before transmission, scoped to mode/origin/vendor/staff. A lost response can be retried after reopening with the same method, route, body, headers and idempotency key. Unconfirmed commands block new writes. Version/permission failures clear the failed command and require a refresh. Permission enforcement remains server-authoritative; the app does not treat the permissions catalogue as the current staff member's effective permission set.

## Mock and Live boundaries

Mock remains the default. Mock policies and costs share the existing recipe, material, purchasing and batch stores. Branch overrides take precedence over business-wide policies. Known costs use exact rational quantities and integer rounding; frozen batch waste is not grossed up again. Actual uses measured consumption plus waste, not a copy of planned usage. A real zero amount remains distinct from unresolved evidence. Shared recipe projections consult the same Mock policies once the cost workspace is initialized, preserving saved recipe snapshots.

Mock accepted purchasing evidence must have a known exact inventory-unit cost; fractional conversion costs remain unresolved, matching the purchasing backend. Reversed receipts are excluded from future calculations. Latest/weighted policies resolve only real Mock purchase observations with matching currency and inventory mapping. Manual amounts are operational assumptions and never modify balances, catalogue prices or financial records. Mixed-currency Mock totals are omitted rather than inventing conversion. The backend is the Live calculation authority.

The batch chooser loads the current first 100 batches; older batches remain accessible through their existing batch detail link. Material discovery loads the existing first 200 active materials, as in the material/recipe workspaces. Broader discovery pagination is a final integration consideration, not a fabricated identifier input.

No deployed Live run was performed. Feature mounting, real staff scope, purchasing evidence and production records still need deployed verification. No backend business feature was changed for this item.

## Focused verification

cost_workspace_test.dart exercises real public API contracts through the Mock transport: active recipe/batch creation, planned quantity costing, unknown costs without zero totals, manual policy and updates, lost snapshot response and reopening, replay without duplicate history, immutable component evidence after policy changes, shared recipe valuation, actual snapshots, incomplete/complete variance and stale policy version rejection. Focused analysis covers the new UI/controller, Mock integration and changed navigation. This is not a claim of final whole-app or deployed Live certification.
