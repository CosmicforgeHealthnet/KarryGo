'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  Truck,
} from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import {
  formatDateTime,
  reconciliationLabel,
  reconciliationTone,
  transferStatusLabel,
  transferStatusTone,
} from '@/lib/utils/warehouseLabels';
import type { TerminalTransfer } from '@/lib/types/warehouse';

export default function TransferDetailPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;
  const transferId = params.transferId as string;

  const [transfer, setTransfer] = useState<TerminalTransfer | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!transferId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .getTerminalTransfer(transferId)
      .then((data) => {
        if (!cancelled) setTransfer(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load transfer.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [transferId]);

  async function refresh() {
    if (!transfer) return;
    const updated = await warehouseService.getTerminalTransfer(transfer.id);
    setTransfer(updated);
  }

  async function runCommand(
    fn: (
      id: string,
      version: number,
      key: string,
    ) => Promise<TerminalTransfer>,
  ) {
    if (!transfer) return;
    setBusy(true);
    setError(null);
    try {
      await fn(transfer.id, transfer.version, newWarehouseIdempotencyKey());
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

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading transfer…
      </div>
    );
  }

  if (error && !transfer) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href={`/warehouse/${warehouseId}/transfers`}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to transfers
        </Link>
      </div>
    );
  }

  if (!transfer) return null;

  const statusTone = transferStatusTone(transfer.status);
  const reconTone = reconciliationTone(transfer.reconciliationStatus);

  const canDepart =
    transfer.status === 'DRAFT' || transfer.status === 'READY';
  const canArrive =
    transfer.status === 'DEPARTED' || transfer.status === 'IN_TRANSIT';
  const canReconcile =
    transfer.status === 'ARRIVED' || transfer.status === 'DISCREPANCY';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Transfer ID', value: <span className="font-mono text-xs">{transfer.id}</span> },
    { label: 'Origin', value: <span className="font-mono text-xs">{transfer.originWarehouseId}</span> },
    { label: 'Destination', value: <span className="font-mono text-xs">{transfer.destinationWarehouseId}</span> },
    { label: 'Manifest', value: <span className="font-mono text-xs">{transfer.manifestId}</span> },
    { label: 'Route reference', value: transfer.routeReference || '—' },
    { label: 'Departed', value: formatDateTime(transfer.departedAt) },
    { label: 'Arrived', value: formatDateTime(transfer.arrivedAt) },
    { label: 'Origin snapshot', value: transfer.originSnapshotReference || '—' },
    { label: 'Dest. snapshot', value: transfer.destinationSnapshotReference || '—' },
    { label: 'Created', value: formatDateTime(transfer.createdAt) },
    { label: 'Updated', value: formatDateTime(transfer.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{transfer.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href={`/warehouse/${warehouseId}/transfers`}
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate font-mono">{transfer.id}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {transferStatusLabel(transfer.status)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${reconTone.bg} ${reconTone.text}`}
              >
                {reconciliationLabel(transfer.reconciliationStatus)}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {transfer.originWarehouseId} → {transfer.destinationWarehouseId}
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
        {canDepart && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.departTransfer(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Truck className="h-3.5 w-3.5" />
            Depart
          </button>
        )}
        {canArrive && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.arriveTransfer(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <CheckCircle2 className="h-3.5 w-3.5" />
            Arrive
          </button>
        )}
        {canReconcile && (
          <button
            onClick={() =>
              runCommand((id, v, k) =>
                warehouseService.reconcileTransfer(id, v, k),
              )
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <CheckCircle2 className="h-3.5 w-3.5" />
            Reconcile
          </button>
        )}
      </div>

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
    </div>
  );
}