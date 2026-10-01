'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  ArrowRight,
  Search,
  SlidersHorizontal,
  Warehouse as WarehouseIcon,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  formatDateTime,
  reconciliationLabel,
  reconciliationTone,
  transferStatusLabel,
  transferStatusTone,
} from '@/lib/utils/warehouseLabels';
import type { TerminalTransfer } from '@/lib/types/warehouse';

const STATUS_FILTERS = [
  'ALL',
  'DRAFT',
  'READY',
  'DEPARTED',
  'IN_TRANSIT',
  'ARRIVED',
  'COMPLETED',
  'DISCREPANCY',
] as const;

export default function TransfersPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [rows, setRows] = useState<TerminalTransfer[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .listTerminalTransfers()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load terminal transfers.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const warehouseRows = useMemo(
    () =>
      rows.filter(
        (r) =>
          r.originWarehouseId === warehouseId ||
          r.destinationWarehouseId === warehouseId,
      ),
    [rows, warehouseId],
  );

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return warehouseRows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.manifestId,
        row.originWarehouseId,
        row.destinationWarehouseId,
        row.routeReference,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [warehouseRows, search, status]);

  const hasActiveFilters = status !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/warehouse/${warehouseId}`}
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Terminal Transfers</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Inter-warehouse transfers with manifest and reconciliation state.
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
            placeholder="Search by manifest, route or ID..."
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
          {filtered.length} of {warehouseRows.length}
        </span>
      </div>

      {showFilters && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
            Status
          </span>
          <div className="flex flex-wrap gap-1.5">
            {STATUS_FILTERS.map((s) => (
              <button
                key={s}
                onClick={() => setStatus(s)}
                className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
                  status === s
                    ? 'bg-primary text-white'
                    : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
                }`}
              >
                {s === 'ALL' ? 'All' : transferStatusLabel(s)}
              </button>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading transfers…
        </div>
      ) : warehouseRows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No transfers yet"
          description="Inter-warehouse transfers involving this site will appear here."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No transfers match your filters"
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
          <table className="w-full min-w-[1000px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <Th>Transfer</Th>
                <Th>Origin</Th>
                <Th>Destination</Th>
                <Th>Manifest</Th>
                <Th>Reconciliation</Th>
                <Th>Status</Th>
                <Th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((t) => {
                const statusTone = transferStatusTone(t.status);
                const reconTone = reconciliationTone(t.reconciliationStatus);
                return (
                  <tr
                    key={t.id}
                    className="border-b border-line last:border-0 hover:bg-surface-2"
                  >
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                          <WarehouseIcon className="h-4 w-4" />
                        </div>
                        <div className="min-w-0">
                          <div className="font-mono text-sm font-medium text-ink-950 truncate">
                            {t.id}
                          </div>
                          <div className="font-mono text-[10px] text-ink-400 truncate">
                            {t.routeReference || '—'}
                          </div>
                        </div>
                      </div>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-500 truncate inline-block max-w-[140px]">
                        {t.originWarehouseId}
                      </span>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-500 truncate inline-block max-w-[140px]">
                        {t.destinationWarehouseId}
                      </span>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-600 truncate inline-block max-w-[140px]">
                        {t.manifestId}
                      </span>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${reconTone.bg} ${reconTone.text}`}
                      >
                        <span className={`h-1 w-1 rounded-full ${reconTone.dot}`} />
                        {reconciliationLabel(t.reconciliationStatus)}
                      </span>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                        {transferStatusLabel(t.status)}
                      </span>
                    </Td>
                    <Td className="text-right">
                      <Link
                        href={`/warehouse/${warehouseId}/transfers/${t.id}`}
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
  );
}

function Th({
  children,
  className = '',
}: {
  children?: React.ReactNode;
  className?: string;
}) {
  return (
    <th
      className={`px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap ${className}`}
    >
      {children}
    </th>
  );
}

function Td({
  children,
  className = '',
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <td className={`px-4 py-3.5 text-sm text-ink-700 ${className}`}>
      {children}
    </td>
  );
}