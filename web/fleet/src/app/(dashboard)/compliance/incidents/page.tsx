"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, ArrowLeft, Plus, RefreshCw, Save, Search } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  EmptyBlock,
  Loading,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { incidentService } from "@/lib/services/safetyService";
import { INCIDENT_SEVERITIES, type Incident } from "@/lib/types/safety";

const INCIDENT_TYPES = ["ROAD_ACCIDENT", "INJURY", "CARGO_DAMAGE", "THEFT", "SECURITY", "VEHICLE_FAILURE", "ENVIRONMENTAL", "OTHER"];

const nowLocal = () => {
  const d = new Date();
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
};

const EMPTY = { incidentType: "ROAD_ACCIDENT", severity: "MEDIUM", description: "", occurredAt: nowLocal(), locationReference: "", tripReference: "" };

// Incidents: reported, triaged, contained, investigated, resolved, closed.
export default function IncidentsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Incident[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");

  const [show, setShow] = useState(false);
  const [form, setForm] = useState(EMPTY);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await incidentService.list());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Incidents could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const created = await incidentService.create(
        {
          incidentType: form.incidentType,
          severity: form.severity,
          description: form.description.trim(),
          occurredAt: new Date(form.occurredAt).toISOString(),
          locationReference: form.locationReference.trim() || undefined,
          tripReference: form.tripReference.trim() || undefined,
        },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/compliance/incidents/${created.id}`);
    } catch (err) {
      setFormError(describeApiError(err, "The incident could not be reported."));
    } finally {
      setSaving(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter(
      (i) => (status === "" || i.status === status) && (q === "" || [i.id, i.incidentType, i.description, i.locationReference, i.tripReference].join(" ").toLowerCase().includes(q)),
    );
  }, [rows, search, status]);

  if (loading) return <Loading label="Loading incidents..." />;

  const open = rows.filter((i) => i.status !== "CLOSED" && i.status !== "RESOLVED");

  return (
    <div className="space-y-5">
      <PageHeader
        title="Incidents"
        description="Accidents, injuries, cargo damage and security events, through investigation to closure."
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setShow(true);
                setFormError(null);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> Report incident
            </PrimaryButton>
            <SecondaryButton onClick={() => router.push("/compliance")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Compliance
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <StatGrid
        items={[
          { label: "Open", value: open.length, tone: "amber" },
          { label: "Critical open", value: open.filter((i) => i.severity === "CRITICAL").length, tone: "red" },
          { label: "Investigating", value: rows.filter((i) => i.status === "INVESTIGATING").length, tone: "primary" },
          { label: "Closed", value: rows.filter((i) => i.status === "CLOSED").length, tone: "green" },
        ]}
      />

      {show && (
        <Panel title="Report an incident" onClose={() => setShow(false)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Type" required>
                <select value={form.incidentType} onChange={(e) => setForm({ ...form, incidentType: e.target.value })} className={INPUT}>
                  {INCIDENT_TYPES.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
                </select>
              </Field>
              <Field label="Severity" required>
                <select value={form.severity} onChange={(e) => setForm({ ...form, severity: e.target.value })} className={INPUT}>
                  {INCIDENT_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
                </select>
              </Field>
              <Field label="Occurred at" required>
                <input type="datetime-local" required value={form.occurredAt} onChange={(e) => setForm({ ...form, occurredAt: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Location">
                <input value={form.locationReference} onChange={(e) => setForm({ ...form, locationReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Trip reference">
                <input value={form.tripReference} onChange={(e) => setForm({ ...form, tripReference: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="What happened" required>
                  <textarea required rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Report incident"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search incidents..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All statuses</option>
          {["OPEN", "TRIAGED", "CONTAINED", "ESCALATED", "INVESTIGATING", "CORRECTIVE_ACTION_REQUIRED", "RESOLVED", "CLOSED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<AlertTriangle className="h-8 w-8" />} title={rows.length === 0 ? "No incidents" : "No incidents found"} description={rows.length === 0 ? "Reported incidents appear here." : "No incidents match your filters."} />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(i) => i.id}
          onRowClick={(i) => router.push(`/compliance/incidents/${i.id}`)}
          columns={[
            { header: "Incident", cell: (i) => <span className="font-mono text-ink-950">{i.id}</span> },
            { header: "Type", cell: (i) => humanize(i.incidentType) },
            { header: "Severity", cell: (i) => <StatusPill status={i.severity} /> },
            { header: "Description", cell: (i) => <span className="line-clamp-1">{i.description}</span> },
            { header: "Status", cell: (i) => <StatusPill status={i.status} /> },
            { header: "Occurred", cell: (i) => formatWhen(i.occurredAt) },
          ]}
        />
      )}
    </div>
  );
}
