"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Archive, ArrowLeft, Edit3, Layers, Plus, RefreshCw, Save, Search } from "lucide-react";
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
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { structureUnitService } from "@/lib/services/governanceService";
import { staffService } from "@/lib/services/staffService";
import { structureService } from "@/lib/services/structureService";
import { LOCATION_TYPES, type CostCentre, type OperatingLocation, type Team } from "@/lib/types/governance";
import type { StaffMember } from "@/lib/types/staff";
import type { Branch, Department } from "@/lib/types/structure";

export type UnitKind = "cost-centres" | "teams" | "locations";
type Unit = CostCentre | Team | OperatingLocation;

const COPY: Record<UnitKind, { title: string; description: string; singular: string; empty: string }> = {
  "cost-centres": {
    title: "Cost centres",
    description: "Accounting units that costs and budgets are attributed to.",
    singular: "cost centre",
    empty: "Add the cost centres your finance team reports against.",
  },
  teams: {
    title: "Teams",
    description: "Working teams inside branches and departments, with their supervisor.",
    singular: "team",
    empty: "Add the teams your staff and providers work in.",
  },
  locations: {
    title: "Operating locations",
    description: "Depots, yards, loading bays and other sites your operations use.",
    singular: "location",
    empty: "Add the sites your vehicles and providers operate from.",
  },
};

type FormState = {
  code: string;
  name: string;
  branchId: string;
  budgetOwnerUserId: string;
  departmentId: string;
  supervisorUserId: string;
  locationType: string;
  address: string;
  latitude: string;
  longitude: string;
  status: string;
};

const EMPTY: FormState = {
  code: "",
  name: "",
  branchId: "",
  budgetOwnerUserId: "",
  departmentId: "",
  supervisorUserId: "",
  locationType: "DEPOT",
  address: "",
  latitude: "",
  longitude: "",
  status: "ACTIVE",
};

const toNumber = (value: string): number | null => {
  const n = value.trim() === "" ? NaN : Number(value);
  return Number.isFinite(n) ? n : null;
};

