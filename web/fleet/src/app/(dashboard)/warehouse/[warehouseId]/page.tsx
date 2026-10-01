'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  Archive,
  Clock,
  MapPin,
  Warehouse as WarehouseIcon,
} from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import {
  formatDateTime,
  warehouseStatusLabel,
  warehouseStatusTone,
} from '@/lib/utils/warehouseLabels';
import type { Warehouse } from '@/lib/types/warehouse';

export default function WarehouseDetailPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [warehouse, setWarehouse] = useState<Warehouse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirmArchive, setConfirmArchive] = useState(false);

  useEffect(() => {
    if (!warehouseId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .getWarehouse(warehouseId)
      .then((data) => {
        if (!cancelled) setWarehouse(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load warehouse.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [warehouseId]);

  async function handleArchive() {
    if (!warehouse) return;
    setBusy(true);
    setError(null);
    try {
      await warehouseService.archiveWarehouse(
        warehouse.id,
        warehouse.version,
      );
      const refreshed = await warehouseService.getWarehouse(warehouse.id);
      setWarehouse(refreshed);
      setConfirmArchive(false);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to archive warehouse.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading warehouse…
      </div>
    );
  }

  if (error && !warehouse) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/warehouse"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to warehouses
        </Link>
      </div>
    );
  }

  if (!warehouse) return null;

  const tone = warehouseStatusTone(warehouse.status);
  const canArchive = warehouse.status !== 'ARCHIVED';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Warehouse ID', value: <span className="font-mono text-xs">{warehouse.id}</span> },
    { label: 'Code', value: <span className="font-mono text-xs">{warehouse.code}</span> },
    { label: 'Name', value: warehouse.name },
    {
      label: 'Branch',
      value: <span className="font-mono text-xs">{warehouse.branchId}</span>,
    },
    { label: 'Site reference', value: warehouse.siteReference || '—' },
    {
      label: 'Address reference',
      value: warehouse.addressReference || '—',
    },
    { label: 'Timezone', value: warehouse.timezone },
    { label: 'Created', value: formatDateTime(warehouse.createdAt) },
    { label: 'Updated', value: formatDateTime(warehouse.updatedAt) },
    {
      label: 'Archived',
      value: warehouse.archivedAt ? formatDateTime(warehouse.archivedAt) : '—',
    },
    {
      label: 'Version',
      value: <span className="font-mono text-xs">{warehouse.version}</span>,
    },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/warehouse"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{warehouse.name}</h1>
              <span className="font-mono text-xs text-ink-400">
                {warehouse.code}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                {warehouseStatusLabel(warehouse.status)}
              </span>
            </div>
            <p className="mt-0.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500">
              <span className="flex items-center gap-1.5">
                <MapPin className="h-3.5 w-3.5" />
                <span className="font-mono">{warehouse.branchId}</span>
              </span>
              <span className="flex items-center gap-1.5">
                <Clock className="h-3.5 w-3.5" />
                {warehouse.timezone}
              </span>
            </p>
          </div>
        </div>
        {canArchive && (
          <button
            onClick={() => setConfirmArchive(true)}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-amber/30 bg-amber-bg px-3.5 text-sm font-semibold text-amber transition-colors hover:bg-amber-bg/70"
          >
            <Archive className="h-3.5 w-3.5" />
            Archive
          </button>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {confirmArchive && (
        <div className="rounded-lg border border-amber/30 bg-amber-bg/20 p-4">
          <div className="flex items-start gap-3">
            <AlertCircle className="h-4 w-4 flex-shrink-0 text-amber mt-0.5" />
            <div className="flex-1">
              <p className="text-sm font-semibold text-ink-950">
                Archive this warehouse?
              </p>
              <p className="mt-0.5 text-xs text-ink-600">
                A warehouse with operational stock cannot be archived. The
                status will change to Archived and it will be hidden from
                active views.
              </p>
              <div className="mt-3 flex gap-2">
                <button
                  onClick={handleArchive}
                  disabled={busy}
                  className="rounded-md bg-amber px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-amber/90 disabled:opacity-60"
                >
                  {busy ? 'Archiving…' : 'Confirm archive'}
                </button>
                <button
                  onClick={() => setConfirmArchive(false)}
                  disabled={busy}
                  className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
                >
                  Cancel
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
          <WarehouseIcon className="h-4 w-4" />
          Details
        </h3>
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