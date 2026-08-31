# Section 20 traceability

| Requirement | Authority / schema | Route or event | Verification |
|---|---|---|---|
| ProductionJob authority and VendorOrder separation | `internal/features/production`, `production_jobs` | existing `/production/jobs` | production package + canonical runner |
| Paid order activation exactly once | `ActivateVendorOrder`, EXTERNAL_ORDER unique index | `POST /production/jobs` with `vendor_order_id`; `production.job.activated` | targeted production tests; migration replay |
| Promised-ready queue ordering | `promised_ready_at`, operational queue index | `GET /production/jobs` | route contract + production tests |
| Keyset production queue | opaque cursor over deadline/rank/created/id ordering | `GET /production/jobs?limit=&cursor=`; `X-Next-Cursor` | cursor validation/round-trip tests |
| Station steps, sequencing and OCC | `production_job_steps`, `CreateStep`, `ChangeStep` | `/jobs/{jobId}/steps*` | targeted production tests |
| Delay evidence and timing projection | `production_job_delay_events`, `commerce_vendor_order_readiness` | `/jobs/{jobId}/delay`; `production.job.delay_reported` | targeted production tests |
| Fulfilment/logistics timing and honest ETA | migration `000092`, mission timing projection, source-version guard | `GET /api/v1/commerce/orders/{orderId}/eta`; timing events | Orders timing tests + contract gates |
| Durable ETA notification | filtered transactional outbox worker and Notification Service adapter | `commerce.customer_eta.changed` | notification publisher test; retrying outbox worker |
| Pastry milestone bridge | explicit custom-cake READY/COMPLETED mapping only | production transition → Pastry READY milestone | pastry/production targeted tests |
| READY packaging evidence | `production_job_ready_evidence`, READY gate | existing `/jobs/{jobId}/ready` | targeted production tests |
| Capacity reuse | existing `capacity_reservations` FK | existing production reservation routes | production integration suite |
| BATCH_STOCK prepared Inventory | actual output × frozen output factor; exact-once posting fingerprint | existing ProductionBatch completion route | productionbatch/inventory output and fingerprint tests |
| Batch/Inventory, Pastry and Cloud Kitchen reuse | existing `productionbatch`, `inventory`, `pastry`, `cloud_kitchen` | existing domain routes | targeted package tests; canonical runner when DB is configured |
| Tenant/IAM/PII boundary | existing `Authorize`, branch predicates; no customer fields in queue | existing vendor bearer routes | canonical security/contract suites |
| Migration and route parity | migration 000092; OpenAPI/register | 782 live routes | migration replay, contract gates |
