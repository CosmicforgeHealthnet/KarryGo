# Karryam staging activation checkpoint ? 8 November 2026

This is preparation evidence, not certification that every live service is connected.

## Completed preparation

The seven remaining candidate binaries compile: Backoffice Bridge, Dispatch/Delivery, Fleet Partner, Vendor, Media/File, Pricing and Analytics. Fleet's worker and Vendor's migration tool also compile. The candidate security scan found no HIGH/CRITICAL vulnerability or secret finding. Analytics was scanned again after its credential-validation change, with the same result.

Pricing and Fleet passed native startup/restart on actual restored staging databases. Vendor passed startup/restart too; its broad container inventory guard observed a disappearing concurrent container whose identity was not retained. That guard is not reported as clean. Follow-up found no unhealthy persistent service and no persistent service restarted during that preview. Original evidence is retained.

Analytics was absent from the deployment register and VPS. This batch adds its private composition and registration, while preserving its separate release hold. Only its new private database/cache have been provisioned. Its application role is not a superuser. Its 11 native migrations are installed. A real backup was restored and checked with 45 native assertions. The updated candidate passed startup/restart on that isolated restore. No operational source data was fabricated or copied into Analytics.

Staging/production Analytics startup now rejects development service credentials, reused caller keys, missing caller keys, short database credentials and unprotected Redis configuration. The development behavior remains available locally. PostgreSQL and Redis have no published ports and are confined to Analytics' own internal network; only its application joins the enrolled source-reader network.

## Existing live work

Notification and Verification are deployed on staging. Verification and campaign subscriptions are active in Headquarters. Actual Verification observations include 10 pending-review cases and 4 verified cases; campaigns currently have no records. Temporary Headquarters setup authorities have been revoked. No new staff grants, MFA changes, business-write permissions or production changes were made.

## Outstanding live acceptance

- The remaining prepared service images still need their supported release paths and activation. The existing Admin/Analytics and Dispatch transition holds have not been bypassed.
- Dispatch's current relay table has no rows. Fleet's new decision-receipt schema is not installed live. The coordinated transition must include stopped legacy containers with restart policies, not just the running API.
- Wallet and Referral still enforce their native staff permissions in addition to Bridge policy. Root workload access is not a staff grant.
- Media requires its native case/source enrollment and eligible case membership before content may be read. Analytics requires genuine owner lineage and coverage before reports can disclose results.
- Pricing currently has only country-scoped policies. Its pricing-zone vocabulary is distinct from operational zones. No country-level access or invented zone mapping has been granted to work around the approved pilot scope.
- A genuine authorized Central Admin Web session still needs to verify the resulting role access. Database observations and isolated tests do not replace that acceptance.

## Test evidence for this batch

Focused Analytics configuration tests passed, including rejected unsafe staging/production cases. Private-composition exposure tests passed. Go dependency repairs allow all seven candidate service binaries to compile with read-only module resolution. Earlier source and PostgreSQL evidence remains applicable where implementation is unchanged; it is not relabeled as live acceptance.
