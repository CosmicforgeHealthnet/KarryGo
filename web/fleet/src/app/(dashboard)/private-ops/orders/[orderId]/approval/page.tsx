"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { KeyValueGrid, Loading, Notice, PageHeader, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import type { Order } from "@/lib/types/operations";
import { OrderActions } from "../../order-actions";

// The approval view of one order: what is being approved and the decision.
export default function OrderApprovalPage() {
  const params = useParams();
  const orderId = params.orderId as string;
  const [order, setOrder] = useState<Order | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setOrder(await privateOpsService.getOrder(orderId));
    } catch (e) {
      setError(describeApiError(e, "The order could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [orderId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  if (loading) return <Loading label="Loading order..." />;

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <PageHeader
        title={order ? `Approve order ${order.orderNumber}` : "Order approval"}
        actions={
          <Link href={`/private-ops/orders/${orderId}`} className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <ArrowLeft className="h-3.5 w-3.5" /> Order
          </Link>
        }
      />
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {order && (
        <>
          <KeyValueGrid
            items={[
              ["Status", <StatusPill key="s" status={order.status} />],
              ["Customer", order.customerReference || order.customerId],
              ["Priority", humanize(order.priority)],
              ["Requested start", formatWhen(order.requestedScheduleStart)],
              ["Requested end", formatWhen(order.requestedScheduleEnd)],
              ["Version", order.version],
            ]}
          />
          <OrderActions
            order={order}
            onChanged={(o, message) => {
              setOrder(o);
              setNotice(message);
            }}
          />
          {order.status !== "DRAFT" && order.status !== "AWAITING_APPROVAL" && (
            <p className="text-sm text-ink-500">This order is {humanize(order.status).toLowerCase()} and needs no approval decision.</p>
          )}
        </>
      )}
    </div>
  );
}
