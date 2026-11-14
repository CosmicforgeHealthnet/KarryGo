"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ClipboardList, Plus, RefreshCw, Save, Search } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  EmptyBlock,
  Loading,
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
import { manifestService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import { MANIFEST_STATUSES, type Manifest } from "@/lib/types/operations";
import type { Branch } from "@/lib/types/structure";

const EMPTY_FORM = { branchId: "", routeReference: "", vehicleReference: "", providerMembershipId: "", sealNumber: "" };

// Load manifests: shipments, packages and stops grouped for one vehicle run.
export default function ManifestsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Manifest[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [status, setStatus] = useState("ALL");
  const [branchId, setBranchId] = useState("");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string, b: string) => {
    setLoading(true);
    try {
      setRows(await manifestService.list({ status: s === "ALL" ? undefined : s, branchId: b || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Manifests could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status, branchId));
  }, [load, status, branchId]);

  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const branchName = (id: string) => branches.find((b) => b.id === id)?.name ?? id;

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const manifest = await manifestService.create(
        {
          branchId: form.branchId,
          routeReference: form.routeReference.trim() || undefined,
          vehicleReference: form.vehicleReference.trim() || undefined,
          providerMembershipId: form.providerMembershipId.trim() || undefined,
          sealNumber: form.sealNumber.trim() || undefined,
        },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/private-ops/manifests/${manifest.id}`);
    } catch (err) {
      setFormError(describeApiError(err, "The manifest could not be created."));
    } finally {
      setSaving(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q === ""
      ? rows
      : rows.filter((m) => [m.manifestReference, m.routeReference, m.vehicleReference, m.sealNumber].join(" ").toLowerCase().includes(q));
  }, [rows, search]);

  if (loading && rows.length === 0 && !error) return <Loading label="Loading manifests..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Manifests"
        description="Group shipments, packages and stops for one vehicle run, then close and seal them."
        actions={
          <>
            <SecondaryButton onClick={() => load(status, branchId)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setForm(EMPTY_FORM);
                setFormError(null);
                setShowCreate(true);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> New manifest
            </PrimaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />

      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "Draft or open", value: rows.filter((m) => m.status === "DRAFT" || m.status === "OPEN").length, tone: "amber" },
          { label: "Closed", value: rows.filter((m) => m.status === "CLOSED").length, tone: "green" },
          { label: "Dispatched", value: rows.filter((m) => m.status === "DISPATCHED").length, tone: "primary" },
        ]}
      />

      {showCreate && (
        <Panel title="New manifest" onClose={() => setShowCreate(false)}>
          <form onSubmit={create} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Branch" required>
                <select required value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Choose a branch</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Route reference">
                <input value={form.routeReference} onChange={(e) => setForm({ ...form, routeReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Vehicle reference">
                <input value={form.vehicleReference} onChange={(e) => setForm({ ...form, vehicleReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Provider membership ID">
                <input value={form.providerMembershipId} onChange={(e) => setForm({ ...form, providerMembershipId: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Seal number">
                <input value={form.sealNumber} onChange={(e) => setForm({ ...form, sealNumber: e.target.value })} className={INPUT} />
              </Field>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Creating..." : "Create manifest"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by reference, route, vehicle, seal..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={branchId} onChange={(e) => setBranchId(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Branch">
          <option value="">All branches</option>
          {branches.map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
        </select>
        <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
          <option value="ALL">All statuses</option>
          {MANIFEST_STATUSES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<ClipboardList className="h-8 w-8" />} title={rows.length === 0 ? "No manifests yet" : "No manifests found"} description={rows.length === 0 ? "Create a manifest to group work for a vehicle run." : "No manifests match your search."} />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(m) => m.id}
          onRowClick={(m) => router.push(`/private-ops/manifests/${m.id}`)}
          columns={[
            { header: "Manifest", cell: (m) => <span className="font-mono text-ink-950">{m.manifestReference}</span> },
            { header: "Branch", cell: (m) => branchName(m.branchId) },
            { header: "Route", cell: (m) => m.routeReference || "—" },
            { header: "Vehicle", cell: (m) => m.vehicleReference || "—" },
            { header: "Seal", cell: (m) => m.sealNumber || "—" },
            { header: "Status", cell: (m) => <StatusPill status={m.status} /> },
            { header: "Updated", cell: (m) => formatWhen(m.updatedAt) },
          ]}
        />
      )}
    </div>
  );
}
