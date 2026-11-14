"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Search } from "lucide-react";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { PageHeader, Panel, PrimaryButton, SecondaryButton, humanize } from "@/components/features/OpsUi";
import { CUSTODY_SUBJECTS } from "@/lib/types/operations";
import { CustodyPanel } from "../custody-panel";

// Chain-of-custody lookup: the handovers recorded for any package, shipment,
// order, asset or specialized record, and recording a new one.
export default function PackagesCustodyPage() {
  const router = useRouter();
  const [subjectType, setSubjectType] = useState<string>("PACKAGE");
  const [reference, setReference] = useState("");
  const [subject, setSubject] = useState<{ type: string; ref: string } | null>(null);

  const lookUp = (e: FormEvent) => {
    e.preventDefault();
    if (reference.trim()) setSubject({ type: subjectType, ref: reference.trim() });
  };

  return (
    <div className="space-y-5">
      <PageHeader
        title="Packages and custody"
        description="Look up who has held a package, shipment, order or asset, and record handovers."
        actions={
          <SecondaryButton onClick={() => router.push("/private-ops/shipments")}>
            <ArrowLeft className="h-3.5 w-3.5" /> Shipments
          </SecondaryButton>
        }
      />
      <Panel title="Find an item">
        <form onSubmit={lookUp} className="flex flex-wrap items-end gap-3">
          <div className="min-w-[160px]">
            <Field label="Item type">
              <select value={subjectType} onChange={(e) => setSubjectType(e.target.value)} className={INPUT}>
                {CUSTODY_SUBJECTS.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
              </select>
            </Field>
          </div>
          <div className="min-w-[240px] flex-1">
            <Field label="Reference">
              <input value={reference} onChange={(e) => setReference(e.target.value)} placeholder="pkg_..., shp_..., ord_..." className={INPUT} />
            </Field>
          </div>
          <PrimaryButton type="submit" disabled={reference.trim() === ""}>
            <Search className="h-3.5 w-3.5" /> Show custody
          </PrimaryButton>
          {subject?.type === "PACKAGE" && (
            <SecondaryButton onClick={() => router.push(`/private-ops/packages/${encodeURIComponent(subject.ref)}`)}>Open package</SecondaryButton>
          )}
        </form>
      </Panel>
      {subject && <CustodyPanel key={`${subject.type}:${subject.ref}`} subjectType={subject.type} subjectReference={subject.ref} />}
    </div>
  );
}
