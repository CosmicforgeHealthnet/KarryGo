"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { ClipboardList, Package, Plus, RefreshCw, Search, Truck } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import {
  DataTable,
  EmptyBlock,
  Loading,
  PageHeader,
  PrimaryButton,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { Order } from "@/lib/types/operations";

const STATUS_FILTERS = ["ALL", "DRAFT", "AWAITING_APPROVAL", "CONFIRMED", "PLANNED", "ASSIGNED", "IN_PROGRESS", "COMPLETED", "CANCELLED"];

// Private (company-owned) orders: drafts, approvals and execution status.
export default function OrdersPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Order[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [status, setStatus] = useState("ALL");
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string) => {
    setLoading(true);
    try {
      const page = await privateOpsService.listOrders({ status: s === "ALL" ? undefined : s, limit: 50 });
      setRows(page.items);
      setNextCursor(page.nextCursor);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Orders could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);

  const loadMore = async () => {
    if (!nextCursor) return;
    setLoadingMore(true);
    try {
      const page = await privateOpsService.listOrders({ status: status === "ALL" ? undefined : status, cursor: nextCursor, limit: 50 });
      setRows((r) => [...r, ...page.items]);
      setNextCursor(page.nextCursor);
    } catch (e) {
      setError(describeApiError(e, "More orders could not be loaded."));
    } finally {
      setLoadingMore(false);
    }
  };

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return q === "" ? rows : rows.filter((o) => [o.orderNumber, o.customerId, o.customerReference ?? "", o.status].join(" ").toLowerCase().includes(q));
  }, [rows, search]);

  if (loading && rows.length === 0 && !error) return <Loading label="Loading orders..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Orders"
        description="Company orders from draft and approval through to execution."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/private-ops/shipments")}>
              <Truck className="h-3.5 w-3.5" /> Shipments
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/private-ops/packages")}>
              <Package className="h-3.5 w-3.5" /> Custody
            </SecondaryButton>
            <PrimaryButton onClick={() => router.push("/private-ops/orders/create")}>
              <Plus className="h-3.5 w-3.5" /> New order
            </PrimaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />

      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "Drafts", value: rows.filter((o) => o.status === "DRAFT").length, tone: "amber" },
          { label: "Awaiting approval", value: rows.filter((o) => o.status === "AWAITING_APPROVAL").length, tone: "primary" },
          { label: "Confirmed", value: rows.filter((o) => o.status === "CONFIRMED").length, tone: "green" },
        ]}
      />

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 min-w-[200px] flex-1 items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder="Search by order number, customer..." className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400" />
        </div>
        <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
          {STATUS_FILTERS.map((s) => (
            <option key={s} value={s}>{s === "ALL" ? "All statuses" : humanize(s)}</option>
          ))}
        </select>
      </div>

      {filtered.length === 0 ? (
        <EmptyBlock icon={<ClipboardList className="h-8 w-8" />} title={rows.length === 0 ? "No orders yet" : "No orders found"} description={rows.length === 0 ? "Create an order for one of your CRM customers." : "No orders match your search."} />
      ) : (
        <DataTable
          rows={filtered}
          rowKey={(o) => o.id}
          onRowClick={(o) => router.push(`/private-ops/orders/${o.id}`)}
          columns={[
            { header: "Order", cell: (o) => <span className="font-mono text-ink-950">{o.orderNumber}</span> },
            { header: "Customer", cell: (o) => <span className="font-mono">{o.customerReference || o.customerId}</span> },
            { header: "Priority", cell: (o) => humanize(o.priority) },
            { header: "Scheduled", cell: (o) => formatWhen(o.requestedScheduleStart) },
            { header: "Status", cell: (o) => <StatusPill status={o.status} /> },
            { header: "Updated", cell: (o) => formatWhen(o.updatedAt) },
          ]}
        />
      )}

      {nextCursor && (
        <div className="flex justify-center">
          <SecondaryButton onClick={loadMore} disabled={loadingMore}>{loadingMore ? "Loading..." : "Load more orders"}</SecondaryButton>
        </div>
      )}
    </div>
  );
}
