"use client";

import { useState, type FormEvent } from "react";
import { Edit3, KeyRound, Plus, Save, Trash2, UserCog } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { staffService } from "@/lib/services/staffService";
import type { Role, StaffMember } from "@/lib/types/staff";
import type { Branch, Department, RoleBinding, RoleScopeType } from "@/lib/types/structure";

const CARD = "rounded-2xl border border-line bg-white p-5 shadow-sm";
const PRIMARY_BUTTON =
  "flex items-center gap-2 rounded-xl bg-primary px-5 py-2 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60";
const SECONDARY_BUTTON = "rounded-xl border border-line px-5 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-white";
const EDIT_BUTTON =
  "flex h-8 items-center gap-1.5 rounded-xl border border-primary/20 bg-primary-bg px-3 text-[11.5px] font-medium text-primary shadow-sm transition hover:bg-primary/10";

function toggle(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value];
}

// ---------------------------------------------------------------------------
// Name, job title, branches and departments (PATCH /staff/:staffId)
// ---------------------------------------------------------------------------

export function StaffDetailsEditor({
  member,
  branches,
  departments,
  onSaved,
}: {
  member: StaffMember;
  branches: Branch[];
  departments: Department[];
  onSaved: () => Promise<void> | void;
}) {
  const [editing, setEditing] = useState(false);
  const [fullName, setFullName] = useState(member.fullName);
  const [jobTitle, setJobTitle] = useState(member.jobTitle ?? "");
  const [branchIds, setBranchIds] = useState<string[]>(member.branchIds);
  // The directory does not report department membership, so departments are
  // only sent when the user picks them; otherwise the current ones are kept.
  const [departmentIds, setDepartmentIds] = useState<string[]>([]);
  const [departmentsTouched, setDepartmentsTouched] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const branchName = new Map(branches.map((b) => [b.id, b.name]));
  const activeBranches = branches.filter((b) => b.status === "ACTIVE" || member.branchIds.includes(b.id));
  const activeDepartments = departments.filter((d) => d.status === "ACTIVE");

  const start = () => {
    setFullName(member.fullName);
    setJobTitle(member.jobTitle ?? "");
    setBranchIds(member.branchIds);
    setDepartmentIds([]);
    setDepartmentsTouched(false);
    setError(null);
    setNotice(null);
    setEditing(true);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    try {
      await staffService.update(
        member.id,
        {
          fullName: fullName.trim(),
          jobTitle: jobTitle.trim(),
          branchIds,
          ...(departmentsTouched ? { departmentIds } : {}),
        },
        member.version,
      );
      setEditing(false);
      setNotice("Staff details saved.");
      await onSaved();
    } catch (err) {
      setError(describeApiError(err, "The staff details could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={CARD}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2 text-[15px] font-semibold text-ink-950">
          <UserCog className="h-4 w-4 text-primary" />
          Details and branch access
        </h3>
        {!editing && (
          <button onClick={start} className={EDIT_BUTTON}>
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
        )}
      </div>
      {notice && !editing && <div className="mt-3 rounded-xl border border-green/30 bg-green-bg px-4 py-2 text-sm text-green">{notice}</div>}

      {!editing ? (
        <dl className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
          <div className="flex items-center justify-between gap-3 border-b border-line py-2">
            <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">Full name</dt>
            <dd className="text-sm text-ink-950">{member.fullName || "—"}</dd>
          </div>
          <div className="flex items-center justify-between gap-3 border-b border-line py-2">
            <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">Job title</dt>
            <dd className="text-sm text-ink-950">{member.jobTitle || "—"}</dd>
          </div>
          <div className="flex items-start justify-between gap-3 py-2 sm:col-span-2">
            <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">Branches</dt>
            <dd className="text-right text-sm text-ink-950">
              {member.branchIds.length === 0
                ? "Company-wide"
                : member.branchIds.map((id) => branchName.get(id) ?? id).join(", ")}
            </dd>
          </div>
        </dl>
      ) : (
        <form onSubmit={submit} className="mt-4 space-y-4">
          <ApiErrorNotice error={error} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Full name" required>
              <input required value={fullName} onChange={(e) => setFullName(e.target.value)} className={INPUT} />
            </Field>
            <Field label="Job title">
              <input value={jobTitle} onChange={(e) => setJobTitle(e.target.value)} className={INPUT} />
            </Field>
          </div>
          <Field label="Branches" hint="Leave all unticked for company-wide access.">
            {activeBranches.length === 0 ? (
              <p className="text-sm text-ink-500">No branches yet.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {activeBranches.map((b) => (
                  <label key={b.id} className="flex items-center gap-2 rounded-lg border border-line px-3 py-1.5 text-sm text-ink-700">
                    <input type="checkbox" checked={branchIds.includes(b.id)} onChange={() => setBranchIds(toggle(branchIds, b.id))} />
                    {b.name}
                  </label>
                ))}
              </div>
            )}
          </Field>
          <Field
            label="Departments"
            hint={departmentsTouched ? "These departments replace the current ones." : "Unchanged unless you pick departments here."}
          >
            {activeDepartments.length === 0 ? (
              <p className="text-sm text-ink-500">No departments yet.</p>
            ) : (
              <div className="flex flex-wrap gap-2">
                {activeDepartments.map((d) => (
                  <label key={d.id} className="flex items-center gap-2 rounded-lg border border-line px-3 py-1.5 text-sm text-ink-700">
                    <input
                      type="checkbox"
                      checked={departmentIds.includes(d.id)}
                      onChange={() => {
                        setDepartmentsTouched(true);
                        setDepartmentIds(toggle(departmentIds, d.id));
                      }}
                    />
                    {d.name}
                  </label>
                ))}
              </div>
            )}
          </Field>
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={saving} className={PRIMARY_BUTTON}>
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save details"}
            </button>
            <button type="button" onClick={() => setEditing(false)} className={SECONDARY_BUTTON}>
              Cancel
            </button>
          </div>
        </form>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Scoped role bindings (GET /staff/:staffId/role-bindings, PUT .../roles)
// ---------------------------------------------------------------------------

type BindingRow = { roleId: string; scopeType: RoleScopeType; scopeId: string; expiresAt: string };

const toRow = (b: RoleBinding): BindingRow => ({
  roleId: b.roleId,
  scopeType: (b.scopeType as RoleScopeType) || "ORGANIZATION",
  scopeId: b.scopeType === "ORGANIZATION" ? "" : b.scopeId,
  expiresAt: b.expiresAt ? b.expiresAt.slice(0, 10) : "",
});

const SCOPE_LABEL: Record<string, string> = {
  ORGANIZATION: "Company-wide",
  BRANCH: "Branch",
  DEPARTMENT: "Department",
};

export function StaffRoleBindingsEditor({
  staffId,
  bindings,
  roles,
  branches,
  departments,
  onSaved,
}: {
  staffId: string;
  bindings: RoleBinding[];
  roles: Role[];
  branches: Branch[];
  departments: Department[];
  onSaved: () => Promise<void> | void;
}) {
  const [editing, setEditing] = useState(false);
  const [rows, setRows] = useState<BindingRow[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const roleName = new Map(roles.map((r) => [r.id, r.name]));
  const branchName = new Map(branches.map((b) => [b.id, b.name]));
  const departmentName = new Map(departments.map((d) => [d.id, d.name]));
  const assignable = roles.filter((r) => r.status === "ACTIVE");

  const scopeName = (b: { scopeType: string; scopeId: string }) =>
    b.scopeType === "BRANCH"
      ? branchName.get(b.scopeId) ?? b.scopeId
      : b.scopeType === "DEPARTMENT"
        ? departmentName.get(b.scopeId) ?? b.scopeId
        : "All";

  const start = () => {
    setRows(bindings.map(toRow));
    setError(null);
    setNotice(null);
    setEditing(true);
  };

  const update = (index: number, patch: Partial<BindingRow>) =>
    setRows(rows.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  const save = async () => {
    setError(null);
    if (rows.length === 0) {
      setError({ message: "Give this staff member at least one role." });
      return;
    }
    if (rows.some((r) => r.roleId === "" || (r.scopeType !== "ORGANIZATION" && r.scopeId === ""))) {
      setError({ message: "Choose a role and, for branch or department roles, where it applies." });
      return;
    }
    setSaving(true);
    try {
      await staffService.replaceRoles(
        staffId,
        rows.map((r) => ({
          roleId: r.roleId,
          scopeType: r.scopeType,
          scopeId: r.scopeType === "ORGANIZATION" ? "" : r.scopeId,
          // End of the chosen day, UTC.
          expiresAt: r.expiresAt ? new Date(`${r.expiresAt}T23:59:59Z`).toISOString() : null,
        })),
      );
      setEditing(false);
      setNotice("Roles saved.");
      await onSaved();
    } catch (err) {
      // The server refuses changes to your own roles
      // (permission_escalation_forbidden) and explains why.
      setError(describeApiError(err, "The roles could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={CARD}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2 text-[15px] font-semibold text-ink-950">
          <KeyRound className="h-4 w-4 text-primary" />
          Roles and scope
          <span className="text-xs font-normal text-ink-400">{bindings.length}</span>
        </h3>
        {!editing && (
          <button onClick={start} className={EDIT_BUTTON}>
            <Edit3 className="h-3.5 w-3.5" />
            Edit roles
          </button>
        )}
      </div>
      {notice && !editing && <div className="mt-3 rounded-xl border border-green/30 bg-green-bg px-4 py-2 text-sm text-green">{notice}</div>}

      {!editing ? (
        bindings.length === 0 ? (
          <p className="mt-3 text-sm text-ink-500">This staff member holds no roles.</p>
        ) : (
          <div className="mt-3 divide-y divide-line">
            {bindings.map((b) => (
              <div key={b.id} className="flex flex-wrap items-center justify-between gap-2 py-2.5">
                <div>
                  <div className="text-sm font-medium text-ink-950">{roleName.get(b.roleId) ?? b.roleId}</div>
                  <div className="text-xs text-ink-500">
                    {SCOPE_LABEL[b.scopeType] ?? b.scopeType}
                    {b.scopeType !== "ORGANIZATION" ? ` · ${scopeName(b)}` : ""}
                  </div>
                </div>
                <div className="text-xs text-ink-500">
                  {b.expiresAt ? `Expires ${new Date(b.expiresAt).toLocaleDateString()}` : "No expiry"}
                </div>
              </div>
            ))}
          </div>
        )
      ) : (
        <div className="mt-4 space-y-3">
          <ApiErrorNotice error={error} />
          {rows.map((row, index) => (
            <div key={index} className="grid grid-cols-1 gap-2 rounded-xl border border-line p-3 sm:grid-cols-[1.4fr_1fr_1.2fr_1fr_auto] sm:items-end">
              <Field label="Role">
                <select value={row.roleId} onChange={(e) => update(index, { roleId: e.target.value })} className={INPUT}>
                  <option value="">Choose a role</option>
                  {assignable.map((r) => (
                    <option key={r.id} value={r.id}>{r.name}</option>
                  ))}
                </select>
              </Field>
              <Field label="Scope">
                <select
                  value={row.scopeType}
                  onChange={(e) => update(index, { scopeType: e.target.value as RoleScopeType, scopeId: "" })}
                  className={INPUT}
                >
                  <option value="ORGANIZATION">Company-wide</option>
                  <option value="BRANCH">Branch</option>
                  <option value="DEPARTMENT">Department</option>
                </select>
              </Field>
              <Field label="Applies to">
                {row.scopeType === "ORGANIZATION" ? (
                  <input disabled value="All" className={INPUT} />
                ) : (
                  <select value={row.scopeId} onChange={(e) => update(index, { scopeId: e.target.value })} className={INPUT}>
                    <option value="">Choose...</option>
                    {(row.scopeType === "BRANCH"
                      ? branches.filter((b) => b.status === "ACTIVE")
                      : departments.filter((d) => d.status === "ACTIVE")
                    ).map((x) => (
                      <option key={x.id} value={x.id}>{x.name}</option>
                    ))}
                  </select>
                )}
              </Field>
              <Field label="Expires">
                <input type="date" value={row.expiresAt} onChange={(e) => update(index, { expiresAt: e.target.value })} className={INPUT} />
              </Field>
              <button
                type="button"
                title="Remove"
                onClick={() => setRows(rows.filter((_, i) => i !== index))}
                className="mb-2.5 text-ink-400 transition-colors hover:text-red"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={() => setRows([...rows, { roleId: "", scopeType: "ORGANIZATION", scopeId: "", expiresAt: "" }])}
            className="flex items-center gap-1 text-xs font-semibold text-primary hover:text-primary/80"
          >
            <Plus className="h-3.5 w-3.5" />
            Add role
          </button>
          <div className="flex flex-wrap gap-3">
            <button type="button" onClick={save} disabled={saving} className={PRIMARY_BUTTON}>
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save roles"}
            </button>
            <button type="button" onClick={() => setEditing(false)} className={SECONDARY_BUTTON}>
              Cancel
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
