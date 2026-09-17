'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  History,
  Package,
} from 'lucide-react';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  formatDateTime,
  formatNumber,
  inventoryStatusLabel,
  inventoryStatusTone,
  movementTypeLabel,
  movementTypeTone,
} from '@/lib/utils/warehouseLabels';
import type { InventoryItem, Movement } from '@/lib/types/warehouse';

export default function InventoryItemDetailPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;
  const itemId = params.itemId as string;

  const [item, setItem] = useState<InventoryItem | null>(null);
  const [movements, setMovements] = useState<Movement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!itemId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([
      warehouseService.getInventoryItem(itemId),
      warehouseService.listItemMovements(itemId),
    ])
      .then(([i, m]) => {
        if (cancelled) return;
        setItem(i);
        setMovements(m);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load inventory item.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [itemId]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading item…
      </div>
    );
  }

  if (error && !item) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href={`/warehouse/${warehouseId}/inventory`}
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to inventory
        </Link>
      </div>
    );
  }

  if (!item) return null;

  const tone = inventoryStatusTone(item.status);
  const available = Math.max(
    0,
    item.quantityProjection - item.reservedQuantity,
  );

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Item ID', value: <span className="font-mono text-xs">{item.id}</span> },
    { label: 'SKU', value: <span className="font-mono text-xs">{item.sku}</span> },
    { label: 'Warehouse', value: <span className="font-mono text-xs">{item.warehouseId}</span> },
    { label: 'Location', value: <span className="font-mono text-xs">{item.locationId}</span> },
    { label: 'Owner type', value: item.ownerType },
    { label: 'Owner reference', value: <span className="font-mono text-xs">{item.ownerReference}</span> },
    { label: 'Lot', value: item.lotReference || '—' },
    { label: 'Batch', value: item.batchReference || '—' },
    { label: 'Serial', value: item.serialReference || '—' },
    { label: 'Unit', value: item.unit },
    { label: 'Condition', value: item.condition || '—' },
    { label: 'Expiry', value: formatDateTime(item.expiryAt) },
    { label: 'Created', value: formatDateTime(item.createdAt) },
    { label: 'Updated', value: formatDateTime(item.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{item.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/warehouse/${warehouseId}/inventory`}
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="truncate font-mono">{item.sku}</h1>
            <span
              className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
              {inventoryStatusLabel(item.status)}
            </span>
          </div>
          <p className="mt-0.5 font-mono text-xs text-ink-500">{item.id}</p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <QuantityCard
          label="Physical"
          value={item.quantityProjection}
          unit={item.unit}
          tone="blue"
        />
        <QuantityCard
          label="Reserved"
          value={item.reservedQuantity}
          unit={item.unit}
          tone="amber"
        />
        <QuantityCard
          label="Available"
          value={available}
          unit={item.unit}
          tone="green"
        />
      </div>

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
          <Package className="h-4 w-4" />
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

      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
            <History className="h-4 w-4" />
            Movement history
          </h3>
          <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
            {movements.length}
          </span>
        </div>

        {movements.length === 0 ? (
          <p className="text-sm text-ink-500">No movements recorded yet.</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[800px] border-collapse text-left">
              <thead>
                <tr className="border-b border-line bg-surface-2">
                  <Th>Type</Th>
                  <Th>Quantity</Th>
                  <Th>From → To</Th>
                  <Th>Reason</Th>
                  <Th>By</Th>
                  <Th>Date</Th>
                </tr>
              </thead>
              <tbody>
                {movements.map((m) => {
                  const mTone = movementTypeTone(m.movementType);
                  return (
                    <tr
                      key={m.id}
                      className="border-b border-line last:border-0 hover:bg-surface-2"
                    >
                      <Td>
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${mTone.bg} ${mTone.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${mTone.dot}`} />
                          {movementTypeLabel(m.movementType)}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-semibold text-ink-950">
                          {formatNumber(m.quantity)}
                        </span>{' '}
                        <span className="text-xs text-ink-400">{item.unit}</span>
                      </Td>
                      <Td>
                        <span className="font-mono text-xs text-ink-600">
                          {m.fromLocationId || '—'} → {m.toLocationId || '—'}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-mono text-[11px] text-ink-600">
                          {m.reasonCode}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-mono text-[11px] text-ink-500">
                          {m.actorReference || '—'}
                        </span>
                      </Td>
                      <Td>
                        <span className="text-xs text-ink-500">
                          {formatDateTime(m.createdAt)}
                        </span>
                      </Td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  );
}

function QuantityCard({
  label,
  value,
  unit,
  tone,
}: {
  label: string;
  value: number;
  unit: string;
  tone: 'green' | 'amber' | 'blue';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    amber: 'bg-amber-bg text-amber',
    blue: 'bg-primary-bg text-primary',
  }[tone];

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
        {label}
      </div>
      <div className={`mt-1 text-2xl font-bold ${cls.split(' ')[1]}`}>
        {formatNumber(value)}
      </div>
      <div className="text-xs text-ink-500">{unit}</div>
    </div>
  );
}

function Th({ children }: { children: React.ReactNode }) {
  return (
    <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
      {children}
    </th>
  );
}

function Td({ children }: { children: React.ReactNode }) {
  return <td className="px-4 py-3.5 text-sm text-ink-700">{children}</td>;
}