"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Archive, ArrowLeft, Building2, Edit3, Plus, RefreshCw, Save, Search, X } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS, FormField as Field } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import {
  BRANCH_TYPES,
  BRANCH_TYPE_LABEL,
  STRUCTURE_STATUS_LABEL,
  structureService,
} from "@/lib/services/structureService";
import type { Branch } from "@/lib/types/structure";

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  ACTIVE: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  INACTIVE: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  ARCHIVED: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const INPUT = FORM_INPUT_CLASS;

type FormState = {
  code: string;
  name: string;
  type: string;
  address: string;
  latitude: string;
  longitude: string;
  operatingHours: string;
  timezone: string;
  status: string;
};

const EMPTY_FORM: FormState = {
  code: "",
  name: "",
  type: "BRANCH",
  address: "",
  latitude: "",
  longitude: "",
  operatingHours: "",
  timezone: "",
  status: "ACTIVE",
};

const toNumber = (value: string): number | null => {
  const trimmed = value.trim();
  if (trimmed === "") return null;
  const n = Number(trimmed);
  return Number.isFinite(n) ? n : null;
};

export default function BranchesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [rows, setRows] = useState<Branch[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [search, setSearch] = useState("");

  // null: no form; "new": create; otherwise the branch being edited.
  const [editing, setEditing] = useState<Branch | "new" | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const branches = await structureService.listBranches();
      setRows(branches);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Branches could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const openCreate = () => {
    setEditing("new");
    setForm(EMPTY_FORM);
    setFormError(null);
    setNotice(null);
  };

  const openEdit = (branch: Branch) => {
    setEditing(branch);
    setForm({
      code: branch.code,
      name: branch.name,
      type: branch.type,
      address: branch.address ?? "",
      latitude: branch.latitude == null ? "" : String(branch.latitude),
      longitude: branch.longitude == null ? "" : String(branch.longitude),
      operatingHours: branch.operatingHours ?? "",
      timezone: branch.timezone ?? "",
      status: branch.status === "INACTIVE" ? "INACTIVE" : "ACTIVE",
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
      type: form.type,
      address: form.address.trim(),
      latitude: toNumber(form.latitude),
      longitude: toNumber(form.longitude),
      operatingHours: form.operatingHours.trim(),
      timezone: form.timezone.trim(),
    };
    try {
      if (editing === "new") {
        const created = await structureService.createBranch({ ...body, code: form.code.trim() });
        setNotice(`Branch ${created.name} created.`);
      } else {
        await structureService.updateBranch(editing.id, { ...body, status: form.status }, editing.version);
        setNotice(`Branch ${body.name} updated.`);
      }
      setEditing(null);
      await load();
    } catch (err) {
      setFormError(describeApiError(err, "The branch could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const archive = async (branch: Branch) => {
    if (!window.confirm(`Archive ${branch.name}? Archived branches cannot take new work.`)) return;
    setBusyId(branch.id);
    setError(null);
    setNotice(null);
    try {
      await structureService.archiveBranch(branch.id, branch.version);
      setNotice(`Branch ${branch.name} archived.`);
      await load();
    } catch (err) {
      setError(describeApiError(err, "The branch could not be archived."));
    } finally {
      setBusyId(null);
    }
  };

  const q = search.trim().toLowerCase();
  const filtered = rows.filter(
    (b) => q === "" || [b.code, b.name, b.address, b.type].join(" ").toLowerCase().includes(q),
  );
  const active = rows.filter((b) => b.status === "ACTIVE").length;
  const inactive = rows.filter((b) => b.status === "INACTIVE").length;

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading branches...</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Branches and sites</h1>
          <p className="mt-1 text-sm text-ink-600">Branches, hubs, depots and offices your company operates from.</p>
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
            onClick={openCreate}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add branch
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
      {notice && (
        <div className="rounded-xl border border-green/30 bg-green-bg px-4 py-3 text-sm text-green">{notice}</div>
      )}

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-3 gap-2.5">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{rows.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{active}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Inactive</div>
          <div className="text-lg font-bold text-amber">{inactive}</div>
        </div>
      </div>

      {/* ===== FORM ===== */}
      {editing && (
        <form onSubmit={submit} className="space-y-4 rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <h3 className="text-[15px] font-semibold text-ink-950">
              {editing === "new" ? "New branch" : `Edit ${editing.name}`}
            </h3>
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
                placeholder="LAG-IKJ"
                className={INPUT}
              />
            </Field>
            <Field label="Name" required>
              <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Ikeja hub" className={INPUT} />
            </Field>
            <Field label="Type" required>
              <select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value })} className={INPUT}>
                {BRANCH_TYPES.map((t) => (
                  <option key={t} value={t}>{BRANCH_TYPE_LABEL[t]}</option>
                ))}
              </select>
            </Field>
            {editing !== "new" ? (
              <Field label="Status">
                <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })} className={INPUT}>
                  <option value="ACTIVE">Active</option>
                  <option value="INACTIVE">Inactive</option>
                </select>
              </Field>
            ) : (
              <Field label="Timezone">
                <input value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} placeholder="Company timezone if blank" className={INPUT} />
              </Field>
            )}
            <div className="sm:col-span-2">
              <Field label="Address">
                <input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} className={INPUT} />
              </Field>
            </div>
            <Field label="Latitude">
              <input inputMode="decimal" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} placeholder="6.6018" className={INPUT} />
            </Field>
            <Field label="Longitude">
              <input inputMode="decimal" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} placeholder="3.3515" className={INPUT} />
            </Field>
            <Field label="Operating hours">
              <input value={form.operatingHours} onChange={(e) => setForm({ ...form, operatingHours: e.target.value })} placeholder="Mon-Sat 07:00-19:00" className={INPUT} />
            </Field>
            {editing !== "new" && (
              <Field label="Timezone">
                <input value={form.timezone} onChange={(e) => setForm({ ...form, timezone: e.target.value })} placeholder="Africa/Lagos" className={INPUT} />
              </Field>
            )}
          </div>
          <div className="flex flex-wrap gap-3">
            <button
              type="submit"
              disabled={saving}
              className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60"
            >
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save branch"}
            </button>
            <button type="button" onClick={() => setEditing(null)} className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-white">
              Cancel
            </button>
          </div>
        </form>
      )}

      {/* ===== SEARCH ===== */}
      <div className="flex h-11 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
        <Search className="h-4 w-4 text-ink-400" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search by code, name or address..."
          className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
        />
      </div>

      {/* ===== TABLE ===== */}
      {filtered.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Building2 className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">{rows.length === 0 ? "No branches yet" : "No branches found"}</h3>
          <p className="mt-1 text-sm text-ink-500">
            {rows.length === 0 ? "Add the branches and sites your company works from." : "No branches match your search."}
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[860px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-2/70">
                  {["Branch", "Type", "Address", "Hours", "Timezone", "Status", ""].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">{h}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((b) => {
                  const style = STATUS_STYLES[b.status] || STATUS_STYLES.ARCHIVED;
                  return (
                    <tr key={b.id} className="border-b border-line transition-colors hover:bg-surface-2">
                      <td className="px-4 py-3">
                        <div className="text-xs font-medium text-ink-950">{b.name}</div>
                        <div className="font-mono text-[10px] text-ink-500">{b.code}</div>
                      </td>
                      <td className="px-4 py-3 text-xs text-ink-600">{BRANCH_TYPE_LABEL[b.type] ?? b.type}</td>
                      <td className="px-4 py-3 text-xs text-ink-600">{b.address || "—"}</td>
                      <td className="px-4 py-3 text-xs text-ink-600">{b.operatingHours || "—"}</td>
                      <td className="px-4 py-3 text-xs text-ink-600">{b.timezone || "—"}</td>
                      <td className="px-4 py-3">
                        <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                          <span className={`h-1.5 w-1.5 rounded-full ${style.dot}`} />
                          {STRUCTURE_STATUS_LABEL[b.status] ?? b.status}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-right">
                        {b.status !== "ARCHIVED" && (
                          <div className="flex justify-end gap-2">
                            <button onClick={() => openEdit(b)} title="Edit" className="text-ink-400 transition-colors hover:text-primary">
                              <Edit3 className="h-4 w-4" />
                            </button>
                            <button onClick={() => archive(b)} disabled={busyId === b.id} title="Archive" className="text-ink-400 transition-colors hover:text-red disabled:opacity-50">
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