// One screen for the three organization structure units that share an API
// shape: list, create, edit (If-Match) and archive (If-Match).
export function StructureUnitsScreen({ kind }: { kind: UnitKind }) {
  const router = useRouter();
  const copy = COPY[kind];
  const [rows, setRows] = useState<Unit[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [departments, setDepartments] = useState<Department[]>([]);
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const [includeArchived, setIncludeArchived] = useState(false);

  const [editing, setEditing] = useState<Unit | "new" | null>(null);
  const [form, setForm] = useState<FormState>(EMPTY);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(
    async (archived: boolean) => {
      try {
        setRows(await structureUnitService.list(kind, { includeArchived: archived }));
        setError(null);
      } catch (e) {
        setError(describeApiError(e, `${copy.title} could not be loaded.`));
      } finally {
        setLoading(false);
      }
    },
    [kind, copy.title],
  );

  useEffect(() => {
    Promise.resolve().then(() => load(includeArchived));
  }, [load, includeArchived]);

  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
    if (kind === "teams") structureService.listDepartments().then(setDepartments).catch(() => setDepartments([]));
    if (kind !== "locations") staffService.list().then(setStaff).catch(() => setStaff([]));
  }, [kind]);

  const openEdit = (u: Unit) => {
    const loc = u as OperatingLocation;
    const address = loc.address && typeof loc.address === "object" ? String((loc.address as Record<string, unknown>).line1 ?? "") : "";
    setEditing(u);
    setForm({
      ...EMPTY,
      code: u.code,
      name: u.name,
      branchId: u.branchId ?? "",
      budgetOwnerUserId: (u as CostCentre).budgetOwnerUserId ?? "",
      departmentId: (u as Team).departmentId ?? "",
      supervisorUserId: (u as Team).supervisorUserId ?? "",
      locationType: loc.locationType ?? "DEPOT",
      address,
      latitude: loc.latitude == null ? "" : String(loc.latitude),
      longitude: loc.longitude == null ? "" : String(loc.longitude),
      status: u.status === "INACTIVE" ? "INACTIVE" : "ACTIVE",
    });
    setFormError(null);
    setNotice(null);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!editing) return;
    setSaving(true);
    setFormError(null);
    const body: Record<string, unknown> = { name: form.name.trim(), branchId: form.branchId || null };
    if (kind === "cost-centres") body.budgetOwnerUserId = form.budgetOwnerUserId || null;
    if (kind === "teams") {
      body.departmentId = form.departmentId || null;
      body.supervisorUserId = form.supervisorUserId || null;
    }
    if (kind === "locations") {
      body.locationType = form.locationType;
      body.address = form.address.trim() ? { line1: form.address.trim() } : {};
      body.latitude = toNumber(form.latitude);
      body.longitude = toNumber(form.longitude);
      if (editing !== "new") body.status = form.status;
    }
    try {
      if (editing === "new") {
        await structureUnitService.create(kind, { ...body, code: form.code.trim() });
        setNotice(`${humanize(copy.singular)} ${form.name.trim()} created.`);
      } else {
        await structureUnitService.update(kind, editing.id, body, editing.version);
        setNotice(`${humanize(copy.singular)} ${form.name.trim()} updated.`);
      }
      setEditing(null);
      await load(includeArchived);
    } catch (err) {
      setFormError(describeApiError(err, `The ${copy.singular} could not be saved.`));
    } finally {
      setSaving(false);
    }
  };

  const archive = async (u: Unit) => {
    if (!window.confirm(`Archive ${u.name}?`)) return;
    setError(null);
    setNotice(null);
    try {
      await structureUnitService.archive(kind, u.id, u.version);
      setNotice(`${humanize(copy.singular)} ${u.name} archived.`);
      await load(includeArchived);
    } catch (e) {
      setError(describeApiError(e, `The ${copy.singular} could not be archived.`));
    }
  };

  const branchName = (id: string | null) => (id ? (branches.find((b) => b.id === id)?.name ?? id) : "Organization-wide");
  const staffName = (id: string | null) => (id ? (staff.find((s) => s.id === id)?.fullName ?? id) : "—");
  const q = search.trim().toLowerCase();
  const filtered = rows.filter((u) => q === "" || [u.code, u.name].join(" ").toLowerCase().includes(q));

  if (loading) return <Loading label={`Loading ${copy.title.toLowerCase()}...`} />;

  const extraColumn =
    kind === "cost-centres"
      ? { header: "Budget owner", cell: (u: Unit) => staffName((u as CostCentre).budgetOwnerUserId) }
      : kind === "teams"
        ? { header: "Supervisor", cell: (u: Unit) => staffName((u as Team).supervisorUserId) }
        : { header: "Type", cell: (u: Unit) => humanize((u as OperatingLocation).locationType) };

  return (
    <div className="space-y-5">
      <PageHeader
        title={copy.title}
        description={copy.description}
        actions={
          <>
            <SecondaryButton onClick={() => load(includeArchived)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setEditing("new");
                setForm(EMPTY);
                setFormError(null);
                setNotice(null);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> Add {copy.singular}
            </PrimaryButton>
            <SecondaryButton onClick={() => router.push("/settings")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      <StatGrid
        items={[
          { label: "Total", value: rows.length },
          { label: "Active", value: rows.filter((u) => u.status === "ACTIVE").length, tone: "green" },
          { label: "Archived", value: rows.filter((u) => u.status === "ARCHIVED").length },
        ]}
      />

      {editing && (
        <Panel title={editing === "new" ? `New ${copy.singular}` : `Edit ${editing.name}`} onClose={() => setEditing(null)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Code" required={editing === "new"}>
                <input required={editing === "new"} disabled={editing !== "new"} value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} placeholder="OPS-01" className={INPUT} />
              </Field>
              <Field label="Name" required>
                <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Branch">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Organization-wide</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              {kind === "cost-centres" && (
                <Field label="Budget owner">
                  <select value={form.budgetOwnerUserId} onChange={(e) => setForm({ ...form, budgetOwnerUserId: e.target.value })} className={INPUT}>
                    <option value="">None</option>
                    {staff.map((s) => <option key={s.id} value={s.id}>{s.fullName}</option>)}
                  </select>
                </Field>
              )}
              {kind === "teams" && (
                <>
                  <Field label="Department">
                    <select value={form.departmentId} onChange={(e) => setForm({ ...form, departmentId: e.target.value })} className={INPUT}>
                      <option value="">None</option>
                      {departments.map((d) => <option key={d.id} value={d.id}>{d.name}</option>)}
                    </select>
                  </Field>
                  <Field label="Supervisor">
                    <select value={form.supervisorUserId} onChange={(e) => setForm({ ...form, supervisorUserId: e.target.value })} className={INPUT}>
                      <option value="">None</option>
                      {staff.map((s) => <option key={s.id} value={s.id}>{s.fullName}</option>)}
                    </select>
                  </Field>
                </>
              )}
              {kind === "locations" && (
                <>
                  <Field label="Location type" required>
                    <select value={form.locationType} onChange={(e) => setForm({ ...form, locationType: e.target.value })} className={INPUT}>
                      {LOCATION_TYPES.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
                    </select>
                  </Field>
                  {editing !== "new" && (
                    <Field label="Status">
                      <select value={form.status} onChange={(e) => setForm({ ...form, status: e.target.value })} className={INPUT}>
                        <option value="ACTIVE">Active</option>
                        <option value="INACTIVE">Inactive</option>
                      </select>
                    </Field>
                  )}
                  <div className="sm:col-span-2">
                    <Field label="Address">
                      <input value={form.address} onChange={(e) => setForm({ ...form, address: e.target.value })} className={INPUT} />
                    </Field>
                  </div>
                  <Field label="Latitude">
                    <input inputMode="decimal" value={form.latitude} onChange={(e) => setForm({ ...form, latitude: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Longitude">
                    <input inputMode="decimal" value={form.longitude} onChange={(e) => setForm({ ...form, longitude: e.target.value })} className={INPUT} />
                  </Field>
                </>
              )}
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : `Save ${copy.singular}`}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by code or name..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <label className="flex items-center gap-2 text-sm text-ink-600">
          <input type="checkbox" checked={includeArchived} onChange={(e) => setIncludeArchived(e.target.checked)} /> Show archived
        </label>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<Layers className="h-8 w-8" />} title={rows.length === 0 ? `No ${copy.title.toLowerCase()} yet` : "Nothing found"} description={rows.length === 0 ? copy.empty : "Nothing matches your search."} />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(u) => u.id}
          columns={[
            {
              header: "Name",
              cell: (u) => (
                <>
                  <div className="font-medium text-ink-950">{u.name}</div>
                  <div className="font-mono text-[10px] text-ink-500">{u.code}</div>
                </>
              ),
            },
            { header: "Branch", cell: (u) => branchName(u.branchId) },
            extraColumn,
            { header: "Status", cell: (u) => <StatusPill status={u.status} /> },
            {
              header: "",
              cell: (u) =>
                u.status !== "ARCHIVED" ? (
                  <div className="flex justify-end gap-2">
                    <button title="Edit" onClick={() => openEdit(u)} className="text-ink-400 hover:text-primary">
                      <Edit3 className="h-4 w-4" />
                    </button>
                    <button title="Archive" onClick={() => archive(u)} className="text-ink-400 hover:text-red">
                      <Archive className="h-4 w-4" />
                    </button>
                  </div>
                ) : null,
            },
          ]}
        />
      )}
    </div>
  );
}
