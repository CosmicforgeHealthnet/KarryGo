// The one place that decides where a member of an organization belongs.
//
// It reads only the backend's lifecycle status (from /organizations/current),
// never a cookie or a cached client value, and the backend independently
// enforces the same rule on every state-changing request
// (fleet-partner-service middleware.RequireOperationalOrganization).
//
// Backend lifecycle (organizations/usecases/lifecycle.go):
//   DRAFT -> CONTACT_VERIFIED -> BUSINESS_SUBMITTED -> UNDER_REVIEW -> ACTIVE
//   UNDER_REVIEW -> CHANGES_REQUIRED -> BUSINESS_SUBMITTED
//   ACTIVE <-> SUSPENDED; ACTIVE|SUSPENDED -> CLOSED
// PENDING_REVIEW is a legacy synonym of DRAFT: it still accepts onboarding
// edits and has not been submitted. There is no organization-level APPROVED
// or REJECTED status; approval activates the company (ACTIVE), and a review
// that needs more information returns it to CHANGES_REQUIRED.

export type OrganizationAccess =
  // Still completing setup: the onboarding flow.
  | "onboarding"
  // The review asked for corrections: back into onboarding to fix them.
  | "corrections"
  // Submitted and being verified: the dashboard, with operations restricted.
  | "pending"
  // Approved and active: the dashboard with every permitted operation.
  | "operational"
  // Suspended, closed or archived: the dashboard, restricted.
  | "restricted"
  // A status this app does not recognise: the dashboard, restricted, which
  // re-reads the authoritative status rather than guessing.
  | "unknown";

const ACCESS: Record<string, OrganizationAccess> = {
  DRAFT: "onboarding",
  PENDING_REVIEW: "onboarding",
  CONTACT_VERIFIED: "onboarding",
  CHANGES_REQUIRED: "corrections",
  BUSINESS_SUBMITTED: "pending",
  UNDER_REVIEW: "pending",
  ACTIVE: "operational",
  SUSPENDED: "restricted",
  CLOSED: "restricted",
  ARCHIVED: "restricted",
};

export const CREATE_COMPANY_DESTINATION = "/setup/create-company";
export const SETUP_DESTINATION = "/setup";
export const DASHBOARD_DESTINATION = "/";

export function organizationAccess(
  status: string | null | undefined,
): OrganizationAccess {
  return ACCESS[(status ?? "").trim().toUpperCase()] ?? "unknown";
}

// Onboarding (including corrections) happens in /setup; everything else uses
// the dashboard.
export function belongsInSetup(status: string | null | undefined): boolean {
  const access = organizationAccess(status);
  return access === "onboarding" || access === "corrections";
}

// Only an ACTIVE company may operate. Completing every onboarding step is not
// approval.
export function canOperate(status: string | null | undefined): boolean {
  return organizationAccess(status) === "operational";
}

// Where a member resumes. `null` means the identity has no company yet.
export function organizationDestination(
  status: string | null | undefined,
): string {
  if (status === null) return CREATE_COMPANY_DESTINATION;
  return belongsInSetup(status) ? SETUP_DESTINATION : DASHBOARD_DESTINATION;
}

// Error codes the backend uses to refuse an action because of the company's
// lifecycle (middleware.RequireOperationalOrganization).
export const LIFECYCLE_REFUSAL_CODES = [
  "organization_verification_pending",
  "organization_changes_required",
  "organization_onboarding_incomplete",
] as const;

export function isLifecycleRefusal(error: unknown): boolean {
  const e = error as {
    response?: { status?: number; data?: { error?: { code?: string } } };
  };
  return (
    e?.response?.status === 403 &&
    (LIFECYCLE_REFUSAL_CODES as readonly string[]).includes(
      e.response.data?.error?.code ?? "",
    )
  );
}
