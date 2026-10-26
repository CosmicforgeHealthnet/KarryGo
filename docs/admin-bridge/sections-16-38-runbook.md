# Karryam Secure Admin — Sections 16–38 operational runbooks

Scope: the private Admin Bridge (`admin-backoffice-service`) and the owner integrations added
in Sections 16–38. These procedures were written against the code on `staging`; none has been
rehearsed on production infrastructure. Every command that changes a live system needs the
approvals named in each runbook. Secret values never appear here — only file/variable names.

Conventions used below:

- **Kill switch** — `ADMIN_REMOTE_WRITES_ENABLED=false` on the Bridge disables every HQ-originated
  mutation (owner commands, HQ approvals, campaign submit/cancel, privacy writes, exports) while
  leaving customer/provider apps and read-only diagnostics running. Family switches:
  `ADMIN_<OWNER>_WRITES_ENABLED`, `ADMIN_<PREFIX>_COMMAND_WRITES_ENABLED`, `ADMIN_HQ_*_ENABLED`.
  Owner-side switches (`<PREFIX>_ADMIN_COMMAND_WRITES_ENABLED`, `NOTIFICATION_CAMPAIGNS_ENABLED`, …)
  are independent; either side alone stops the effect.
- **Original-identity read-back** — `GET /internal/karryam-admin/v1/<domain>/commands/{operation_id}`
  or `GET /hq/campaign-operations/{operation_id}`. Never resubmit an `UNKNOWN_OUTCOME`.
- **Evidence** — Bridge audit (`admin_audit_entries`), operation/approval tables, owner immutable
  decision ledgers (`admin_owner_command_decisions`), event inbox/quarantine, export download audit.
- Re-enabling anything requires the same approvals as enabling it, plus a record of why it was
  disabled.

Each runbook lists: owner · prerequisites · permissions · procedure · expected confirmation ·
failure paths · audit evidence · rollback · escalation.

---

## 1. Private network bootstrap and revocation
- **Owner:** Platform/network. **Prerequisites:** approved topology (decision register §37),
  bind IP for `ADMIN_PRIVATE_BIND_IP`, HQ peer IP/VPN interface. **Permissions:** host root.
- **Procedure:** render candidate rules `python3 deploy/platform/network/admin_policy.py --hq-ip <ip> --vpn-interface <if>`
  and `--public-interface <if>`; review; validate with `nft --check -f <file>`; apply only under change
  approval; `bash deploy/platform/bin/close-public-ports.sh` (dry run) then `--apply`; recreate the admin
  stack `docker compose -f deploy/platform/compose/admin-backoffice-service.yml up -d`.
- **Confirmation:** `bash scripts/admin-bridge/exposure/capture_host_evidence.sh` shows 8110 bound
  only to the private IP; `scripts/admin-bridge/exposure/external_scan.sh --i-am-authorized …` from a
  public vantage point reports every admin/internal path 404 and ports closed.
- **Failure paths:** scan finds an open port → STOP trigger; revert rules (`close-public-ports.sh --undo`)
  only if they broke public customer traffic, never to expose admin.
- **Revocation:** remove the HQ peer from the VPN/ACL and delete its `admin_remote_principals` row
  (set `revoked_at`); the Bridge refuses the certificate immediately.
- **Evidence:** capture/scan JSON files; change ticket. **Escalation:** Security on-call.

## 2. TLS and HMAC key lifecycle
- **Owner:** Security. **Prerequisites:** private CA; new key material generated off-host.
- **Procedure:** add the new HQ key id to `ADMIN_TRUST_FILE` (`service_keys`, `service_key_expires`)
  alongside the old one; restart Bridge; HQ switches `X-HQ-Key-ID`; remove the old key after the overlap.
  Owner audience keys (`*_SECRET_FILE`) are rotated one audience at a time: deploy the new secret to the
  owner's verifier and the Bridge's client in the same window; start-up refuses reuse across audiences.
  Event ingress keys: update `ADMIN_EVENT_SOURCE_SECRETS_FILE` and the owner's
  `<PREFIX>_ADMIN_EVENTS_SECRET_FILE` together; owner relays retry until both match (no event loss).
