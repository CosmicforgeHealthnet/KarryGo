# Staging release approval gates

Status: engineering controls implemented; **NOT AUTHORIZATION TO MERGE OR RELEASE**.
The deployment workflows must remain unused until an administrator completes and
independently verifies the configuration below. No production access/policy key,
Admin grant, suppression threshold or Media lifetime is provisioned here.

## Release boundaries

`deploy-platform.yml` and `deploy-vendor-service.yml` now have only
`workflow_dispatch`. Neither staging pushes nor PRs, workflow reuse, successful CI
events or an `all` selector can start a runtime rollout. There is no `skip_tests`.
Both use the shared `staging-release-runtime` concurrency lock without cancellation.
All mandatory acceptance workflows now run for every staging push/PR, so a
documentation-only release SHA cannot inherit another SHA's certification.

Manual inputs are the full current staging SHA, explicit comma-separated service
keys, and a successful `certify-staging-restore.yml` run ID for that SHA. The
workflow itself, checked-out source, API staging head and release SHA must agree.
Only a fresh first-attempt dispatch on `refs/heads/staging` is accepted. A rerun
requires a new decision. If staging advances while images build, pre-host
reauthorization rejects the old release. API errors or incomplete provenance deny.

Dispatch, Admin Bridge, Analytics, Gateway and Partner Web are forbidden selectors.
Vendor is accepted only by its own workflow. Platform accepts only explicit other
registry keys. Duplicate, wildcard, whitespace and injected selectors fail.
Direct Dispatch deployment and rollback scripts also reject that service before
Docker/migration commands; there is no environment-variable escape hatch.

After environment approval, `scripts/release/gate.py` independently checks:

- Actual required reviewers, self-review prevention, disabled admin bypass and a
  custom branch-only `staging` environment policy with no extra branch/tag rules.
- An environment-owned initiator allowlist, current maintain/admin permission,
  unchanged actor and separate approval in this run's GitHub review history.
- Environment-scoped credential names; forbidden same/legacy deployment secret
  names at repository or inherited organization scope cause denial. Audit API
  credentials require read-only metadata permissions, not deployment authority.
- All six existing acceptance workflows plus Deployment safety acceptance, on the
  exact SHA. The latest run must succeed with its required jobs all successful;
  missing, pending, failed, skipped or insufficient jobs block release.
- A separately approved, successful restore certification from the canonical
  workflow, same SHA/staging branch, no rerun, unexpired v4 artifact, exact ZIP
  digest and the explicit service proofs described below.

The release builds only the approved selection. Images are deployed by content
digest, never `latest` or mutable `staging`. Unique release tags identify the run.
The authorization, API environment snapshot, CI IDs, restore digest and image
digests are uploaded with `overwrite: false` **before** host mutation. Upload
failure blocks release. Artifacts are retained 90 days; mirror their digests and
files into operator-approved long-term retention before they expire. GitHub
administrators can delete artifacts; this is not a claim of permanent WORM storage.

The executor repeats API authorization immediately before SSH/SCP. It enforces
pinned SSH known hosts, copies only release files into a unique
`/opt/karrygo/releases/<sha>/<run-id>` directory, and leaves the existing platform
directory, Nginx, Gateway/Partner files and live env files untouched. It stops at
the first service error. No automatic image cleanup can delete fallback images.
New privileged Admin flags found enabled in an owner's env file block replacement;
the deployment never changes those flags. Vendor stops its existing worker before
migration, starts/health-checks the API, then starts the worker. Failure can leave
the worker stopped: recovery needs the approved compatible API/worker pair.

These controls do not certify the actual VPS topology or compatibility. Existing
service-specific health fallback is not a schema rollback; migrations/evidence
remain. Do not invoke a release until live owners and operators approve it.

## Administrator configuration (human-selected identities required)

The repository owner is a GitHub **User** account. Select actual individual
reviewers with repository access; do not invent a team or approver. Release
initiators and approvers must be separate people for each run. The pending user
question requests these identities. Having API admin access is not permission to
assign release authority arbitrarily.

1. In repository Settings → Environments → `staging`, select Required reviewers,
   enable Prevent self-review, and turn OFF administrator bypass. Restrict
   deployment branches to Selected branches and tags: the sole rule must be
   **branch `staging`**, no tag rule and no wildcard. Remove any extra rules only
   after reviewing their existing purpose. Required reviewers are distinct from
   the branch's ordinary PR review rules.
2. Configure environment variable `STAGING_RELEASE_INITIATORS` as a nonempty JSON
   array of approved GitHub logins. Those users must independently have maintain
   or admin permission. Do not set a repository variable with that purpose.
