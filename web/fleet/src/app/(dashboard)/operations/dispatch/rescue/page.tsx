"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { LifeBuoy, Plus, RefreshCw, Save, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
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
import { rescueService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import { RESCUE_RESOLUTIONS, RESCUE_SEVERITIES, RESCUE_TRIGGERS, type RescueCase } from "@/lib/types/operations";
import type { Branch } from "@/lib/types/structure";

const EMPTY = { branchId: "", triggerType: "BREAKDOWN", severity: "HIGH", reason: "", canonicalTripReference: "", privateAssignmentId: "", assetId: "" };

// Rescue cases: a trip or assignment that needs help (breakdown, delay,
// unsafe vehicle...). A replacement provider is offered like any other
// private assignment and is never forced to accept.
export default function RescueCasesPage() {
  const [rows, setRows] = useState<RescueCase[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [status, setStatus] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState(EMPTY);
  const [openKey, setOpenKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const [selected, setSelected] = useState<RescueCase | null>(null);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);
  const [busy, setBusy] = useState(false);
  const [replacement, setReplacement] = useState({ providerMembershipId: "", vehicleOrTruckReference: "" });
  const [resolution, setResolution] = useState({ code: "REPLACEMENT_COMPLETED", notes: "" });
  const [cancelReason, setCancelReason] = useState("");
  const [keys, setKeys] = useState({ replacement: newIdempotencyKey(), resolve: newIdempotencyKey(), cancel: newIdempotencyKey() });

  const load = useCallback(async (s: string) => {
    try {
      setRows(await rescueService.list({ status: s || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Rescue cases could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);

  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const created = await rescueService.open(
        {
          branchId: form.branchId || undefined,
          triggerType: form.triggerType,
          severity: form.severity,
          reason: form.reason.trim(),
          canonicalTripReference: form.canonicalTripReference.trim() || undefined,
          privateAssignmentId: form.privateAssignmentId.trim() || undefined,
          assetId: form.assetId.trim() || undefined,
        },
        openKey,
      );
      setOpenKey(newIdempotencyKey());
      setShowForm(false);
      setForm(EMPTY);
      setNotice("Rescue case opened.");
      setSelected(created);
      await load(status);
    } catch (err) {
      setFormError(describeApiError(err, "The rescue case could not be opened."));
    } finally {
      setSaving(false);
    }
  };

  const select = async (row: RescueCase) => {
    setActionError(null);
    try {
      setSelected(await rescueService.get(row.id));
    } catch (e) {
      setActionError(describeApiError(e, "The rescue case could not be loaded."));
      setSelected(row);
    }
  };

  const act = async (kind: "replacement" | "resolve" | "cancel", message: string, run: (c: RescueCase) => Promise<RescueCase>) => {
    if (!selected) return;
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const updated = await run(selected);
      setKeys((k) => ({ ...k, [kind]: newIdempotencyKey() }));
      setSelected(updated);
      setNotice(message);
      await load(status);
    } catch (e) {
      setActionError(describeApiError(e, "The rescue case could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Loading label="Loading rescue cases..." />;

  const active = selected && !["RESOLVED", "CANCELLED"].includes(selected.status);

  return (
    <div className="space-y-5">
      <PageHeader
        title="Rescue cases"
        description="Trips and assignments that need intervention, and the replacement offered."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setShowForm(true);
                setFormError(null);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> Open rescue case
            </PrimaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <StatGrid
        items={[
          { label: "Open", value: rows.filter((r) => r.status === "OPEN").length, tone: "red" },
          { label: "Replacement pending", value: rows.filter((r) => r.status === "REPLACEMENT_PENDING").length, tone: "amber" },
          { label: "Replacement accepted", value: rows.filter((r) => r.status === "REPLACEMENT_ACCEPTED").length, tone: "primary" },
          { label: "Resolved", value: rows.filter((r) => r.status === "RESOLVED").length, tone: "green" },
        ]}
      />

      {showForm && (
        <Panel title="Open a rescue case" onClose={() => setShowForm(false)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Trigger" required>
                <select value={form.triggerType} onChange={(e) => setForm({ ...form, triggerType: e.target.value })} className={INPUT}>
                  {RESCUE_TRIGGERS.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
                </select>
              </Field>
              <Field label="Severity" required>
                <select value={form.severity} onChange={(e) => setForm({ ...form, severity: e.target.value })} className={INPUT}>
                  {RESCUE_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
                </select>
              </Field>
              <Field label="Branch">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">From the assignment or trip</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Trip reference">
                <input value={form.canonicalTripReference} onChange={(e) => setForm({ ...form, canonicalTripReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Private assignment">
                <input value={form.privateAssignmentId} onChange={(e) => setForm({ ...form, privateAssignmentId: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Asset">
                <input value={form.assetId} onChange={(e) => setForm({ ...form, assetId: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="What happened" required>
                  <textarea required rows={3} value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Opening..." : "Open case"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {selected && (
        <Panel title={`Rescue case ${selected.id}`} description={selected.reason} onClose={() => setSelected(null)}>
          <ApiErrorNotice error={actionError} />
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={selected.status} />],
              ["Severity", <StatusPill key="v" status={selected.severity} />],
              ["Trigger", humanize(selected.triggerType)],
              ["Trip", selected.canonicalTripReference || "—"],
              ["Current assignment", selected.currentAssignmentId || "—"],
              ["Replacement", selected.replacementMembershipId ? `${selected.replacementMembershipId} (${humanize(selected.replacementAssignmentStatus)})` : "—"],
              ["Attempts", selected.replacementAttempts],
              ["Opened", formatWhen(selected.createdAt)],
              ["Resolution", humanize(selected.resolutionCode)],
            ]}
          />
          {active && (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              <div className="space-y-2 rounded-xl border border-line p-3">
                <div className="text-xs font-semibold text-ink-950">Offer a replacement</div>
                <input value={replacement.providerMembershipId} onChange={(e) => setReplacement({ ...replacement, providerMembershipId: e.target.value })} placeholder="Provider membership ID" aria-label="Replacement provider" className={INPUT} />
                <input value={replacement.vehicleOrTruckReference} onChange={(e) => setReplacement({ ...replacement, vehicleOrTruckReference: e.target.value })} placeholder="Vehicle reference (optional)" aria-label="Replacement vehicle" className={INPUT} />
                <PrimaryButton
                  disabled={busy || replacement.providerMembershipId.trim() === ""}
                  onClick={() =>
                    act("replacement", "Replacement offered to the provider.", (c) =>
                      rescueService.assignReplacement(
                        c.id,
                        { providerMembershipId: replacement.providerMembershipId.trim(), vehicleOrTruckReference: replacement.vehicleOrTruckReference.trim() || undefined },
                        c.version,
                        keys.replacement,
                      ),
                    )
                  }
                >
                  Offer replacement
                </PrimaryButton>
              </div>
              <div className="space-y-2 rounded-xl border border-line p-3">
                <div className="text-xs font-semibold text-ink-950">Resolve</div>
                <select value={resolution.code} onChange={(e) => setResolution({ ...resolution, code: e.target.value })} aria-label="Resolution" className={INPUT}>
                  {RESCUE_RESOLUTIONS.map((r) => <option key={r} value={r}>{humanize(r)}</option>)}
                </select>
                <input value={resolution.notes} onChange={(e) => setResolution({ ...resolution, notes: e.target.value })} placeholder="Notes" aria-label="Resolution notes" className={INPUT} />
                <PrimaryButton disabled={busy} onClick={() => act("resolve", "Rescue case resolved.", (c) => rescueService.resolve(c.id, resolution.code, resolution.notes.trim(), c.version, keys.resolve))}>
                  Resolve case
                </PrimaryButton>
              </div>
              <div className="space-y-2 rounded-xl border border-line p-3">
                <div className="text-xs font-semibold text-ink-950">Cancel</div>
                <input value={cancelReason} onChange={(e) => setCancelReason(e.target.value)} placeholder="Why it is no longer needed" aria-label="Cancellation reason" className={INPUT} />
                <DangerButton disabled={busy || cancelReason.trim() === ""} onClick={() => act("cancel", "Rescue case cancelled.", (c) => rescueService.cancel(c.id, cancelReason.trim(), c.version, keys.cancel))}>
                  <XCircle className="h-3.5 w-3.5" /> Cancel case
                </DangerButton>
              </div>
            </div>
          )}
        </Panel>
      )}

      <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
        <option value="">All statuses</option>
        {["OPEN", "REPLACEMENT_PENDING", "REPLACEMENT_ACCEPTED", "RESOLVED", "CANCELLED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
      </select>

      {rows.length === 0 ? (
        <EmptyBlock icon={<LifeBuoy className="h-8 w-8" />} title="No rescue cases" description="Open a rescue case when a trip or assignment needs intervention." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(r) => r.id}
          onRowClick={select}
          columns={[
            { header: "Case", cell: (r) => <span className="font-mono text-ink-950">{r.id}</span> },
            { header: "Trigger", cell: (r) => humanize(r.triggerType) },
            { header: "Severity", cell: (r) => <StatusPill status={r.severity} /> },
            { header: "Trip", cell: (r) => r.canonicalTripReference || "—" },
            { header: "Status", cell: (r) => <StatusPill status={r.status} /> },
            { header: "Opened", cell: (r) => formatWhen(r.createdAt) },
          ]}
        />
      )}
    </div>
  );
}
