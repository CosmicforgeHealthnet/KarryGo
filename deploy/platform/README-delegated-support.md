# Delegated Support/Dispute staging releases

For the Karryam integration task, the owner delegated completion to David Odimayo. The task operator explicitly directed proceeding without a separate deployment reviewer on 2026-11-04. scripts/release/delegated-support.json records that narrow deployment instruction; it is not an independently signed governance or source data-access approval.

Only support-dispute-service is covered, and only the named GitHub initiator may use it. The initiator must also be in the staging environment allowlist and have Maintain/Admin access. Release evidence records authorization.mode=delegated_operator and an empty approvers list. It never claims an independent review occurred. Other services continue to require independent review; absent or invalid delegation fails closed.

Exact current staging SHA, manual dispatch, protected staging branch selection, environment-only credentials, no administrator bypass, successful acceptance jobs, real fresh restore proof and runtime compatibility attestation, artifact integrity, immutable images, disabled admin flags and health checks remain enforced. The Dispatch hold is unchanged. This does not grant access to Karryam records or remove source policy, context, staff MFA or product onboarding requirements.

Revert the delegation file to restore mandatory independent review for Support as well. Configure real required reviewers before using that mode. Existing legacy repository deployment secrets must be reviewed and migrated without breaking older workflows; this delegation does not bypass that check.