- **Confirmation:** start-up logs `admin_credential_expiry` for each key; a signed probe request succeeds.
- **Failure:** mismatch → 401s and owner outbox backlog; roll forward, never disable signature checks.
- **Evidence:** expiry log lines, change record. **Escalation:** Security.

## 3. Approved peering changes
- **Owner:** Platform + Security. Only add peers named in a signed topology change. Update the nftables
  candidate inputs, re-render, `nft --check`, apply, then run runbook §4. Never grant a subnet.

## 4. External exposure verification
- **Owner:** Security. **Procedure:** from an authorized external host run
  `bash scripts/admin-bridge/exposure/external_scan.sh --i-am-authorized --base https://<public-host> --host <public-host> --out <dir>`.
- **Confirmation:** exit code 0 and every probe 404/closed. **Failure:** any non-404/open port → STOP;
  run §1 revocation and §22 containment. **Evidence:** the JSON report.

## 5. HQ service-principal enrollment and deactivation
- **Owner:** Security + HQ platform. Insert/expire the certificate fingerprint in
  `admin_remote_principals` (workload URI, audience, expiry) using the migration/operator role only.
  Deactivate by setting `revoked_at`. Confirmation: probe request succeeds/fails accordingly.

## 6. Remote command kill switch
- **Owner:** Bridge on-call. **Procedure:** set `ADMIN_REMOTE_WRITES_ENABLED=false` (or the family flag),
  redeploy the Bridge container only. **Confirmation:** any HQ POST returns `remote_writes_disabled` /
  `capability_blocked`; `GET /hq/projections/status` still answers; customer/provider apps unaffected.
- **In-flight work:** list `UNKNOWN_OUTCOME` operations (`karryam_admin_owner_command_unknown_outcomes`,
  `karryam_admin_campaign_unknown_outcomes`) and resolve by read-back (§11).
- **Rollback:** re-enable only with the original approvals. **Escalation:** Security + owner.

## 7. Compromised or stolen staff token
- Insert a `staff` and/or `session` row in `admin_revocations`; rotate the HQ issuer key if the key
  itself leaked (§2). Review `admin_audit_entries` for the staff ref; pending approvals by that staff
  member expire (12 h) and can be left to lapse. Confirmation: requests with that subject return 401.

## 8. Suspicious admin command
- Freeze: family kill switch (§6). Inspect the operation, its approval row and the owner decision
  ledger. If an owner effect occurred, the reversal is a new owner command with its own approval —
  never a database edit. Escalate to the owning domain lead and Security.

## 9. Event lag and dead-letter recovery
- **Signals:** `karryam_admin_projection_lag_seconds`, `karryam_admin_projection_not_live`,
  owner `admin_event_outbox` rows in `DEAD_LETTERED`.
- **Procedure:** check owner relay env (`<PREFIX>_ADMIN_EVENTS_ENABLED`, origin, secret); check Bridge
  ingress (`ADMIN_EVENT_INGRESS_ENABLED`, address, source secrets). After fixing, replay owner dead
  letters with the owner's outbox replay (shared `outbox.ReplayDeadLettered`), then run a reconciliation
  sweep (§10). Quarantined events (`admin_event_quarantine`) are schema problems: fix the owner emitter
  or adapter in code; quarantine rows are evidence and are never deleted.

## 10. Projection reconciliation
- Enable `ADMIN_EVENT_BACKFILL_ENABLED` with snapshot origins/secrets; the worker backfills once and then
  reconciles every `ADMIN_EVENT_RECONCILE_INTERVAL`. Results: `admin_backfill_runs`,
  `admin_reconciliation_runs` (drift, repaired, missing_at_source). Missing-at-source entities are
  recorded, never deleted. Coverage only reads COMPLETE when the latest run is COMPLETE and no gap is open.

