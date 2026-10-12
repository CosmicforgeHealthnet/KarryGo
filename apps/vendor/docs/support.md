# Order support

Open **Orders > order details > Order support > Reports and issues**. The dedicated screen lists cases for that order. **Report an issue** opens a full-screen form; category selection and review use focused sheets. Submission shows animated success or error feedback. Tapping a case opens its returned status/reference/resolution details.

Both modes use the public GET and POST `/api/v1/vendor/commerce/orders/{vendorOrderId}/support-cases` operations. GET returns `{cases: [...]}`. POST submits only category, description and optional selected line_refs. The vendor handler derives the vendor order from the path; the app does not submit customer context, invented priorities, attachments or operator actions.

Category is one of the 11 string values defined in `shared/go/supportcase/supportcase.go`. The generator now reads those enum constants; it previously mistook this imported scalar for an object. Descriptions are limited to the server's 1,000-byte bound and numeric credential-shaped text is sanitized before recovery storage and submission. Only actual order lines can be selected, up to 20.

The required Idempotency-Key is saved with the exact request before sending. Live attempts use existing secure preferences, scoped by origin, vendor and order; Mock attempts stay in memory and are isolated by runtime revision. Unknown responses preserve the original key/body and block a new report. Refresh reconciles a matching returned operation_ref without another write, or the original report can be explicitly retried. Storage failures prevent an unsafe new write; a known accepted report remains accepted even if recovery cleanup fails.

Read and write denials are independent. The server owns case status, resolution and refund progress. The app has no vendor reply, close, refund approval or attachment-upload action because these two public routes do not provide them. Case projections do not expose the saved description or affected lines, so case details do not fabricate either.

Mock reports reference actual fixture orders and lines. Reports persist across navigation within the runtime and duplicate identical commands return the original case. The Mock form prefills a quality issue description for quick testing; Live starts with an explicit category choice and empty description. Mock never calls the Live server.
