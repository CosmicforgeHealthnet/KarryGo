"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, RefreshCw } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import {
  DataTable,
  KeyValueGrid,
  Loading,
  PageHeader,
  Panel,
  SecondaryButton,
  StatGrid,
  StatusPill,
  Timeline,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateAssignmentService, privateOpsService } from "@/lib/services/operationsService";
import type { Order, OrderHistoryEntry, OrderStop, PrivateAssignment, Shipment } from "@/lib/types/operations";
import { CustodyPanel } from "../../../custody-panel";

// Execution view of one order: its stops, shipments, provider assignments
// and status history, as the backend records them.
export default function OrderExecutionPage() {
  const params = useParams();
  const router = useRouter();
  const orderId = params.orderId as string;

  const [order, setOrder] = useState<Order | null>(null);
  const [stops, setStops] = useState<OrderStop[]>([]);
  const [shipments, setShipments] = useState<Shipment[]>([]);
  const [assignments, setAssignments] = useState<PrivateAssignment[]>([]);
  const [history, setHistory] = useState<OrderHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [assignmentError, setAssignmentError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      setOrder(await privateOpsService.getOrder(orderId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The order could not be loaded."));
      setLoading(false);
      return;
    }
    privateOpsService.orderStops(orderId).then(setStops).catch(() => setStops([]));
    privateOpsService.orderHistory(orderId).then(setHistory).catch(() => setHistory([]));
    privateOpsService.listShipments({ orderId, limit: 100 }).then((p) => setShipments(p.items)).catch(() => setShipments([]));
    privateAssignmentService
      .list()
      .then((all) => {
        setAssignments(all.filter((a) => a.privateOrderId === orderId));
        setAssignmentError(null);
      })
      .catch((e) => {
        setAssignments([]);
        setAssignmentError(describeApiError(e, "Assignments could not be loaded."));
      });
    setLoading(false);
  }, [orderId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  if (loading) return <Loading label="Loading execution..." />;
  if (!order) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/private-ops/orders" className="text-sm text-primary">Back to orders</Link>
      </div>
    );
  }

  const completedStops = stops.filter((s) => s.status === "COMPLETED").length;

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Execution · ${order.orderNumber}`}
        description="Stops, shipments and provider assignments for this order."
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push(`/private-ops/orders/${order.id}`)}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back to order
            </SecondaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />

      <StatGrid
        items={[
          { label: "Order status", value: <StatusPill status={order.status} /> },
          { label: "Stops completed", value: `${completedStops} / ${stops.length}` },
          { label: "Shipments", value: shipments.length },
          { label: "Assignments", value: assignments.length, tone: "primary" },
        ]}
      />

      <KeyValueGrid
        items={[
          ["Requested start", formatWhen(order.requestedScheduleStart)],
          ["Requested end", formatWhen(order.requestedScheduleEnd)],
          ["Confirmed", formatWhen(order.confirmedAt)],
          ["Completed", formatWhen(order.completedAt)],
          ["Cancelled", formatWhen(order.cancelledAt)],
          ["Priority", humanize(order.priority)],
        ]}
      />

      <Panel title="Stops">
        {stops.length === 0 ? (
          <p className="text-sm text-ink-500">No stops added yet.</p>
        ) : (
          <DataTable
            minWidth={640}
            rows={stops}
            rowKey={(s) => s.id}
            columns={[
              { header: "#", cell: (s) => s.sequence },
              { header: "Type", cell: (s) => humanize(s.stopType) },
              { header: "Window", cell: (s) => `${formatWhen(s.timeWindowStart)} – ${formatWhen(s.timeWindowEnd)}` },
              { header: "Status", cell: (s) => <StatusPill status={s.status} /> },
            ]}
          />
        )}
      </Panel>

      <Panel title="Shipments">
        {shipments.length === 0 ? (
          <p className="text-sm text-ink-500">No shipments for this order yet.</p>
        ) : (
          <DataTable
            minWidth={640}
            rows={shipments}
            rowKey={(s) => s.id}
            onRowClick={(s) => router.push(`/private-ops/shipments/${s.id}`)}
            columns={[
              { header: "Shipment", cell: (s) => <span className="font-mono">{s.id}</span> },
              { header: "Packages", cell: (s) => s.packageCount },
              { header: "Vehicle class", cell: (s) => s.requiredVehicleClass || "—" },
              { header: "Status", cell: (s) => <StatusPill status={s.status} /> },
              { header: "Updated", cell: (s) => formatWhen(s.updatedAt) },
            ]}
          />
        )}
      </Panel>

      <Panel title="Provider assignments" description="Company providers offered or assigned this order's work.">
        <ApiErrorNotice error={assignmentError} />
        {assignments.length === 0 ? (
          <p className="text-sm text-ink-500">No provider assignments for this order yet.</p>
        ) : (
          <DataTable
            minWidth={760}
            rows={assignments}
            rowKey={(a) => a.id}
            columns={[
              { header: "Assignment", cell: (a) => <span className="font-mono">{a.id}</span> },
              { header: "Provider", cell: (a) => <span className="font-mono">{a.providerMembershipId || "—"}</span> },
              { header: "Vehicle", cell: (a) => a.vehicleOrTruckReference || "—" },
              { header: "Service", cell: (a) => humanize(a.executionService) },
              { header: "Status", cell: (a) => <StatusPill status={a.status} /> },
              { header: "Accepted", cell: (a) => formatWhen(a.acceptedAt) },
            ]}
          />
        )}
      </Panel>

      <CustodyPanel subjectType="ORDER" subjectReference={order.id} />

      <Panel title="Status history">
        <Timeline
          entries={history.map((h, i) => ({
            id: `${h.occurredAt}-${i}`,
            title: `${humanize(h.previousStatus)} → ${humanize(h.newStatus)}`,
            detail: [h.reasonCode, humanize(h.actorType)].filter(Boolean).join(" · "),
            at: h.occurredAt,
          }))}
        />
      </Panel>
    </div>
  );
}
