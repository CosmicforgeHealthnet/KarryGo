"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Pause, Play, Plus, RefreshCw, Repeat, RotateCcw, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  EmptyBlock,
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
import { crmService } from "@/lib/services/crmService";
import { privateOpsService, recurringOrderService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import {
  ORDER_PRIORITIES,
  RECURRENCE_TYPES,
  RECURRING_STATUSES,
  type RecurringOrderTemplate,
  type ServiceCatalogItem,
} from "@/lib/types/operations";
import type { CustomerView } from "@/lib/types/crm";
import type { Branch } from "@/lib/types/structure";

const localZone = () => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return "";
  }
};

const EMPTY_FORM = {
  branchId: "",
  customerId: "",
  serviceCatalogItemId: "",
  recurrenceType: "WEEKLY",
  rrule: "",
  timezone: "",
  start: "",
  end: "",
  customerReference: "",
  priority: "STANDARD",
  durationMinutes: "",
  specialInstructions: "",
};

type RowCommand = "activate" | "deactivate" | "replay";

// Recurring order templates generate private orders on a schedule.
export default function RecurringOrdersPage() {
  const [rows, setRows] = useState<RecurringOrderTemplate[]>([]);
  const [status, setStatus] = useState("ALL");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [customers, setCustomers] = useState<CustomerView[]>([]);
  const [services, setServices] = useState<ServiceCatalogItem[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  // One key per (template, command) attempt, renewed after success.
  const [rowKeys, setRowKeys] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [rowError, setRowError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string) => {
    setLoading(true);
    try {
      setRows(await recurringOrderService.list({ status: s === "ALL" ? undefined : s }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Recurring orders could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);

  useEffect(() => {
    crmService.listCustomers({ limit: 100 }).then((r) => setCustomers(r.data)).catch(() => setCustomers([]));
    privateOpsService.listServiceCatalog().then((s) => setServices(s.filter((x) => x.status === "ACTIVE"))).catch(() => setServices([]));
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const customerName = (id: string) => {
    const c = customers.find((x) => x.id === id);
    return c ? c.displayName || c.legalName || id : id;
  };
  const serviceName = (id: string) => services.find((x) => x.id === id)?.name ?? id;

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const duration = form.durationMinutes.trim() === "" ? undefined : Number(form.durationMinutes);
      await recurringOrderService.create(
        {
          branchId: form.branchId,
          customerId: form.customerId,
          serviceCatalogItemId: form.serviceCatalogItemId,
          recurrenceType: form.recurrenceType,
          rrule: form.recurrenceType === "CUSTOM_RRULE" ? form.rrule.trim() : undefined,
          timezone: form.timezone.trim(),
          startDate: form.start ? new Date(form.start).toISOString() : undefined,
          endDate: form.end ? new Date(form.end).toISOString() : undefined,
          templateData: {
            customerReference: form.customerReference.trim() || undefined,
            priority: form.priority,
            specialInstructions: form.specialInstructions.trim() || undefined,
            durationMinutes: duration != null && Number.isFinite(duration) ? Math.trunc(duration) : undefined,
          },
        },
        key,
      );
      setKey(newIdempotencyKey());
      setShowCreate(false);
      setNotice("Recurring order created as a draft. Activate it to start generating orders.");
      await load(status);
    } catch (err) {
      setFormError(describeApiError(err, "The recurring order could not be created."));
    } finally {
      setSaving(false);
    }
  };

  const runCommand = async (template: RecurringOrderTemplate, command: RowCommand) => {
    const slot = `${template.ID}:${command}`;
    const idem = rowKeys[slot] ?? newIdempotencyKey();
    setRowKeys((k) => ({ ...k, [slot]: idem }));
    setBusy(slot);
    setRowError(null);
    try {
      if (command === "replay") {
        await recurringOrderService.replay(template.ID, idem);
        setNotice("The failed occurrence was queued again.");
      } else {
        const updated = await recurringOrderService.setActive(template.ID, command === "activate", template.Version, idem);
        setRows((r) => r.map((x) => (x.ID === updated.ID ? updated : x)));
        setNotice(command === "activate" ? "Recurring order resumed." : "Recurring order paused.");
      }
      setRowKeys((k) => {
        const next = { ...k };
        delete next[slot];
        return next;
      });
    } catch (err) {
      setRowError(describeApiError(err, "The recurring order could not be updated."));
    } finally {
      setBusy(null);
    }
  };

  if (loading && rows.length === 0 && !error) return <Loading label="Loading recurring orders..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Recurring orders"
        description="Templates that generate company orders on a schedule."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setForm({ ...EMPTY_FORM, timezone: localZone() });
                setFormError(null);
                setShowCreate(true);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> New recurring order
            </PrimaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />
      <ApiErrorNotice error={rowError} />
      <Notice message={notice} />

      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "Draft", value: rows.filter((t) => t.Status === "DRAFT").length, tone: "amber" },
          { label: "Active", value: rows.filter((t) => t.Status === "ACTIVE").length, tone: "green" },
          { label: "Paused", value: rows.filter((t) => t.Status === "INACTIVE").length },
        ]}
      />

      {showCreate && (
        <Panel title="New recurring order" onClose={() => setShowCreate(false)}>
          <form onSubmit={create} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Branch" required>
                <select required value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Choose a branch</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Customer" required>
                <select required value={form.customerId} onChange={(e) => setForm({ ...form, customerId: e.target.value })} className={INPUT}>
                  <option value="">Choose a customer</option>
                  {customers.map((c) => <option key={c.id} value={c.id}>{c.displayName || c.legalName || c.id}</option>)}
                </select>
              </Field>
              <Field label="Service" required>
                <select required value={form.serviceCatalogItemId} onChange={(e) => setForm({ ...form, serviceCatalogItemId: e.target.value })} className={INPUT}>
                  <option value="">Choose a service</option>
                  {services.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
                </select>
              </Field>
              <Field label="Repeats" required>
                <select value={form.recurrenceType} onChange={(e) => setForm({ ...form, recurrenceType: e.target.value })} className={INPUT}>
                  {RECURRENCE_TYPES.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
                </select>
              </Field>
              {form.recurrenceType === "CUSTOM_RRULE" && (
                <Field label="Rule" required hint="For example FREQ=WEEKLY;INTERVAL=2">
                  <input required value={form.rrule} onChange={(e) => setForm({ ...form, rrule: e.target.value })} className={INPUT} />
                </Field>
              )}
              <Field label="Timezone" required hint="IANA name, for example Africa/Lagos.">
                <input required value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} className={INPUT} />
              </Field>
              <Field label="First run">
                <input type="datetime-local" value={form.start} onChange={(e) => setForm({ ...form, start: e.target.value })} className={INPUT} />
              </Field>
              <Field label="End date">
                <input type="datetime-local" value={form.end} onChange={(e) => setForm({ ...form, end: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Customer reference">
                <input value={form.customerReference} onChange={(e) => setForm({ ...form, customerReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Priority">
                <select value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })} className={INPUT}>
                  {ORDER_PRIORITIES.map((p) => <option key={p} value={p}>{humanize(p)}</option>)}
                </select>
              </Field>
              <Field label="Duration (minutes)">
                <input type="number" min={0} step={1} value={form.durationMinutes} onChange={(e) => setForm({ ...form, durationMinutes: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Special instructions">
                  <textarea rows={3} value={form.specialInstructions} onChange={(e) => setForm({ ...form, specialInstructions: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Creating..." : "Create recurring order"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
          <option value="ALL">All statuses</option>
          {RECURRING_STATUSES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {rows.length === 0 ? (
        <EmptyBlock icon={<Repeat className="h-8 w-8" />} title="No recurring orders" description="Create a template to generate orders on a schedule." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(t) => t.ID}
          minWidth={980}
          columns={[
            { header: "Customer", cell: (t) => <span className="text-ink-950">{customerName(t.CustomerID)}</span> },
            { header: "Service", cell: (t) => serviceName(t.ServiceCatalogItemID) },
            { header: "Schedule", cell: (t) => `${humanize(t.RecurrenceType)} · ${t.NormalizedRRule}` },
            { header: "Next run", cell: (t) => formatWhen(t.NextRunAt) },
            { header: "Last error", cell: (t) => t.LastErrorCode || "—" },
            { header: "Status", cell: (t) => <StatusPill status={t.Status} /> },
            {
              header: "Actions",
              cell: (t) => (
                <div className="flex flex-wrap gap-1.5">
                  {(t.Status === "DRAFT" || t.Status === "INACTIVE") && (
                    <SecondaryButton onClick={() => runCommand(t, "activate")} disabled={busy !== null}>
                      <Play className="h-3 w-3" /> {t.Status === "DRAFT" ? "Activate" : "Resume"}
                    </SecondaryButton>
                  )}
                  {t.Status === "ACTIVE" && (
                    <SecondaryButton onClick={() => runCommand(t, "deactivate")} disabled={busy !== null}>
                      <Pause className="h-3 w-3" /> Pause
                    </SecondaryButton>
                  )}
                  {t.LastErrorCode && (
                    <SecondaryButton onClick={() => runCommand(t, "replay")} disabled={busy !== null} title="Queue the failed occurrence again">
                      <RotateCcw className="h-3 w-3" /> Replay
                    </SecondaryButton>
                  )}
                </div>
              ),
            },
          ]}
        />
      )}
    </div>
  );
}
