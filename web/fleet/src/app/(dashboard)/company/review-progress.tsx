"use client";
// What still stands between the company and activation: the reviewer's
// requested changes and each activation check, from the server.
import { useEffect, useState } from "react";
import { CheckCircle2, Circle, AlertTriangle } from "lucide-react";
import { Section, humanize } from "../../(setup)/setup/onboarding-ui";
import {
  organizationService,
  type ActivationReadiness,
  type VerificationStatusResponse,
} from "@/lib/services/organizationService";

const GATE_LABELS: Record<string, string> = {
  organization_under_review: "Submitted for review",
  onboarding_stages_complete: "Company information complete",
  verification_approved: "Approved by the Karryam review team",
  organization_wallet_active: "Company wallet ready",
  active_default_branch: "Head office branch set up",
  settings_valid: "Company settings in place",
};

export function ReviewProgress({ organizationId }: { organizationId: string }) {
  const [readiness, setReadiness] = useState<ActivationReadiness | null>(null);
  const [verification, setVerification] = useState<VerificationStatusResponse | null>(null);
  const [error, setError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    Promise.all([
      organizationService.getActivationReadiness(organizationId),
      organizationService.getVerificationStatus(organizationId),
    ])
      .then(([gates, status]) => {
        if (cancelled) return;
        setReadiness(gates);
        setVerification(status);
      })
      .catch(() => {
        if (!cancelled) setError(true);
      });
    return () => {
      cancelled = true;
    };
  }, [organizationId]);

  if (error) {
    return (
      <Section title="Review progress">
        <p className="text-sm text-ink-600">Review progress could not be loaded. Please refresh to try again.</p>
      </Section>
    );
  }
  if (!readiness) return null;

  const issues = verification?.issues ?? [];
  return (
    <Section title="Review progress">
      {issues.length > 0 && (
        <div role="alert" className="mb-4 rounded-xl border border-amber/30 bg-amber-bg/40 p-4">
          <p className="flex items-center gap-2 text-sm font-semibold text-amber">
            <AlertTriangle className="h-4 w-4" />
            The review team asked for changes
          </p>
          <ul className="mt-2 space-y-1.5 text-sm text-ink-700">
            {issues.map((issue) => (
              <li key={issue.id}>
                <span className="font-medium">{humanize(issue.issueCode)}</span>
                {issue.documentReference && <> &middot; {humanize(issue.documentReference)}</>}
                {issue.remediation && <span className="text-ink-500"> &mdash; {humanize(issue.remediation)}</span>}
              </li>
            ))}
          </ul>
          <p className="mt-2 text-xs text-ink-500">Update the details in setup and submit again.</p>
        </div>
      )}
      <ul className="space-y-2.5">
        {readiness.gates.map((gate) => (
          <li key={gate.name} className="flex items-center gap-2.5 text-sm">
            {gate.passed ? (
              <CheckCircle2 className="h-4 w-4 flex-shrink-0 text-green" aria-hidden />
            ) : (
              <Circle className="h-4 w-4 flex-shrink-0 text-ink-300" aria-hidden />
            )}
            <span className={gate.passed ? "text-ink-800" : "text-ink-500"}>
              {GATE_LABELS[gate.name] ?? humanize(gate.name)}
              {gate.passed && gate.detail === "waived" && (
                <span className="text-ink-400"> (not required on this environment)</span>
              )}
            </span>
            <span className="sr-only">{gate.passed ? "done" : "pending"}</span>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-xs text-ink-500">
        {readiness.ready
          ? "Everything is in place. Your company will be activated shortly."
          : "Your company is activated automatically once every step is done."}
      </p>
    </Section>
  );
}
