'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Plus,
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Warehouse as WarehouseIcon,
  MapPin,
  Clock,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  warehouseStatusLabel,
  warehouseStatusTone,
  formatDateTime,
} from '@/lib/utils/warehouseLabels';
import type { Warehouse } from '@/lib/types/warehouse';

const STATUS_FILTERS = ['ALL', 'ACTIVE', 'INACTIVE', 'ARCHIVED'] as const;

export default function WarehousesPage() {
  const [rows, setRows] = useState<Warehouse[]>([]);
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
      .listWarehouses()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load warehouses.',
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
    let active = 0;
    let inactive = 0;
    let archived = 0;
    for (const r of rows) {
      if (r.status === 'ACTIVE') active++;
      else if (r.status === 'INACTIVE') inactive++;
      else if (r.status === 'ARCHIVED') archived++;
    }
    return { active, inactive, archived };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (q === '') return true;
      const haystack = [
        row.code,
        row.name,
        row.siteReference,
        row.addressReference,
        row.branchId,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status]);

  const hasActiveFilters = status !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Warehouses</h1>
          <p className="mt-1.5">
            Warehouse sites with code, timezone and lifecycle status.
          </p>
        </div>
        <Link
          href="/warehouse/create"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add warehouse
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Active" value={stats.active} tone="green" />
        <StatCard label="Inactive" value={stats.inactive} tone="amber" />
        <StatCard label="Archived" value={stats.archived} tone="neutral" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by code, name or reference..."
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
                {s === 'ALL' ? 'All' : warehouseStatusLabel(s)}
              </button>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading warehouses…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No warehouses yet"
          description="Add your first warehouse to begin tracking storage and inventory."
          action={
            <Link
              href="/warehouse/create"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add warehouse
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No warehouses match your filters"
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
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {filtered.map((w) => {
            const tone = warehouseStatusTone(w.status);
            return (
              <Link
                key={w.id}
                href={`/warehouse/${w.id}`}
                className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-3 min-w-0">
                    <div
                      className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md ${tone.bg} ${tone.text}`}
                    >
                      <WarehouseIcon className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950 truncate">
                          {w.name}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {w.code}
                        </span>
                      </div>
                      <div className="font-mono text-[10px] text-ink-400 truncate">
                        {w.id}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                    {warehouseStatusLabel(w.status)}
                  </span>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  <div className="flex items-center gap-1.5 truncate">
                    <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-mono text-[10px] truncate">
                      {w.branchId}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5 truncate">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{w.timezone}</span>
                  </div>
                  <div className="col-span-2 truncate font-mono text-[10px] text-ink-400">
                    {w.addressReference || '—'}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="text-[10px] text-ink-400">
                    {formatDateTime(w.updatedAt)}
                  </span>
                  <span className="flex items-center gap-1 text-xs font-medium text-ink-400 transition-colors group-hover:text-primary">
                    View <ArrowRight className="h-3.5 w-3.5" />
                  </span>
                </div>
              </Link>
            );
          })}
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
  tone: 'green' | 'amber' | 'neutral';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    amber: 'bg-amber-bg text-amber',
    neutral: 'bg-surface-2 text-ink-500',
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