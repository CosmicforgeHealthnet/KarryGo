"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Archive, ArrowLeft, Edit3, Layers, Plus, RefreshCw, Save, Search, X } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { STRUCTURE_STATUS_LABEL, structureService } from "@/lib/services/structureService";
import type { Branch, Department } from "@/lib/types/structure";

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  ACTIVE: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  ARCHIVED: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

type FormState = { code: string; name: string; branchId: string; parentId: string; costCentreId: string };
const EMPTY_FORM: FormState = { code: "", name: "", branchId: "", parentId: "", costCentreId: "" };
const orNull = (value: string) => (value.trim() === "" ? null : value.trim());

export default function DepartmentsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<Department[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [includeArchived, setIncludeArchived] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [search, setSearch] = useState("");

  const [editing, setEditing] = useState<Department | "new" | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const [departments, allBranches] = await Promise.all([
        structureService.listDepartments({ includeArchived }),
        structureService.listBranches().catch(() => [] as Branch[]),
      ]);
      setRows(departments);
      setBranches(allBranches);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Departments could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [includeArchived]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const branchName = new Map(branches.map((b) => [b.id, b.name]));
  const departmentName = new Map(rows.map((d) => [d.id, d.name]));

  const openEdit = (department: Department) => {
    setEditing(department);
    setForm({
      code: department.code,
      name: department.name,
      branchId: department.branchId ?? "",
      parentId: department.parentId ?? "",
      costCentreId: department.costCentreId ?? "",
    });
    setFormError(null);
    setNotice(null);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    setSaving(true);
    setFormError(null);
    const body = {
      name: form.name.trim(),
      branchId: orNull(form.branchId),
      parentId: orNull(form.parentId),
      costCentreId: orNull(form.costCentreId),
    };
    try {
      if (editing === "new") {
        await structureService.createDepartment({ ...body, code: form.code.trim() });
        setNotice(`Department ${body.name} created.`);
      } else {
        await structureService.updateDepartment(editing.id, body, editing.version);
        setNotice(`Department ${body.name} updated.`);
      }
      setEditing(null);
      await load();
    } catch (err) {
      setFormError(describeApiError(err, "The department could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const archive = async (department: Department) => {
    if (!window.confirm(`Archive ${department.name}?`)) return;
    setBusyId(department.id);
    setError(null);
    setNotice(null);
    try {
      await structureService.archiveDepartment(department.id, department.version);
      setNotice(`Department ${department.name} archived.`);
      await load();
    } catch (err) {
      setError(describeApiError(err, "The department could not be archived."));
    } finally {
      setBusyId(null);
    }
  };

  const q = search.trim().toLowerCase();
  const filtered = rows.filter((d) => q === "" || [d.code, d.name].join(" ").toLowerCase().includes(q));
  const parentOptions = rows.filter((d) => d.status === "ACTIVE" && (editing === "new" || editing?.id !== d.id));

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading departments...</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Departments</h1>
          <p className="mt-1 text-sm text-ink-600">Teams inside your company, optionally tied to a branch and cost centre.</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => load()}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            Refresh
          </button>
          <button
            onClick={() => {
              setEditing("new");
              setForm(EMPTY_FORM);
              setFormError(null);
              setNotice(null);
            }}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add department
          </button>
          <button
            onClick={() => router.push("/settings")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <ArrowLeft className="h-3.5 w-3.5" />
            Back
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />
      {notice && <div className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{notice}</div>}

      {editing && (
        <form onSubmit={submit} className="space-y-4 rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <h3 className="text-[15px] font-semibold text-ink-950">{editing === "new" ? "New department" : `Edit ${editing.name}`}</h3>
            <button type="button" onClick={() => setEditing(null)} className="text-ink-400 hover:text-ink-700">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Code" required={editing === "new"}>
              <input
                required={editing === "new"}
                disabled={editing !== "new"}
                value={form.code}
                onChange={(e) => setForm({ ...form, code: e.target.value })}
                placeholder="OPS"
                className={INPUT}
              />
            </Field>
            <Field label="Name" required>
              <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Operations" className={INPUT} />
            </Field>
            <Field label="Branch">
              <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                <option value="">Company-wide</option>
                {branches.filter((b) => b.status === "ACTIVE" || b.id === form.branchId).map((b) => (
                  <option key={b.id} value={b.id}>{b.name}</option>
                ))}
              </select>
            </Field>
            <Field label="Parent department">
              <select value={form.parentId} onChange={(e) => setForm({ ...form, parentId: e.target.value })} className={INPUT}>
                <option value="">None</option>
                {parentOptions.map((d) => (
                  <option key={d.id} value={d.id}>{d.name}</option>
                ))}
              </select>
            </Field>
            <Field label="Cost centre ID">
              <input value={form.costCentreId} onChange={(e) => setForm({ ...form, costCentreId: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={saving} className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60">
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save department"}
            </button>
            <button type="button" onClick={() => setEditing(null)} className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-white">
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by code or name..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <label className="flex h-11 items-center gap-2 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm">
          <input type="checkbox" checked={includeArchived} onChange={(e) => setIncludeArchived(e.target.checked)} />
          Show archived
        </label>
      </div>

      {filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Layers className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{rows.length === 0 ? "No departments yet" : "No departments found"}</h3>
          <p className="mt-1 text-sm text-ink-500">{rows.length === 0 ? "Add departments to organise staff and approvals." : "No departments match your search."}</p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  {["Department", "Branch", "Parent", "Cost centre", "Status", ""].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((d) => {
                  const style = STATUS_STYLES[d.status] || STATUS_STYLES.ARCHIVED;
                  return (
                    <tr key={d.id} className="border-b border-line transition-colors hover:bg-surface-2">
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{d.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{d.code}</div>
                      </td>
                      <td className="px-4 py-3 text-xs text-ink-600">{d.branchId ? branchName.get(d.branchId) ?? d.branchId : "Company-wide"}</td>
                      <td className="px-4 py-3 text-xs text-ink-600">{d.parentId ? departmentName.get(d.parentId) ?? d.parentId : "—"}</td>
                      <td className="px-4 py-3 font-mono text-[10px] text-ink-500">{d.costCentreId ?? "—"}</td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
                          {STRUCTURE_STATUS_LABEL[d.status] ?? d.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        {d.status !== "ARCHIVED" && (
                          <div className="flex justify-end gap-2">
                            <button onClick={() => openEdit(d)} title="Edit" className="text-ink-400 transition-colors hover:text-primary">
                              <Edit3 className="h-4 w-4" />
                            </button>
                            <button onClick={() => archive(d)} disabled={busyId === d.id} title="Archive" className="text-ink-400 transition-colors hover:text-red disabled:opacity-50">
                              <Archive className="h-4 w-4" />
                            </button>
                          </div>
                        )}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
