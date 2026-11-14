"use client";

import { useCallback, useEffect, useState, type FormEvent, type ReactNode } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft, CheckCircle2, Edit3, Save, UserCheck } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  Timeline,
  formatWhen,
  humanize,
  toMinor,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { crmService } from "@/lib/services/crmService";
import { leadService } from "@/lib/services/operationsService";
import { LEAD_SOURCES, type Lead, type LeadHistoryEntry } from "@/lib/types/operations";
import type { CustomerView } from "@/lib/types/crm";

const NEXT_STATUSES: Record<string, string[]> = {
  NEW: ["CONTACTED", "QUALIFIED", "DISQUALIFIED"],
  CONTACTED: ["QUALIFIED", "DISQUALIFIED"],
  QUALIFIED: ["CONTACTED", "DISQUALIFIED"],
  DISQUALIFIED: [],
  CONVERTED: [],
};

export default function LeadDetailPage() {
  const params = useParams();
  const leadId = params.leadId as string;

  const [lead, setLead] = useState<Lead | null>(null);
  const [history, setHistory] = useState<LeadHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [editing, setEditing] = useState(false);
  const [edit, setEdit] = useState({ companyName: "", source: "", nextAction: "", notes: "", score: "" });

  const [owner, setOwner] = useState("");
  const [assignKey, setAssignKey] = useState(newIdempotencyKey);
  const [nextStatus, setNextStatus] = useState("");
  const [reasonCode, setReasonCode] = useState("");
  const [statusKey, setStatusKey] = useState(newIdempotencyKey);

  const [customers, setCustomers] = useState<CustomerView[]>([]);
  const [convertMode, setConvertMode] = useState<"new" | "existing">("new");
  const [convert, setConvert] = useState({ customerId: "", legalName: "", withOpportunity: false, title: "", value: "", currency: "NGN" });
  const [convertKey, setConvertKey] = useState(newIdempotencyKey);

  const load = useCallback(async () => {
    try {
      const [l, h] = await Promise.all([leadService.get(leadId), leadService.history(leadId).catch(() => [])]);
      setLead(l);
      setHistory(h);
      setOwner(l.ownerUserId ?? "");
      setNextStatus(NEXT_STATUSES[l.status]?.[0] ?? "");
      setConvert((c) => ({ ...c, legalName: c.legalName || l.companyName, title: c.title || `${l.companyName} opportunity` }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The lead could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [leadId]);

  useEffect(() => {
    Promise.resolve().then(load);
    crmService
      .listCustomers({ limit: 100 })
      .then((r) => setCustomers(r.data))
      .catch(() => setCustomers([]));
  }, [load]);

  const run = async (label: string, action: () => Promise<Lead>, after?: () => void) => {
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const updated = await action();
      setLead(updated);
      after?.();
      setNotice(label);
      leadService.history(leadId).then(setHistory).catch(() => undefined);
    } catch (e) {
      setActionError(describeApiError(e, "The change could not be saved."));
    } finally {
      setBusy(false);
    }
  };

  const saveEdit = (e: FormEvent) => {
    e.preventDefault();
    if (!lead) return;
    const score = edit.score.trim();
    run(
      "Lead updated.",
      () =>
        leadService.update(
          lead.id,
          {
            companyName: edit.companyName.trim(),
            source: edit.source,
            nextAction: edit.nextAction.trim(),
            notes: edit.notes.trim(),
            ...(score === "" ? { clearScore: true } : { score: Number(score) }),
          },
          lead.version,
        ),
      () => setEditing(false),
    );
  };

  const doConvert = async (e: FormEvent) => {
    e.preventDefault();
    if (!lead) return;
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      const amount = toMinor(convert.value);
      const result = await leadService.convert(
        lead.id,
        {
          ...(convertMode === "existing"
            ? { customerId: convert.customerId }
            : { newCustomer: { legalName: convert.legalName.trim(), displayName: convert.legalName.trim(), customerType: "BUSINESS" } }),
          ...(convert.withOpportunity
            ? {
                opportunity: {
                  title: convert.title.trim(),
                  ...(amount != null ? { expectedValue: { amountMinor: amount, currency: convert.currency } } : {}),
                },
              }
            : {}),
        },
        lead.version,
        convertKey,
      );
      setConvertKey(newIdempotencyKey());
      setLead(result.lead);
      setNotice(
        `Lead converted${result.customerCreated ? " into a new customer" : ""}: ${result.customer.displayName || result.customer.legalName}.`,
      );
      leadService.history(leadId).then(setHistory).catch(() => undefined);
    } catch (err) {
      setActionError(describeApiError(err, "The lead could not be converted."));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Loading label="Loading lead..." />;
  if (!lead) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/crm/leads" className="text-sm text-primary">Back to leads</Link>
      </div>
    );
  }

  const contact = lead.contactSnapshot ?? {};
  const open = lead.status !== "CONVERTED" && lead.status !== "DISQUALIFIED";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={lead.companyName}
        description={`Lead from ${humanize(lead.source)} · created ${formatWhen(lead.createdAt)}`}
        actions={
          <>
            {open && !editing && (
              <SecondaryButton
                onClick={() => {
                  setEdit({
                    companyName: lead.companyName,
                    source: lead.source,
                    nextAction: lead.nextAction ?? "",
                    notes: lead.notes ?? "",
                    score: lead.score == null ? "" : String(lead.score),
                  });
                  setEditing(true);
                }}
              >
                <Edit3 className="h-3.5 w-3.5" /> Edit lead
              </SecondaryButton>
            )}
            <Link href="/crm/leads" className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </Link>
          </>
        }
      />

      <ApiErrorNotice error={actionError} />
      <Notice message={notice} />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={lead.status} />],
          ["Owner", lead.ownerUserId ?? "Unassigned"],
          ["Score", lead.score ?? "—"],
          ["Next action", lead.nextAction ?? "—"],
          ["Contact", [contact.name, contact.email, contact.phone].filter(Boolean).join(" · ") || "—"],
          ["Version", lead.version],
          ...(lead.disqualificationReason ? ([["Disqualified because", lead.disqualificationReason]] as [string, string][]) : []),
          ...(lead.convertedCustomerId
            ? ([["Converted customer", <Link key="c" className="text-primary" href={`/crm/customers/${lead.convertedCustomerId}`}>{lead.convertedCustomerId}</Link>]] as [string, ReactNode][])
            : []),
        ]}
      />

      {editing && (
        <Panel title="Edit lead" onClose={() => setEditing(false)}>
          <form onSubmit={saveEdit} className="space-y-4">
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Company name" required>
                <input required value={edit.companyName} onChange={(e) => setEdit({ ...edit, companyName: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Source">
                <select value={edit.source} onChange={(e) => setEdit({ ...edit, source: e.target.value })} className={INPUT}>
                  {LEAD_SOURCES.map((s) => (
                    <option key={s} value={s}>{humanize(s)}</option>
                  ))}
                </select>
              </Field>
              <Field label="Score" hint="Leave blank to clear">
                <input inputMode="numeric" value={edit.score} onChange={(e) => setEdit({ ...edit, score: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Next action">
                <input value={edit.nextAction} onChange={(e) => setEdit({ ...edit, nextAction: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Notes">
                  <textarea rows={3} value={edit.notes} onChange={(e) => setEdit({ ...edit, notes: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={busy}>
              <Save className="h-3.5 w-3.5" /> Save lead
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {open && (
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
          <Panel title="Owner" description="The staff member responsible for the lead.">
            <div className="flex flex-wrap items-end gap-2">
              <div className="min-w-[200px] flex-1">
                <Field label="Owner user ID">
                  <input value={owner} onChange={(e) => setOwner(e.target.value)} placeholder="Organization user ID" className={INPUT} />
                </Field>
              </div>
              <PrimaryButton
                disabled={busy || owner.trim() === ""}
                onClick={() => run("Lead assigned.", () => leadService.assign(lead.id, owner.trim(), lead.version, assignKey), () => setAssignKey(newIdempotencyKey()))}
              >
                <UserCheck className="h-3.5 w-3.5" /> Assign
              </PrimaryButton>
            </div>
          </Panel>

          <Panel title="Status">
            <div className="flex flex-wrap items-end gap-2">
              <div className="min-w-[140px] flex-1">
                <Field label="Move to">
                  <select value={nextStatus} onChange={(e) => setNextStatus(e.target.value)} className={INPUT}>
                    {(NEXT_STATUSES[lead.status] ?? []).map((s) => (
                      <option key={s} value={s}>{humanize(s)}</option>
                    ))}
                  </select>
                </Field>
              </div>
              <div className="min-w-[140px] flex-1">
                <Field label="Reason code">
                  <input value={reasonCode} onChange={(e) => setReasonCode(e.target.value)} placeholder={nextStatus === "DISQUALIFIED" ? "Required" : "Optional"} className={INPUT} />
                </Field>
              </div>
              <PrimaryButton
                disabled={busy || nextStatus === ""}
                onClick={() =>
                  run(`Lead marked ${humanize(nextStatus).toLowerCase()}.`, () => leadService.changeStatus(lead.id, nextStatus, reasonCode.trim(), lead.version, statusKey), () => {
                    setStatusKey(newIdempotencyKey());
                    setReasonCode("");
                  })
                }
              >
                Update status
              </PrimaryButton>
            </div>
          </Panel>
        </div>
      )}

      {open && (
        <Panel title="Convert lead" description="Turn the lead into a CRM customer, optionally opening an opportunity.">
          <form onSubmit={doConvert} className="space-y-4">
            <div className="flex gap-2">
              {(["new", "existing"] as const).map((m) => (
                <button
                  key={m}
                  type="button"
                  onClick={() => setConvertMode(m)}
                  className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${convertMode === m ? "bg-primary-bg text-primary ring-2 ring-primary/30" : "bg-surface-2 text-ink-600"}`}
                >
                  {m === "new" ? "New customer" : "Existing customer"}
                </button>
              ))}
            </div>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {convertMode === "new" ? (
                <Field label="Customer legal name" required>
                  <input required value={convert.legalName} onChange={(e) => setConvert({ ...convert, legalName: e.target.value })} className={INPUT} />
                </Field>
              ) : (
                <Field label="Customer" required>
                  <select required value={convert.customerId} onChange={(e) => setConvert({ ...convert, customerId: e.target.value })} className={INPUT}>
                    <option value="">Choose a customer</option>
                    {customers.map((c) => (
                      <option key={c.id} value={c.id}>{c.displayName || c.legalName}</option>
                    ))}
                  </select>
                </Field>
              )}
              <label className="flex items-center gap-2 self-end pb-2 text-sm text-ink-700">
                <input type="checkbox" checked={convert.withOpportunity} onChange={(e) => setConvert({ ...convert, withOpportunity: e.target.checked })} />
                Also open an opportunity
              </label>
              {convert.withOpportunity && (
                <>
                  <Field label="Opportunity title" required>
                    <input required value={convert.title} onChange={(e) => setConvert({ ...convert, title: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label={`Expected value (${convert.currency})`}>
                    <input inputMode="decimal" value={convert.value} onChange={(e) => setConvert({ ...convert, value: e.target.value })} className={INPUT} />
                  </Field>
                </>
              )}
            </div>
            <PrimaryButton type="submit" disabled={busy}>
              <CheckCircle2 className="h-3.5 w-3.5" /> Convert lead
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <Panel title="History">
        <Timeline
          entries={history.map((h) => ({
            id: h.id,
            title: `${humanize(h.changeType)}${h.previousStatus ? ` · ${humanize(h.previousStatus)} → ${humanize(h.newStatus)}` : ` · ${humanize(h.newStatus)}`}`,
            detail: [h.reasonCode, humanize(h.actorType)].filter(Boolean).join(" · "),
            at: h.occurredAt,
          }))}
        />
      </Panel>
    </div>
  );
}
