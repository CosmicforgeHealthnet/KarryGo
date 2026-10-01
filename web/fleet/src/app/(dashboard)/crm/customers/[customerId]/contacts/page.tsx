"use client";

import { useEffect, useState, useCallback, useMemo } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Users,
  Search,
  RefreshCw,
  SlidersHorizontal,
  X,
  Clock,
  CheckCircle2,
  AlertCircle,
  Mail,
  Phone,
  Star,
  Plus,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type {
  ContactView,
  ContactPurpose,
  RecordStatus,
} from "@/lib/types/crm";
import {
  channelLabel,
  consentLabel,
  consentTone,
  contactPurposeLabel,
  initialsOf,
  recordStatusLabel,
  recordStatusTone,
  timeAgo,
} from "@/lib/utils/crmLabels";

const PURPOSE_FILTERS: Array<{ value: "ALL" | ContactPurpose; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "GENERAL", label: "General" },
  { value: "OPERATIONS", label: "Operations" },
  { value: "BILLING", label: "Billing" },
  { value: "LEGAL", label: "Legal" },
  { value: "SALES", label: "Sales" },
  { value: "EMERGENCY", label: "Emergency" },
  { value: "OTHER", label: "Other" },
];

const STATUS_FILTERS: Array<{ value: "ALL" | RecordStatus; label: string }> = [
  { value: "ALL", label: "All" },
  { value: "ACTIVE", label: "Active" },
  { value: "INACTIVE", label: "Inactive" },
  { value: "ARCHIVED", label: "Archived" },
];

export default function CustomerContactsPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [search, setSearch] = useState("");
  const [purposeFilter, setPurposeFilter] = useState<"ALL" | ContactPurpose>("ALL");
  const [statusFilter, setStatusFilter] = useState<"ALL" | RecordStatus>("ALL");
  const [showFilters, setShowFilters] = useState(false);

  const [data, setData] = useState<ContactView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchContacts = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.listContacts(customerId, {
          status: statusFilter === "ALL" ? undefined : statusFilter,
          contactPurpose: purposeFilter === "ALL" ? undefined : purposeFilter,
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load contacts.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [customerId, statusFilter, purposeFilter]
  );

  useEffect(() => {
    if (customerId) fetchContacts();
  }, [customerId, fetchContacts]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return data;
    return data.filter((c) => {
      const haystack = [
        c.fullName,
        c.roleTitle ?? "",
        c.emailMasked ?? "",
        c.phoneMasked ?? "",
      ]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [data, search]);

  const hasActiveFilters =
    purposeFilter !== "ALL" || statusFilter !== "ALL" || search.trim() !== "";

  const clearFilters = () => {
    setSearch("");
    setPurposeFilter("ALL");
    setStatusFilter("ALL");
  };

  const stats = useMemo(() => {
    const total = data.length;
    const primary = data.filter((d) => d.isPrimary).length;
    const consented = data.filter((d) => d.consentStatus === "GRANTED").length;
    return { total, primary, consented };
  }, [data]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading contacts…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer Contacts
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            People, roles, preferred channels, and consent state.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => fetchContacts({ silent: true })}
            disabled={refreshing}
            className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
            aria-label="Refresh"
          >
            <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
          </button>
          <button
            onClick={() => router.push(`/crm/customers/${customerId}/contacts/create`)}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Contact
          </button>
        </div>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchContacts()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      <div className="grid grid-cols-3 gap-2.5">
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Total
          </div>
          <div className="text-lg font-bold text-ink-950">{stats.total}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Primary
          </div>
          <div className="text-lg font-bold text-primary">{stats.primary}</div>
        </div>
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">
            Consented
          </div>
          <div className="text-lg font-bold text-green">{stats.consented}</div>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-surface px-4">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, role, email…"
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-600 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(purposeFilter !== "ALL" ? 1 : 0) + (statusFilter !== "ALL" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-surface p-3.5">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Purpose
              </div>
              <div className="flex flex-wrap gap-1.5">
                {PURPOSE_FILTERS.map((p) => (
                  <button
                    key={p.value}
                    onClick={() => setPurposeFilter(p.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      purposeFilter === p.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </div>
              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((s) => (
                  <button
                    key={s.value}
                    onClick={() => setStatusFilter(s.value)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      statusFilter === s.value
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s.label}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 transition hover:bg-surface-2"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {filtered.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/alerts-empty.png"
            title={
              hasActiveFilters
                ? "No contacts match your filters"
                : "No contacts yet"
            }
            description={
              hasActiveFilters
                ? "Try adjusting your search or clearing the filters."
                : "Add your first contact for this customer."
            }
            action={
              hasActiveFilters ? (
                <button
                  onClick={clearFilters}
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Clear filters
                </button>
              ) : (
                <button
                  onClick={() =>
                    router.push(`/crm/customers/${customerId}/contacts/create`)
                  }
                  className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                >
                  Add contact
                </button>
              )
            }
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
          {filtered.map((item) => {
            const consentToneCls = consentTone(item.consentStatus);
            const recordTone = recordStatusTone(item.status);
            return (
              <div
                key={item.id}
                onClick={() =>
                  router.push(`/crm/customers/${customerId}/contacts/${item.id}`)
                }
                className={`group cursor-pointer rounded-xl border p-4 transition hover:shadow-md ${
                  item.isPrimary
                    ? "border-primary/30 bg-primary-bg/10 hover:border-primary/50"
                    : "border-line bg-surface hover:border-ink-400"
                }`}
              >
                <div className="flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-[14px] font-bold text-white ${
                        item.isPrimary ? "bg-primary" : "bg-ink-400"
                      }`}
                    >
                      {initialsOf(item.fullName)}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.fullName}
                        </span>
                        {item.isPrimary && (
                          <span className="flex items-center gap-1 rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">
                            <Star className="h-3 w-3" />
                            Primary
                          </span>
                        )}
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        {item.roleTitle && (
                          <span className="text-[12px] text-ink-500">
                            {item.roleTitle}
                          </span>
                        )}
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">
                          {contactPurposeLabel(item.contactPurpose)}
                        </span>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="mt-3 space-y-1.5 text-sm">
                  {item.emailMasked && (
                    <div className="flex items-center gap-2 text-ink-600">
                      <Mail className="h-4 w-4 text-ink-400" />
                      <span className="font-mono text-[12px]">
                        {item.emailMasked}
                      </span>
                    </div>
                  )}
                  {item.phoneMasked && (
                    <div className="flex items-center gap-2 text-ink-600">
                      <Phone className="h-4 w-4 text-ink-400" />
                      <span className="font-mono text-[12px]">
                        {item.phoneMasked}
                      </span>
                    </div>
                  )}
                </div>

                <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-2.5">
                  <span
                    className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${consentToneCls.bg} ${consentToneCls.text}`}
                  >
                    {item.consentStatus === "GRANTED" ? (
                      <CheckCircle2 className="h-3 w-3" />
                    ) : item.consentStatus === "WITHDRAWN" ? (
                      <AlertCircle className="h-3 w-3" />
                    ) : (
                      <Clock className="h-3 w-3" />
                    )}
                    {consentLabel(item.consentStatus)}
                  </span>
                  <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                    {channelLabel(item.preferredChannel)}
                  </span>
                  <span
                    className={`rounded-full px-2.5 py-0.5 text-[10px] font-medium ${recordTone.bg} ${recordTone.text}`}
                  >
                    {recordStatusLabel(item.status)}
                  </span>
                  <span className="ml-auto text-[11px] text-ink-400">
                    Added {timeAgo(item.createdAt)}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}