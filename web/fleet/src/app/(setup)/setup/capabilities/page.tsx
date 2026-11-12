"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding, useOnboardingDraft } from "../onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import {
  Section,
  StepForm,
  SaveFooter,
  humanize,
  useStepNavigation,
} from "../onboarding-ui";
export default function Capabilities() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const [selected, change, clear] = useOnboardingDraft("services", () =>
    Object.keys(data.capabilities),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const guard = useRef(false);
  const go = useStepNavigation("services");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (guard.current || !editable) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      await api.updateCapabilities(data.organization!.organizationId, selected);
      await reload();
      clear();
      go();
    } catch (err) {
      setError(err);
    } finally {
      guard.current = false;
      setBusy(false);
    }
  }
  return (
    <StepForm
      title="Services you offer"
      description="Select the services your company intends to offer or manage. Certain services require additional review."
      onSubmit={save}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      <Section title="Select Capabilities">
        <fieldset
          disabled={busy || !editable}
          className="grid gap-3 sm:grid-cols-2"
        >
          {data.status!.supportedCapabilities.map((code) => (
            <label
              key={code}
              className="flex items-center gap-3 rounded-xl border border-line p-4 text-sm"
            >
              <input
                type="checkbox"
                checked={selected.includes(code)}
                onChange={(e) =>
                  change(
                    e.target.checked
                      ? [...selected, code]
                      : selected.filter((c) => c !== code),
                  )
                }
              />
              {humanize(code)}
            </label>
          ))}
        </fieldset>
      </Section>
      <SaveFooter
        step="services"
        busy={busy}
        disabled={selected.length === 0}
        editable={editable}
      />
    </StepForm>
  );
}
