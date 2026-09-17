'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, Edit3, Package, Plus } from 'lucide-react';
import { partService } from '@/lib/services/partService';
import {
  formatDateTime,
  formatNumber,
  movementTypeLabel,
  movementTypeTone,
  partStatusLabel,
  partStatusTone,
  stockStateLabel,
} from '@/lib/utils/partLabels';
import type { Part, StockMovement } from '@/lib/types/part';

export default function PartDetailPage() {
  const params = useParams();
  const partId = params.partId as string;

  const [part, setPart] = useState<Part | null>(null);
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!partId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([
      partService.get(partId),
      partService.listMovements(),
    ])
      .then(([p, allMovements]) => {
        if (cancelled) return;
        setPart(p);
        setMovements(allMovements.filter((m) => m.partId === p.id));
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load part.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [partId]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading part…
      </div>
    );
  }

  if (error && !part) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/parts"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to parts
        </Link>
      </div>
    );
  }

  if (!part) return null;

  const statusTone = partStatusTone(part.status);
  const stockInfo = stockStateLabel(part);

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Part ID', value: <span className="font-mono text-xs">{part.id}</span> },
    { label: 'SKU', value: <span className="font-mono text-xs">{part.sku}</span> },
    { label: 'Name', value: part.name },
    { label: 'Branch', value: <span className="font-mono text-xs">{part.branchId}</span> },
    { label: 'Stock unit', value: part.stockUnit || '—' },
    { label: 'Current quantity', value: formatNumber(part.currentQuantityProjection) },
    { label: 'Reorder level', value: formatNumber(part.reorderLevel) },
    { label: 'Warranty info', value: part.warrantyInformation || '—' },
    { label: 'Created', value: formatDateTime(part.createdAt) },
    { label: 'Updated', value: formatDateTime(part.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{part.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/parts"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{part.name}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {partStatusLabel(part.status)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${stockInfo.tone.bg} ${stockInfo.tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${stockInfo.tone.dot}`} />
                {stockInfo.label}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {part.sku} · {part.branchId}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Link
            href={`/fleet/parts/${part.id}/edit`}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </Link>
          <Link
            href={`/fleet/parts/${part.id}/movement/add`}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Record movement
          </Link>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

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

      <div className="rounded-lg border border-line bg-surface">
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <h3 className="text-sm font-semibold text-ink-950">
            Recent movements
          </h3>
          <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
            {movements.length}
          </span>
        </div>

        {movements.length === 0 ? (
          <div className="px-4 py-8 text-center text-sm text-ink-500">
            No stock movements recorded yet.
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[720px] border-collapse text-left">
              <thead>
                <tr className="border-b border-line bg-surface-2">
                  <Th>Type</Th>
                  <Th>Quantity</Th>
                  <Th>Reason</Th>
                  <Th>Location</Th>
                  <Th>By</Th>
                  <Th>Date</Th>
                </tr>
              </thead>
              <tbody>
                {movements.map((m) => {
                  const tone = movementTypeTone(m.movementType);
                  return (
                    <tr
                      key={m.id}
                      className="border-b border-line last:border-0 hover:bg-surface-2"
                    >
                      <Td>
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                          {movementTypeLabel(m.movementType)}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-semibold text-ink-950">
                          {formatNumber(m.quantity)}
                        </span>{' '}
                        <span className="text-ink-400">{part.stockUnit}</span>
                      </Td>
                      <Td>
                        <span className="text-xs text-ink-600 truncate max-w-[200px] inline-block">
                          {m.reason}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-mono text-xs text-ink-500">
                          {m.locationReference || '—'}
                        </span>
                      </Td>
                      <Td>
                        <span className="font-mono text-xs text-ink-500">
                          {m.createdBy || '—'}
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