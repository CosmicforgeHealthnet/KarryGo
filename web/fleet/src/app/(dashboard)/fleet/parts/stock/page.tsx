'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  ArrowLeft,
  ArrowRight,
  Plus,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { partService } from '@/lib/services/partService';
import {
  formatDateTime,
  formatNumber,
  movementTypeLabel,
  movementTypeTone,
  stockStateLabel,
} from '@/lib/utils/partLabels';
import type { Part, StockMovement } from '@/lib/types/part';

const MOVEMENT_FILTERS = [
  'ALL',
  'RECEIVE',
  'ISSUE',
  'ADJUST',
  'TRANSFER_IN',
  'TRANSFER_OUT',
  'RETURN',
] as const;

export default function PartsStockPage() {
  const [parts, setParts] = useState<Part[]>([]);
  const [movements, setMovements] = useState<StockMovement[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [movementType, setMovementType] =
    useState<(typeof MOVEMENT_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    Promise.all([partService.list(), partService.listMovements()])
      .then(([p, m]) => {
        if (cancelled) return;
        setParts(p);
        setMovements(m);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load stock data.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const partLookup = useMemo(() => {
    const map = new Map<string, Part>();
    for (const p of parts) map.set(p.id, p);
    return map;
  }, [parts]);

  const stats = useMemo(() => {
    let total = 0;
    let lowOrOut = 0;
    for (const p of parts) {
      total += p.currentQuantityProjection;
      if (p.currentQuantityProjection <= p.reorderLevel) lowOrOut++;
    }
    return { total, lowOrOut, movementCount: movements.length };
  }, [parts, movements]);

  const filteredMovements = useMemo(() => {
    const q = search.trim().toLowerCase();
    return movements.filter((row) => {
      if (movementType !== 'ALL' && row.movementType !== movementType) {
        return false;
      }
      if (q === '') return true;
      const part = partLookup.get(row.partId);
      const haystack = [
        row.id,
        row.partId,
        part?.sku ?? '',
        part?.name ?? '',
        row.reason,
        row.locationReference,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [movements, search, movementType, partLookup]);

  const hasActiveFilters =
    movementType !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setMovementType('ALL');
  };

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
          <div>
            <h1>Stock</h1>
            <p className="mt-0.5 text-sm text-ink-500">
              Current stock levels and movement history.
            </p>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Total stock" value={stats.total} tone="green" />
        <StatCard label="Low / out" value={stats.lowOrOut} tone="amber" />
        <StatCard label="Movements" value={stats.movementCount} tone="blue" />
      </div>

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading stock data…
        </div>
      ) : (
        <>
          <div className="rounded-lg border border-line bg-surface">
            <div className="flex items-center justify-between border-b border-line px-4 py-3">
              <h3 className="text-sm font-semibold text-ink-950">
                Current stock levels
              </h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {parts.length} item{parts.length === 1 ? '' : 's'}
              </span>
            </div>

            {parts.length === 0 ? (
              <div className="px-4 py-10 text-center text-sm text-ink-500">
                No parts yet.
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[800px] border-collapse text-left">
                  <thead>
                    <tr className="border-b border-line bg-surface-2">
                      <Th>Part</Th>
                      <Th>SKU</Th>
                      <Th>Quantity</Th>
                      <Th>Reorder</Th>
                      <Th>Stock state</Th>
                      <Th />
                    </tr>
                  </thead>
                  <tbody>
                    {parts.map((p) => {
                      const info = stockStateLabel(p);
                      return (
                        <tr
                          key={p.id}
                          className="border-b border-line last:border-0 hover:bg-surface-2"
                        >
                          <Td>
                            <div className="font-medium text-ink-950">
                              {p.name}
                            </div>
                            <div className="font-mono text-[10px] text-ink-400">
                              {p.id}
                            </div>
                          </Td>
                          <Td>
                            <span className="font-mono text-xs text-ink-600">
                              {p.sku}
                            </span>
                          </Td>
                          <Td>
                            <span className="font-semibold text-ink-950">
                              {formatNumber(p.currentQuantityProjection)}
                            </span>{' '}
                            <span className="text-ink-400">{p.stockUnit}</span>
                          </Td>
                          <Td>
                            <span className="text-ink-500">
                              {formatNumber(p.reorderLevel)}
                            </span>
                          </Td>
                          <Td>
                            <span
                              className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${info.tone.bg} ${info.tone.text}`}
                            >
                              <span
                                className={`h-1.5 w-1.5 rounded-full ${info.tone.dot}`}
                              />
                              {info.label}
                            </span>
                          </Td>
                          <Td className="text-right">
                            <Link
                              href={`/fleet/parts/${p.id}`}
                              className="inline-flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors hover:text-primary"
                            >
                              View <ArrowRight className="h-3.5 w-3.5" />
                            </Link>
                          </Td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>

          <div className="rounded-lg border border-line bg-surface">
            <div className="flex flex-col gap-3 border-b border-line px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
              <h3 className="text-sm font-semibold text-ink-950">
                Movement history
              </h3>
              <div className="flex flex-wrap items-center gap-2">
                <div className="flex h-9 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:w-60">
                  <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
                  <input
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search movements..."
                    className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
                  />
                </div>
                <button
                  onClick={() => setShowFilters((v) => !v)}
                  className={`flex h-9 items-center gap-1.5 rounded-lg border px-3 text-xs font-medium transition-colors whitespace-nowrap ${
                    showFilters || hasActiveFilters
                      ? 'border-primary bg-primary-bg text-primary'
                      : 'border-line bg-surface text-ink-700 hover:bg-surface-2'
                  }`}
                >
                  <SlidersHorizontal className="h-3.5 w-3.5" />
                  Type
                </button>
                {hasActiveFilters && (
                  <button
                    onClick={clearFilters}
                    className="flex h-9 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-xs font-medium text-ink-500 transition-colors hover:bg-surface-2 whitespace-nowrap"
                  >
                    <X className="h-3.5 w-3.5" />
                    Clear
                  </button>
                )}
              </div>
            </div>

            {showFilters && (
              <div className="border-b border-line px-4 py-3">
                <div className="flex flex-wrap gap-1.5">
                  {MOVEMENT_FILTERS.map((t) => (
                    <button
                      key={t}
                      onClick={() => setMovementType(t)}
                      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                        movementType === t
                          ? 'bg-primary text-white'
                          : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
                      }`}
                    >
                      {t === 'ALL' ? 'All' : movementTypeLabel(t)}
                    </button>
                  ))}
                </div>
              </div>
            )}

            {filteredMovements.length === 0 ? (
              <EmptyState
                image="/images/trips-empty.png"
                title={
                  movements.length === 0
                    ? 'No movements yet'
                    : 'No movements match your filters'
                }
                description={
                  movements.length === 0
                    ? 'Stock movements appear here as parts are received or issued.'
                    : 'Try adjusting your search or clearing the filters.'
                }
                action={
                  movements.length > 0 ? (
                    <button
                      onClick={clearFilters}
                      className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
                    >
                      Clear filters
                    </button>
                  ) : undefined
                }
              />
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full min-w-[900px] border-collapse text-left">
                  <thead>
                    <tr className="border-b border-line bg-surface-2">
                      <Th>Type</Th>
                      <Th>Part</Th>
                      <Th>Quantity</Th>
                      <Th>Reason</Th>
                      <Th>Location</Th>
                      <Th>Recorded</Th>
                    </tr>
                  </thead>
                  <tbody>
                    {filteredMovements.map((m) => {
                      const tone = movementTypeTone(m.movementType);
                      const p = partLookup.get(m.partId);
                      return (
                        <tr
                          key={m.id}
                          className="border-b border-line last:border-0 hover:bg-surface-2"
                        >
                          <Td>
                            <span
                              className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                            >
                              <span
                                className={`h-1.5 w-1.5 rounded-full ${tone.dot}`}
                              />
                              {movementTypeLabel(m.movementType)}
                            </span>
                          </Td>
                          <Td>
                            {p ? (
                              <>
                                <div className="text-sm font-medium text-ink-950">
                                  {p.name}
                                </div>
                                <div className="font-mono text-[10px] text-ink-400">
                                  {p.sku}
                                </div>
                              </>
                            ) : (
                              <span className="font-mono text-xs text-ink-500">
                                {m.partId}
                              </span>
                            )}
                          </Td>
                          <Td>
                            <span className="font-semibold text-ink-950">
                              {formatNumber(m.quantity)}
                            </span>{' '}
                            <span className="text-ink-400">
                              {p?.stockUnit ?? ''}
                            </span>
                          </Td>
                          <Td>
                            <span className="text-xs text-ink-600 truncate max-w-[220px] inline-block">
                              {m.reason}
                            </span>
                          </Td>
                          <Td>
                            <span className="font-mono text-xs text-ink-500">
                              {m.locationReference || '—'}
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
        </>
      )}
    </div>
  );
}

function StatCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: number;
  tone: 'green' | 'amber' | 'blue';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    amber: 'bg-amber-bg text-amber',
    blue: 'bg-primary-bg text-primary',
  }[tone];

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="flex items-center gap-2.5">
        <div className={`flex h-8 w-8 items-center justify-center rounded-md ${cls}`}>
          <span className="text-sm font-bold">{value}</span>
        </div>
        <span className="text-xs font-semibold text-ink-600">{label}</span>
      </div>
    </div>
  );
}

function Th({ children }: { children?: React.ReactNode }) {
  return (
    <th className="px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap">
      {children}
    </th>
  );
}

function Td({ children }: { children: React.ReactNode }) {
  return <td className="px-4 py-3.5 text-sm text-ink-700">{children}</td>;
}