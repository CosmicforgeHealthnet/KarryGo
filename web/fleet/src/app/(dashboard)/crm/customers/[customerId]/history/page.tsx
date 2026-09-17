"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
import {
  Clock,
  RefreshCw,
  User,
  ArrowRight,
  AlertTriangle,
} from "lucide-react";
import EmptyState from "@/components/EmptyState";
import { crmService } from "@/lib/services/crmService";
import type { CustomerHistoryView } from "@/lib/types/crm";
import { formatDateTime, timeAgo } from "@/lib/utils/crmLabels";

function actorLabel(actorType: string): string {
  switch (actorType) {
    case "CUSTOMER":
      return "Customer";
    case "ORGANIZATION_USER":
      return "Staff";
    case "EXTERNAL_INTEGRATION":
      return "Integration";
    case "SYSTEM":
      return "System";
    default:
      return actorType;
  }
}

function changeLabel(changeType: string): string {
  switch (changeType) {
    case "CREATED":
      return "Created";
    case "UPDATED":
      return "Updated";
    case "STATUS_CHANGED":
      return "Status changed";
    case "RISK_CHANGED":
      return "Risk changed";
    case "ARCHIVED":
      return "Archived";
    case "PRIMARY_CHANGED":
      return "Primary contact changed";
    default:
      return changeType;
  }
}

export default function CustomerHistoryPage() {
  const params = useParams();
  const customerId = params.customerId as string;

  const [data, setData] = useState<CustomerHistoryView[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const fetchHistory = useCallback(
    async (opts: { silent?: boolean } = {}) => {
      if (opts.silent) setRefreshing(true);
      else setLoading(true);
      setError(null);
      try {
        const result = await crmService.getCustomerHistory(customerId, {
          limit: 100,
        });
        setData(result.data);
      } catch (err: unknown) {
        const message =
          (err as { response?: { data?: { error?: { message?: string } } } })
            ?.response?.data?.error?.message ??
          (err as Error)?.message ??
          "Failed to load history.";
        setError(message);
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [customerId]
  );

  useEffect(() => {
    if (customerId) fetchHistory();
  }, [customerId, fetchHistory]);

  if (loading && data.length === 0) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
        <p className="mt-3 text-sm text-ink-600">Loading history…</p>
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-[15px] font-semibold text-ink-950">
            Customer History
          </h2>
          <p className="mt-0.5 text-sm text-ink-500">
            Immutable record of changes to this customer.
          </p>
        </div>
        <button
          onClick={() => fetchHistory({ silent: true })}
          disabled={refreshing}
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-surface text-ink-500 transition hover:bg-surface-2 disabled:opacity-50"
          aria-label="Refresh"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
        </button>
      </div>

      {error && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-red/30 bg-red-bg px-4 py-2.5 text-sm text-red">
          <span>{error}</span>
          <button
            onClick={() => fetchHistory()}
            className="whitespace-nowrap text-xs font-semibold underline"
          >
            Retry
          </button>
        </div>
      )}

      {data.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface">
          <EmptyState
            image="/images/alerts-empty.png"
            title="No history yet"
            description="Changes to this customer will appear here as they happen."
          />
        </div>
      ) : (
        <div className="space-y-3">
          {data.map((event, index) => (
            <div
              key={event.id}
              className="relative rounded-xl border border-line bg-surface p-4"
            >
              <div className="flex items-start gap-4">
                <div className="flex flex-col items-center">
                  <div
                    className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full ${
                      event.changeType === "ARCHIVED"
                        ? "bg-red-bg text-red"
                        : event.changeType === "CREATED"
                          ? "bg-green-bg text-green"
                          : event.changeType === "STATUS_CHANGED"
                            ? "bg-primary-bg text-primary"
                            : event.changeType === "RISK_CHANGED"
                              ? "bg-amber-bg text-amber"
                              : "bg-surface-2 text-ink-500"
                    }`}
                  >
                    {event.changeType === "ARCHIVED" ? (
                      <AlertTriangle className="h-4 w-4" />
                    ) : (
                      <Clock className="h-4 w-4" />
                    )}
                  </div>
                  {index < data.length - 1 && (
                    <div className="mt-1 h-full w-0.5 bg-line" />
                  )}
                </div>

                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">
                          {changeLabel(event.changeType)}
                        </span>
                        {event.reasonCode && (
                          <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                            {event.reasonCode}
                          </span>
                        )}
                      </div>

                      {(event.previousStatus || event.newStatus) && (
                        <div className="mt-1 flex items-center gap-2 text-xs">
                          {event.previousStatus && (
                            <span className="rounded-md bg-surface-2 px-2 py-0.5 text-ink-500">
                              {event.previousStatus}
                            </span>
                          )}
                          {event.previousStatus && event.newStatus && (
                            <ArrowRight className="h-3 w-3 text-ink-400" />
                          )}
                          {event.newStatus && (
                            <span className="rounded-md bg-primary-bg px-2 py-0.5 text-primary">
                              {event.newStatus}
                            </span>
                          )}
                        </div>
                      )}

                      {(event.previousRiskStatus || event.newRiskStatus) && (
                        <div className="mt-1 flex items-center gap-2 text-xs">
                          {event.previousRiskStatus && (
                            <span className="rounded-md bg-surface-2 px-2 py-0.5 text-ink-500">
                              {event.previousRiskStatus}
                            </span>
                          )}
                          {event.previousRiskStatus && event.newRiskStatus && (
                            <ArrowRight className="h-3 w-3 text-ink-400" />
                          )}
                          {event.newRiskStatus && (
                            <span className="rounded-md bg-amber-bg px-2 py-0.5 text-amber">
                              {event.newRiskStatus}
                            </span>
                          )}
                        </div>
                      )}

                      {event.changedFields.length > 0 && (
                        <div className="mt-1.5 flex flex-wrap gap-1">
                          {event.changedFields.map((field) => (
                            <span
                              key={field}
                              className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500"
                            >
                              {field}
                            </span>
                          ))}
                        </div>
                      )}

                      {event.approvalRequestId && (
                        <div className="mt-1 font-mono text-[10px] text-ink-400">
                          Approval: {event.approvalRequestId}
                        </div>
                      )}
                    </div>

                    <div className="flex-shrink-0 text-right">
                      <div className="flex items-center justify-end gap-1.5 text-xs text-ink-500">
                        <User className="h-3 w-3" />
                        <span>{actorLabel(event.actorType)}</span>
                      </div>
                      <div className="mt-0.5 text-[10px] text-ink-400">
                        {formatDateTime(event.occurredAt)}
                      </div>
                      <div className="text-[10px] text-ink-400">
                        {timeAgo(event.occurredAt)}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}