3. Provision these secrets **only in environment `staging`** via an approved
   operator channel: `STAGING_DEPLOY_HOST`, `STAGING_DEPLOY_USER`,
   `STAGING_DEPLOY_SSH_KEY`, `STAGING_DEPLOY_KNOWN_HOSTS`, `RELEASE_AUDIT_TOKEN`.
   No values belong in source, reports, chat, workflow inputs or artifacts. The
   audit token needs read-only Actions, Environments/Administration, Variables,
   Secrets metadata and repository access; it must be able to enumerate secret
   names and review history. It needs no secret-value, SSH, settings-write or
   merge authority. Use a narrowly scoped GitHub App or fine-grained credential.
   If inherited organization-secret scope cannot be audited, fail closed.
4. Confirm no repository/inherited organization secrets with those names or
   `VPS_HOST`, `VPS_USER`, `VPS_SSH_KEY`. Existing `VPS_*` environment-only secrets
   are not used by the revised workflows. Do not remove credentials used by other
   approved operations without separately reviewing their ownership.
5. Configure staging PR protection/rulesets to require the six acceptance suites
   plus Deployment safety acceptance, dismiss stale reviews and require review
   of workflow/release-script changes by the selected responsible owner. No check
   is disabled here. Review the current branch drift before merge approval.

Equivalent environment API payload (replace the reviewer ID only after selection):

```json
{
  "wait_timer": 0,
  "prevent_self_review": true,
  "reviewers": [{"type": "User", "id": "REPLACE_WITH_APPROVED_NUMERIC_USER_ID"}],
  "can_admins_bypass": false,
  "deployment_branch_policy": {"protected_branches": false, "custom_branch_policies": true}
}
```

Resolve IDs with `gh api users/APPROVED_LOGIN --jq .id`. The JSON `id` must be a
number, not the illustrative string above. Save the reviewed payload to a file,
then use `gh api --method PUT repos/CosmicforgeHealthnet/KarryGo/environments/staging
--input <reviewed-file>`. Create the sole branch policy with `gh api --method POST
repos/CosmicforgeHealthnet/KarryGo/environments/staging/deployment-branch-policies
-f name=staging -f type=branch` only if absent. Set the allowlist with
`gh variable set STAGING_RELEASE_INITIATORS --env staging --body '<approved JSON>'`.
Provision secrets interactively using `gh secret set NAME --env staging`; do not
put secret values on command lines or use the current task to enroll keys.

Independently inspect API responses, rather than relying on the UI or a YAML
environment name:

```powershell
gh api repos/CosmicforgeHealthnet/KarryGo/environments/staging
gh api repos/CosmicforgeHealthnet/KarryGo/environments/staging/deployment-branch-policies
gh api repos/CosmicforgeHealthnet/KarryGo/environments/staging/secrets --jq '.secrets[].name'
gh api repos/CosmicforgeHealthnet/KarryGo/environments/staging/variables/STAGING_RELEASE_INITIATORS
gh api repos/CosmicforgeHealthnet/KarryGo/actions/secrets --jq '.secrets[].name'
```

For organization-owned repositories also inspect inherited accessible organization
secret metadata. Here the owner is a User, so organization inheritance cannot
apply. Run `gate.environment(GitHub(read_only_metadata_token))` independently after
configuration; it must return successfully. Do not dispatch a deployment as a test.
Current verified snapshot (21 October 2026): no reviewer rules, admin bypass true,
no branch policy or initiator variable, only legacy `VPS_*` environment credentials.
The real-API gate denied this state. It is **not configured/operational** yet.

## Backup, isolated restore and certification

`backup.sh` now requires `/etc/karrygo/restore-validation/<container>.json`, an
operator-owned map of every non-template source DB to named SELECT assertions.
Include actual migration ledgers, canonical identities, pending operations,
uncertainty flags, receipts, constraints/triggers and representative data. Queries
run in read-only transactions. Quiesce the source or use an approved consistent
snapshot so validation and dump describe the same state. No claims of cross-DB
atomic consistency or external-side-effect rollback are made.

The helper records the archive digest, source PostgreSQL major and expected query
result digests. Dumps use `pg_dumpall --clean --if-exists`: **never restore this SQL
into an existing database**. Verification creates a fresh, network-isolated,
version-matched PostgreSQL container with a distinct bootstrap role/database,
bounded readiness and no published port. Strict `psql -X -v ON_ERROR_STOP=1`, gzip
CRC, digest/version checks, named semantic assertions and cleanup must all succeed.
It no longer masks restore failures. Missing archive, manifest, assertion or
failed command exits nonzero. Evidence is written only after successful cleanup.
The bootstrap database must not be in the source backup. Native/Docker test
fixtures exercise the same record/verify functions against real databases.

