"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Archive, ArrowLeft, Edit3, FileCheck, History, Plus, RefreshCw, Save, Search, Trash2 } from "lucide-react";
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
  Timeline,
  formatMinor,
  humanize,
  toMinor,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { approvalPolicyService } from "@/lib/services/governanceService";
import { staffService } from "@/lib/services/staffService";
import { structureService } from "@/lib/services/structureService";
import type { ApprovalPolicy, ApprovalPolicyStep, ApprovalPolicyVersion } from "@/lib/types/governance";
import type { Role } from "@/lib/types/staff";
import type { Branch } from "@/lib/types/structure";

type FormState = {
  code: string;
  name: string;
  description: string;
  resourceType: string;
  action: string;
  branchId: string;
  threshold: string;
  currency: string;
  steps: ApprovalPolicyStep[];
  requesterMayApprove: boolean;
  separationOfDuties: boolean;
  expiryHours: string;
  reason: string;
};

const EMPTY: FormState = {
  code: "",
  name: "",
  description: "",
  resourceType: "",
  action: "",
  branchId: "",
  threshold: "",
  currency: "NGN",
  steps: [{ quorum: 1, eligibleRoleIds: [] }],
  requesterMayApprove: false,
  separationOfDuties: true,
  expiryHours: "72",
  reason: "",
};

