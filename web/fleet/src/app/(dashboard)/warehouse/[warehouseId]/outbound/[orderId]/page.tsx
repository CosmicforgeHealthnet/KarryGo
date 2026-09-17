'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  Package,
  Play,
  Send,
  X,
} from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import {
  formatDateTime,
  fulfilmentStatusLabel,
  fulfilmentStatusTone,
  priorityLabel,
  priorityTone,
} from '@/lib/utils/warehouseLabels';
import type { FulfilmentOrder } from '@/lib/types/warehouse';

export default function FulfilmentOrderDetailPage() {
  const params = useParams();
  const router = useRouter();
  const warehouseId = params.warehouseId as string;
  const orderId = params.orderId as string;

  const [order, setOrder] = useState<FulfilmentOrder | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [dispatchForm, setDispatchForm] = useState({
    shipmentReference: '',
    manifestReference: '',
  });
  const [showDispatch, setShowDispatch] = useState(false);

  useEffect(() => {
    if (!orderId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .getFulfilmentOrder(orderId)
      .then((data) => {
        if (!cancelled) setOrder(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load order.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [orderId]);

  async function refresh() {
    if (!order) return;
    const updated = await warehouseService.getFulfilmentOrder(order.id);
    setOrder(updated);
  }

  async function runCommand(
    fn: (id: string, version: number, key: string) => Promise<FulfilmentOrder>,
  ) {
    if (!order) return;
    setBusy(true);
    setError(null);
    try {
      await fn(order.id, order.version, newWarehouseIdempotencyKey());
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Command failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleDispatch() {
    if (!order) return;
    setBusy(true);
    setError(null);
    try {
      await warehouseService.dispatchFulfilment(
        order.id,
        {
          expectedVersion: order.version,
          shipmentReference: dispatchForm.shipmentReference.trim() || undefined,
          manifestReference: dispatchForm.manifestReference.trim() || undefined,
        },
        newWarehouseIdempotencyKey(),
      );
      setShowDispatch(false);
      setDispatchForm({ shipmentReference: '', manifestReference: '' });
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Dispatch failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading order…
      </div>
    );
  }

  if (error && !order) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href={`/warehouse/${warehouseId}/outbound`}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to outbound
        </Link>
      </div>
    );
  }

  if (!order) return null;

  const statusTone = fulfilmentStatusTone(order.status);
  const priorityToneCls = priorityTone(order.priority);

  const canRelease = order.status === 'DRAFT';
  const canPick = order.status === 'RELEASED';
  const canPack = order.status === 'PICKED';
  const canDispatch = order.status === 'PACKED';
  const canCancel = order.status === 'DRAFT' || order.status === 'RELEASED';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Order ID', value: <span className="font-mono text-xs">{order.id}</span> },
    { label: 'Warehouse', value: <span className="font-mono text-xs">{order.warehouseId}</span> },
    { label: 'Customer ref', value: order.customerReference || '—' },
    { label: 'Source order', value: order.sourceOrderId || '—' },
    { label: 'Assigned team', value: order.assignedTeamReference || '—' },
    { label: 'Proof policy', value: order.proofPolicyReference || '—' },
    { label: 'Shipment ref', value: order.shipmentReference || '—' },
    { label: 'Manifest ref', value: order.manifestReference || '—' },
    {
      label: 'Requested dispatch',
      value: formatDateTime(order.requestedDispatchAt),
    },
    { label: 'Created', value: formatDateTime(order.createdAt) },
    { label: 'Updated', value: formatDateTime(order.updatedAt) },
    { label: 'Completed', value: formatDateTime(order.completedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{order.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href={`/warehouse/${warehouseId}/outbound`}
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate font-mono">{order.sourceOrderId || order.id}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {fulfilmentStatusLabel(order.status)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${priorityToneCls.bg} ${priorityToneCls.text}`}
              >
                {priorityLabel(order.priority)}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">
              {order.customerReference || 'No customer reference'}
            </p>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="flex flex-wrap gap-2.5">
        {canRelease && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.releaseFulfilment(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Play className="h-3.5 w-3.5" />
            Release
          </button>
        )}
        {canPick && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.pickFulfilment(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Package className="h-3.5 w-3.5" />
            Pick
          </button>
        )}
        {canPack && (
          <button
            onClick={() => {
              if (!order.lines) return;
              runCommand((id, v, k) =>
                warehouseService.packFulfilment(
                  id,
                  {
                    expectedVersion: v,
                    lines: order.lines!.map((l) => ({
                      inventoryItemId: l.inventoryItemId,
                      packageReference: l.packageReference || '',
                    })),
                  },
                  k,
                ),
              );
            }}
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Package className="h-3.5 w-3.5" />
            Pack
          </button>
        )}
        {canDispatch && !showDispatch && (
          <button
            onClick={() => setShowDispatch(true)}
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Send className="h-3.5 w-3.5" />
            Dispatch
          </button>
        )}
        {canCancel && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.cancelFulfilment(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg border border-red/30 bg-red-bg px-3.5 text-xs font-semibold text-red transition-colors hover:bg-red-bg/70 disabled:opacity-60"
          >
            <X className="h-3.5 w-3.5" />
            Cancel
          </button>
        )}
      </div>

      {showDispatch && (
        <div className="space-y-3 rounded-lg border border-line bg-surface p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Send className="h-4 w-4" />
            Dispatch order
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Shipment reference
              </label>
              <input
                value={dispatchForm.shipmentReference}
                onChange={(e) =>
                  setDispatchForm({
                    ...dispatchForm,
                    shipmentReference: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Manifest reference
              </label>
              <input
                value={dispatchForm.manifestReference}
                onChange={(e) =>
                  setDispatchForm({
                    ...dispatchForm,
                    manifestReference: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleDispatch}
              disabled={busy}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              {busy ? 'Dispatching…' : 'Confirm dispatch'}
            </button>
            <button
              onClick={() => setShowDispatch(false)}
              disabled={busy}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">Details</h3>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
          {rows.map((r) => (
            <div
              key={r.label}
              className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0"
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {r.label}
              </dt>
              <dd className="text-sm text-ink-950 text-right truncate">
                {r.value}
              </dd>
            </div>
          ))}
        </dl>
      </div>

      {order.lines && order.lines.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <Package className="h-4 w-4" />
            Order lines
            <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
              {order.lines.length}
            </span>
          </h3>
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] border-collapse text-left">
              <thead>
                <tr className="border-b border-line bg-surface-2">
                  <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                    Inventory item
                  </th>
                  <th className="px-4 py-3 text-right text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                    Requested
                  </th>
                  <th className="px-4 py-3 text-right text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                    Picked
                  </th>
                  <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500">
                    Package ref
                  </th>
                </tr>
              </thead>
              <tbody>
                {order.lines.map((line) => (
                  <tr
                    key={line.id}
                    className="border-b border-line last:border-0"
                  >
                    <td className="px-4 py-3.5">
                      <span className="font-mono text-xs text-ink-600">
                        {line.inventoryItemId}
                      </span>
                    </td>
                    <td className="px-4 py-3.5 text-right font-semibold text-ink-950">
                      {line.requestedQuantity}
                    </td>
                    <td className="px-4 py-3.5 text-right text-ink-700">
                      {line.pickedQuantity}
                    </td>
                    <td className="px-4 py-3.5">
                      <span className="font-mono text-xs text-ink-500">
                        {line.packageReference || '—'}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}