'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  Undo2,
} from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import {
  formatDateTime,
  returnStatusLabel,
  returnStatusTone,
} from '@/lib/utils/warehouseLabels';
import type { Return } from '@/lib/types/warehouse';

const DISPOSITIONS = [
  'RETURN_TO_STOCK',
  'QUARANTINE',
  'DISPOSE',
  'REPAIR',
  'SCRAP',
  'OTHER',
] as const;

export default function ReturnDetailPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;
  const returnId = params.returnId as string;

  const [record, setRecord] = useState<Return | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [showDecide, setShowDecide] = useState(false);
  const [decideForm, setDecideForm] = useState({
    disposition: 'RETURN_TO_STOCK' as string,
    reason: '',
  });

  useEffect(() => {
    if (!returnId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .getReturn(returnId)
      .then((data) => {
        if (!cancelled) setRecord(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load return.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [returnId]);

  async function refresh() {
    if (!record) return;
    const updated = await warehouseService.getReturn(record.id);
    setRecord(updated);
  }

  async function runCommand(
    fn: (id: string, version: number, key: string) => Promise<Return>,
  ) {
    if (!record) return;
    setBusy(true);
    setError(null);
    try {
      await fn(record.id, record.version, newWarehouseIdempotencyKey());
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

  async function handleDecide() {
    if (!record) return;
    setBusy(true);
    setError(null);
    try {
      await warehouseService.decideReturn(
        record.id,
        {
          expectedVersion: record.version,
          disposition: decideForm.disposition,
          reason: decideForm.reason.trim(),
        },
        newWarehouseIdempotencyKey(),
      );
      setShowDecide(false);
      setDecideForm({ disposition: 'RETURN_TO_STOCK', reason: '' });
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Decision failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading return…
      </div>
    );
  }

  if (error && !record) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href={`/warehouse/${warehouseId}/returns`}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to returns
        </Link>
      </div>
    );
  }

  if (!record) return null;

  const tone = returnStatusTone(record.status);

  const canReceive = record.status === 'REQUESTED';
  const canDecide =
    record.status === 'RECEIVED' || record.status === 'INSPECTING';
  const canComplete = record.status === 'DECIDED';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Return ID', value: <span className="font-mono text-xs">{record.id}</span> },
    { label: 'Warehouse', value: <span className="font-mono text-xs">{record.warehouseId}</span> },
    { label: 'Source order', value: record.sourceOrderId || '—' },
    { label: 'Source shipment', value: record.sourceShipmentId || '—' },
    { label: 'Package', value: record.packageId || '—' },
    { label: 'Reason code', value: <span className="font-mono text-xs">{record.reasonCode}</span> },
    { label: 'Condition', value: record.condition || '—' },
    { label: 'Disposition', value: record.disposition || '—' },
    { label: 'Financial ref', value: record.financialReference || '—' },
    { label: 'Inventory item', value: record.inventoryItemId || '—' },
    { label: 'Created', value: formatDateTime(record.createdAt) },
    { label: 'Updated', value: formatDateTime(record.updatedAt) },
    { label: 'Completed', value: formatDateTime(record.completedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{record.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/warehouse/${warehouseId}/returns`}
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="truncate font-mono">{record.id}</h1>
            <span
              className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
              {returnStatusLabel(record.status)}
            </span>
          </div>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {record.reasonCode}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="flex flex-wrap gap-2.5">
        {canReceive && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.receiveReturn(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Undo2 className="h-3.5 w-3.5" />
            Receive
          </button>
        )}
        {canDecide && !showDecide && (
          <button
            onClick={() => setShowDecide(true)}
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <CheckCircle2 className="h-3.5 w-3.5" />
            Decide
          </button>
        )}
        {canComplete && (
          <button
            onClick={() =>
              runCommand((id, v, k) => warehouseService.completeReturn(id, v, k))
            }
            disabled={busy}
            className="flex h-9 items-center gap-2 rounded-lg bg-primary px-3.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <CheckCircle2 className="h-3.5 w-3.5" />
            Complete
          </button>
        )}
      </div>

      {showDecide && (
        <div className="space-y-3 rounded-lg border border-line bg-surface p-4">
          <h3 className="text-sm font-semibold text-ink-950">
            Decide disposition
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Disposition
              </label>
              <select
                value={decideForm.disposition}
                onChange={(e) =>
                  setDecideForm({
                    ...decideForm,
                    disposition: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                {DISPOSITIONS.map((d) => (
                  <option key={d} value={d}>
                    {d}
                  </option>
                ))}
              </select>
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Reason
              </label>
              <input
                value={decideForm.reason}
                onChange={(e) =>
                  setDecideForm({ ...decideForm, reason: e.target.value })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleDecide}
              disabled={busy || !decideForm.reason.trim()}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              {busy ? 'Submitting…' : 'Confirm decision'}
            </button>
            <button
              onClick={() => setShowDecide(false)}
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
    </div>
  );
}