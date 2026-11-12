"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding, useOnboardingDraft } from "../onboarding-context";
import {
  organizationService as api,
  type Representative,
} from "@/lib/services/organizationService";
import {
  Field,
  SelectField,
  Section,
  StepForm,
  SaveFooter,
  humanize,
  useStepNavigation,
} from "../onboarding-ui";
export default function RepresentativePage() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const org = data.organization!;
  const identity = data.identity;
  const reps = data.representatives;
  // One saved record is unambiguous. Multiple records require explicit selection.
  const initial = reps.length === 1 ? reps[0] : undefined;
  const fromRecord = (rep?: Representative) => ({
    id: rep?.id ?? "",
    version: rep?.version,
    choice: rep
      ? rep.identityReference === identity.identityId
        ? "self"
        : "other"
      : "",
    fullName: rep?.fullName ?? "",
    roleTitle: rep?.roleTitle ?? "",
    email: rep?.email ?? "",
    phone: rep?.phone ?? "",
    authorityType: rep?.authorityType ?? "",
    status: rep?.status ?? "ACTIVE",
  });
  const [form, change, clear] = useOnboardingDraft("representative", () =>
    fromRecord(initial),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const guard = useRef(false);
  const go = useStepNavigation("representative");
  function choose(choice: string) {
    change({
      ...form,
      choice,
      ...(choice === "self"
        ? {
            fullName:
              identity.fullName ||
              (form.choice === "self" ? form.fullName : ""),
            email: identity.email,
            phone: identity.phone,
          }
        : { fullName: "", email: "", phone: "" }),
    });
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (guard.current || !editable) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      const values = {
        fullName: form.fullName,
        roleTitle: form.roleTitle,
        email: form.email,
        phone: form.phone,
        authorityType: form.authorityType,
        isOnboardingOwner: form.choice === "self",
      };
      let saved: Representative;
      if (form.id)
        saved = await api.updateRepresentative(
          org.organizationId,
          form.id,
          { ...values, status: form.status },
          form.version!,
        );
      else saved = await api.createRepresentative(org.organizationId, values);
      // Preserve the confirmed ID/version even if the following read encounters a network failure.
      change({ ...form, id: saved.id, version: saved.version });
      const current = await reload();
      if (
        !current.representatives.some(
          (r) => r.id === saved.id && r.version === saved.version,
        )
      )
        throw new Error("The saved representative could not be confirmed.");
      if (
        current.status?.stages.find(
          (s) => s.stage === "AUTHORIZED_REPRESENTATIVE",
        )?.status !== "COMPLETE"
      )
        throw {
          response: {
            data: {
              error: {
                message:
                  "The record was saved. An active authorized representative is required before continuing.",
              },
            },
          },
        };
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
      title="Authorized representative"
      description="Choose the person legally authorized to act for this company. Review their contact details before saving."
      onSubmit={save}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      <fieldset disabled={!editable || busy} className="space-y-6">
        {reps.length > 1 && (
          <Section title="Choose the record to edit">
            <SelectField
              name="representativeId"
              label="Saved representative"
              required
              value={form.id}
              onChange={(e) =>
                change(fromRecord(reps.find((r) => r.id === e.target.value)))
              }
            >
              <option value="">Select a representative</option>
              {reps.map((r) => (
                <option key={r.id} value={r.id}>
                  {r.fullName} · {r.roleTitle} · {humanize(r.status)}
                </option>
              ))}
            </SelectField>
          </Section>
        )}
        <Section title="Representative details">
          <p className="mb-4 text-sm text-ink-600">Who is the authorized representative of this company?</p>
          <div className="space-y-3">
            {[
              ["self", "I am the authorized representative."],
              ["other", "Someone else represents the company."],
            ].map(([value, label]) => (
              <label
                key={value}
                className="flex items-center gap-3 rounded-xl border border-line p-3 text-sm"
              >
                <input
                  type="radio"
                  name="representativeChoice"
                  value={value}
                  checked={form.choice === value}
                  required
                  onChange={() => choose(value)}
                  disabled={value === "self" && !identity.isOrganizationOwner}
                />
                {label}
              </label>
            ))}
          </div>
          {!identity.isOrganizationOwner && (
            <p className="mt-3 text-xs text-ink-500">
              Your current membership is not the company owner. Select the
              legally authorized person.
            </p>
          )}
        {form.choice && (
          <div className="mt-6 border-t border-line pt-6">
            <div className="grid gap-x-6 gap-y-5 sm:grid-cols-2">
              <Field
                name="fullName"
                label="Full name"
                required
                maxLength={200}
                value={form.fullName}
                onChange={(e) => change({ ...form, fullName: e.target.value })}
              />
              <Field
                name="roleTitle"
                label="Role title"
                required
                maxLength={200}
                value={form.roleTitle}
                onChange={(e) => change({ ...form, roleTitle: e.target.value })}
              />
              <Field
                name="email"
                label="Email"
                type="email"
                required
                value={form.email}
                onChange={(e) => change({ ...form, email: e.target.value })}
              />
              <Field
                name="phone"
                label="Phone"
                type="tel"
                required
                placeholder="+2348012345678"
                hint="Include the country code. No SMS verification is required."
                value={form.phone}
                onChange={(e) => change({ ...form, phone: e.target.value })}
              />
              <SelectField
                name="authorityType"
                label="Authority type"
                required
                value={form.authorityType}
                onChange={(e) =>
                  change({ ...form, authorityType: e.target.value })
                }
              >
                <option value="">Choose authority type</option>
                {data.status!.authorityTypes.map((t) => (
                  <option key={t} value={t}>
                    {humanize(t)}
                  </option>
                ))}
              </SelectField>
              {form.id && (
                <p className="self-end pb-3 text-xs text-ink-500">
                  Record status: {humanize(form.status)}
                </p>
              )}
            </div>
            <p className="mt-4 text-xs text-ink-500">
              Representative contacts are saved separately from account login
              details. Supporting authority evidence may be requested during
              company review.
            </p>
          </div>
        )}
        </Section>
      </fieldset>
      <SaveFooter
        step="representative"
        busy={busy}
        editable={editable}
        disabled={!form.choice || (reps.length > 1 && !form.id)}
      />
    </StepForm>
  );
}
