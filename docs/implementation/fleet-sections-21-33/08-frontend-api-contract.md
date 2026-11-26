# Fleet Sections 21-33: frontend API contract

Written for: frontend engineers building the corporate (tenant) web/mobile surfaces against these routes.

Base path: `/fleet-partner/v1`. Authentication: the existing corporate bearer token. Responses use the
platform envelope (`{"success":true,"data":...}` / `{"success":false,"error":{code,message,request_id,fields}}`).

Rules every client must follow:

- **Writes** (`POST`, `PATCH`) need an `Idempotency-Key` header. Reusing the key with the same body returns
  the original result; with a different body returns 409.
- Routes marked **version** need the object's current `version` as `expectedVersion` in the body (or the
  `If-Match` header). A stale version returns 409; refetch and retry.
- A disabled feature flag returns 503 `service_unavailable`; a missing permission returns 403. Hide the
  feature when either occurs rather than showing an error loop.
- Branch-scoped users only see their branches. Report runs keep the branch scope the requester had when
  the run was requested.
- Money fields are integers in minor units with a separate currency; never add amounts across currencies.

The **Flag** column is the route manifest's flag label. Two differ from their environment variable:
`RECOMMENDATIONS_ENABLED` is switched by `FEATURE_AI_RECOMMENDATIONS`, and `AUTOMATION_ENABLED` by `FEATURE_AUTOMATION`.
All 68 routes are listed below; full request/response schemas are in the OpenAPI files named in 04.

