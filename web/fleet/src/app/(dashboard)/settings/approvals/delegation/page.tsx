"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, Plus, RefreshCw, Save, UserCheck, XCircle } from "lucide-react";
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
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { approvalPolicyService } from "@/lib/services/governanceService";
import { staffService } from "@/lib/services/staffService";
import { structureService } from "@/lib/services/structureService";
import type { ApprovalDelegation } from "@/lib/types/governance";
import type { StaffMember } from "@/lib/types/staff";
import type { Branch } from "@/lib/types/structure";

const EMPTY = { delegateId: "", resourceType: "", action: "", branchId: "", effectiveFrom: "", expiresAt: "", reason: "" };

// Temporary handover of your approval authority to a colleague (holiday,
// absence). A delegation is time-boxed and can be revoked at any time.
export default function ApprovalDelegationPage() {
  const router = useRouter();
  const [rows, setRows] = useState<ApprovalDelegation[]>([]);
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [status, setStatus] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [show, setShow] = useState(false);
  const [form, setForm] = useState(EMPTY);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string) => {
    try {
      setRows(await approvalPolicyService.listDelegations({ status: s || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Delegations could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);
  useEffect(() => {
    staffService.list().then(setStaff).catch(() => setStaff([]));
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      await approvalPolicyService.createDelegation(
        {
          delegateId: form.delegateId,
          resourceType: form.resourceType.trim() || undefined,
          action: form.action.trim() || undefined,
          branchId: form.branchId || undefined,
          effectiveFrom: form.effectiveFrom ? new Date(form.effectiveFrom).toISOString() : undefined,
          expiresAt: new Date(form.expiresAt).toISOString(),
          reason: form.reason.trim(),
        },
        key,
      );
      setKey(newIdempotencyKey());
      setShow(false);
      setForm(EMPTY);
      setNotice("Delegation created.");
      await load(status);
    } catch (err) {
      setFormError(describeApiError(err, "The delegation could not be created."));
    } finally {
      setSaving(false);
    }
  };

  const revoke = async (d: ApprovalDelegation) => {
    const reason = window.prompt("Revoke this delegation? Give a reason.");
    if (reason == null) return;
    setError(null);
    setNotice(null);
    try {
      await approvalPolicyService.revokeDelegation(d.id, reason, d.version);
      setNotice("Delegation revoked.");
      await load(status);
    } catch (e) {
      setError(describeApiError(e, "The delegation could not be revoked."));
    }
  };

  const name = (id: string) => staff.find((s) => s.id === id)?.fullName ?? id;

  if (loading) return <Loading label="Loading delegations..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Approval Delegation"
        description="Hand your approval authority to a colleague for a fixed period."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setShow(true);
                setFormError(null);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> New delegation
            </PrimaryButton>
            <SecondaryButton onClick={() => router.push("/settings/approvals")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Approvals
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      <StatGrid
        items={[
          { label: "Shown", value: rows.length },
          { label: "Active", value: rows.filter((d) => d.status === "ACTIVE").length, tone: "green" },
          { label: "Scheduled", value: rows.filter((d) => d.status === "SCHEDULED").length, tone: "amber" },
          { label: "Revoked", value: rows.filter((d) => d.status === "REVOKED").length, tone: "red" },
        ]}
      />

      {show && (
        <Panel title="New delegation" onClose={() => setShow(false)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Delegate to" required>
                <select required value={form.delegateId} onChange={(e) => setForm({ ...form, delegateId: e.target.value })} className={INPUT}>
                  <option value="">Choose a colleague</option>
                  {staff.filter((s) => s.status === "ACTIVE").map((s) => <option key={s.id} value={s.id}>{s.fullName}</option>)}
                </select>
              </Field>
              <Field label="Branch">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">All my branches</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Resource type" hint="Blank for every controlled action">
                <input value={form.resourceType} onChange={(e) => setForm({ ...form, resourceType: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Action">
                <input value={form.action} onChange={(e) => setForm({ ...form, action: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Starts" hint="Now if blank">
                <input type="datetime-local" value={form.effectiveFrom} onChange={(e) => setForm({ ...form, effectiveFrom: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Ends" required>
                <input type="datetime-local" required value={form.expiresAt} onChange={(e) => setForm({ ...form, expiresAt: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Reason" required>
                  <input required value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} placeholder="Annual leave" className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Delegate"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
        <option value="">All statuses</option>
        {["SCHEDULED", "ACTIVE", "EXPIRED", "REVOKED"].map((s) => <option key={s} value={s}>{s.charAt(0) + s.slice(1).toLowerCase()}</option>)}
      </select>

      {rows.length === 0 ? (
        <EmptyBlock icon={<UserCheck className="h-8 w-8" />} title="No delegations" description="Delegations you give or receive appear here." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(d) => d.id}
          columns={[
            { header: "From", cell: (d) => <span className="text-ink-950">{name(d.delegatorId)}</span> },
            { header: "To", cell: (d) => <span className="text-ink-950">{name(d.delegateId)}</span> },
            { header: "Scope", cell: (d) => [d.resourceType, d.action].filter(Boolean).join(" · ") || "All actions" },
            { header: "Period", cell: (d) => `${formatWhen(d.effectiveFrom)} – ${formatWhen(d.expiresAt)}` },
            { header: "Reason", cell: (d) => d.reason },
            { header: "Status", cell: (d) => <StatusPill status={d.status} /> },
            {
              header: "",
              cell: (d) =>
                d.status === "ACTIVE" || d.status === "SCHEDULED" ? (
                  <button title="Revoke" onClick={() => revoke(d)} className="text-ink-400 hover:text-red">
                    <XCircle className="h-4 w-4" />
                  </button>
                ) : null,
            },
          ]}
        />
      )}
    </div>
  );
}
