// src/app/(dashboard)/settings/approvals/page.tsx

"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  Inbox,
  RefreshCw,
  Clock,
  CheckCircle2,
  Eye,
  Check,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  AlertTriangle,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { formatMinor, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { approvalRequestService } from "@/lib/services/governanceService";
import type { ApprovalRequest } from "@/lib/types/governance";

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  PENDING: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  APPROVED: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  REJECTED: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  CANCELLED: { bg: "bg-surface-2", text: "text-ink-500", icon: <X className="h-3.5 w-3.5" /> },
  EXPIRED: { bg: "bg-surface-2", text: "text-ink-500", icon: <Clock className="h-3.5 w-3.5" /> },
  INVALIDATED: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  EXECUTED: { bg: "bg-blue-bg", text: "text-blue", icon: <Check className="h-3.5 w-3.5" /> },
};

const STATUS_FILTERS = ["All", "PENDING", "APPROVED", "REJECTED", "CANCELLED", "EXPIRED", "INVALIDATED", "EXECUTED"];

const DAY_MS = 24 * 60 * 60 * 1000;

export default function ApprovalsInboxPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ApprovalRequest[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [loadedAt, setLoadedAt] = useState(0);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [showFilters, setShowFilters] = useState(false);

  // The status filter is applied by the server; search runs over the page.
  const load = useCallback(async (status: string) => {
    try {
      setData(await approvalRequestService.list({ status: status === "All" ? undefined : status, limit: 200 }));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Approval requests could not be loaded."));
    } finally {
      setLoadedAt(Date.now());
      setSecondsAgo(0);
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(selectedStatus));
  }, [load, selectedStatus]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const q = search.trim().toLowerCase();
    return (
      q === "" ||
      item.id.toLowerCase().includes(q) ||
      item.action.toLowerCase().includes(q) ||
      item.resourceType.toLowerCase().includes(q) ||
      item.resourceId.toLowerCase().includes(q) ||
      item.requesterId.toLowerCase().includes(q)
    );
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading approvals...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const filterCount = selectedStatus !== "All" ? 1 : 0;

  const formatTime = (date: string) => {
    const diffMs = loadedAt - new Date(date).getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  const formatDate = (date: string | null | undefined) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const expiringSoon = (item: ApprovalRequest) => {
    const left = new Date(item.expiresAt).getTime() - loadedAt;
    return item.status === "PENDING" && left > 0 && left <= DAY_MS;
  };

  const goToRequestDetail = (id: string) => {
    router.push(`/settings/approvals/${id}`);
  };

  // Stats
  const totalPending = data.filter((d) => d.status === "PENDING").length;
  const totalExpiring = data.filter(expiringSoon).length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Approvals Inbox
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pending, approved, rejected, cancelled, expired and executed requests with requester/action/resource.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => load(selectedStatus)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={() => router.push("/settings/approval-policies")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            Policies
          </button>
          <button
            onClick={() => router.push("/settings/approvals/delegation")}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            Delegation
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pending</div>
          <div className="text-lg font-bold text-amber">{totalPending}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Expiring in 24h</div>
          <div className="text-lg font-bold text-red">{totalExpiring}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Executed</div>
          <div className="text-lg font-bold text-blue">{data.filter((d) => d.status === "EXECUTED").length}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by reference, action, resource..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const nextIndex = (STATUS_FILTERS.indexOf(selectedStatus) + 1) % STATUS_FILTERS.length;
            setSelectedStatus(STATUS_FILTERS[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Inbox className="h-4 w-4 text-ink-400" />
          {selectedStatus === "All" ? "All" : humanize(selectedStatus)}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {filterCount}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {showFilters && (
        <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
          <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</span>
          <div className="flex flex-wrap gap-1.5">
            {STATUS_FILTERS.map((status) => (
              <button
                key={status}
                onClick={() => setSelectedStatus(status)}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                  selectedStatus === status ? "bg-primary text-white" : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                }`}
              >
                {status === "All" ? "All" : humanize(status)}
              </button>
            ))}
          </div>
        </div>
      )}

      {/* ===== APPROVALS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Inbox className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No approval requests found</h3>
          <p className="mt-1 text-sm text-ink-500">
            {hasActiveFilters ? "No requests match your current filters." : "Nothing is waiting for approval."}
          </p>
          {hasActiveFilters && (
            <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
              Clear filters
            </button>
          )}
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1000px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Request</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Action</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Amount</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Approvals</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Expires</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Age</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.PENDING;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToRequestDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{humanize(item.resourceType)}</div>
                        <div className="font-mono text-[10px] text-ink-500">{item.resourceId}</div>
                        <div className="text-[9px] text-ink-400">by {item.requesterId}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{humanize(item.action)}</td>
                      <td className="px-4 py-3">
                        <span className={`flex w-fit items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                          {statusStyle.icon}
                          {humanize(item.status)}
                        </span>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.amountMinor != null ? formatMinor(item.amountMinor, item.amountCurrency) : "—"}
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">
                        {item.minimumApprovals} required · step {item.currentStep}
                      </td>
                      <td className={`px-4 py-3 text-xs ${expiringSoon(item) ? "font-semibold text-red" : "text-ink-500"}`}>
                        {formatDate(item.expiresAt)}
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">{formatTime(item.createdAt)}</td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
                          <Eye className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Inbox className="h-4 w-4" />
          IAM-09 · Approvals Inbox
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} requests shown
        </span>
      </div>
    </div>
  );
}
