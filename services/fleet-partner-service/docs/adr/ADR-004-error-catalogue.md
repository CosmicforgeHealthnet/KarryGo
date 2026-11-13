# ADR-004: Section 63 is the error code catalogue

- Status: ACCEPTED (product owner, 2026-11-13)
- Affected specification: Sections 63, Appendix C, 36

## Decision

Section 63 is the binding error catalogue. Where Appendix C and Section 63
disagree, Section 63 wins. Codes the service already returns and that clients
depend on are kept only where Section 63 has no equivalent; where it has one,
the service returns the Section 63 code and the HTTP status Section 63 gives.
The mapping is maintained in `internal/platform/errorcodes` and covered by a
catalogue contract test.

## Effects

- API: some error codes and statuses change (for example validation errors
  become 422). Web and provider apps must key behaviour on the Section 63
  codes.
- Backward compatibility: the web app is updated in the same change; provider
  services that relay Fleet codes are listed in the developer brief.