// Approval policies: which actions need approval, by whom (steps, quorum,
// eligible roles), above what threshold, and for how long a request stays
// open. Creating is idempotent; edits and archives carry the version.
export default function ApprovalPoliciesPage() {
  const router = useRouter();
  const [rows, setRows] = useState<ApprovalPolicy[]>([]);
  const [roles, setRoles] = useState<Role[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("ACTIVE");

  const [editing, setEditing] = useState<ApprovalPolicy | "new" | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY);
  const [createKey, setCreateKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [versions, setVersions] = useState<{ policy: ApprovalPolicy; rows: ApprovalPolicyVersion[] } | null>(null);

  const load = useCallback(async (s: string) => {
    try {
      setRows(await approvalPolicyService.listPolicies({ status: s || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Approval policies could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);
  useEffect(() => {
    staffService.roles().then(setRoles).catch(() => setRoles([]));
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const openCreate = () => {
    setEditing("new");
    setForm(EMPTY);
    setFormError(null);
    setNotice(null);
  };

  const openEdit = (p: ApprovalPolicy) => {
    setEditing(p);
    setForm({
      code: p.code,
      name: p.name,
      description: p.description,
      resourceType: p.resourceType,
      action: p.action,
      branchId: p.branchId ?? "",
      threshold: p.thresholdAmountMinor == null ? "" : String(p.thresholdAmountMinor / 100),
      currency: p.thresholdCurrency ?? "NGN",
      steps: p.steps.length ? p.steps.map((s) => ({ ...s, eligibleRoleIds: [...s.eligibleRoleIds] })) : EMPTY.steps,
      requesterMayApprove: p.requesterMayApprove,
      separationOfDuties: p.separationOfDuties,
      expiryHours: String(Math.round(p.expirySeconds / 3600)),
      reason: "",
    });
    setFormError(null);
    setNotice(null);
  };

  const setStep = (index: number, step: ApprovalPolicyStep) => setForm((f) => ({ ...f, steps: f.steps.map((s, i) => (i === index ? step : s)) }));

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    setSaving(true);
    setFormError(null);
    const threshold = toMinor(form.threshold);
    const body = {
      name: form.name.trim(),
      description: form.description.trim(),
      resourceType: form.resourceType.trim(),
      action: form.action.trim(),
      branchId: form.branchId || null,
      ...(threshold == null ? (editing === "new" ? {} : { clearThreshold: true }) : { thresholdAmountMinor: threshold, thresholdCurrency: form.currency.trim().toUpperCase() }),
      steps: form.steps,
      requesterMayApprove: form.requesterMayApprove,
      separationOfDuties: form.separationOfDuties,
      expirySeconds: Math.round(Number(form.expiryHours) * 3600),
      reason: form.reason.trim() || undefined,
    };
    try {
      if (editing === "new") {
        const created = await approvalPolicyService.createPolicy({ ...body, code: form.code.trim() }, createKey);
        setCreateKey(newIdempotencyKey());
        setNotice(`Approval policy ${created.name} created.`);
      } else {
        await approvalPolicyService.updatePolicy(editing.id, body, editing.version);
        setNotice(`Approval policy ${body.name} updated.`);
      }
      setEditing(null);
      await load(status);
    } catch (err) {
      setFormError(describeApiError(err, "The approval policy could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const archive = async (p: ApprovalPolicy) => {
    const reason = window.prompt(`Archive ${p.name}? Give a reason.`);
    if (reason == null) return;
    setError(null);
    setNotice(null);
    try {
      await approvalPolicyService.archivePolicy(p.id, reason, p.version);
      setNotice(`Approval policy ${p.name} archived.`);
      await load(status);
    } catch (e) {
      setError(describeApiError(e, "The approval policy could not be archived."));
    }
  };

  const showVersions = async (p: ApprovalPolicy) => {
    try {
      setVersions({ policy: p, rows: await approvalPolicyService.policyVersions(p.id) });
    } catch (e) {
      setError(describeApiError(e, "Policy versions could not be loaded."));
    }
  };

  const roleName = (id: string) => roles.find((r) => r.id === id)?.name ?? id;
  const q = search.trim().toLowerCase();
  const filtered = rows.filter((p) => q === "" || [p.name, p.code, p.resourceType, p.action, p.description].join(" ").toLowerCase().includes(q));

  if (loading) return <Loading label="Loading approval policies..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Approval Policies"
        description="Controlled actions, approval steps and quorum, separation of duty, expiry and thresholds."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton onClick={openCreate}>
              <Plus className="h-3.5 w-3.5" /> Create Policy
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
          { label: "Active", value: rows.filter((p) => p.status === "ACTIVE").length, tone: "green" },
          { label: "Separation of Duty", value: rows.filter((p) => p.separationOfDuties).length, tone: "primary" },
          { label: "Self-Approval", value: rows.filter((p) => p.requesterMayApprove).length, tone: "amber" },
        ]}
      />

      {editing && (
        <Panel title={editing === "new" ? "New approval policy" : `Edit ${editing.name}`} onClose={() => setEditing(null)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Code" required={editing === "new"}>
                <input required={editing === "new"} disabled={editing !== "new"} value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} placeholder="WALLET_WITHDRAWAL" className={INPUT} />
              </Field>
              <Field label="Name" required>
                <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Wallet withdrawal" className={INPUT} />
              </Field>
              <Field label="Resource type" required>
                <input required value={form.resourceType} onChange={(e) => setForm({ ...form, resourceType: e.target.value })} placeholder="company_wallet" className={INPUT} />
              </Field>
              <Field label="Action" required>
                <input required value={form.action} onChange={(e) => setForm({ ...form, action: e.target.value })} placeholder="withdraw" className={INPUT} />
              </Field>
              <Field label="Branch">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Whole organization</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Request expires after (hours)" required hint="Between 5 minutes and 30 days">
                <input required inputMode="decimal" value={form.expiryHours} onChange={(e) => setForm({ ...form, expiryHours: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Threshold amount" hint="Only requests at or above this need approval; blank for all">
                <input inputMode="decimal" value={form.threshold} onChange={(e) => setForm({ ...form, threshold: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Threshold currency">
                <input maxLength={3} value={form.currency} onChange={(e) => setForm({ ...form, currency: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Description">
                  <input value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
                </Field>
              </div>
              <label className="flex items-center gap-2 text-sm text-ink-700">
                <input type="checkbox" checked={form.separationOfDuties} onChange={(e) => setForm({ ...form, separationOfDuties: e.target.checked })} />
                Separation of duty (one person approves one step)
              </label>
              <label className="flex items-center gap-2 text-sm text-ink-700">
                <input type="checkbox" checked={form.requesterMayApprove} onChange={(e) => setForm({ ...form, requesterMayApprove: e.target.checked })} />
                The requester may approve their own request
              </label>
            </div>

            <div className="space-y-3">
              <div className="text-xs font-medium uppercase tracking-wide text-ink-500">Approval steps (in order)</div>
              {form.steps.map((step, index) => (
                <div key={index} className="space-y-2 rounded-xl border border-line p-3">
                  <div className="flex flex-wrap items-end gap-3">
                    <span className="text-sm font-semibold text-ink-950">Step {index + 1}</span>
                    <div className="w-28">
                      <Field label="Approvals needed">
                        <input inputMode="numeric" value={step.quorum} onChange={(e) => setStep(index, { ...step, quorum: Number(e.target.value) || 0 })} className={INPUT} />
                      </Field>
                    </div>
                    {form.steps.length > 1 && (
                      <button type="button" title="Remove step" onClick={() => setForm({ ...form, steps: form.steps.filter((_, i) => i !== index) })} className="text-ink-400 hover:text-red">
                        <Trash2 className="h-4 w-4" />
                      </button>
                    )}
                  </div>
                  <div className="flex flex-wrap gap-1.5">
                    {roles.map((r) => {
                      const on = step.eligibleRoleIds.includes(r.id);
                      return (
                        <button
                          key={r.id}
                          type="button"
                          onClick={() => setStep(index, { ...step, eligibleRoleIds: on ? step.eligibleRoleIds.filter((x) => x !== r.id) : [...step.eligibleRoleIds, r.id] })}
                          className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${on ? "bg-primary-bg text-primary ring-2 ring-primary/30" : "bg-surface-2 text-ink-600"}`}
                        >
                          {r.name}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ))}
              {form.steps.length < 5 && (
                <SecondaryButton onClick={() => setForm({ ...form, steps: [...form.steps, { quorum: 1, eligibleRoleIds: [] }] })}>
                  <Plus className="h-3.5 w-3.5" /> Add step
                </SecondaryButton>
              )}
            </div>

            {editing !== "new" && (
              <Field label="Reason for the change">
                <input value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} className={INPUT} />
              </Field>
            )}
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save policy"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {versions && (
        <Panel title={`Versions of ${versions.policy.name}`} onClose={() => setVersions(null)}>
          <Timeline entries={versions.rows.map((v) => ({ id: v.id, title: `Version ${v.version} · ${humanize(v.status)}`, detail: v.changeReason || undefined, at: v.createdAt }))} />
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by name, action, code..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All statuses</option>
          <option value="ACTIVE">Active</option>
          <option value="ARCHIVED">Archived</option>
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<FileCheck className="h-8 w-8" />} title="No policies found" description="Create a policy to require approval for a controlled action." />
      ) : (
        <DataTable
          minWidth={1000}
          rows={filtered}
          rowKey={(p) => p.id}
          columns={[
            {
              header: "Policy",
              cell: (p) => (
                <>
                  <div className="font-medium text-ink-950">{p.name}</div>
                  <div className="font-mono text-[10px] text-ink-500">{p.code}</div>
                </>
              ),
            },
            { header: "Action", cell: (p) => `${p.resourceType} · ${p.action}` },
            { header: "Steps", cell: (p) => p.steps.map((s, i) => `${i + 1}: ${s.quorum} of ${s.eligibleRoleIds.map(roleName).join(", ") || "any approver"}`).join(" → ") },
            { header: "Threshold", cell: (p) => (p.thresholdAmountMinor == null ? "—" : formatMinor(p.thresholdAmountMinor, p.thresholdCurrency)) },
            { header: "SOD", cell: (p) => (p.separationOfDuties ? <span className="font-medium text-green">Yes</span> : <span className="text-ink-400">No</span>) },
            { header: "Status", cell: (p) => <StatusPill status={p.status} /> },
            {
              header: "",
              cell: (p) => (
                <div className="flex justify-end gap-2">
                  <button title="Versions" onClick={() => showVersions(p)} className="text-ink-400 hover:text-primary">
                    <History className="h-4 w-4" />
                  </button>
                  {p.status !== "ARCHIVED" && (
                    <>
                      <button title="Edit" onClick={() => openEdit(p)} className="text-ink-400 hover:text-primary">
                        <Edit3 className="h-4 w-4" />
                      </button>
                      <button title="Archive" onClick={() => archive(p)} className="text-ink-400 hover:text-red">
                        <Archive className="h-4 w-4" />
                      </button>
                    </>
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
