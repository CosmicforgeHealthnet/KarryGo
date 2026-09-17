'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  History,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  formatDateTime,
  formatNumber,
  movementTypeLabel,
  movementTypeTone,
} from '@/lib/utils/warehouseLabels';
import type { InventoryItem, Movement } from '@/lib/types/warehouse';

const TYPE_FILTERS = [
  'ALL',
  'RECEIPT',
  'ADJUSTMENT',
  'DISPATCH',
  'DAMAGE',
  'EXPIRY',
  'TRANSFER',
  'PUT_AWAY',
  'QUARANTINE',
  'RELEASE',
  'PICK',
  'PACK',
  'RETURN',
] as const;

type MovementRow = Movement & { item?: InventoryItem };

export default function MovementsPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [rows, setRows] = useState<MovementRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [type, setType] = useState<(typeof TYPE_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .listInventory()
      .then(async (items) => {
        if (cancelled) return;
        const scoped = items.filter((i) => i.warehouseId === warehouseId);
        const withMovements = await Promise.all(
          scoped.map(async (item) => {
            try {
              const movements = await warehouseService.listItemMovements(
                item.id,
              );
              return movements.map((m) => ({ ...m, item }));
            } catch {
              return [] as MovementRow[];
            }
          }),
        );
        if (cancelled) return;
        const flat = withMovements
          .flat()
          .sort(
            (a, b) =>
              new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime(),
          );
        setRows(flat);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load movements.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [warehouseId]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (type !== 'ALL' && row.movementType !== type) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.inventoryItemId,
        row.reasonCode,
        row.actorReference,
        row.item?.sku ?? '',
        row.item?.lotReference ?? '',
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, type]);

  const hasActiveFilters = type !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setType('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/warehouse/${warehouseId}/inventory`}
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Stock Movements</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Movement history across all items at this warehouse.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by SKU, reason or actor..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>

        <button
          onClick={() => setShowFilters((v) => !v)}
          className={`flex h-10 items-center gap-1.5 rounded-lg border px-3 text-sm font-medium transition-colors whitespace-nowrap ${
            showFilters || hasActiveFilters
              ? 'border-primary bg-primary-bg text-primary'
              : 'border-line bg-surface text-ink-700 hover:bg-surface-2'
          }`}
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
          Filters
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-10 items-center gap-1 rounded-lg border border-line bg-surface px-3 text-sm font-medium text-ink-500 transition-colors hover:bg-surface-2 whitespace-nowrap"
          >
            <X className="h-3.5 w-3.5" />
            Clear
          </button>
        )}

        <span className="ml-auto text-sm text-ink-500 whitespace-nowrap">
          {filtered.length} of {rows.length}
        </span>
      </div>

      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
            Type
          </span>
          <div className="flex flex-wrap gap-1.5">
            {TYPE_FILTERS.map((t) => (
              <button
                key={t}
                onClick={() => setType(t)}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                  type === t
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

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading movements…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No movements yet"
          description="Stock movements appear here as items are received, moved or dispatched."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No movements match your filters"
          description="Try adjusting your search or clearing the filters."
          action={
            <button
              onClick={clearFilters}
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Clear filters
            </button>
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-line bg-surface">
          <table className="w-full min-w-[900px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <Th>Type</Th>
                <Th>Item</Th>
                <Th>Quantity</Th>
                <Th>From → To</Th>
                <Th>Reason</Th>
                <Th>Date</Th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((m) => {
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
                      {m.item ? (
                        <Link
                          href={`/warehouse/${warehouseId}/inventory/${m.item.id}`}
                          className="font-mono text-xs text-primary hover:underline"
                        >
                          {m.item.sku}
                        </Link>
                      ) : (
                        <span className="font-mono text-xs text-ink-400">
                          {m.inventoryItemId}
                        </span>
                      )}
                    </Td>
                    <Td>
                      <span className="font-semibold text-ink-950">
                        {formatNumber(m.quantity)}
                      </span>{' '}
                      <span className="text-xs text-ink-400">
                        {m.item?.unit ?? ''}
                      </span>
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