| Method | Path | Operation | Permission | Flag | Idempotent | Version |
|---|---|---|---|---|---|---|
| GET | `/warehouse-receipts` | listWarehouseReceipts | warehouse_receipts.read | WAREHOUSE_INVENTORY_ENABLED |  |  |
| POST | `/warehouse-receipts` | createWarehouseReceipt | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes |  |
| GET | `/warehouse-receipts/:receiptId` | getWarehouseReceipt | warehouse_receipts.read | WAREHOUSE_INVENTORY_ENABLED |  |  |
| POST | `/warehouse-receipts/:receiptId/arrive` | arriveWarehouseReceipt | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes | yes |
| POST | `/warehouse-receipts/:receiptId/receive` | receiveWarehouseReceipt | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes | yes |
| POST | `/warehouse-receipts/:receiptId/resolve-discrepancy` | resolveWarehouseReceiptDiscrepancy | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes | yes |
| POST | `/warehouse-receipts/:receiptId/complete` | completeWarehouseReceipt | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes | yes |
| POST | `/warehouse-receipts/:receiptId/cancel` | cancelWarehouseReceipt | warehouse_receipts.manage | WAREHOUSE_INVENTORY_ENABLED | yes | yes |
| GET | `/putaway-tasks` | listPutawayTasks | warehouse_tasks.read | WAREHOUSE_INVENTORY_ENABLED |  |  |
| GET | `/putaway-tasks/:taskId` | getPutawayTask | warehouse_tasks.read | WAREHOUSE_INVENTORY_ENABLED |  |  |
| POST | `/putaway-tasks/:taskId/claim` | claimPutawayTask | warehouse_tasks.execute | WAREHOUSE_INVENTORY_ENABLED | yes |  |
| POST | `/putaway-tasks/:taskId/complete` | completePutawayTask | warehouse_tasks.execute | WAREHOUSE_INVENTORY_ENABLED | yes |  |
| GET | `/pick-waves` | listPickWaves | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| POST | `/pick-waves` | createPickWave | pick_waves.manage | FULFILMENT_ENABLED | yes |  |
| GET | `/pick-waves/:waveId` | getPickWave | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| GET | `/pick-tasks` | listPickTasks | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| GET | `/pick-tasks/:taskId` | getPickTask | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| POST | `/pick-tasks/:taskId/claim` | claimPickTask | warehouse_tasks.execute | FULFILMENT_ENABLED | yes |  |
| POST | `/pick-tasks/:taskId/complete` | completePickTask | warehouse_tasks.execute | FULFILMENT_ENABLED | yes |  |
| GET | `/packing-units` | listPackingUnits | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| POST | `/packing-units` | createPackingUnit | packing.manage | FULFILMENT_ENABLED | yes |  |
| GET | `/packing-units/:packingUnitId` | getPackingUnit | warehouse_tasks.read | FULFILMENT_ENABLED |  |  |
| GET | `/yard-visits/:yardVisitId/gate-events` | listGateEvents | yard.read | YARD_OPERATIONS_ENABLED |  |  |
| POST | `/yard-visits/:yardVisitId/gate-events` | createGateEvent | yard.gate | YARD_OPERATIONS_ENABLED | yes |  |
| POST | `/invoices/:invoiceId/submit` | submitInvoice | invoices.manage | INVOICES_ENABLED | yes | yes |
| GET | `/receivables/ageing` | getReceivablesAgeing | receivables.read | RECEIVABLES_ENABLED |  |  |
| GET | `/profitability` | getProfitability | profitability.read | EXPENSES_ENABLED |  |  |
| GET | `/profitability/snapshots` | listProfitabilitySnapshots | profitability.read | EXPENSES_ENABLED |  |  |
| POST | `/profitability/snapshots` | createProfitabilitySnapshot | profitability.snapshot | EXPENSES_ENABLED | yes |  |
| GET | `/cost-allocations` | listCostAllocations | cost_allocations.read | EXPENSES_ENABLED |  |  |
| POST | `/cost-allocations` | createCostAllocation | cost_allocations.manage | EXPENSES_ENABLED | yes |  |
| GET | `/exceptions` | listExceptions | exceptions.read | OPERATIONAL_EXCEPTIONS_ENABLED |  |  |
| POST | `/exceptions` | createException | exceptions.manage | OPERATIONAL_EXCEPTIONS_ENABLED | yes |  |
| GET | `/exceptions/:exceptionId` | getException | exceptions.read | OPERATIONAL_EXCEPTIONS_ENABLED |  |  |
| PATCH | `/exceptions/:exceptionId` | updateException | exceptions.manage | OPERATIONAL_EXCEPTIONS_ENABLED | yes | yes |
| POST | `/exceptions/:exceptionId/acknowledge` | acknowledgeException | exceptions.manage | OPERATIONAL_EXCEPTIONS_ENABLED | yes | yes |
| POST | `/exceptions/:exceptionId/escalate` | escalateException | exceptions.manage | OPERATIONAL_EXCEPTIONS_ENABLED | yes | yes |
| POST | `/exceptions/:exceptionId/resolve` | resolveException | exceptions.resolve | OPERATIONAL_EXCEPTIONS_ENABLED | yes | yes |
| GET | `/sla-breaches` | listSLABreaches | sla.read | SLA_MEASUREMENTS_ENABLED |  |  |
| GET | `/notification-preferences` | listNotificationPreferences | notification_preferences.manage | NOTIFICATION_PREFERENCES_ENABLED |  |  |
| PATCH | `/notification-preferences` | saveNotificationPreference | notification_preferences.manage | NOTIFICATION_PREFERENCES_ENABLED | yes |  |
| PATCH | `/notification-preferences/organization` | saveOrganizationNotificationPreference | notification_preferences.manage_organization | NOTIFICATION_PREFERENCES_ENABLED | yes |  |
| GET | `/dashboards/:dashboard` | getDashboard | analytics.read | ANALYTICS_REPORTS_ENABLED |  |  |
| GET | `/reports` | listReports | reports.read | ANALYTICS_REPORTS_ENABLED |  |  |
| POST | `/reports` | createReport | reports.create | ANALYTICS_REPORTS_ENABLED | yes |  |
| GET | `/reports/:reportId` | getReport | reports.read | ANALYTICS_REPORTS_ENABLED |  |  |
| POST | `/reports/:reportId/run` | runReport | reports.create | ANALYTICS_REPORTS_ENABLED | yes |  |
| GET | `/report-runs` | listReportRuns | reports.read | ANALYTICS_REPORTS_ENABLED |  |  |
| GET | `/report-runs/:runId` | getReportRun | reports.read | ANALYTICS_REPORTS_ENABLED |  |  |
| GET | `/recommendations` | listRecommendations | recommendations.read | RECOMMENDATIONS_ENABLED |  |  |
| POST | `/recommendations/generate` | generateRecommendations | recommendations.generate | RECOMMENDATIONS_ENABLED | yes |  |
| GET | `/recommendations/:recommendationId` | getRecommendation | recommendations.read | RECOMMENDATIONS_ENABLED |  |  |
| POST | `/recommendations/:recommendationId/decisions` | decideRecommendation | recommendations.decide | RECOMMENDATIONS_ENABLED | yes | yes |
| GET | `/model-versions` | listModelVersions | recommendations.read | RECOMMENDATIONS_ENABLED |  |  |
| GET | `/automation-rules` | listAutomationRules | automation.read | AUTOMATION_ENABLED |  |  |
| POST | `/automation-rules` | createAutomationRule | automation.manage | AUTOMATION_ENABLED | yes |  |
| GET | `/automation-rules/:ruleId` | getAutomationRule | automation.read | AUTOMATION_ENABLED |  |  |
| PATCH | `/automation-rules/:ruleId` | updateAutomationRule | automation.manage | AUTOMATION_ENABLED | yes | yes |
| POST | `/automation-rules/:ruleId/submit` | submitAutomationRule | automation.manage | AUTOMATION_ENABLED | yes | yes |
| POST | `/automation-rules/:ruleId/approve` | approveAutomationRule | automation.approve | AUTOMATION_ENABLED | yes | yes |
| POST | `/automation-rules/:ruleId/disable` | disableAutomationRule | automation.approve | AUTOMATION_ENABLED | yes | yes |
| GET | `/automation-executions` | listAutomationExecutions | automation.read | AUTOMATION_ENABLED |  |  |
| GET | `/security-events` | listSecurityEvents | security_events.read | SECURITY_EVENTS_ENABLED |  |  |
| GET | `/privacy-requests` | listPrivacyRequests | privacy.manage | PRIVACY_REQUESTS_ENABLED |  |  |
| POST | `/privacy-requests` | createPrivacyRequest | privacy.manage | PRIVACY_REQUESTS_ENABLED | yes |  |
| GET | `/privacy-requests/:requestId` | getPrivacyRequest | privacy.manage | PRIVACY_REQUESTS_ENABLED |  |  |
| POST | `/privacy-requests/:requestId/decisions` | decidePrivacyRequest | privacy.decide | PRIVACY_REQUESTS_ENABLED | yes | yes |
| POST | `/privacy-requests/:requestId/complete` | completePrivacyRequest | privacy.manage | PRIVACY_REQUESTS_ENABLED | yes | yes |
