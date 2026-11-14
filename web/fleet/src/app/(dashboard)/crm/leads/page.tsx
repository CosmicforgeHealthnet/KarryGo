"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Building2, Clock, Plus, RefreshCw, Save, Search, SlidersHorizontal, Target, Users, X } from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { StatusPill, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { leadService } from "@/lib/services/operationsService";
import { LEAD_SOURCES, LEAD_STATUSES, type Lead } from "@/lib/types/operations";
import { timeAgo } from "@/lib/utils/crmLabels";

const STATUS_FILTERS: Array<"ALL" | (typeof LEAD_STATUSES)[number]> = ["ALL", ...LEAD_STATUSES];

const EMPTY_FORM = { companyName: "", source: "WEBSITE", contactName: "", contactEmail: "", contactPhone: "", score: "", nextAction: "", notes: "" };

export default function LeadsPage() {
  const router = useRouter();

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState<"ALL" | string>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<Lead[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [createKey, setCreateKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const fetchLeads = useCallback(async (status: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await leadService.list({ status: status === "ALL" ? undefined : status, limit: 50 });
      setData(result.items);
      setNextCursor(result.nextCursor);
    } catch (err) {
      setError(describeApiError(err, "Leads could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => fetchLeads(statusFilter));
  }, [fetchLeads, statusFilter]);

  const loadMore = async () => {
    if (!nextCursor) return;
    setLoadingMore(true);
    try {
      const result = await leadService.list({ status: statusFilter === "ALL" ? undefined : statusFilter, cursor: nextCursor, limit: 50 });
      setData((rows) => [...rows, ...result.items]);
      setNextCursor(result.nextCursor);
    } catch (err) {
      setError(describeApiError(err, "More leads could not be loaded."));
    } finally {
      setLoadingMore(false);
    }
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    const contact: Record<string, string> = {};
    if (form.contactName.trim()) contact.name = form.contactName.trim();
    if (form.contactEmail.trim()) contact.email = form.contactEmail.trim();
    if (form.contactPhone.trim()) contact.phone = form.contactPhone.trim();
    try {
      const created = await leadService.create(
        {
          companyName: form.companyName.trim(),
          source: form.source,
          contactSnapshot: contact,
          score: form.score.trim() === "" ? undefined : Number(form.score),
          nextAction: form.nextAction.trim() || undefined,
          notes: form.notes.trim() || undefined,
        },
        createKey,
      );
      setCreateKey(newIdempotencyKey());
      setShowCreate(false);
      setForm(EMPTY_FORM);
      router.push(`/crm/leads/${created.id}`);
    } catch (err) {
      setFormError(describeApiError(err, "The lead could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data;
    return data.filter((item) => [item.companyName, item.source, item.ownerUserId ?? "", item.nextAction ?? ""].join(" ").toLowerCase().includes(q));
  }, [data, search]);

  const hasActiveFilters = statusFilter !== "ALL" || search.trim() !== "";
  const clearFilters = () => {
    setSearch("");
    setStatusFilter("ALL");
  };

  const stats = useMemo(
    () => ({
      total: data.length,
      fresh: data.filter((d) => d.status === "NEW" || d.status === "CONTACTED").length,
      qualified: data.filter((d) => d.status === "QUALIFIED").length,
      converted: data.filter((d) => d.status === "CONVERTED").length,
    }),
    [data],
  );

  if (loading && data.length === 0 && !error) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading leads…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">Leads</h1>
          <p className="mt-1 text-sm text-ink-600">Companies you are talking to before they become customers.</p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchLeads(statusFilter)}
            disabled={loading}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${loading ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => {
              setShowCreate(true);
              setFormError(null);
            }}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Lead
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />

      {showCreate && (
        <form onSubmit={submit} className="space-y-4 rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <h3 className="text-[15px] font-semibold text-ink-950">New lead</h3>
            <button type="button" onClick={() => setShowCreate(false)} className="text-ink-400 hover:text-ink-700" aria-label="Close">
              <X className="h-4 w-4" />
            </button>
          </div>
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Company name" required>
              <input required value={form.companyName} onChange={(e) => setForm({ ...form, companyName: e.target.value })} placeholder="Acme Distribution Ltd" className={INPUT} />
            </Field>
            <Field label="Source" required>
              <select value={form.source} onChange={(e) => setForm({ ...form, source: e.target.value })} className={INPUT}>
                {LEAD_SOURCES.map((s) => (
                  <option key={s} value={s}>{humanize(s)}</option>
                ))}
              </select>
            </Field>
            <Field label="Contact name">
              <input value={form.contactName} onChange={(e) => setForm({ ...form, contactName: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Contact email">
              <input type="email" value={form.contactEmail} onChange={(e) => setForm({ ...form, contactEmail: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Contact phone">
              <input value={form.contactPhone} onChange={(e) => setForm({ ...form, contactPhone: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Score" hint="0-100">
              <input inputMode="numeric" value={form.score} onChange={(e) => setForm({ ...form, score: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Next action">
              <input value={form.nextAction} onChange={(e) => setForm({ ...form, nextAction: e.target.value })} placeholder="Call to discuss volumes" className={INPUT} />
            </Field>
            <Field label="Notes">
              <input value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <div className="flex flex-wrap gap-3">
            <button type="submit" disabled={saving} className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-60">
              <Save className="h-4 w-4" />
              {saving ? "Saving..." : "Save lead"}
            </button>
            <button type="button" onClick={() => setShowCreate(false)} className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-white">
              Cancel
            </button>
          </div>
        </form>
      )}

      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        {[
          ["Loaded", stats.total, "text-ink-950"],
          ["New / contacted", stats.fresh, "text-primary"],
          ["Qualified", stats.qualified, "text-green"],
          ["Converted", stats.converted, "text-ink-950"],
        ].map(([label, value, tone]) => (
          <div key={label as string} className="rounded-xl border border-line bg-surface p-3.5">
            <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">{label}</div>
            <div className={`text-lg font-bold ${tone}`}>{value}</div>
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by company, source, owner…"
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>
        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters ? "border-primary bg-primary-bg text-primary" : "border-line bg-surface text-ink-600 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">{statusFilter !== "ALL" ? 1 : 0}</span>}
        </button>
        {hasActiveFilters && (
          <button onClick={clearFilters} className="flex h-11 items-center gap-1 rounded-xl border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition hover:bg-surface-2">
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
          <div className="flex flex-wrap gap-1.5">
            {STATUS_FILTERS.map((s) => (
              <button
                key={s}
                onClick={() => setStatusFilter(s)}
                className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                  statusFilter === s ? "bg-primary-bg text-primary ring-2 ring-primary/30" : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                }`}
              >
                {s === "ALL" ? "All" : humanize(s)}
              </button>
            ))}
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/trips-empty.png"
            title={hasActiveFilters ? "No leads match your filters" : "No leads yet"}
            description={hasActiveFilters ? "Try adjusting your search or clearing the filters." : "Add your first lead to get started."}
            action={
              <button
                onClick={hasActiveFilters ? clearFilters : () => setShowCreate(true)}
                className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                {hasActiveFilters ? "Clear filters" : "Add lead"}
              </button>
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => (
            <div
              key={item.id}
              onClick={() => router.push(`/crm/leads/${item.id}`)}
              className="group cursor-pointer rounded-xl border border-line bg-surface p-4 transition hover:shadow-md"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="truncate text-[15px] font-semibold text-ink-950">{item.companyName}</span>
                <StatusPill status={item.status} />
              </div>
              <div className="mt-1 flex flex-wrap items-center gap-3 text-[12px] text-ink-500">
                <span className="flex items-center gap-1.5">
                  <Building2 className="h-3.5 w-3.5" />
                  {humanize(item.source)}
                </span>
                {item.ownerUserId && (
                  <span className="flex items-center gap-1.5">
                    <Users className="h-3.5 w-3.5" />
                    <span className="font-mono">{item.ownerUserId}</span>
                  </span>
                )}
                {item.score != null && (
                  <span className="flex items-center gap-1.5">
                    <Target className="h-3.5 w-3.5" />
                    Score {item.score}
                  </span>
                )}
              </div>
              {item.nextAction && (
                <div className="mt-3 border-t border-line pt-2.5 text-[12px]">
                  <span className="text-ink-500">Next: </span>
                  <span className="font-medium text-ink-950">{item.nextAction}</span>
                </div>
              )}
              <div className="mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                <div className="flex items-center gap-1.5 text-[10px] text-ink-400">
                  <Clock className="h-3.5 w-3.5" />
                  {timeAgo(item.createdAt)}
                </div>
                <div className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                  View <ArrowRight className="h-3.5 w-3.5" />
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {nextCursor && (
        <div className="flex justify-center">
          <button onClick={loadMore} disabled={loadingMore} className="rounded-xl border border-line bg-surface px-4 py-2 text-xs font-medium text-ink-600 hover:bg-surface-2 disabled:opacity-60">
            {loadingMore ? "Loading…" : "Load more leads"}
          </button>
        </div>
      )}
    </div>
  );
}
