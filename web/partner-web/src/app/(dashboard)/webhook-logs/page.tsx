"use client";

import { useState } from "react";
import Link from "next/link";
import { Search, SlidersHorizontal } from "lucide-react";
import { cn } from "@/lib/utils";

interface WebhookLog {
  id: string;
  timestamp: string;
  endpoint: string;
  event: string;
  deliveryId: string;
  status: "success" | "failed" | "pending";
  httpResponse: number;
}

const MOCK_LOGS: WebhookLog[] = [
  { id: "wh_1001", timestamp: "06 Mar 2026, 11:42", endpoint: "/webhooks/delivery-status", event: "delivery.created", deliveryId: "#1234445", status: "success", httpResponse: 200 },
  { id: "wh_1002", timestamp: "06 Mar 2026, 11:38", endpoint: "/webhooks/driver-assigned", event: "driver.assigned", deliveryId: "#1234446", status: "success", httpResponse: 200 },
  { id: "wh_1003", timestamp: "06 Mar 2026, 11:20", endpoint: "/webhooks/delivery-status", event: "delivery.failed", deliveryId: "#1234447", status: "failed", httpResponse: 500 },
  { id: "wh_1004", timestamp: "06 Mar 2026, 10:58", endpoint: "/webhooks/payment", event: "payment.captured", deliveryId: "#1234448", status: "success", httpResponse: 200 },
  { id: "wh_1005", timestamp: "06 Mar 2026, 10:41", endpoint: "/webhooks/delivery-status", event: "delivery.retrying", deliveryId: "#1234449", status: "pending", httpResponse: 408 },
  { id: "wh_1006", timestamp: "06 Mar 2026, 10:15", endpoint: "/webhooks/driver-assigned", event: "driver.reassigned", deliveryId: "#1234450", status: "failed", httpResponse: 404 },
  { id: "wh_1007", timestamp: "06 Mar 2026, 09:52", endpoint: "/webhooks/delivery-status", event: "delivery.completed", deliveryId: "#1234451", status: "success", httpResponse: 200 },
  { id: "wh_1008", timestamp: "06 Mar 2026, 09:30", endpoint: "/webhooks/payment", event: "payment.failed", deliveryId: "#1234452", status: "failed", httpResponse: 402 },
];

function StatusPill({ status }: { status: WebhookLog["status"] }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-semibold capitalize",
        status === "success" && "bg-brand-100 text-brand-700",
        status === "failed" && "bg-red-100 text-danger",
        status === "pending" && "bg-amber-100 text-warning"
      )}
    >
      {status}
    </span>
  );
}

export default function WebhookLogsPage() {
  const [query, setQuery] = useState("");

  const filtered = MOCK_LOGS.filter((log) =>
    [log.event, log.deliveryId, log.endpoint].some((field) =>
      field.toLowerCase().includes(query.toLowerCase())
    )
  );

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
                <th className="px-5 py-4">Endpoint</th>
                <th className="px-5 py-4">Event</th>
                <th className="px-5 py-4">Delivery ID</th>
                <th className="px-5 py-4">Status</th>
                <th className="px-5 py-4">HTTP Response</th>
                <th className="px-5 py-4 text-right">Actions</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((log) => (
                <tr key={log.id} className="border-b border-ink-100 last:border-0">
                  <td className="px-5 py-4 text-ink-700">{log.timestamp}</td>
                  <td className="px-5 py-4 text-ink-700">{log.endpoint}</td>
                  <td className="px-5 py-4 text-ink-700">{log.event}</td>
                  <td className="px-5 py-4 font-medium text-ink-900">{log.deliveryId}</td>
                  <td className="px-5 py-4"><StatusPill status={log.status} /></td>
                  <td className="px-5 py-4 text-ink-700">{log.httpResponse}</td>
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
                  <p className="text-sm font-semibold text-ink-900">{log.event}</p>
                  <p className="mt-0.5 text-xs text-ink-500">{log.timestamp}</p>
                </div>
                <StatusPill status={log.status} />
              </div>
              <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                <div>
                  <p className="text-ink-500">Delivery ID</p>
                  <p className="font-medium text-ink-900">{log.deliveryId}</p>
                </div>
                <div>
                  <p className="text-ink-500">HTTP Response</p>
                  <p className="font-medium text-ink-900">{log.httpResponse}</p>
                </div>
                <div className="col-span-2">
                  <p className="text-ink-500">Endpoint</p>
                  <p className="font-medium text-ink-900">{log.endpoint}</p>
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
      </div>
    </div>
  );
}