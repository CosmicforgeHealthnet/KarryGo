"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { CircleDot, Plus, RefreshCw, Save, Search } from "lucide-react";
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
import { assetCareService } from "@/lib/services/assetCareService";
import { structureService } from "@/lib/services/structureService";
import type { Tyre } from "@/lib/types/assetCare";
import type { Branch } from "@/lib/types/structure";

const EMPTY = { branchId: "", serialNumber: "", brand: "", size: "", minTreadDepthMm: "", treadDepthMm: "" };
const num = (v: string) => (v.trim() === "" ? undefined : Number(v));

// Tyre register: stock, fitted positions and tread depth. A reading below
// the tyre's minimum opens a defect on the asset.
export default function TyresPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Tyre[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [status, setStatus] = useState("");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [show, setShow] = useState(false);
  const [form, setForm] = useState(EMPTY);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string) => {
    try {
      setRows(await assetCareService.listTyres({ status: s || undefined }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Tyres could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);
  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const tyre = await assetCareService.registerTyre(
        {
          branchId: form.branchId || undefined,
          serialNumber: form.serialNumber.trim(),
          brand: form.brand.trim() || undefined,
          size: form.size.trim() || undefined,
          minTreadDepthMm: num(form.minTreadDepthMm),
          treadDepthMm: num(form.treadDepthMm),
        },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/fleet/tyres/${tyre.id}`);
    } catch (err) {
      setFormError(describeApiError(err, "The tyre could not be registered."));
    } finally {
      setSaving(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q === "" ? rows : rows.filter((t) => [t.serialNumber, t.brand ?? "", t.size ?? "", t.assetId ?? "", t.position ?? ""].join(" ").toLowerCase().includes(q));
  }, [rows, search]);

  if (loading) return <Loading label="Loading tyres..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Tyres"
        description="Tyre stock, fitted positions and tread depth."
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
              <Plus className="h-3.5 w-3.5" /> Register tyre
            </PrimaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <StatGrid
        items={[
          { label: "Total", value: rows.length },
          { label: "In stock", value: rows.filter((t) => t.status === "IN_STOCK").length, tone: "green" },
          { label: "Installed", value: rows.filter((t) => t.status === "INSTALLED").length, tone: "primary" },
          {
            label: "Below minimum tread",
            value: rows.filter((t) => t.lastTreadDepthMm != null && t.lastTreadDepthMm < t.minTreadDepthMm).length,
            tone: "red",
          },
        ]}
      />

      {show && (
        <Panel title="Register a tyre" onClose={() => setShow(false)}>
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Serial number" required>
                <input required value={form.serialNumber} onChange={(e) => setForm({ ...form, serialNumber: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Brand">
                <input value={form.brand} onChange={(e) => setForm({ ...form, brand: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Size">
                <input value={form.size} onChange={(e) => setForm({ ...form, size: e.target.value })} placeholder="295/80R22.5" className={INPUT} />
              </Field>
              <Field label="Branch">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Choose a branch</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Minimum tread (mm)">
                <input inputMode="decimal" value={form.minTreadDepthMm} onChange={(e) => setForm({ ...form, minTreadDepthMm: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Current tread (mm)">
                <input inputMode="decimal" value={form.treadDepthMm} onChange={(e) => setForm({ ...form, treadDepthMm: e.target.value })} className={INPUT} />
              </Field>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Register tyre"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by serial, brand, asset..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm">
          <option value="">All statuses</option>
          {["IN_STOCK", "INSTALLED", "REMOVED", "SCRAPPED"].map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<CircleDot className="h-8 w-8" />} title={rows.length === 0 ? "No tyres registered" : "No tyres found"} description="Register tyres to track fitting, rotation and tread wear." />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(t) => t.id}
          onRowClick={(t) => router.push(`/fleet/tyres/${t.id}`)}
          columns={[
            { header: "Serial", cell: (t) => <span className="font-mono text-ink-950">{t.serialNumber}</span> },
            { header: "Brand / size", cell: (t) => [t.brand, t.size].filter(Boolean).join(" · ") || "—" },
            { header: "Asset", cell: (t) => (t.assetId ? `${t.assetId} · ${t.position ?? ""}` : "—") },
            { header: "Tread", cell: (t) => (t.lastTreadDepthMm != null ? `${t.lastTreadDepthMm} mm (min ${t.minTreadDepthMm})` : "—") },
            { header: "Status", cell: (t) => <StatusPill status={t.status} /> },
            { header: "Updated", cell: (t) => formatWhen(t.updatedAt) },
          ]}
        />
      )}
    </div>
  );
}