The operator must separately verify off-host backup coverage (external DBs,
Media/object data, Dispatch uploads, configuration and recovery-required keys,
Redis/queue semantics), encryption/retention and recovery objectives. The original
script's automatic DB discovery and seven-day local retention are not a full
disaster-recovery strategy. No live backup has been taken or certified here.

After a **real isolated restore of staging backup**, assemble a manifest:

```json
{
  "schema": 1,
  "environment": "staging",
  "release_sha": "<exact 40-character current staging SHA>",
  "completed_at": "<UTC ISO timestamp, no older than 24 hours>",
  "operator_attested": true,
  "services": {
    "customer-service": {
      "validated": true,
      "commands_succeeded": true,
      "backup_sha256": "<64 lowercase hex>",
      "validation_sha256": "<64 lowercase hex>",
      "source_database_version": 16,
      "restore_database_version": 16,
      "artifact_location": "<protected evidence archive reference, no credentials>",
      "validation_summary": "<native data, invariants and restore checks performed>",
      "runtime_compatibility_attested": true
    }
  }
}
```

Use the real version/digests and real service proofs, never these illustrative
values. The 24-hour freshness limit is a release safeguard, not an Analytics or
Media policy value. A disposable test fixture is labelled `disposable` and is
explicitly rejected as staging proof. Neither JSON syntax nor automation can
prove an operator's statements are truthful: the initiator and separate reviewer
must inspect actual protected logs/data evidence before certification. The
certification workflow records their decision; it does not perform a live restore.

Only after those operational approvals, manually dispatch
`certify-staging-restore.yml` on staging with exact SHA, services and manifest.
Its separate reviewer must inspect the evidence. A successful immutable artifact
is then the `restore_run_id` input to the separately approved release workflow.
The certification approval does not approve that later release job. Any SHA drift,
expired/changed proof, missing approval or failed acceptance blocks the release.

## Dispatch 070 stop/migrate/verify/start and recovery hold

No automated Dispatch transition is implemented. The gate, service script and
rollback script all block it. Before any separately approved transition:

1. Inventory every old/new API, worker, retry/dead-letter consumer and scheduled
   runner that can deliver Fleet decisions. Record images, original operation
   IDs/keys, in-flight leases, uncertain/dead-letter rows and authoritative Fleet
   receipt coverage. Confirm safe backup/restore and sole-consumer ownership.
2. Under an approved maintenance window, drain/stop **all** old incompatible
   consumers and disable their independent restart/supervisor paths. Prove zero
   remaining consumers; merely stopping one Compose container is insufficient.
   Do not mutate uncertainty flags or manufacture new operation keys.
3. Prepare and verify Fleet receipt schema/API/worker compatibility, including
   `000081` offer receipts. Apply Dispatch `000070`, `000071`, `000072` only after
   old consumers are stopped. Check backfill counts and immutable audit; old
   binaries must not run even briefly between migration and new startup.
4. Start only the certified new consumer; missing historical proof remains
   fenced. Verify source-owned signed GET read-back, no POST retry fallback,
   lease exclusion, no duplicate decisions, audit persistence and native public
   workload behavior. Resume traffic only with operator approval.
5. On failure, stop delivery and preserve every fence/receipt/audit row. Prefer
   a forward fix. `000070`/`000071` down migrations deliberately refuse. Never
   restart an old relay, clear `readback_required`, reset UNKNOWN_OUTCOME to
   PENDING or replay dead letters as recovery. An exceptional operator rollback
   needs authoritative outcomes, no unresolved/in-flight decisions and proof
   that no incompatible consumer/supervisor can restart. Database restoration
   also needs external side-effect reconciliation and separate approval.

Admin/Analytics policy approval and Partner/Gateway worker/callback cutover remain
separate holds. No deployment approval is implied by code or backup acceptance.

## Tests and evidence limits

Run `python -m unittest discover -s scripts/release -p 'test_*.py' -v`.
Unit cases exercise positive authorized plans and failure paths; actual Dispatch
shell scripts are executed against an empty temporary service root. Genuine
isolated PostgreSQL backup/restore tests validate data, migration/fence identities
and audit immutability, then inject SQL failure, semantic mismatch, missing or
damaged artifacts and incompatible versions. Linux CI uses disposable Docker;
Windows uses fresh PostgreSQL 18 clusters on random loopback ports, all stopped
afterward. This test does not replace staging restore evidence or a real approved
environment. Real GitHub settings are inspected separately through its API.

For the draft PR update only, `vercel.json` at repository and known application
roots disables Git previews on the exact review branch, preserving other branches'
behavior. See [Vercel's branch-specific deployment configuration](https://vercel.com/docs/project-configuration/git-configuration).
No Vercel release is requested. Treat Vercel statuses separately from backend
acceptance and verify the branch suppression after pushing. The prior two quota
failures do not certify the revised head.
