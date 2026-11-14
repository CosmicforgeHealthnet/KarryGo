"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, RefreshCw, Truck } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import {
  DataTable,
  EmptyBlock,
  Loading,
  PageHeader,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatMinor,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { Shipment } from "@/lib/types/operations";

const STATUS_FILTERS = ["ALL", "DRAFT", "READY", "PLANNED", "ASSIGNED", "IN_TRANSIT", "COMPLETED", "CANCELLED"];

export default function ShipmentsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<Shipment[]>([]);
  const [nextCursor, setNextCursor] = useState<string | undefined>();
  const [status, setStatus] = useState("ALL");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async (s: string, cursor?: string) => {
    if (!cursor) setLoading(true);
    try {
      const page = await privateOpsService.listShipments({ status: s === "ALL" ? undefined : s, cursor, limit: 50 });
      setRows((r) => (cursor ? [...r, ...page.items] : page.items));
      setNextCursor(page.nextCursor);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Shipments could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(() => load(status));
  }, [load, status]);

  if (loading && rows.length === 0 && !error) return <Loading label="Loading shipments..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Shipments"
        description="What moves for each order, from draft to delivery."
        actions={
          <>
            <SecondaryButton onClick={() => load(status)}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/private-ops/orders")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Orders
            </SecondaryButton>
          </>
        }
      />
      <ApiErrorNotice error={error} />
      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "Draft", value: rows.filter((s) => s.status === "DRAFT").length, tone: "amber" },
          { label: "Ready", value: rows.filter((s) => s.status === "READY").length, tone: "green" },
          { label: "In transit", value: rows.filter((s) => s.status === "IN_TRANSIT").length, tone: "primary" },
        ]}
      />
      <select value={status} onChange={(e) => setStatus(e.target.value)} className="h-11 rounded-xl border border-line bg-white/70 px-3 text-sm text-ink-600 shadow-sm" aria-label="Status">
        {STATUS_FILTERS.map((s) => <option key={s} value={s}>{s === "ALL" ? "All statuses" : humanize(s)}</option>)}
      </select>
      {rows.length === 0 ? (
        <EmptyBlock icon={<Truck className="h-8 w-8" />} title="No shipments" description="Shipments appear here once orders have them." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(s) => s.id}
          onRowClick={(s) => router.push(`/private-ops/shipments/${s.id}`)}
          columns={[
            { header: "Shipment", cell: (s) => <span className="font-mono text-ink-950">{s.id}</span> },
            { header: "Order", cell: (s) => <span className="font-mono">{s.orderId}</span> },
            { header: "Packages", cell: (s) => s.packageCount },
            { header: "Declared value", cell: (s) => formatMinor(s.declaredValueAmountMinor, s.declaredValueCurrency) },
            { header: "Status", cell: (s) => <StatusPill status={s.status} /> },
            { header: "Updated", cell: (s) => formatWhen(s.updatedAt) },
          ]}
        />
      )}
      {nextCursor && (
        <div className="flex justify-center">
          <SecondaryButton onClick={() => load(status, nextCursor)}>Load more shipments</SecondaryButton>
        </div>
      )}
    </div>
  );
}
