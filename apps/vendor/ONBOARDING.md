# Vendor onboarding

Mock remains the default on every launch. Open Data source to select Live or a Mock business state. Changing source closes open setup forms and starts a separate session. Mock records live in memory and never contact a server.

## Phone testing

From `apps/vendor`:

```powershell
flutter run --no-pub -d 15192375CF009258
```

Restart Flutter for the new file-picker plugin; hot reload cannot install a native plugin. Sign in with any Mock contact and code `123456`, then select a vendor when prompted. Security checks also use `123456` in Mock. Use the Data source controls for new business, pending review, action required, expired, suspended, ready, activated, denied access and dependency failure.

## Supported flow

1. Sign in as an existing affiliated vendor staff member. Multiple affiliations use the server's vendor-choice challenge; no public signup is invented.
2. Read business, onboarding overview and activation readiness. The server provides required checks and blockers.
3. Save legal/trading names, support contacts, currency and timezone with the current business version.
4. Save an authorized representative using OWNER, DIRECTOR or AUTHORIZED_SIGNATORY. Replacing an identity-verified representative requires `onboarding.representative_replace` security verification.
5. Add business types from the server catalogue. Brands are optional; create branches with exact backend address, coordinates and timezone. The branch settings button opens capability linking, multiple operating windows, radius service area and pickup/order settings.
6. Bank registration always requires `finance.bank_change` security verification, including the first account. Full account numbers are write-only and not persisted by the app.
7. Select a verification dimension and an existing subject. Upload a JPEG, PNG, WebP or PDF up to 10 MB using an authenticated vendor grant, then redeem that grant against Media. Attach the resulting opaque media reference as a draft document. Submitted documents are immutable; drafts can be replaced/deleted.
8. Submit selected document IDs for verification. The app displays pending review, rejection reason, remediation and expiry from the server. Rejected/expired cases use resubmit. The app cannot approve verification.
9. Submit onboarding and request activation when server readiness is satisfied. Re-read business and overview before displaying activated status. Suspended/closed businesses remain blocked. No dashboard is included.

Writes use current expected versions and preserve idempotency keys on identical failed retries. Changing data creates a new command. Conflict responses require refresh. An interrupted byte upload is checked against the tenant-owned Media receipt before attachment; the single-use upload token is never blindly retried.

## Live upload support

Added Vendor routes:

- `POST /api/v1/vendor/onboarding/document-uploads`: session authentication, `vendor.onboarding.manage`, subject ownership and branch scope, required Idempotency-Key. Body: dimension, subject_type, subject_ref, mime_type, byte_size.
- `GET /api/v1/vendor/onboarding/document-uploads/{mediaRef}`: tenant-owned compliance upload receipt only; no private bytes or storage locations.

Added Media redemption route:

- `POST /api/v1/media-files/vendor-uploads/{mediaObjectId}/content`: raw bytes, X-Upload-Authorization-ID, X-Upload-Token, Content-Type, X-Declared-Size, X-Declared-Checksum-SHA256. Existing Media validation enforces expiry, single use, checksum, byte size, actual MIME and safety checks.

Deploy/restart both changed services. Set server-side `VENDOR_MEDIA_URL` and `VENDOR_MEDIA_SECRET` for the existing private HMAC authority, plus `VENDOR_MEDIA_PUBLIC_URL` as the phone-reachable Media origin. Media must enable its existing private API and permit vendor-service as an HMAC caller. Keep those credentials exclusively on the server. The app receives only a short-lived private COMPLIANCE_DOCUMENT grant owned by its selected vendor business.

Use the existing Vendor write flags, IAM permissions, verification and Wallet authority configuration. Live continues to require real staff affiliations and configured OTP delivery. No Live OTP delivery or remote deployment is implied by local tests. When using adb reverse, forward both the Vendor port and the Media public port and configure phone-reachable loopback URLs accordingly. The older Dispatch Provider dart-define does not configure the Vendor app: use the app's Data source sheet or `--dart-define=VENDOR_API_ORIGIN=http://127.0.0.1:<vendor-port>`.

Generated API contracts now contain 724 operations with no unresolved request bodies. Regeneration: `python tool/extract_backend.py`, `python tool/generate_contracts.py`, then `python tool/verify_contracts.py` from this app directory. Security-check routes explicitly require the vendor bearer session.

Security-code verification does not automatically replay a rejected OTP or rotate its bound session. The app refreshes before starting a fresh security challenge. A wrong code leaves the current challenge available for correction.
