"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { Search, SlidersHorizontal } from "lucide-react";
import { cn } from "@/lib/utils";
import { useAuthStore } from "@/store/auth-store";
import { webhookService } from "@/services/webhook-service";
import type { WebhookLog } from "@/types";

function StatusPill({ status }: { status: WebhookLog["status"] }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold capitalize",
        status === "delivered" && "bg-brand-100 text-brand-700",
        status === "failed_permanent" && "bg-red-100 text-danger",
        status === "pending" && "bg-amber-100 text-warning"
      )}
    >
      {status === "failed_permanent" ? "failed" : status}
    </span>
  );
}

export default function WebhookLogsPage() {
  const token = useAuthStore((s) => s.token);
  const [logs, setLogs] = useState<WebhookLog[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [page, setPage] = useState(1);
  const [total, setTotal] = useState(0);
  const limit = 20;

  useEffect(() => {
    if (!token) return;
    fetchLogs();
  }, [token, page]);

  const fetchLogs = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await webhookService.list(token!, { page, limit });
      setLogs(response.deliveries);
      setTotal(response.meta.total);
    } catch (err) {
      setError("Failed to load webhook logs");
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  const filtered = logs.filter((log) =>
    [log.event_type, log.id].some((field) =>
      field.toLowerCase().includes(query.toLowerCase())
    )
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-ink-500">Loading webhook logs...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-danger">{error}</div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="rounded-2xl bg-white p-5">
        <h1 className="text-lg font-bold text-ink-900">Webhook Delivery Logs</h1>
        <p className="mt-1 text-sm text-ink-500">Here&apos;s what&apos;s on your counter today.</p>
      </div>

      <div className="flex items-center gap-3">
        <div className="flex flex-1 items-center gap-2 rounded-xl bg-ink-100 px-4 py-3">
          <Search className="h-4 w-4 shrink-0 text-ink-500" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            type="text"
            placeholder="Search"
            className="w-full bg-transparent text-sm text-ink-900 placeholder:text-ink-500 focus:outline-none"
          />
        </div>
        <button
          type="button"
          aria-label="Filter"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl bg-ink-100 text-ink-700"
        >
          <SlidersHorizontal className="h-4 w-4" />
        </button>
      </div>

      <div>
        <h2 className="mb-3 text-base font-bold text-ink-900">Delivery Logs</h2>

        {/* Table: md and up */}
        <div className="hidden overflow-x-auto rounded-2xl bg-white md:block">
          <table className="w-full min-w-[820px] text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs font-semibold uppercase tracking-wide text-ink-500">
                <th className="px-5 py-4">Time Stamp</th>
                <th className="px-5 py-4">Event</th>
                <th className="px-5 py-4">Delivery ID</th>
                <th className="px-5 py-4">Status</th>
                <th className="px-5 py-4">HTTP Response</th>
                <th className="px-5 py-4">Attempts</th>
                <th className="px-5 py-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((log) => (
                <tr key={log.id} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 text-ink-700">
                    {new Date(log.created_at).toLocaleString()}
                  </td>
                  <td className="px-5 py-4 text-ink-700">{log.event_type}</td>
                  <td className="px-5 py-4 font-medium text-ink-900">{log.id.slice(0, 8)}</td>
                  <td className="px-5 py-4"><StatusPill status={log.status} /></td>
                  <td className="px-5 py-4 text-ink-700">{log.response_status ?? "—"}</td>
                  <td className="px-5 py-4 text-ink-700">{log.attempts}</td>
                  <td className="px-5 py-4 text-right">
                    <Link href={`/webhook-logs/${log.id}`} className="font-semibold text-brand-600 hover:text-brand-700">
                      View
                    </Link>
                  </td>
                </tr>
              ))}
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-5 py-10 text-center text-ink-500">
                    No logs match your search.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>

        {/* Cards: below md */}
        <div className="space-y-3 md:hidden">
          {filtered.map((log) => (
            <div key={log.id} className="rounded-2xl bg-white p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <p className="text-sm font-semibold text-ink-900">{log.event_type}</p>
                  <p className="mt-0.5 text-xs text-ink-500">
                    {new Date(log.created_at).toLocaleString()}
                  </p>
                </div>
                <StatusPill status={log.status} />
              </div>
              <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                <div>
                  <p className="text-ink-500">Delivery ID</p>
                  <p className="font-medium text-ink-900">{log.id.slice(0, 8)}</p>
                </div>
                <div>
                  <p className="text-ink-500">HTTP Response</p>
                  <p className="font-medium text-ink-900">{log.response_status ?? "—"}</p>
                </div>
                <div>
                  <p className="text-ink-500">Attempts</p>
                  <p className="font-medium text-ink-900">{log.attempts}</p>
                </div>
                <div>
                  <p className="text-ink-500">Last Attempt</p>
                  <p className="font-medium text-ink-900">
                    {log.last_attempt_at ? new Date(log.last_attempt_at).toLocaleString() : "—"}
                  </p>
                </div>
              </div>
              <Link
                href={`/webhook-logs/${log.id}`}
                className="mt-3 inline-block text-sm font-semibold text-brand-600"
              >
                View details
              </Link>
            </div>
          ))}
          {filtered.length === 0 && (
            <p className="rounded-2xl bg-white p-6 text-center text-sm text-ink-500">
              No logs match your search.
            </p>
          )}
        </div>

        {/* Pagination */}
        {total > limit && (
          <div className="mt-4 flex items-center justify-between">
            <p className="text-sm text-ink-500">
              Showing {(page - 1) * limit + 1}–{Math.min(page * limit, total)} of {total}
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="rounded-lg border border-ink-200 px-4 py-2 text-sm disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setPage(p => p + 1)}
                disabled={page * limit >= total}
                className="rounded-lg border border-ink-200 px-4 py-2 text-sm disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}