"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding, useOnboardingDraft } from "./onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import { COUNTRIES } from "@/lib/countries";
import {
  Field,
  SelectField,
  Section,
  StepForm,
  SaveFooter,
  TextValue,
  humanize,
  useStepNavigation,
} from "./onboarding-ui";
const companyTypes = [
  "PRIVATE_LIMITED",
  "PUBLIC_LIMITED",
  "SOLE_PROPRIETORSHIP",
  "PARTNERSHIP",
  "LLC",
  "OTHER",
];
export default function CompanyForm() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const org = data.organization;
  const identity = data.identity;
  const [form, change, clear] = useOnboardingDraft("company", () => ({
    legalName: org?.legalName ?? "",
    tradingName: org?.tradingName ?? "",
    registrationNumber: org?.registrationNumber ?? "",
    countryCode: org?.countryCode ?? "NG",
    organizationType: org?.organizationType ?? "",
    timezone: org?.timezone ?? "Africa/Lagos",
    defaultCurrency: org?.defaultCurrency ?? "NGN",
    ownerName: identity.fullName ?? "",
    version: org?.version,
  }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const inFlight = useRef(false);
  const key = useRef(crypto.randomUUID());
  const go = useStepNavigation("company");
  const set = (name: string, value: string) =>
    change({ ...form, [name]: value });
  async function save(event: FormEvent) {
    event.preventDefault();
    if (inFlight.current || !editable) return;
    inFlight.current = true;
    setBusy(true);
    setError(null);
    try {
      if (org)
        await api.updateOrganization(org.organizationId, form, form.version);
      else await api.createOrganization(form, key.current);
      const saved = await reload();
      if (
        !saved.organization ||
        saved.organization.legalName !== form.legalName.trim() ||
        saved.organization.registrationNumber !==
          form.registrationNumber.trim() ||
        saved.organization.organizationType !== form.organizationType
      )
        throw {
          response: {
            data: {
              error: {
                message:
                  "The saved company details did not match your entries. Keep this page open and retry.",
              },
            },
          },
        };
      clear();
      go();
    } catch (err) {
      setError(err);
    } finally {
      inFlight.current = false;
      setBusy(false);
    }
  }
  return (
    <StepForm
      title="Company details"
      description="Tell us about your legal company. Your registered account contacts are already available."
      onSubmit={save}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      <Section
        title={
          identity.isOrganizationOwner || !org
            ? "Owner account contact"
            : "Your account contact"
        }
      >
        <dl className="grid gap-4 sm:grid-cols-2">
          <TextValue label="Registered email" value={identity.email} />
          <TextValue label="Registered phone" value={identity.phone} />
        </dl>
        <p className="mt-3 text-xs text-ink-500">
          These are your account details. Business contacts can be different and
          are entered in the next step.
        </p>
      </Section>
      <Section title="Legal information">
        <fieldset
          disabled={!editable || busy}
          className="grid gap-4 sm:grid-cols-2"
        >
          <Field
            label="Legal company name"
            name="legalName"
            required
            maxLength={200}
            value={form.legalName}
            onChange={(e) => set("legalName", e.target.value)}
          />
          <Field
            label="Trading name (optional)"
            name="tradingName"
            maxLength={200}
            value={form.tradingName}
            onChange={(e) => set("tradingName", e.target.value)}
          />
          <Field
            label="Registration number"
            name="registrationNumber"
            required
            value={form.registrationNumber}
            onChange={(e) => set("registrationNumber", e.target.value)}
          />
          <SelectField
            label="Company type"
            name="organizationType"
            required
            value={form.organizationType}
            onChange={(e) => set("organizationType", e.target.value)}
          >
            <option value="">Choose company type</option>
            {(data.status?.organizationTypes ?? companyTypes).map((t) => (
              <option key={t} value={t}>
                {humanize(t)}
              </option>
            ))}
          </SelectField>
          <SelectField
            label="Country"
            name="countryCode"
            required
            value={form.countryCode}
            onChange={(e) => set("countryCode", e.target.value)}
          >
            {!COUNTRIES.some((c) => c.code === form.countryCode) && (
              <option value={form.countryCode}>{form.countryCode}</option>
            )}
            {COUNTRIES.map((c) => (
              <option key={c.code} value={c.code}>
                {c.name}
              </option>
            ))}
          </SelectField>
          <Field
            label="Timezone"
            name="timezone"
            required
            value={form.timezone}
            placeholder="Africa/Lagos"
            onChange={(e) => set("timezone", e.target.value)}
            hint="Use an IANA timezone, such as Africa/Lagos or Europe/London."
          />
          <Field
            label="Currency"
            name="defaultCurrency"
            required
            pattern="[A-Z]{3}"
            maxLength={3}
            value={form.defaultCurrency}
            onChange={(e) =>
              set("defaultCurrency", e.target.value.toUpperCase())
            }
          />
          {!org && !identity.fullName && (
            <Field
              label="Your full name"
              name="ownerName"
              required
              autoComplete="name"
              value={form.ownerName}
              onChange={(e) => set("ownerName", e.target.value)}
            />
          )}
        </fieldset>
      </Section>
      <SaveFooter step="company" busy={busy} editable={editable} />
    </StepForm>
  );
}
