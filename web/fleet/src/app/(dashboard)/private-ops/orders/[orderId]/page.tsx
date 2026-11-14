"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Edit3, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  Timeline,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import { ORDER_PRIORITIES, type Order, type OrderHistoryEntry, type OrderStop, type Shipment } from "@/lib/types/operations";
import { CustodyPanel } from "../../custody-panel";
import { OrderActions } from "../order-actions";
import { SpecializedDataPanel } from "../specialized-panel";

const toLocal = (iso?: string) => {
  if (!iso) return "";
  const d = new Date(iso);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  return d.toISOString().slice(0, 16);
};

export default function OrderDetailPage() {
  const params = useParams();
  const router = useRouter();
  const orderId = params.orderId as string;

  const [order, setOrder] = useState<Order | null>(null);
  const [history, setHistory] = useState<OrderHistoryEntry[]>([]);
  const [stops, setStops] = useState<OrderStop[]>([]);
  const [shipments, setShipments] = useState<Shipment[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [editing, setEditing] = useState(false);
  const [edit, setEdit] = useState({ customerReference: "", priority: "STANDARD", start: "", end: "", specialInstructions: "" });
  const [saving, setSaving] = useState(false);
  const [editError, setEditError] = useState<ApiErrorInfo | null>(null);

  const loadRelated = useCallback(() => {
    privateOpsService.orderHistory(orderId).then(setHistory).catch(() => setHistory([]));
    privateOpsService.orderStops(orderId).then(setStops).catch(() => setStops([]));
    privateOpsService.listShipments({ orderId, limit: 100 }).then((p) => setShipments(p.items)).catch(() => setShipments([]));
  }, [orderId]);

  const load = useCallback(async () => {
    try {
      setOrder(await privateOpsService.getOrder(orderId));
      setError(null);
      loadRelated();
    } catch (e) {
      setError(describeApiError(e, "The order could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [orderId, loadRelated]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const saveEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!order) return;
    setSaving(true);
    setEditError(null);
    try {
      const updated = await privateOpsService.updateOrder(
        order.id,
        {
          customerReference: edit.customerReference.trim(),
          priority: edit.priority,
          requestedScheduleStart: edit.start ? new Date(edit.start).toISOString() : undefined,
          requestedScheduleEnd: edit.end ? new Date(edit.end).toISOString() : undefined,
          specialInstructions: edit.specialInstructions.trim(),
        },
        order.version,
      );
      setOrder(updated);
      setEditing(false);
      setNotice("Order updated.");
    } catch (err) {
      setEditError(describeApiError(err, "The order could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <Loading label="Loading order..." />;
  if (!order) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/private-ops/orders" className="text-sm text-primary">Back to orders</Link>
      </div>
    );
  }

  const editable = order.status === "DRAFT" || order.status === "AWAITING_APPROVAL";

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Order ${order.orderNumber}`}
        description={`Created ${formatWhen(order.createdAt)} · ${humanize(order.source)}`}
        actions={
          <>
            {editable && !editing && (
              <SecondaryButton
                onClick={() => {
                  setEdit({
                    customerReference: order.customerReference ?? "",
                    priority: order.priority,
                    start: toLocal(order.requestedScheduleStart),
                    end: toLocal(order.requestedScheduleEnd),
                    specialInstructions: order.specialInstructions ?? "",
                  });
                  setEditError(null);
                  setEditing(true);
                }}
              >
                <Edit3 className="h-3.5 w-3.5" /> Edit order
              </SecondaryButton>
            )}
            <SecondaryButton onClick={() => router.push("/private-ops/orders")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </SecondaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={order.status} />],
          ["Customer", order.customerId],
          ["Customer reference", order.customerReference ?? "—"],
          ["Priority", humanize(order.priority)],
          ["Requested start", formatWhen(order.requestedScheduleStart)],
          ["Requested end", formatWhen(order.requestedScheduleEnd)],
          ["Timezone", order.timezone],
          ["Service", order.serviceCatalogItemId],
          ["Version", order.version],
        ]}
      />
      {order.specialInstructions && <p className="rounded-xl border border-line bg-white p-4 text-sm text-ink-700">{order.specialInstructions}</p>}

      <OrderActions
        order={order}
        onChanged={(o, message) => {
          setOrder(o);
          setNotice(message);
          loadRelated();
        }}
      />

      {editing && (
        <Panel title="Edit order" onClose={() => setEditing(false)}>
          <form onSubmit={saveEdit} className="space-y-4">
            <ApiErrorNotice error={editError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Customer reference">
                <input value={edit.customerReference} onChange={(e) => setEdit({ ...edit, customerReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Priority">
                <select value={edit.priority} onChange={(e) => setEdit({ ...edit, priority: e.target.value })} className={INPUT}>
                  {ORDER_PRIORITIES.map((p) => <option key={p} value={p}>{humanize(p)}</option>)}
                </select>
              </Field>
              <Field label="Requested start">
                <input type="datetime-local" value={edit.start} onChange={(e) => setEdit({ ...edit, start: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Requested end">
                <input type="datetime-local" value={edit.end} onChange={(e) => setEdit({ ...edit, end: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Special instructions">
                  <textarea rows={3} value={edit.specialInstructions} onChange={(e) => setEdit({ ...edit, specialInstructions: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save order"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

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
              { header: "Instructions", cell: (s) => s.instructions || "—" },
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
            ]}
          />
        )}
      </Panel>

      <SpecializedDataPanel orderId={order.id} />
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
