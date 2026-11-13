"use client";
// The saved company information, shown on the setup review step and, read
// only, on the dashboard's company page while the company is verified.
import { useOnboarding } from "./onboarding-context";
import { Section, TextValue, humanize } from "./onboarding-ui";
import { useCallback, useState } from "react";
import { DocumentPreview } from "./document-preview";
import type { BusinessAddress, Document } from "@/lib/services/organizationService";
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
export function CompanyReview({
  edit,
}: {
  // Link to edit a section, or undefined when the company cannot be edited.
  edit: (path: string) => string | undefined;
}) {
  const { data, editable } = useOnboarding();
  const org = data.organization!;
  const [previewing, setPreviewing] = useState<Document | null>(null);
  const closePreview = useCallback(() => setPreviewing(null), []);
  const profile = data.profile;
  return (
    <>
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
              {/* Read-only view (dashboard): saved documents stay viewable. */}
              {!editable &&
                data.documents
                  .filter((d) => d.documentType === type && d.status === "READY")
                  .map((d) => (
                    <button
                      key={d.id}
                      type="button"
                      data-document-preview
                      className="btn btn-secondary ml-3"
                      onClick={() => setPreviewing(d)}
                    >
                      Preview
                      <span className="sr-only"> {humanize(type)}</span>
                    </button>
                  ))}
            </li>
          ))}
        </ul>
      </Section>
      {previewing && org && (
        <DocumentPreview
          organizationId={org.organizationId}
          document={previewing}
          onClose={closePreview}
        />
      )}
    </>
  );
}
