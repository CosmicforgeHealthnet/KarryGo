"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, ListChecks, Play, RefreshCw } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  EmptyBlock,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { complianceService } from "@/lib/services/safetyService";
import { COMPLIANCE_SUBJECTS, type ComplianceCheck } from "@/lib/types/safety";

// Compliance checks: evaluate one subject against every applicable
// requirement now. A failed blocking requirement raises a restriction.
export default function ComplianceChecksPage() {
  const router = useRouter();
  const [rows, setRows] = useState<ComplianceCheck[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [form, setForm] = useState({ subjectType: "PROVIDER", subjectReference: "" });
  const [key, setKey] = useState(newIdempotencyKey);
  const [running, setRunning] = useState(false);
  const [runError, setRunError] = useState<ApiErrorInfo | null>(null);
  const [selected, setSelected] = useState<ComplianceCheck | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await complianceService.listChecks());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Compliance checks could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const run = async (e: FormEvent) => {
    e.preventDefault();
    setRunning(true);
    setRunError(null);
    setNotice(null);
    try {
      const check = await complianceService.runCheck(
        { SubjectType: form.subjectType, SubjectReference: form.subjectReference.trim(), SourceType: "MANUAL_REVIEW", SourceVersion: 0 },
        key,
      );
      setKey(newIdempotencyKey());
      setSelected(check);
      setNotice(`Check complete: ${humanize(check.OverallState)}.`);
      await load();
    } catch (err) {
      setRunError(describeApiError(err, "The check could not be run."));
    } finally {
      setRunning(false);
    }
  };

  const open = async (c: ComplianceCheck) => {
    try {
      setSelected(await complianceService.getCheck(c.ID));
    } catch (e) {
      setError(describeApiError(e, "The check could not be loaded."));
    }
  };

  if (loading) return <Loading label="Loading compliance checks..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Compliance checks"
        description="Evaluate a provider, asset or other subject against your requirements."
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/compliance")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Compliance
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <Panel title="Run a check">
        <form onSubmit={run} className="flex flex-wrap items-end gap-3">
          <ApiErrorNotice error={runError} className="w-full" />
          <div className="min-w-[160px]">
            <Field label="Subject type">
              <select value={form.subjectType} onChange={(e) => setForm({ ...form, subjectType: e.target.value })} className={INPUT}>
                {COMPLIANCE_SUBJECTS.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
              </select>
            </Field>
          </div>
          <div className="min-w-[240px] flex-1">
            <Field label="Subject reference" required>
              <input required value={form.subjectReference} onChange={(e) => setForm({ ...form, subjectReference: e.target.value })} placeholder="Membership, asset or organization ID" className={INPUT} />
            </Field>
          </div>
          <PrimaryButton type="submit" disabled={running || form.subjectReference.trim() === ""}>
            <Play className="h-3.5 w-3.5" /> {running ? "Checking..." : "Run check"}
          </PrimaryButton>
        </form>
      </Panel>

      {selected && (
        <Panel title={`Check ${selected.ID}`} description={`${humanize(selected.SubjectType)} ${selected.SubjectReference}`} onClose={() => setSelected(null)}>
          <KeyValueGrid
            items={[
              ["Overall", <StatusPill key="s" status={selected.OverallState} />],
              ["Requirements evaluated", selected.RequirementCount],
              ["Restrictions raised", (selected.RestrictionReferences ?? []).length],
              ["Run", formatWhen(selected.CreatedAt)],
            ]}
          />
          {(selected.Results ?? []).length > 0 && (
            <DataTable
              minWidth={600}
              rows={selected.Results ?? []}
              rowKey={(r) => r.requirementId}
              columns={[
                { header: "Requirement", cell: (r) => r.requirementCode },
                { header: "Severity", cell: (r) => <StatusPill status={r.severity} /> },
                { header: "Blocks work", cell: (r) => (r.blocksOperation ? "Yes" : "No") },
                { header: "Result", cell: (r) => <StatusPill status={r.state} /> },
              ]}
            />
          )}
        </Panel>
      )}

      {rows.length === 0 ? (
        <EmptyBlock icon={<ListChecks className="h-8 w-8" />} title="No checks run yet" description="Run a check to see where a subject stands against your requirements." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(c) => c.ID}
          onRowClick={open}
          columns={[
            { header: "Subject", cell: (c) => <><div className="text-ink-950">{humanize(c.SubjectType)}</div><div className="font-mono text-[10px] text-ink-500">{c.SubjectReference}</div></> },
            { header: "Source", cell: (c) => humanize(c.SourceType) },
            { header: "Requirements", cell: (c) => c.RequirementCount },
            { header: "Result", cell: (c) => <StatusPill status={c.OverallState} /> },
            { header: "Run", cell: (c) => formatWhen(c.CreatedAt) },
          ]}
        />
      )}
    </div>
  );
}
