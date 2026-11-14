"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { AlertTriangle, ArrowLeft, Plus, RefreshCw, Save } from "lucide-react";
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
  StatGrid,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { assetCareService } from "@/lib/services/assetCareService";
import { DEFECT_SEVERITIES, type Defect } from "@/lib/types/assetCare";

const EMPTY = { assetId: "", description: "", severity: "MEDIUM", itemCode: "" };

// Asset defects across the fleet: report, defer (with approval permission),
// start repair and resolve. Every transition carries the defect version.
export default function DefectsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Defect[]>([]);
  const [filter, setFilter] = useState({ assetId: "", status: "", severity: "" });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [show, setShow] = useState(false);
  const [form, setForm] = useState(EMPTY);
  const [reportKey, setReportKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const [selected, setSelected] = useState<Defect | null>(null);
  const [action, setAction] = useState({ reason: "", notes: "", deferredUntil: "", workOrderId: "" });
  const [keys, setKeys] = useState({ defer: newIdempotencyKey(), "start-repair": newIdempotencyKey(), resolve: newIdempotencyKey() });
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (f: typeof filter) => {
    try {
      setRows(await assetCareService.listDefects({ assetId: f.assetId || undefined, status: f.status || undefined, severity: f.severity || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Defects could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const assetId = new URLSearchParams(window.location.search).get("assetId") ?? "";
    Promise.resolve().then(() => {
      if (assetId) {
        setFilter((f) => ({ ...f, assetId }));
        setForm((f) => ({ ...f, assetId }));
      }
      return load({ assetId, status: "", severity: "" });
    });
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const d = await assetCareService.reportDefect(
        { assetId: form.assetId.trim(), description: form.description.trim(), severity: form.severity, itemCode: form.itemCode.trim() || undefined },
        reportKey,
      );
      setReportKey(newIdempotencyKey());
      setShow(false);
      setForm({ ...EMPTY, assetId: filter.assetId });
      setNotice(d.critical ? "Critical defect reported. The asset is blocked until it is repaired." : "Defect reported.");
      setSelected(d);
      await load(filter);
    } catch (err) {
      setFormError(describeApiError(err, "The defect could not be reported."));
    } finally {
      setSaving(false);
    }
  };

  const transition = async (kind: "defer" | "start-repair" | "resolve", message: string) => {
    if (!selected) return;
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const updated = await assetCareService.defectAction(
        selected.id,
        kind,
        {
          reason: action.reason.trim() || undefined,
          notes: action.notes.trim() || undefined,
          deferredUntil: kind === "defer" && action.deferredUntil ? new Date(action.deferredUntil).toISOString() : undefined,
          workOrderId: kind === "start-repair" && action.workOrderId.trim() ? action.workOrderId.trim() : undefined,
        },
        selected.version,
        keys[kind],
      );
      setKeys((k) => ({ ...k, [kind]: newIdempotencyKey() }));
      setSelected(updated);
      setAction({ reason: "", notes: "", deferredUntil: "", workOrderId: "" });
      setNotice(message);
      await load(filter);
    } catch (e) {
      setActionError(describeApiError(e, "The defect could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Loading label="Loading defects..." />;

  const open = selected && selected.status !== "RESOLVED";

  return (
    <div className="space-y-5">
      <PageHeader
        title="Defects"
        description="Faults found on vehicles and equipment, from report to repair."
        actions={
          <>
            <SecondaryButton onClick={() => load(filter)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setShow(true);
                setFormError(null);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> Report defect
            </PrimaryButton>
            <SecondaryButton onClick={() => router.push("/fleet/assets")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Assets
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <StatGrid
        items={[
          { label: "Open", value: rows.filter((d) => d.status === "OPEN").length, tone: "amber" },
          { label: "Critical", value: rows.filter((d) => d.critical && d.status !== "RESOLVED").length, tone: "red" },
          { label: "In repair", value: rows.filter((d) => d.status === "REPAIRING").length, tone: "primary" },
          { label: "Resolved", value: rows.filter((d) => d.status === "RESOLVED").length, tone: "green" },
        ]}
      />

      {show && (
        <Panel title="Report a defect" onClose={() => setShow(false)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Asset ID" required>
                <input required value={form.assetId} onChange={(e) => setForm({ ...form, assetId: e.target.value })} placeholder="ast_..." className={INPUT} />
              </Field>
              <Field label="Severity" required>
                <select value={form.severity} onChange={(e) => setForm({ ...form, severity: e.target.value })} className={INPUT}>
                  {DEFECT_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
                </select>
              </Field>
              <Field label="Item code">
                <input value={form.itemCode} onChange={(e) => setForm({ ...form, itemCode: e.target.value })} placeholder="brakes" className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Description" required>
                  <textarea required rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save defect"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {selected && (
        <Panel title={selected.description} description={`Defect ${selected.id}`} onClose={() => setSelected(null)}>
          <ApiErrorNotice error={actionError} />
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={selected.status} />],
              ["Severity", <StatusPill key="v" status={selected.severity} />],
              ["Asset", <Link key="a" className="text-primary" href={`/fleet/assets/${selected.assetId}`}>{selected.assetId}</Link>],
              ["Work order", selected.workOrderId || "—"],
              ["Deferred until", formatWhen(selected.deferredUntil)],
              ["Reported", formatWhen(selected.createdAt)],
              ...(selected.resolutionNotes ? ([["Resolution", selected.resolutionNotes]] as [string, string][]) : []),
            ]}
          />
          {open && (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <Field label="Reason">
                <input value={action.reason} onChange={(e) => setAction({ ...action, reason: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Notes">
                <input value={action.notes} onChange={(e) => setAction({ ...action, notes: e.target.value })} className={INPUT} />
              </Field>
              {selected.status === "OPEN" && (
                <Field label="Defer until">
                  <input type="datetime-local" value={action.deferredUntil} onChange={(e) => setAction({ ...action, deferredUntil: e.target.value })} className={INPUT} />
                </Field>
              )}
              {selected.status !== "REPAIRING" && (
                <Field label="Work order">
                  <input value={action.workOrderId} onChange={(e) => setAction({ ...action, workOrderId: e.target.value })} placeholder="Optional" className={INPUT} />
                </Field>
              )}
              <div className="flex flex-wrap gap-2 sm:col-span-2">
                {selected.status === "OPEN" && !selected.critical && (
                  <SecondaryButton disabled={busy} onClick={() => transition("defer", "Defect deferred.")}>Defer</SecondaryButton>
                )}
                {selected.status !== "REPAIRING" && (
                  <SecondaryButton disabled={busy} onClick={() => transition("start-repair", "Repair started.")}>Start repair</SecondaryButton>
                )}
                <PrimaryButton disabled={busy} onClick={() => transition("resolve", "Defect resolved.")}>Resolve defect</PrimaryButton>
              </div>
            </div>
          )}
        </Panel>
      )}

      <div className="flex flex-wrap gap-2">
        <input
          value={filter.assetId}
          onChange={(e) => setFilter({ ...filter, assetId: e.target.value })}
          onBlur={() => load(filter)}
          placeholder="Filter by asset ID"
          aria-label="Asset filter"
          className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm shadow-sm"
        />
        <select value={filter.status} onChange={(e) => { const f = { ...filter, status: e.target.value }; setFilter(f); load(f); }} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All statuses</option>
          {["OPEN", "DEFERRED", "REPAIRING", "RESOLVED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
        <select value={filter.severity} onChange={(e) => { const f = { ...filter, severity: e.target.value }; setFilter(f); load(f); }} aria-label="Severity" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All severities</option>
          {DEFECT_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {rows.length === 0 ? (
        <EmptyBlock icon={<AlertTriangle className="h-8 w-8" />} title="No defects" description="Defects reported on assets, by inspections or by providers appear here." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(d) => d.id}
          onRowClick={(d) => {
            setSelected(d);
            setActionError(null);
          }}
          columns={[
            { header: "Defect", cell: (d) => <span className="text-ink-950">{d.description}</span> },
            { header: "Asset", cell: (d) => <span className="font-mono">{d.assetId}</span> },
            { header: "Severity", cell: (d) => <StatusPill status={d.severity} /> },
            { header: "Source", cell: (d) => humanize(d.sourceType) },
            { header: "Status", cell: (d) => <StatusPill status={d.status} /> },
            { header: "Reported", cell: (d) => formatWhen(d.createdAt) },
          ]}
        />
      )}
    </div>
  );
}
