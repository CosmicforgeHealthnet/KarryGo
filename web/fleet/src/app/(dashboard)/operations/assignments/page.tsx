// src/app/(dashboard)/assignments/page.tsx

"use client";

import React, { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import {
  Search,
  SlidersHorizontal,
  X,
  Eye,
  RefreshCw,
  Clock3,
  CheckCircle2,
  XCircle,
  Archive,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { marketplaceService } from "@/lib/services/marketplaceService";
import type { Assignment } from "@/lib/types/marketplace";

// ---------------------------------------------------------------------------
// Presentation helpers
// ---------------------------------------------------------------------------

type Tone = "blue" | "green" | "amber" | "red";

const TONE_BG: Record<Tone, string> = {
  blue: "bg-primary-bg text-primary",
  green: "bg-green-bg text-green",
  amber: "bg-amber-bg text-amber",
  red: "bg-red-bg text-red",
};

function statusLabel(status: string): string {
  switch (status) {
    case "CREATED":
      return "Created";
    case "PENDING_PROVIDER_RESPONSE":
      return "Pending response";
    case "ACCEPTED":
      return "Accepted";
    case "READY_FOR_EXECUTION":
      return "Ready";
    case "EXECUTION_START_PENDING":
      return "Starting";
    case "IN_PROGRESS":
      return "In progress";
    case "COMPLETED":
      return "Completed";
    case "DECLINED":
      return "Declined";
    case "EXPIRED":
      return "Expired";
    case "CANCELLED":
      return "Cancelled";
    case "RECONCILIATION_REQUIRED":
      return "Needs reconciliation";
    default:
      return status || "—";
  }
}

function statusStyle(status: string): { bg: string; text: string; dot: string } {
  switch (status) {
    case "ACCEPTED":
    case "READY_FOR_EXECUTION":
    case "COMPLETED":
      return { bg: "bg-green-bg", text: "text-green", dot: "bg-green" };
    case "PENDING_PROVIDER_RESPONSE":
    case "CREATED":
    case "RECONCILIATION_REQUIRED":
      return { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" };
    case "DECLINED":
    case "CANCELLED":
      return { bg: "bg-red-bg", text: "text-red", dot: "bg-red" };
    case "EXECUTION_START_PENDING":
    case "IN_PROGRESS":
      return { bg: "bg-primary-bg", text: "text-primary", dot: "bg-primary" };
    default:
      return { bg: "bg-surface-2", text: "text-ink-600", dot: "bg-ink-400" };
  }
}

function deadlineDisplay(a: Assignment): {
  text: string;
  minutes: number;
  isPending: boolean;
} {
  const isPending = a.Status === "PENDING_PROVIDER_RESPONSE";
  if (!isPending) {
    if (a.AcceptedAt) return { text: "Accepted", minutes: -1, isPending: false };
    if (a.DeclinedAt) return { text: "Declined", minutes: -1, isPending: false };
    if (a.ExpiredAt) return { text: "Expired", minutes: -1, isPending: false };
    if (a.CancelledAt) return { text: "Cancelled", minutes: -1, isPending: false };
    return { text: "—", minutes: -1, isPending: false };
  }
  const t = new Date(a.ResponseDeadline).getTime();
  if (!Number.isFinite(t)) return { text: "—", minutes: -1, isPending: true };
  const minutes = Math.max(0, Math.floor((t - Date.now()) / 60_000));
  if (minutes <= 0) return { text: "Closed", minutes: 0, isPending: true };
  if (minutes < 60) return { text: `${minutes} min`, minutes, isPending: true };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { text: `${hours} h`, minutes, isPending: true };
  const days = Math.floor(hours / 24);
  return { text: `${days} d`, minutes, isPending: true };
}

function canReassign(status: string): boolean {
  return (
    status === "PENDING_PROVIDER_RESPONSE" ||
    status === "DECLINED" ||
    status === "EXPIRED" ||
    status === "CANCELLED"
  );
}

// ---------------------------------------------------------------------------
// Status buckets for the four summary cards.
//
// The backend has no metrics endpoint for assignments, so these are computed
// from the list we already fetched. That is the correct posture here: the
// counts are trivially derived from data we hold, they cannot drift from the
// rows below, and they cost no extra request.
//
// The buckets are mutually exclusive and cover every status, so the four
// numbers always sum to the total.
// ---------------------------------------------------------------------------

function isPendingBucket(status: string): boolean {
  return status === "PENDING_PROVIDER_RESPONSE" || status === "CREATED";
}

function isActiveBucket(status: string): boolean {
  return (
    status === "ACCEPTED" ||
    status === "READY_FOR_EXECUTION" ||
    status === "IN_PROGRESS" ||
    status === "EXECUTION_START_PENDING" ||
    status === "RECONCILIATION_REQUIRED"
  );
}

function isDeclinedBucket(status: string): boolean {
  return status === "DECLINED";
}

function isClosedBucket(status: string): boolean {
  return (
    status === "EXPIRED" ||
    status === "CANCELLED" ||
    status === "COMPLETED"
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AssignmentsListPage() {
  const [assignments, setAssignments] = useState<Assignment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    marketplaceService
      .listAssignments({ limit: 100 })
      .then((data) => {
        if (!cancelled) setAssignments(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          "Failed to load assignments.";
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const counts = useMemo(() => {
    let pending = 0;
    let active = 0;
    let declined = 0;
    let closed = 0;
    for (const a of assignments) {
      if (isPendingBucket(a.Status)) pending++;
      else if (isActiveBucket(a.Status)) active++;
      else if (isDeclinedBucket(a.Status)) declined++;
      else if (isClosedBucket(a.Status)) closed++;
    }
    return { pending, active, declined, closed };
  }, [assignments]);

  const statuses = useMemo(() => {
    const set = new Set<string>();
    assignments.forEach((a) => {
      if (a.Status) set.add(a.Status);
    });
    return ["All", ...Array.from(set)];
  }, [assignments]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return assignments.filter((a) => {
      if (selectedStatus !== "All" && a.Status !== selectedStatus) return false;
      if (q === "") return true;
      const haystack = [a.ID, a.OfferID ?? "", a.MembershipID]
        .join(" ")
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [assignments, search, selectedStatus]);

  const hasActiveFilters = selectedStatus !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
  };

  const metricCards: {
    label: string;
    value: number;
    tone: Tone;
    icon: React.ReactNode;
  }[] = [
    {
      label: "Pending response",
      value: counts.pending,
      tone: "amber",
      icon: <Clock3 className="h-[18px] w-[18px]" />,
    },
    {
      label: "Active",
      value: counts.active,
      tone: "green",
      icon: <CheckCircle2 className="h-[18px] w-[18px]" />,
    },
    {
      label: "Declined",
      value: counts.declined,
      tone: "red",
      icon: <XCircle className="h-[18px] w-[18px]" />,
    },
    {
      label: "Closed",
      value: counts.closed,
      tone: "blue",
      icon: <Archive className="h-[18px] w-[18px]" />,
    },
  ];

  return (
    <div className="mx-auto max-w-[1600px] space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Assignments</h1>
        <p className="max-w-2xl">
          Provider assignments created from marketplace offers.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red/20 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {/* ── Summary cards ── */}
      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        {metricCards.map((m) => (
          <div
            key={m.label}
            className="rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400"
          >
            <div className="flex items-center gap-2.5">
              <div
                className={`flex h-8 w-8 items-center justify-center rounded-md ${TONE_BG[m.tone]}`}
              >
                {m.icon}
              </div>
              <span className="text-xs font-semibold text-ink-600">
                {m.label}
              </span>
            </div>
            <div className="mt-3">
              <span className="font-display text-3xl font-bold tracking-[-0.02em] text-ink-950">
                {m.value}
              </span>
            </div>
          </div>
        ))}
      </div>

      {/* ── Toolbar ── */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search assignments..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || selectedStatus !== "All"
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-surface text-ink-700 hover:bg-surface-2"
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
          {selectedStatus !== "All" && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              1
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 hover:bg-surface-2 transition-colors whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} assignments
        </span>
      </div>

      {/* ── Filter drawer ── */}
      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
                Status
              </span>
              <div className="flex flex-wrap gap-1.5">
                {statuses.map((s) => (
                  <button
                    key={s}
                    onClick={() => setSelectedStatus(s)}
                    className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                      selectedStatus === s
                        ? "bg-primary text-white"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {s === "All" ? "All" : statusLabel(s)}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-md border border-line px-3.5 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors whitespace-nowrap"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ── Table ── */}
      <div className="overflow-x-auto rounded-lg border border-line bg-surface">
        {loading ? (
          <div className="px-4 py-16 text-center text-sm text-ink-500">
            Loading assignments…
          </div>
        ) : assignments.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No assignments yet"
            description="Assignments created from marketplace offers will appear here."
            actionHref="/operations/marketplace"
            actionLabel="View marketplace offers"
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            image="/images/trips-empty.png"
            title="No assignments match your filters"
            description="Try adjusting your search or clearing the filters."
            action={
              <button
                onClick={clearFilters}
                className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
              >
                Clear filters
              </button>
            }
          />
        ) : (
          <table className="w-full min-w-[900px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Assignment
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Offer
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Provider
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Status
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
                  Deadline
                </th>
                <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 text-right whitespace-nowrap">
                  Action
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((a) => {
                const pill = statusStyle(a.Status);
                const deadline = deadlineDisplay(a);
                const isUrgent =
                  deadline.isPending &&
                  deadline.minutes > 0 &&
                  deadline.minutes <= 15;

                return (
                  <tr
                    key={a.ID}
                    className="border-b border-line transition-colors hover:bg-surface-2"
                  >
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <Link
                        href={`/operations/assignments/${a.ID}`}
                        className="font-mono text-sm font-semibold text-ink-950 hover:text-primary transition-colors"
                      >
                        {a.ID}
                      </Link>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      {a.OfferID ? (
                        <Link
                          href={`/operations/marketplace/${a.OfferID}`}
                          className="font-mono text-xs text-ink-600 hover:text-primary transition-colors"
                        >
                          {a.OfferID}
                        </Link>
                      ) : (
                        <span className="text-ink-400">—</span>
                      )}
                    </td>
                    <td className="px-4 py-3.5 font-mono text-xs text-ink-700 whitespace-nowrap">
                      {a.MembershipID}
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span
                        className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-[11px] font-semibold ${pill.bg} ${pill.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${pill.dot}`} />
                        {statusLabel(a.Status)}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 whitespace-nowrap">
                      <span
                        className={
                          isUrgent
                            ? "font-mono text-xs font-bold text-red"
                            : deadline.isPending
                            ? "font-mono text-xs font-medium text-ink-700"
                            : "text-xs text-ink-500"
                        }
                      >
                        {deadline.text}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-right whitespace-nowrap">
                      <div className="flex items-center justify-end gap-2">
                        <Link
                          href={`/operations/assignments/${a.ID}`}
                          className="inline-flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-medium text-ink-600 hover:bg-surface-2 transition-colors"
                        >
                          <Eye className="h-3.5 w-3.5" />
                          View
                        </Link>
                        {canReassign(a.Status) && (
                          <Link
                            href={`/operations/assignments/${a.ID}/reassign`}
                            className="inline-flex items-center gap-1 rounded-md bg-primary px-3 py-1.5 text-xs font-semibold text-white hover:bg-primary-hover transition-colors"
                          >
                            <RefreshCw className="h-3.5 w-3.5" />
                            Reassign
                          </Link>
                        )}
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}