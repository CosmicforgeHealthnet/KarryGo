"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { MoreVertical, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import { deliveryService } from "@/services/delivery-service";
import { useAuthStore } from "@/store/auth-store";
import { ApiError } from "@/lib/api-client";
import type { DeliverySummary } from "@/types";

// Real backend statuses (delivery.Status* constants) - shown as-is, not
// collapsed into a simplified bucket set.
const STATUS_OPTIONS = [
  "pending",
  "dispatching",
  "accepted",
  "arrived",
  "in_progress",
  "completed",
  "cancelled",
  "failed",
  "no_provider",
];

const STATUS_STYLES: Record<string, string> = {
  pending: "bg-ink-100 text-ink-500",
  dispatching: "bg-amber-100 text-warning",
  accepted: "bg-brand-100 text-brand-700",
  arrived: "bg-brand-100 text-brand-700",
  in_progress: "bg-amber-100 text-warning",
  completed: "bg-ink-100 text-ink-700",
  cancelled: "bg-red-100 text-danger",
  failed: "bg-red-100 text-danger",
  no_provider: "bg-red-100 text-danger",
};

function statusLabel(status: string): string {
  return status
    .split("_")
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString("en-US", {
    day: "2-digit",
    month: "short",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

const PAGE_LIMIT = 20;

export default function DeliveriesPage() {
  const token = useAuthStore((s) => s.token);

  const [deliveries, setDeliveries] = useState<DeliverySummary[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [statusFilter, setStatusFilter] = useState("");
  const [appliedStatus, setAppliedStatus] = useState("");
  const [query, setQuery] = useState("");
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!token) {
      setIsLoading(false);
      return;
    }
    let cancelled = false;

    async function load() {
      setIsLoading(true);
      setError(null);
      try {
        const res = await deliveryService.list(token!, {
          page,
          limit: PAGE_LIMIT,
          status: appliedStatus || undefined,
        });
        if (cancelled) return;
        setDeliveries(res.deliveries);
        setTotal(res.meta.total);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? err.message
            : "Couldn't load deliveries. Please try again."
        );
      } finally {
        if (!cancelled) setIsLoading(false);
      }
    }

    load();
    return () => {
      cancelled = true;
    };
  }, [token, page, appliedStatus]);

  // Client-side, page-scoped filter only - the Partner API has no full-text
  // search endpoint, so this only searches the currently loaded page of
  // results, not the entire delivery history.
  const visible = deliveries.filter((d) =>
    [d.receiver_name, d.booking_ref].some((field) =>
      field.toLowerCase().includes(query.toLowerCase())
    )
  );

  const totalPages = Math.max(1, Math.ceil(total / PAGE_LIMIT));

  return (
    <div className="space-y-6">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Delivery List</h1>
        <p className="mt-1 text-sm text-ink-500">
          Here&apos;s what&apos;s on your counter today.
        </p>
      </div>

      <div className="flex items-center gap-2 rounded-xl bg-ink-100 px-4 py-3">
        <Search className="h-4 w-4 shrink-0 text-ink-500" />
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          type="text"
          placeholder="Search this page (recipient or booking ref)"
          className="w-full bg-transparent text-sm text-ink-900 placeholder:text-ink-500 focus:outline-none"
        />
      </div>

      <div className="rounded-2xl bg-brand-100 p-4">
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 sm:items-end content:grid-cols-[1fr_auto_auto]">
          <div>
            <label className="mb-2 block text-sm font-medium text-ink-900">
              Status
            </label>
            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value)}
              className="w-full rounded-xl border border-ink-100 bg-white px-4 py-3 text-sm text-ink-900 focus:border-brand-500 focus:outline-none"
            >
              <option value="">All</option>
              {STATUS_OPTIONS.map((s) => (
                <option key={s} value={s}>
                  {statusLabel(s)}
                </option>
              ))}
            </select>
          </div>
          <button
            type="button"
            onClick={() => {
              setPage(1);
              setAppliedStatus(statusFilter);
            }}
            className="h-fit rounded-xl bg-brand-600 px-5 py-3 text-sm font-semibold text-white hover:bg-brand-700"
          >
            Apply Filter
          </button>
          <button
            type="button"
            onClick={() => {
              setStatusFilter("");
              setAppliedStatus("");
              setQuery("");
              setPage(1);
            }}
            className="h-fit rounded-xl bg-brand-200 px-5 py-3 text-sm font-semibold text-brand-700 hover:bg-brand-300"
          >
            Reset
          </button>
        </div>
      </div>

      <div className="flex items-center justify-between">
        <h2 className="text-base font-bold text-ink-900">
          Delivery List {total > 0 && `(${total})`}
        </h2>
      </div>

      {isLoading ? (
        <div className="space-y-4">
          {[0, 1, 2].map((i) => (
            <div
              key={i}
              className="h-40 animate-pulse rounded-2xl bg-white/60"
            />
          ))}
        </div>
      ) : error ? (
        <div className="rounded-2xl border border-red-100 bg-red-50 p-5 text-sm text-red-600">
          {error}
        </div>
      ) : (
        <div className="space-y-4">
          {visible.map((delivery) => (
            <Link
              key={delivery.id}
              href={`/deliveries/${delivery.id}`}
              className="block rounded-2xl bg-white p-4 transition-shadow hover:shadow-md"
            >
              <div className="flex items-start justify-between">
                <div>
                  <p className="text-sm font-bold text-ink-900">
                    #{delivery.booking_ref}
                  </p>
                  <p className="mt-0.5 text-xs text-ink-500">
                    {formatDate(delivery.created_at)}
                  </p>
                </div>
                <button
                  type="button"
                  aria-label="More options"
                  onClick={(e) => {
                    e.preventDefault();
                    e.stopPropagation();
                  }}
                  className="text-ink-500 hover:text-ink-700"
                >
                  <MoreVertical className="h-5 w-5" />
                </button>
              </div>

              <p className="mt-3 text-xs font-semibold text-brand-600">
                Recipient
              </p>
              <p className="text-sm font-semibold text-ink-900">
                {delivery.receiver_name}
              </p>

              <div className="mt-3 flex gap-3">
                <div className="flex flex-col items-center pt-1">
                  <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-brand-500 ring-4 ring-brand-100" />
                  <span className="my-1 w-px flex-1 bg-brand-200" />
                  <span className="h-2.5 w-2.5 shrink-0 rounded-full bg-brand-500 ring-4 ring-brand-100" />
                </div>
                <div className="flex-1 space-y-3 text-sm">
                  <div>
                    <p className="text-xs text-ink-500">Pick-up</p>
                    <p className="font-medium text-ink-900">
                      {delivery.pickup_address}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-ink-500">Drop off</p>
                    <p className="font-medium text-ink-900">
                      {delivery.dropoff_address}
                    </p>
                  </div>
                </div>
              </div>

              <div className="mt-4 flex items-center justify-between">
                <p className="text-sm font-bold text-ink-900">
                  {delivery.fare_amount.display}
                </p>
                <span
                  className={cn(
                    "rounded-full px-3 py-1 text-xs font-semibold",
                    STATUS_STYLES[delivery.status] ?? "bg-ink-100 text-ink-700"
                  )}
                >
                  {statusLabel(delivery.status)}
                </span>
              </div>
            </Link>
          ))}

          {visible.length === 0 && (
            <p className="rounded-2xl bg-white p-6 text-center text-sm text-ink-500">
              No deliveries match your filters.
            </p>
          )}
        </div>
      )}

      {!isLoading && !error && totalPages > 1 && (
        <div className="flex items-center justify-between rounded-2xl bg-white p-4">
          <button
            type="button"
            disabled={page <= 1}
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            className="rounded-xl bg-brand-100 px-4 py-2 text-sm font-semibold text-brand-700 disabled:opacity-40"
          >
            Previous
          </button>
          <p className="text-sm text-ink-500">
            Page {page} of {totalPages}
          </p>
          <button
            type="button"
            disabled={page >= totalPages}
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            className="rounded-xl bg-brand-100 px-4 py-2 text-sm font-semibold text-brand-700 disabled:opacity-40"
          >
            Next
          </button>
        </div>
      )}
    </div>
  );
}