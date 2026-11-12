"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding, useOnboardingDraft } from "../onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import {
  Field,
  Section,
  StepForm,
  SaveFooter,
  useStepNavigation,
} from "../onboarding-ui";
import { AddressGroup, normalizeAddress } from "../address-group";
export default function Headquarters() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const org = data.organization!;
  const profile = data.profile;
  const [form, change, clear] = useOnboardingDraft("address", () => {
    const registeredAddress = normalizeAddress(
      profile?.registeredAddress,
      org.countryCode,
    );
    const operatingAddress = normalizeAddress(
      profile?.operatingAddress,
      org.countryCode,
    );
    return {
      registeredAddress,
      operatingAddress,
      same:
        !profile ||
        JSON.stringify(registeredAddress) === JSON.stringify(operatingAddress),
      businessEmail:
        profile?.businessEmail ??
        (data.identity.isOrganizationOwner ? data.identity.email : ""),
      businessPhone:
        profile?.businessPhone ??
        (data.identity.isOrganizationOwner ? data.identity.phone : ""),
      website: profile?.website ?? "",
      businessDescription: profile?.businessDescription ?? "",
      version: profile?.version,
    };
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const guard = useRef(false);
  const go = useStepNavigation("address");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (guard.current || !editable) return;
    guard.current = true;
    setBusy(true);
    setError(null);
    try {
      await api.updateProfile(
        org.organizationId,
        {
          registeredAddress: form.registeredAddress,
          operatingAddress: form.same
            ? form.registeredAddress
            : form.operatingAddress,
          businessEmail: form.businessEmail,
          businessPhone: form.businessPhone,
          website: form.website,
          businessDescription: form.businessDescription,
        },
        form.version,
      );
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
      title="Business address"
      description="Save your company's registered address, operating address and business contacts."
      onSubmit={save}
      error={error}
      onReload={() => {
        void reloadSaved().catch(setError);
      }}
    >
      <fieldset disabled={!editable || busy} className="space-y-6">
        <Section title="Registered Address">
          <AddressGroup
            prefix="registered"
            value={form.registeredAddress}
            onChange={(value) => change({ ...form, registeredAddress: value })}
          />
          <div className="mt-6 border-t border-line pt-5">
            <p className="mb-3 text-[13px] font-semibold text-ink-900">Operating address</p>
            <label className="flex items-center gap-3 text-sm">
            <input
              type="checkbox"
              checked={form.same}
              onChange={(e) => change({ ...form, same: e.target.checked })}
            />
            Same as registered business address
          </label>
          {!form.same && (
            <div className="mt-4">
              <AddressGroup
                prefix="operating"
                value={form.operatingAddress}
                onChange={(value) =>
                  change({ ...form, operatingAddress: value })
                }
              />
            </div>
          )}
          </div>
        </Section>
        <Section title="Contact Information">
          {!profile?.businessEmail && !profile?.businessPhone && (
            <p className="mb-4 text-sm text-ink-600">
              We&apos;ve filled this using your registered details. You can
              change it if your business uses different contact information.
            </p>
          )}
          <div className="grid gap-4 sm:grid-cols-2">
            <Field
              name="businessEmail"
              label="Business email"
              type="email"
              required
              value={form.businessEmail}
              onChange={(e) =>
                change({ ...form, businessEmail: e.target.value })
              }
            />
            <Field
              name="businessPhone"
              label="Business phone"
              type="tel"
              required
              value={form.businessPhone}
              placeholder="+2348012345678"
              hint="Include the country code. This changes only the business contact."
              onChange={(e) =>
                change({ ...form, businessPhone: e.target.value })
              }
            />
            <Field
              name="website"
              label="Website (optional)"
              type="url"
              value={form.website}
              placeholder="https://example.com"
              onChange={(e) => change({ ...form, website: e.target.value })}
            />
            <Field
              name="businessDescription"
              label="Business description (optional)"
              maxLength={2000}
              value={form.businessDescription}
              onChange={(e) =>
                change({ ...form, businessDescription: e.target.value })
              }
            />
          </div>
        </Section>
      </fieldset>
      <SaveFooter step="address" busy={busy} editable={editable} />
    </StepForm>
  );
}