## 11. Unknown Wallet financial outcome
- Read back by original operation id; if the owner has no receipt yet, wait and read again — the slot
  stays held so nothing can race it. Never resubmit. If the Wallet ledger shows the effect, the read-back
  confirms it. Escalate to Finance after the owner's own reconciliation window.

## 12. Contested Verification decision
- Use `REOPEN_CASE` with an `appeal_ref` (requires a second reviewer). Never edit case rows. Vendor is
  notified by the existing signed callback; check callback dead letters (`verification_outbox`) and
  replay if needed.

## 13. Pricing freeze and rollback
- Unchanged from Section 14: use Pricing's own freeze/unfreeze and four-eye activation; quote cutover
  remains disabled.

## 14. Dispatch stranded mission
- Unchanged from Sections 9/10: `RETRY_INCIDENT_RECOVERY`, `REQUEST_REASSIGNMENT` (approval) through the
  Dispatch command dictionary; custody state stays owner-controlled.

## 15. SOS emergency escalation
- **Owner:** Support operations. SOS cases arrive in `GET /hq/support/cases` first (EMERGENCY sorts
  first) and `karryam_admin_sos_open_cases` / `…oldest_open_age_seconds` alert. Escalate with
  `ESCALATE_SOS` (`SAFETY_LEAD` / `EMERGENCY_LIAISON`). Precise GPS is never exported; authorized staff
  read it only through the owner. No SLA is configured (`POLICY_UNAVAILABLE`) — staffing policy is an open
  decision; until it exists, on-call must watch the metric manually.

## 16. Vendor marketplace incident
- Unchanged from Section 11: branch pause/reopen through the Vendor command dictionary.

## 17. Marketing campaign cancellation
- `POST /hq/campaign-operations/{id}/cancellations` with a cancellation key. Unsent recipients become
  CANCELLED; already-sent records stay. To stop everything immediately: `NOTIFICATION_CAMPAIGNS_ENABLED=false`
  on notification-service (owner side) — workers stop claiming; nothing is deleted.

## 18. Notification provider outage
- Deliveries retry with backoff then dead-letter; campaign recipients stay QUEUED (offline queue).
  Logging providers are counted as LOGGED_ONLY, never SENT. After recovery, owner retry workers resend.

## 19. Consent withdrawal and privacy deletion
- Customer withdraws in-app (`PUT /api/v1/customer/consents/marketing`) — enforced at send time.
  Formal requests: register in the Bridge (`POST /hq/privacy-requests`), act through Customer's
  `OPEN/ACCEPT/FULFIL_PRIVACY_REQUEST` commands, then record the confirmed operation with
  `POST /hq/privacy-requests/{id}/fulfilments`. Other holders stay `POLICY_REQUIRED` until a retention
  schedule is approved; legal hold blocks completion.

## 20. Backup, restore and disaster recovery
- Bridge DB: restore from the platform backup tooling (`deploy/platform/bin/backup.sh`, restore
  certification workflow). After restore: projections and cursors may be behind; owners redeliver from
  their outboxes, inbox dedupe prevents duplicates, and a reconciliation sweep (§10) repairs gaps.
  Operations in `EXECUTING`/`UNKNOWN_OUTCOME` are read back, never resent.

## 21. Provider emergency restriction
- Use the owner command (`RESTRICT_PROVIDER` for Taxi/Hauling, Dispatch reassignment) with approval;
  if HQ is unavailable, owners' native admin tooling remains the fallback.

## 22. Evidence or PII leak containment
- Kill switch (§6) and `ADMIN_HQ_EVENTS_ENABLED=false`; revoke affected export tokens by letting them
  expire (15 min) — downloads are single-use and audited (`admin_export_downloads`); revoke Media case
  grants (`POST /media/case-grants/{id}/revocations`); identify the field in quarantine/projection and fix
  the adapter or owner emitter. Notify the DPO per legal process.
