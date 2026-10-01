'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  ArrowRight,
  Package,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  formatDateTime,
  fulfilmentStatusLabel,
  fulfilmentStatusTone,
  priorityLabel,
  priorityTone,
} from '@/lib/utils/warehouseLabels';
import type { FulfilmentOrder } from '@/lib/types/warehouse';

const STATUS_FILTERS = [
  'ALL',
  'DRAFT',
  'RELEASED',
  'PICKING',
  'PICKED',
  'PACKING',
  'PACKED',
  'DISPATCHED',
  'COMPLETED',
  'CANCELLED',
] as const;

const PRIORITY_FILTERS = ['ALL', 'NORMAL', 'HIGH', 'URGENT'] as const;

export default function OutboundPage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [rows, setRows] = useState<FulfilmentOrder[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [priority, setPriority] =
    useState<(typeof PRIORITY_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .listFulfilmentOrders()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load fulfilment orders.',
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
    () => rows.filter((r) => r.warehouseId === warehouseId),
    [rows, warehouseId],
  );

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return warehouseRows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (priority !== 'ALL' && row.priority !== priority) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.customerReference,
        row.sourceOrderId,
        row.shipmentReference,
        row.manifestReference,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [warehouseRows, search, status, priority]);

  const hasActiveFilters =
    status !== 'ALL' || priority !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setPriority('ALL');
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
          <h1>Outbound Fulfilment</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Orders being picked, packed and dispatched from this warehouse.
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
            placeholder="Search by customer or reference..."
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
        <div className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <FilterGroup
            label="Status"
            value={status}
            options={STATUS_FILTERS.map((s) => ({
              value: s,
              label: s === 'ALL' ? 'All' : fulfilmentStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
          <FilterGroup
            label="Priority"
            value={priority}
            options={PRIORITY_FILTERS.map((p) => ({
              value: p,
              label: p === 'ALL' ? 'All' : priorityLabel(p),
            }))}
            onChange={(v) => setPriority(v as (typeof PRIORITY_FILTERS)[number])}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading orders…
        </div>
      ) : warehouseRows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No outbound orders yet"
          description="Fulfilment orders for this warehouse will appear here."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No orders match your filters"
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
                <Th>Order</Th>
                <Th>Customer</Th>
                <Th>Priority</Th>
                <Th>Requested dispatch</Th>
                <Th>Shipment / manifest</Th>
                <Th>Status</Th>
                <Th />
              </tr>
            </thead>
            <tbody>
              {filtered.map((order) => {
                const statusTone = fulfilmentStatusTone(order.status);
                const priorityToneCls = priorityTone(order.priority);
                return (
                  <tr
                    key={order.id}
                    className="border-b border-line last:border-0 hover:bg-surface-2"
                  >
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                          <Package className="h-4 w-4" />
                        </div>
                        <div className="min-w-0">
                          <div className="font-mono text-sm font-medium text-ink-950 truncate">
                            {order.sourceOrderId || order.id}
                          </div>
                          <div className="font-mono text-[10px] text-ink-400 truncate">
                            {order.id}
                          </div>
                        </div>
                      </div>
                    </Td>
                    <Td>
                      <span className="text-xs text-ink-600 truncate inline-block max-w-[180px]">
                        {order.customerReference || '—'}
                      </span>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${priorityToneCls.bg} ${priorityToneCls.text}`}
                      >
                        <span className={`h-1 w-1 rounded-full ${priorityToneCls.dot}`} />
                        {priorityLabel(order.priority)}
                      </span>
                    </Td>
                    <Td>
                      <span className="text-xs text-ink-500">
                        {order.requestedDispatchAt
                          ? formatDateTime(order.requestedDispatchAt)
                          : '—'}
                      </span>
                    </Td>
                    <Td>
                      <div className="font-mono text-[10px] text-ink-500 truncate">
                        {order.shipmentReference || '—'}
                      </div>
                      <div className="font-mono text-[10px] text-ink-400 truncate">
                        {order.manifestReference || '—'}
                      </div>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                        {fulfilmentStatusLabel(order.status)}
                      </span>
                    </Td>
                    <Td className="text-right">
                      <Link
                        href={`/warehouse/${warehouseId}/outbound/${order.id}`}
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

function FilterGroup({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (v: string) => void;
}) {
  return (
    <div>
      <span className="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
        {label}
      </span>
      <div className="flex flex-wrap gap-1.5">
        {options.map((o) => (
          <button
            key={o.value}
            onClick={() => onChange(o.value)}
            className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
              value === o.value
                ? 'bg-primary text-white'
                : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
            }`}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  );
}