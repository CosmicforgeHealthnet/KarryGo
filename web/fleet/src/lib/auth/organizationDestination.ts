// Organization lifecycle decides where a newly authenticated member resumes.
export function organizationDestination(status: string): string {
  if (
    [
      "DRAFT",
      "CONTACT_VERIFIED",
      "PENDING_REVIEW",
      "CHANGES_REQUIRED",
    ].includes(status)
  )
    return "/setup";
  if (["BUSINESS_SUBMITTED", "UNDER_REVIEW"].includes(status))
    return "/setup/submit";
  return "/";
}
