"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Plus, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { DataTable, Notice, Panel, PrimaryButton, SecondaryButton, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { CustodyEvent } from "@/lib/types/operations";

const ACTOR_TYPES = ["ORGANIZATION", "PROVIDER", "CUSTOMER", "WAREHOUSE", "THIRD_PARTY"];
const CONDITIONS = ["GOOD", "DAMAGED", "PARTIAL", "MISSING"];

const nowLocal = () => {
  const d = new Date();
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
};

// The chain of custody of one subject (package, shipment, order, asset or
// specialized record): who held it, from when, and in what condition.
// Recording a handover is idempotent.
export function CustodyPanel({ subjectType, subjectReference }: { subjectType: string; subjectReference: string }) {
  const [events, setEvents] = useState<CustodyEvent[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [selected, setSelected] = useState<CustodyEvent | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [form, setForm] = useState({
    fromActorType: "ORGANIZATION",
    fromActorReference: "",
    toActorType: "PROVIDER",
    toActorReference: "",
    toLocationReference: "",
    condition: "GOOD",
    occurredAt: nowLocal(),
  });
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setEvents(await privateOpsService.listCustodyEvents(subjectType, subjectReference));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The chain of custody could not be loaded."));
    }
  }, [subjectType, subjectReference]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const open = async (id: string) => {
    try {
      setSelected(await privateOpsService.getCustodyEvent(id));
    } catch (e) {
      setError(describeApiError(e, "The custody event could not be loaded."));
    }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      await privateOpsService.recordCustodyEvent(
        {
          subjectType,
          subjectReference,
          fromActorType: form.fromActorType,
          fromActorReference: form.fromActorReference.trim(),
          toActorType: form.toActorType,
          toActorReference: form.toActorReference.trim(),
          toLocationReference: form.toLocationReference.trim() || undefined,
          condition: form.condition,
          occurredAt: new Date(form.occurredAt).toISOString(),
        },
        key,
      );
      setKey(newIdempotencyKey());
      setShowForm(false);
      setNotice("Custody handover recorded.");
      await load();
    } catch (err) {
      setFormError(describeApiError(err, "The handover could not be recorded."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Panel
      title="Chain of custody"
      description="Every handover recorded for this item, newest last."
      actions={
        !showForm && (
          <SecondaryButton onClick={() => setShowForm(true)}>
            <Plus className="h-3.5 w-3.5" /> Record handover
          </SecondaryButton>
        )
      }
    >
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {showForm && (
        <form onSubmit={submit} className="space-y-4 rounded-xl border border-line p-4">
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="From (type)" required>
              <select value={form.fromActorType} onChange={(e) => setForm({ ...form, fromActorType: e.target.value })} className={INPUT}>
                {ACTOR_TYPES.map((a) => <option key={a} value={a}>{humanize(a)}</option>)}
              </select>
            </Field>
            <Field label="From (reference)">
              <input value={form.fromActorReference} onChange={(e) => setForm({ ...form, fromActorReference: e.target.value })} className={INPUT} />
            </Field>
            <Field label="To (type)" required>
              <select value={form.toActorType} onChange={(e) => setForm({ ...form, toActorType: e.target.value })} className={INPUT}>
                {ACTOR_TYPES.map((a) => <option key={a} value={a}>{humanize(a)}</option>)}
              </select>
            </Field>
            <Field label="To (reference)" required>
              <input required value={form.toActorReference} onChange={(e) => setForm({ ...form, toActorReference: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Location">
              <input value={form.toLocationReference} onChange={(e) => setForm({ ...form, toLocationReference: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Condition">
              <select value={form.condition} onChange={(e) => setForm({ ...form, condition: e.target.value })} className={INPUT}>
                {CONDITIONS.map((c) => <option key={c} value={c}>{humanize(c)}</option>)}
              </select>
            </Field>
            <Field label="Handed over at" required>
              <input type="datetime-local" required value={form.occurredAt} onChange={(e) => setForm({ ...form, occurredAt: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <div className="flex gap-2">
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save handover"}
            </PrimaryButton>
            <SecondaryButton onClick={() => setShowForm(false)}>Cancel</SecondaryButton>
          </div>
        </form>
      )}
      {events.length === 0 ? (
        <p className="text-sm text-ink-500">No custody handovers recorded yet.</p>
      ) : (
        <DataTable
          minWidth={720}
          rows={events}
          rowKey={(e) => e.id}
          onRowClick={(e) => open(e.id)}
          columns={[
            { header: "When", cell: (e) => formatWhen(e.occurredAt) },
            { header: "From", cell: (e) => `${humanize(e.fromActorType)} ${e.fromActorReference}` },
            { header: "To", cell: (e) => `${humanize(e.toActorType)} ${e.toActorReference}` },
            { header: "Condition", cell: (e) => humanize(e.condition) },
            { header: "Source", cell: (e) => humanize(e.source) },
          ]}
        />
      )}
      {selected && (
        <div className="rounded-xl border border-line bg-surface-2/40 p-3 text-xs text-ink-700">
          <div className="font-mono text-ink-950">{selected.id}</div>
          <div>Received {formatWhen(selected.receivedAt)}{selected.toLocationReference ? ` · at ${selected.toLocationReference}` : ""}</div>
          {selected.correctionOfEventId && <div>Corrects {selected.correctionOfEventId}</div>}
          {(selected.evidenceMediaIds ?? []).length > 0 && <div>Evidence: {(selected.evidenceMediaIds ?? []).join(", ")}</div>}
        </div>
      )}
    </Panel>
  );
}
