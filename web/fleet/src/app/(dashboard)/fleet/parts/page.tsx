'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Package,
  BarChart3,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { partService } from '@/lib/services/partService';
import {
  formatNumber,
  partStatusLabel,
  partStatusTone,
  stockStateLabel,
} from '@/lib/utils/partLabels';
import type { Part } from '@/lib/types/part';

const STATUS_FILTERS = ['ALL', 'ACTIVE', 'INACTIVE', 'ARCHIVED'] as const;
const STOCK_FILTERS = ['ALL', 'IN_STOCK', 'LOW_STOCK', 'OUT_OF_STOCK'] as const;

type StockFilter = (typeof STOCK_FILTERS)[number];

export default function PartsPage() {
  const [rows, setRows] = useState<Part[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [stock, setStock] = useState<StockFilter>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    partService
      .list()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load parts.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const stats = useMemo(() => {
    let total = 0;
    let lowOrOut = 0;
    for (const r of rows) {
      total += r.currentQuantityProjection;
      if (r.currentQuantityProjection <= r.reorderLevel) lowOrOut++;
    }
    return { total, lowOrOut, categories: new Set(rows.map((r) => r.stockUnit)).size };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;

      if (stock !== 'ALL') {
        const label = stockStateLabel(row).label;
        if (stock === 'IN_STOCK' && label !== 'In stock') return false;
        if (stock === 'LOW_STOCK' && label !== 'Low stock') return false;
        if (stock === 'OUT_OF_STOCK' && label !== 'Out of stock') return false;
      }

      if (q === '') return true;
      const haystack = [row.id, row.sku, row.name, row.branchId]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status, stock]);

  const hasActiveFilters =
    status !== 'ALL' || stock !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setStock('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Parts Catalogue</h1>
          <p className="mt-1.5">
            Parts inventory with stock levels, locations and reorder status.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link
            href="/fleet/parts/stock"
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <BarChart3 className="h-3.5 w-3.5" />
            Stock movements
          </Link>
          <Link
            href="/fleet/parts/create"
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Package className="h-3.5 w-3.5" />
            Add part
          </Link>
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
        <StatCard label="Stock units" value={stats.categories} tone="blue" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by SKU, name, branch..."
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
        <div className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <FilterGroup
            label="Status"
            value={status}
            options={STATUS_FILTERS.map((s) => ({
              value: s,
              label: s === 'ALL' ? 'All' : partStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
          <FilterGroup
            label="Stock state"
            value={stock}
            options={STOCK_FILTERS.map((s) => ({
              value: s,
              label:
                s === 'ALL'
                  ? 'All'
                  : s === 'IN_STOCK'
                  ? 'In stock'
                  : s === 'LOW_STOCK'
                  ? 'Low stock'
                  : 'Out of stock',
            }))}
            onChange={(v) => setStock(v as StockFilter)}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading parts…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No parts yet"
          description="Add parts to start tracking inventory."
          action={
            <Link
              href="/fleet/parts/create"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add part
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No parts match your filters"
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
                <Th>Part</Th>
                <Th>SKU</Th>
                <Th>Branch</Th>
                <Th>Quantity</Th>
                <Th>Reorder</Th>
                <Th>Stock state</Th>
                <Th>Status</Th>
                <Th className="text-right" />
              </tr>
            </thead>
            <tbody>
              {filtered.map((row) => {
                const statusTone = partStatusTone(row.status);
                const stockInfo = stockStateLabel(row);

                return (
                  <tr
                    key={row.id}
                    className="border-b border-line last:border-0 hover:bg-surface-2"
                  >
                    <Td>
                      <div className="font-medium text-ink-950">{row.name}</div>
                      <div className="font-mono text-[10px] text-ink-400">
                        {row.id}
                      </div>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-600">
                        {row.sku}
                      </span>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-500">
                        {row.branchId}
                      </span>
                    </Td>
                    <Td>
                      <span className="font-semibold text-ink-950">
                        {formatNumber(row.currentQuantityProjection)}
                      </span>{' '}
                      <span className="text-ink-400">{row.stockUnit}</span>
                    </Td>
                    <Td>
                      <span className="text-ink-500">
                        {formatNumber(row.reorderLevel)}
                      </span>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stockInfo.tone.bg} ${stockInfo.tone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${stockInfo.tone.dot}`} />
                        {stockInfo.label}
                      </span>
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                        {partStatusLabel(row.status)}
                      </span>
                    </Td>
                    <Td className="text-right">
                      <Link
                        href={`/fleet/parts/${row.id}`}
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