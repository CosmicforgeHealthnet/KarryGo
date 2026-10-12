# Order evidence

The existing authenticated Shop picking GET endpoints now return the frozen picking snapshots plus stored measurement and quality evidence. Existing order-read permission, tenant identity and branch scope apply; no new endpoint or migration is required beyond the existing picking/measurement schema.

`Measurement` is nullable. It includes the persisted actual/estimated quantities, amounts, limits, financial state and its own `version`. The app uses that version for the existing actual-quantity command; it never substitutes the line-outcome `Version`. Entries remain in the original frozen measurement units, with no invented kilogram/gram conversion. Zero is accepted by the existing backend contract. The backend computes the amount and enforces reservation/payment bounds. Recorded quantity does not imply financial finalization or customer reapproval.

`QualityResult` is the outcome's current quality status, including the backend default. It is not proof that a picker submitted a check. `QualityResults` contains actual saved preference observations, reasons and versions. Updating existing observations increments their persisted result version. `HasOutcome` states whether an outcome exists. The app labels overall status separately from saved observations and retains warning/error colors for partial, unavailable and reapproval states. For lines with no saved quality preferences, the current backend stores only overall status; it has no per-preference observation row to display.

Mock uses the same response shape. The preparing Market oranges order supplies variable-count evidence with an independent measurement version. Grocery picking includes a sorting preference so submitted observations remain visible after refresh. Mock rejects stale measurement versions and out-of-bound quantities, preserves reapproval state, and does not invent Wallet settlement.

Focused checks cover independent measurement versions, limit/reapproval states and saved quality observations/revisions. Deployed database and real payment/worker/device verification remain item 5.
