"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding } from "../onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import { useRouter } from "next/navigation";
import {
  Section,
  StepForm,
  SaveFooter,
  humanize,
} from "../onboarding-ui";
import { CompanyReview } from "../company-review";
import { organizationDestination } from "@/lib/auth/organizationDestination";
export default function Review() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const router = useRouter();
  const org = data.organization!;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const guard = useRef(false);
  const edit = (path: string) =>
    editable ? `/setup/${path}?return=review` : undefined;
  const missing = (data.status?.stages ?? []).filter(
    (s) =>
      [
        "OWNER_CONTACT",
        "BUSINESS_PROFILE",
        "HEADQUARTERS",
        "AUTHORIZED_REPRESENTATIVE",
        "OPERATING_CAPABILITIES",
        "BUSINESS_DOCUMENTS",
      ].includes(s.stage) && s.status !== "COMPLETE",
  );
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (guard.current || !editable) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      await api.submitOrganization(org.organizationId, org.version);
      // Read the authoritative status back, then continue where it says:
      // a submitted company goes to the dashboard while it is verified.
      const saved = await reload();
      router.replace(organizationDestination(saved.organization?.status));
    } catch (err) {
      setError(err);
    } finally {
      guard.current = false;
      setBusy(false);
    }
  }
  return (
    <StepForm
      title="Review & submit"
      description="Review your saved company information. Submitting starts verification; it does not approve or activate the company."
      onSubmit={submit}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      <CompanyReview edit={edit} />
      {editable && missing.length > 0 && (
        <Section title="Information still required">
          <ul className="list-disc space-y-2 pl-5 text-sm">
            {missing.map((s) => (
              <li key={s.stage}>
                {humanize(s.stage)}:{" "}
                {(s.missingFields?.length
                  ? s.missingFields
                  : ["Complete this section"]
                )
                  .map(humanize)
                  .join(", ")}
              </li>
            ))}
          </ul>
        </Section>
      )}
      <SaveFooter
        step="review"
        busy={busy}
        editable={editable}
        disabled={
          missing.length > 0 ||
          !data.status?.allowedNextActions.includes("SUBMIT_FOR_VERIFICATION")
        }
        label="Submit company for review"
      />
    </StepForm>
  );
}
