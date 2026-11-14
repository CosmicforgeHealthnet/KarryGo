"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { Plus, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { DataTable, Notice, Panel, PrimaryButton, SecondaryButton, StatusPill, Timeline, formatMinor, formatWhen, humanize, toMinor } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { assetCareService } from "@/lib/services/assetCareService";
import { COST_TYPES, DEFECT_SEVERITIES, METER_TYPES, type AssetHistoryEntry, type CostHistory, type Defect, type MeterReading } from "@/lib/types/assetCare";

const nowLocal = () => {
  const d = new Date();
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
};

// Operating records of one asset: open defects, meter readings, operating
// costs and its status history.
export function AssetCare({ assetId, currency }: { assetId: string; currency?: string }) {
  return (
    <div className="space-y-5">
      <AssetDefects assetId={assetId} />
      <MeterReadings assetId={assetId} />
      <AssetCosts assetId={assetId} currency={currency} />
      <AssetHistory assetId={assetId} />
    </div>
  );
}

function AssetDefects({ assetId }: { assetId: string }) {
  const [rows, setRows] = useState<Defect[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [show, setShow] = useState(false);
  const [form, setForm] = useState({ description: "", severity: "MEDIUM", itemCode: "" });
  const [key, setKey] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await assetCareService.listDefects({ assetId }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Defects could not be loaded."));
    }
  }, [assetId]);
  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await assetCareService.reportDefect({ assetId, description: form.description.trim(), severity: form.severity, itemCode: form.itemCode.trim() || undefined }, key);
      setKey(newIdempotencyKey());
      setShow(false);
      setForm({ description: "", severity: "MEDIUM", itemCode: "" });
      setNotice("Defect reported.");
      await load();
    } catch (err) {
      setError(describeApiError(err, "The defect could not be reported."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Panel
      title="Defects"
      description="Critical defects block the asset until repaired."
      actions={
        <>
          <Link href={`/fleet/defects?assetId=${encodeURIComponent(assetId)}`} className="text-xs font-medium text-primary">Manage</Link>
          {!show && (
            <SecondaryButton onClick={() => setShow(true)}>
              <Plus className="h-3.5 w-3.5" /> Report defect
            </SecondaryButton>
          )}
        </>
      }
    >
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {show && (
        <form onSubmit={submit} className="grid grid-cols-1 gap-3 rounded-xl border border-line p-4 sm:grid-cols-3">
          <div className="sm:col-span-3">
            <Field label="Description" required>
              <input required value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <Field label="Severity">
            <select value={form.severity} onChange={(e) => setForm({ ...form, severity: e.target.value })} className={INPUT}>
              {DEFECT_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
            </select>
          </Field>
          <Field label="Item code">
            <input value={form.itemCode} onChange={(e) => setForm({ ...form, itemCode: e.target.value })} placeholder="brakes" className={INPUT} />
          </Field>
          <div className="flex items-end gap-2">
            <PrimaryButton type="submit" disabled={busy}>
              <Save className="h-3.5 w-3.5" /> Save defect
            </PrimaryButton>
            <SecondaryButton onClick={() => setShow(false)}>Cancel</SecondaryButton>
          </div>
        </form>
      )}
      {rows.length === 0 ? (
        <p className="text-sm text-ink-500">No defects reported for this asset.</p>
      ) : (
        <DataTable
          minWidth={640}
          rows={rows}
          rowKey={(d) => d.id}
          columns={[
            { header: "Defect", cell: (d) => <span className="text-ink-950">{d.description}</span> },
            { header: "Severity", cell: (d) => <StatusPill status={d.severity} /> },
            { header: "Status", cell: (d) => <StatusPill status={d.status} /> },
            { header: "Reported", cell: (d) => formatWhen(d.createdAt) },
          ]}
        />
      )}
    </Panel>
  );
}

function MeterReadings({ assetId }: { assetId: string }) {
  const [rows, setRows] = useState<MeterReading[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [form, setForm] = useState({ meterType: "ODOMETER", reading: "", recordedAt: nowLocal() });
  const [key, setKey] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setRows(await assetCareService.meterReadings(assetId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Meter readings could not be loaded."));
    }
  }, [assetId]);
  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const saved = await assetCareService.recordMeterReading(
        { assetId, meterType: form.meterType, reading: Number(form.reading), recordedAt: new Date(form.recordedAt).toISOString() },
        key,
      );
      setKey(newIdempotencyKey());
      setForm({ ...form, reading: "" });
      setNotice(saved.anomaly ? `Reading saved and flagged: ${(saved.anomalyReasons ?? []).map(humanize).join(", ")}.` : "Reading saved.");
      await load();
    } catch (err) {
      setError(describeApiError(err, "The reading could not be saved."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Panel title="Meter readings" description="Odometer and engine hours drive maintenance schedules.">
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
        <div className="min-w-[140px]">
          <Field label="Meter">
            <select value={form.meterType} onChange={(e) => setForm({ ...form, meterType: e.target.value })} className={INPUT}>
              {METER_TYPES.map((m) => <option key={m} value={m}>{humanize(m)}</option>)}
            </select>
          </Field>
        </div>
        <div className="min-w-[140px]">
          <Field label="Reading" required>
            <input required inputMode="decimal" value={form.reading} onChange={(e) => setForm({ ...form, reading: e.target.value })} className={INPUT} />
          </Field>
        </div>
        <div className="min-w-[180px]">
          <Field label="Recorded at">
            <input type="datetime-local" value={form.recordedAt} onChange={(e) => setForm({ ...form, recordedAt: e.target.value })} className={INPUT} />
          </Field>
        </div>
        <PrimaryButton type="submit" disabled={busy || form.reading.trim() === ""}>
          <Save className="h-3.5 w-3.5" /> Record reading
        </PrimaryButton>
      </form>
      {rows.length > 0 && (
        <DataTable
          minWidth={600}
          rows={rows}
          rowKey={(r) => r.id}
          columns={[
            { header: "When", cell: (r) => formatWhen(r.recordedAt) },
            { header: "Meter", cell: (r) => humanize(r.meterType) },
            { header: "Reading", cell: (r) => r.reading.toLocaleString() },
            { header: "Previous", cell: (r) => r.previousReading.toLocaleString() },
            { header: "Source", cell: (r) => humanize(r.source) },
            { header: "Check", cell: (r) => (r.anomaly ? <StatusPill status="FLAGGED" tone="amber" label="Flagged" /> : "—") },
          ]}
        />
      )}
    </Panel>
  );
}

function AssetCosts({ assetId, currency }: { assetId: string; currency?: string }) {
  const [costs, setCosts] = useState<CostHistory | null>(null);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [show, setShow] = useState(false);
  const [form, setForm] = useState({ costType: "MAINTENANCE", amount: "", currency: currency || "NGN", incurredAt: nowLocal(), description: "" });
  const [key, setKey] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setCosts(await assetCareService.costs(assetId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Operating costs could not be loaded."));
    }
  }, [assetId]);
  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const amountMinor = toMinor(form.amount);
    if (amountMinor == null || amountMinor <= 0) {
      setError({ message: "Enter a positive amount." });
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await assetCareService.recordCost(
        assetId,
        { costType: form.costType, amountMinor, currency: form.currency.trim().toUpperCase(), incurredAt: new Date(form.incurredAt).toISOString(), description: form.description.trim() || undefined },
        key,
      );
      setKey(newIdempotencyKey());
      setShow(false);
      setForm({ ...form, amount: "", description: "" });
      await load();
    } catch (err) {
      setError(describeApiError(err, "The cost could not be recorded."));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Panel
      title="Operating costs"
      description="Reporting only; never a wallet or ledger movement."
      actions={
        !show && (
          <SecondaryButton onClick={() => setShow(true)}>
            <Plus className="h-3.5 w-3.5" /> Record cost
          </SecondaryButton>
        )
      }
    >
      <ApiErrorNotice error={error} />
      {show && (
        <form onSubmit={submit} className="grid grid-cols-1 gap-3 rounded-xl border border-line p-4 sm:grid-cols-3">
          <Field label="Type">
            <select value={form.costType} onChange={(e) => setForm({ ...form, costType: e.target.value })} className={INPUT}>
              {COST_TYPES.map((c) => <option key={c} value={c}>{humanize(c)}</option>)}
            </select>
          </Field>
          <Field label="Amount" required>
            <input required inputMode="decimal" value={form.amount} onChange={(e) => setForm({ ...form, amount: e.target.value })} className={INPUT} />
          </Field>
          <Field label="Currency">
            <input value={form.currency} maxLength={3} onChange={(e) => setForm({ ...form, currency: e.target.value })} className={INPUT} />
          </Field>
          <Field label="Incurred at">
            <input type="datetime-local" value={form.incurredAt} onChange={(e) => setForm({ ...form, incurredAt: e.target.value })} className={INPUT} />
          </Field>
          <Field label="Description">
            <input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
          </Field>
          <div className="flex items-end gap-2">
            <PrimaryButton type="submit" disabled={busy}>
              <Save className="h-3.5 w-3.5" /> Save cost
            </PrimaryButton>
            <SecondaryButton onClick={() => setShow(false)}>Cancel</SecondaryButton>
          </div>
        </form>
      )}
      {costs && costs.totals.length > 0 && (
        <div className="flex flex-wrap gap-2">
          {costs.totals.map((t) => (
            <span key={`${t.costType}-${t.currency}`} className="rounded-full bg-surface-2 px-3 py-1 text-[11px] font-semibold text-ink-700">
              {humanize(t.costType)}: {formatMinor(t.amountMinor, t.currency)}
            </span>
          ))}
        </div>
      )}
      {!costs || costs.entries.length === 0 ? (
        <p className="text-sm text-ink-500">No operating costs recorded.</p>
      ) : (
        <DataTable
          minWidth={600}
          rows={costs.entries}
          rowKey={(c) => c.id}
          columns={[
            { header: "When", cell: (c) => formatWhen(c.incurredAt) },
            { header: "Type", cell: (c) => humanize(c.costType) },
            { header: "Amount", cell: (c) => formatMinor(c.amountMinor, c.currency) },
            { header: "Source", cell: (c) => humanize(c.sourceType) },
            { header: "Description", cell: (c) => c.description || "—" },
          ]}
        />
      )}
    </Panel>
  );
}

function AssetHistory({ assetId }: { assetId: string }) {
  const [rows, setRows] = useState<AssetHistoryEntry[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  useEffect(() => {
    assetCareService
      .history(assetId)
      .then(setRows)
      .catch((e) => setError(describeApiError(e, "Asset history could not be loaded.")));
  }, [assetId]);
  return (
    <Panel title="Status history">
      <ApiErrorNotice error={error} />
      <Timeline
        entries={rows.map((h) => ({
          id: h.id,
          title: `${humanize(h.reason)}: ${humanize(h.previousOperationalStatus)} → ${humanize(h.newOperationalStatus)}`,
          detail: h.previousLifecycleStatus !== h.newLifecycleStatus ? `Lifecycle ${humanize(h.previousLifecycleStatus)} → ${humanize(h.newLifecycleStatus)}` : undefined,
          at: h.occurredAt,
        }))}
      />
    </Panel>
  );
}
