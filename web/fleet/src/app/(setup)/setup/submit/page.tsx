"use client";
import { useRef, useState, type FormEvent } from "react";
import { useOnboarding } from "../onboarding-context";
import { organizationService as api } from "@/lib/services/organizationService";
import {
  Section,
  StepForm,
  SaveFooter,
  TextValue,
  humanize,
} from "../onboarding-ui";
import type { BusinessAddress } from "@/lib/services/organizationService";
function Address({ value }: { value?: BusinessAddress }) {
  return (
    <p className="break-words text-sm">
      {value
        ? [
            value.street ?? value.streetAddress ?? value.addressLine1,
            value.city,
            value.state,
            value.postalCode,
            value.country ?? value.countryCode,
          ]
            .filter(Boolean)
            .join(", ")
        : "Not provided"}
    </p>
  );
}
export default function Review() {
  const { data, reload, reloadSaved, editable } = useOnboarding();
  const org = data.organization!;
  const profile = data.profile;
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
      await reload();
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
      {!editable && (
        <Section title="Submission status">
          <p role="status" className="font-semibold">
            {humanize(org.status)}
          </p>
          {data.status?.verification && (
            <p className="mt-2 text-sm">
              Verification: {humanize(data.status.verification.status)}
            </p>
          )}
          <p className="mt-2 text-sm text-ink-600">
            Your information is saved. We will update this status when the
            verification service responds.
          </p>
        </Section>
      )}
      <Section title="Company details" edit={edit("profile")}>
        <dl className="grid gap-4 sm:grid-cols-2">
          {Object.entries({
            "Legal name": org.legalName,
            "Trading name": org.tradingName,
            "Registration number": org.registrationNumber,
            "Company type": org.organizationType
              ? humanize(org.organizationType)
              : "",
            Country: org.countryCode,
            Timezone: org.timezone,
            Currency: org.defaultCurrency,
          }).map(([label, value]) => (
            <TextValue key={label} label={label} value={value} />
          ))}
        </dl>
      </Section>
      <Section title="Owner and business contacts" edit={edit("headquarters")}>
        <dl className="grid gap-4 sm:grid-cols-2">
          <TextValue
            label="Your registered email"
            value={data.identity.email}
          />
          <TextValue
            label="Your registered phone"
            value={data.identity.phone}
          />
          <TextValue label="Business email" value={profile?.businessEmail} />
          <TextValue label="Business phone" value={profile?.businessPhone} />
          <TextValue label="Website" value={profile?.website} />
          <TextValue
            label="Business description"
            value={profile?.businessDescription}
          />
        </dl>
        <p className="mt-3 text-xs text-ink-500">
          Account contacts and company contacts are stored separately.
        </p>
      </Section>
      <Section title="Business addresses" edit={edit("headquarters")}>
        <div className="space-y-4">
          <div>
            <h3 className="mb-1 text-xs text-ink-500">Registered address</h3>
            <Address value={profile?.registeredAddress} />
          </div>
          <div>
            <h3 className="mb-1 text-xs text-ink-500">Operating address</h3>
            <Address value={profile?.operatingAddress} />
          </div>
        </div>
      </Section>
      <Section
        title="Authorized representative"
        edit={edit("authorized-representative")}
      >
        {data.representatives.length ? (
          data.representatives.map((rep) => (
            <dl
              key={rep.id}
              className="mb-4 grid gap-4 border-b border-line pb-4 sm:grid-cols-2"
            >
              <TextValue label="Name" value={rep.fullName} />
              <TextValue label="Position" value={rep.roleTitle} />
              <TextValue label="Email" value={rep.email} />
              <TextValue label="Phone" value={rep.phone} />
              <TextValue
                label="Authority"
                value={humanize(rep.authorityType)}
              />
              <TextValue label="Record status" value={humanize(rep.status)} />
            </dl>
          ))
        ) : (
          <p className="text-sm">Not provided</p>
        )}
      </Section>
      <Section title="Logistics services" edit={edit("capabilities")}>
        <ul className="list-disc space-y-2 pl-5 text-sm">
          {Object.keys(data.capabilities).map((code) => (
            <li key={code}>
              {humanize(code)} · {humanize(data.capabilities[code])}
            </li>
          ))}
        </ul>
        {Object.keys(data.capabilities).length === 0 && (
          <p className="text-sm">No services selected</p>
        )}
      </Section>
      <Section title="Business documents" edit={edit("business-docs")}>
        <ul className="space-y-3 text-sm">
          {data.status!.requiredDocumentTypes.map((type) => (
            <li key={type}>
              <span className="font-semibold">{humanize(type)}</span>:{" "}
              {data.documents
                .filter((d) => d.documentType === type)
                .map((d) => humanize(d.status))
                .join(", ") || "Missing"}
            </li>
          ))}
        </ul>
      </Section>
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
