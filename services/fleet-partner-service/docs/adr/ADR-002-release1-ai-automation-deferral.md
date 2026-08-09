# ADR-002: Release 1 AI and automation implementation deferral

- Status: RELEASE-1-APPROVED-SCOPE-EXCEPTION
- Affected specification: Sections 54, 59–62, 64, 66–71
- Decision owner/approval: Product, Architecture, Security, and Compliance approval required before future activation; no named approver is asserted here.

## Decision and rationale

Release 1 does not execute AI models, generate operational recommendations, infer fraud/adverse decisions, perform natural-language write actions, or run automation rules/schedulers. Existing safe reports, searches, exports, dashboards, and non-executing scaffolding remain available where already implemented.

AI or automation cannot alter live assignments, money, safety, compliance, access, or provider outcomes in Release 1. Deferred endpoints and workers remain disabled or unmounted as appropriate. No automatic adverse action is permitted.

## Release 1 effects

- API: no AI recommendation/model-execution or automation-action API is activated.
- Data/schema: existing scaffolding is retained; no generated decision is presented as authoritative business state.
- Events: no model/automation event is emitted as proof of an action that did not occur.
- Security: protected-domain writes remain under human-authenticated permissions, approvals, step-up, audit, and authoritative services.
- Backward compatibility: ordinary reporting/export and existing operational workflows remain unchanged.
- Tests: tests cover disabled/fail-closed posture and exclude AI/automation E2E success.
- Rollout: AI and automation stay disabled through general availability.
- Rollback: keep feature gates off; no automated business mutations need reversal.

## Future activation conditions

Future implementation requires a separately approved implementation package covering model/vendor boundaries, data governance, explainability, human review, adverse-action safeguards, authorization, audit, prompt/input/output security, evaluation, monitoring, rollback, migrations, and domain-specific tests. Human approval is mandatory before any protected-domain write is possible.
