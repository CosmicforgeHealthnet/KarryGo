"use client";
// The company's submitted information and verification status, read only,
// inside the dashboard. It reuses the onboarding data provider and the review
// sections, so it always shows the saved, server-backed records.
import { OnboardingProvider, useOnboarding } from "../../(setup)/setup/onboarding-context";
import { CompanyReview } from "../../(setup)/setup/company-review";
import { ReviewProgress } from "./review-progress";

function CompanySubmission() {
  const { data } = useOnboarding();
  const name = data.organization?.tradingName || data.organization?.legalName;
  return (
    <div className="onboarding mx-auto w-full max-w-5xl space-y-6">
      <div>
        <h1 className="text-ink-950">{name || "Your company"}</h1>
        <p className="mt-1.5 text-sm text-ink-600">
          The company information you submitted and its verification status.
        </p>
      </div>
      {data.organization?.organizationId && data.organization.status !== "ACTIVE" && (
        <ReviewProgress organizationId={data.organization.organizationId} />
      )}
      <CompanyReview edit={() => undefined} />
    </div>
  );
}

export default function CompanyPage() {
  return (
    <OnboardingProvider>
      <CompanySubmission />
    </OnboardingProvider>
  );
}
