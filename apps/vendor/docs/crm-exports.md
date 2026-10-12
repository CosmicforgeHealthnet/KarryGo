# CRM exports

Open More > Customers & relationships > Exports. Create an export, choose a backend dataset and CSV or JSONL, then select the permitted business/branch scope. Segment exports require an existing segment; campaign and customer/intelligence filters use the backend contract. Review and verify before submission.

The list and detail expose server state, exported fields, counts and expiry. Refresh queued/running jobs. Ready files require fresh crm.export.download OTP verification, then the phone save dialog. Cancelling the save dialog does not report success. Eligible jobs can be cancelled with their current version.

Mock mode generates allowlisted sample CSV/JSONL content from existing CRM fixtures and advances queued jobs on reads. It does not simulate real worker execution or populated intelligence scores. Creation recovery stores frozen settings and an idempotency key, excluding verification grants and file bytes. A fresh create grant retries the original key without duplicating an accepted job.

Live mode uses existing export handlers plus POST /api/v1/vendor/crm/exports/{exportId}/file. The vendor server checks permission/scope/status/expiry, consumes a download grant, reads private Media with server credentials, and verifies length/checksum. It returns uncached bytes; its idempotency record holds only a completion marker. Redirects are refused on the app file request and private server content read. Service credentials are never put in the app.

Deploy the updated vendor service and configure CRM/export/write feature flags, private Media service access and export workers. Source tests do not establish deployed permissions, privacy fencing, worker execution or phone file-picker behavior. The client limits downloads to 20 MB.

Focused verification: export interruption/recovery and one-use download grants; backend stable creation identity across fresh OTP, changed settings/staff rejection; private content size bounds and redirect refusal; OpenAPI route parity. Final Live/device verification remains in remaining-work.md.
