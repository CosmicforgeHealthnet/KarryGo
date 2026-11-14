"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Layers, Plus, RefreshCw, Save, Search } from "lucide-react";
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
import { serviceCatalogService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import { SERVICE_CATEGORIES, SERVICE_STATUSES, type ServiceCatalogItem } from "@/lib/types/operations";
import type { Branch } from "@/lib/types/structure";

const splitList = (value: string) => value.split(",").map((v) => v.trim()).filter(Boolean);

const EMPTY_FORM = { name: "", code: "", category: "PACKAGE_DELIVERY", branchId: "", description: "", vehicleClasses: "", packageTypes: "" };

const filterParams = (s: string, c: string) => ({
  status: s === "ALL" ? undefined : s,
  category: c === "ALL" ? undefined : c,
  limit: 50,
});

// Service catalogue: the services the company offers on its own orders.
export default function ServiceCataloguePage() {
  const router = useRouter();
  const [rows, setRows] = useState<ServiceCatalogItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [status, setStatus] = useState("ALL");
  const [category, setCategory] = useState("ALL");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [branches, setBranches] = useState<Branch[]>([]);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string, c: string) => {
    setLoading(true);
    try {
      const result = await serviceCatalogService.list(filterParams(s, c));
      setRows(result.items);
      setNextCursor(result.nextCursor);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Services could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status, category));
  }, [load, status, category]);

  useEffect(() => {
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, []);

  const loadMore = async () => {
    if (!nextCursor) return;
    setLoadingMore(true);
    try {
      const result = await serviceCatalogService.list({ ...filterParams(status, category), cursor: nextCursor });
      setRows((r) => [...r, ...result.items]);
      setNextCursor(result.nextCursor);
    } catch (e) {
      setError(describeApiError(e, "More services could not be loaded."));
    } finally {
      setLoadingMore(false);
    }
  };

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const item = await serviceCatalogService.create(
        {
          name: form.name.trim(),
          code: form.code.trim(),
          category: form.category,
          branchId: form.branchId || undefined,
          description: form.description.trim() || undefined,
          supportedVehicleClasses: splitList(form.vehicleClasses),
          supportedPackageTypes: splitList(form.packageTypes),
        },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/private-ops/services/${item.id}`);
    } catch (err) {
      setFormError(describeApiError(err, "The service could not be created."));
    } finally {
      setSaving(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q === "" ? rows : rows.filter((s) => [s.name, s.code ?? "", s.category ?? ""].join(" ").toLowerCase().includes(q));
  }, [rows, search]);

  if (loading && rows.length === 0 && !error) return <Loading label="Loading services..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Service catalogue"
        description="Services your company offers on its own orders."
        actions={
          <>
            <SecondaryButton onClick={() => load(status, category)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setForm(EMPTY_FORM);
                setFormError(null);
                setShowCreate(true);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> New service
            </PrimaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />

      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "Draft", value: rows.filter((s) => s.status === "DRAFT").length, tone: "amber" },
          { label: "Active", value: rows.filter((s) => s.status === "ACTIVE").length, tone: "green" },
          { label: "Inactive", value: rows.filter((s) => s.status === "INACTIVE").length },
        ]}
      />

      {showCreate && (
        <Panel title="New service" description="New services start as drafts; activate them before ordering." onClose={() => setShowCreate(false)}>
          <form onSubmit={create} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Name" required>
                <input required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Code" required hint="Unique within your organization.">
                <input required value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Category" required>
                <select value={form.category} onChange={(e) => setForm({ ...form, category: e.target.value })} className={INPUT}>
                  {SERVICE_CATEGORIES.map((c) => <option key={c} value={c}>{humanize(c)}</option>)}
                </select>
              </Field>
              <Field label="Branch" hint="Leave empty to offer it in every branch.">
                <select value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">All branches</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Supported vehicle classes" hint="Comma separated.">
                <input value={form.vehicleClasses} onChange={(e) => setForm({ ...form, vehicleClasses: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Supported package types" hint="Comma separated.">
                <input value={form.packageTypes} onChange={(e) => setForm({ ...form, packageTypes: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Description">
                  <textarea rows={3} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Creating..." : "Create service"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by name, code, category..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={category} onChange={(e) => setCategory(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Category">
          <option value="ALL">All categories</option>
          {SERVICE_CATEGORIES.map((c) => <option key={c} value={c}>{humanize(c)}</option>)}
        </select>
        <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
          <option value="ALL">All statuses</option>
          {SERVICE_STATUSES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<Layers className="h-8 w-8" />} title={rows.length === 0 ? "No services yet" : "No services found"} description={rows.length === 0 ? "Add the services your company offers." : "No services match your search."} />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(s) => s.id}
          onRowClick={(s) => router.push(`/private-ops/services/${s.id}`)}
          columns={[
            { header: "Service", cell: (s) => <span className="font-medium text-ink-950">{s.name}</span> },
            { header: "Code", cell: (s) => <span className="font-mono">{s.code || "—"}</span> },
            { header: "Category", cell: (s) => humanize(s.category) },
            { header: "Vehicle classes", cell: (s) => (s.supportedVehicleClasses ?? []).join(", ") || "—" },
            { header: "Status", cell: (s) => <StatusPill status={s.status} /> },
            { header: "Updated", cell: (s) => formatWhen(s.updatedAt) },
          ]}
        />
      )}

      {nextCursor && (
        <div className="flex justify-center">
          <SecondaryButton onClick={loadMore} disabled={loadingMore}>{loadingMore ? "Loading..." : "Load more services"}</SecondaryButton>
        </div>
      )}
    </div>
  );
}
