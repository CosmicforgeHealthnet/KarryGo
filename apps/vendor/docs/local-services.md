# Local Vendor and Media runtime

Started on 11 October 2026 from current source. Vendor listens at http://127.0.0.1:8112 and Media at http://127.0.0.1:8109. Both /health and /ready returned HTTP 200. Vendor auth/start returned HTTP 200 with development-only OTP support. Media initially returned 503 during startup; its subsequent readiness check passed.

Existing PostgreSQL (5432) and Redis (6379) were reused. New isolated local databases vendor_app_local and vendor_media_local were created and migrated. Vendor uses Redis database 9 and a dedicated auth key prefix. Runtime settings, credentials, binaries, logs and process IDs are under ignored tmp/vendor-local. No production deployment was performed. Existing databases were not migrated or modified.

Run scripts/start-vendor-local.ps1 to start the configured processes. It leaves already-running owned processes alone. Run it with -Stop to stop only the recorded processes whose executable paths match. Stop and restart to rebuild current source; -SkipBuild uses existing binaries. Local configuration is workstation-specific and is not committed. Media uses the existing Firebase bucket credentials; private storage is real Firebase storage.

Vendor runs in API mode; background Vendor workers and downstream Wallet/Verification/dispatch integrations have not been started or certified. Development auth returns an OTP in its API response and does not require SMS delivery. Mock stays the app default. The local owner +2348012345678 is provisioned through the existing HMAC-protected internal owner endpoint for vendor-local-karryam. A draft Karryam Test Kitchen business was created through the authenticated public business API. Phone sign-in, own-access (163 grants), onboarding and activation-readiness reads returned HTTP 200. Email owner@example.com is not the provisioned identity: provisioning prefers phone when both are supplied. Onboarding remains incomplete; no activation or verification approval was fabricated. Mock orders/data were not imported. Repeated auth/start hashing was corrected to bind the new code to the existing challenge ID; repeated-start verification passed on the running backend. Check tokens were kept in memory and the successful check session was logged out.

Native Windows ClamAV is now configured; see the scanner verification below. Safe-download enforcement remains enabled. A successful upload scan does not approve compliance or activate the business.

## Android phone

ADB detected no attached device during this startup. Once USB debugging is enabled and the phone is authorized, run from apps/vendor:

```powershell
adb devices
adb reverse tcp:8112 tcp:8112
adb reverse tcp:8109 tcp:8109
flutter run -d <device-id> --no-pub --dart-define=VENDOR_API_ORIGIN=http://127.0.0.1:8112
```

In the app's Data source controls select Live, use http://127.0.0.1:8112 and apply. Do not use the Dispatch origin/define. Local servers bind to loopback, so USB reverse is the phone connection path. Both ports are needed because authorized uploads reach Media directly.

For development OTP entry, first request a code in the app using +2348012345678. Then run this on the host to request the latest code through the real resend API; it reuses the app's active challenge:

```powershell
(Invoke-RestMethod -Method Post -Uri 'http://127.0.0.1:8112/api/v1/vendor/auth/resend' -ContentType 'application/json' -Body '{"phone":"+2348012345678"}').data.debug_otp
```

Enter the returned code in the app. This development setting is not permitted in production.

## Wallet setup

Payment & Wallet now runs at http://127.0.0.1:8105 against dedicated vendor_wallet_local with Redis database 10. The local start script includes Wallet and applies its tracked migrations through tools/migrate. Vendor uses matching Wallet URL/HMAC credentials. User-provided Paystack test keys are stored only in ignored tmp/vendor-local/wallet.env, not in the app. No payment/transfer was initiated. Wallet, Media and Vendor /health and /ready returned 200 after restart. A signed Vendor caller bank-profile read reached Wallet and returned the expected not-found response for a profile not yet registered.

The external Paystack bank-directory probe returned HTTP 403 with error code 1010. Credential validity and successful account-name resolution have not been established. Retry the app's bank form with a fresh security code and inspect its actual authority response; do not record the bank profile as verified until the real authority succeeds.

## Native scanner verified, 12 October 2026

Docker is not required or used by this scanner. ClamAV 1.4.6 Windows x64 was downloaded from the official Cisco-Talos GitHub release and its SHA-256 matched the published asset digest: 57b6fd1d60cd87bafe800f97407ecdef0576d36b3900b8b7abcfbbabe88295fd. The archive and extracted binaries are inside ignored tmp/vendor-local, not committed. FreshClam downloaded and validated daily, main and bytecode databases.

scripts/start-vendor-scanner.ps1 starts the native daemon in a hidden window at 127.0.0.1:3310. The local service start script starts it when Media's scanner configuration points there. Use start-vendor-scanner.ps1 -UpdateSignatures to refresh official databases. Scanner PID/config/logs are under tmp/vendor-local/clamav-native. Media uses TCP scanning with a 30-second timeout; only the owned Media process was restarted for this configuration change.

Actual scanner checks returned PONG, clean stream OK, and Eicar-Test-Signature FOUND for the harmless in-memory antivirus test. Media readiness returned 200. A real local Vendor owner session authorized the bundled demo PDF for BUSINESS scope, uploaded it to Media and read back upload_status COMPLETED and safety_status SAFE through Vendor. The check session was logged out and tokens were not persisted. The PDF was not submitted as compliance evidence or review; this is upload/scanning verification, not business verification approval. Phone-originated file picking/upload remains the user's next check.

## Verification connected, 12 October 2026

Verification Compliance runs at http://127.0.0.1:8111 with dedicated vendor_verification_local and tracked migrations. Vendor verification integration is enabled with matching outbound HMAC credentials and reciprocal Verification-to-Vendor callback credentials. The local startup script includes Verification; only Vendor was restarted to load the new authority configuration. Both readiness endpoints returned 200. A signed Vendor caller review lookup reached Verification and returned the expected 404 for an absent test reference, demonstrating route/auth connectivity. No review or approval was fabricated. User document submission and actual review/callback processing remain the next Live checks.


Orders and Finance dashboard routes are enabled in the local Vendor runtime. Orders also enables its required checkout, fulfilment planning and quote-lock prerequisites; fulfilment workers and external logistics integrations remain disabled. Authenticated reads of Finance overview, Finance transactions and Vendor orders returned HTTP 200. These read routes do not require business activation; menu publication and customer ordering still require the active business/branch/capability graph.
