'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Plus,
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Bike,
  Car,
  Truck,
  Package,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { assetService } from '@/lib/services/assetService';
import {
  assetDisplayName,
  assetTypeLabel,
  formatOdometer,
  lifecycleStatusLabel,
  lifecycleStatusTone,
  operationalStatusLabel,
  operationalStatusTone,
} from '@/lib/utils/assetLabels';
import type { Asset } from '@/lib/types/asset';

const STATUS_FILTERS = [
  'ALL',
  'AVAILABLE',
  'ASSIGNED',
  'BLOCKED',
  'UNDER_MAINTENANCE',
  'OUT_OF_SERVICE',
] as const;

const TYPE_FILTERS = [
  'ALL',
  'MOTORCYCLE',
  'VAN',
  'TRUCK',
  'CAR',
  'BIKE',
  'OTHER',
] as const;

const TYPE_ICON: Record<string, typeof Truck> = {
  MOTORCYCLE: Bike,
  BIKE: Bike,
  VAN: Car,
  CAR: Car,
  TRUCK: Truck,
  OTHER: Package,
};

export default function AssetsPage() {
  const [rows, setRows] = useState<Asset[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [type, setType] = useState<(typeof TYPE_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    assetService
      .list()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load assets.',
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
    const counts = {
      available: 0,
      assigned: 0,
      blocked: 0,
      other: 0,
    };
    for (const r of rows) {
      if (r.operationalStatus === 'AVAILABLE') counts.available++;
      else if (r.operationalStatus === 'ASSIGNED') counts.assigned++;
      else if (
        r.operationalStatus === 'BLOCKED' ||
        r.operationalStatus === 'OUT_OF_SERVICE'
      )
        counts.blocked++;
      else counts.other++;
    }
    return counts;
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.operationalStatus !== status) return false;
      if (type !== 'ALL' && row.assetType !== type) return false;
      if (q === '') return true;
      const haystack = [
        row.assetCode,
        row.registrationReference,
        row.manufacturer,
        row.model,
        row.platformVehicleId,
        row.branchId,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status, type]);

  const hasActiveFilters =
    status !== 'ALL' || type !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setType('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Fleet Assets</h1>
          <p className="mt-1.5">
            Operational assets with lifecycle, operational state, and
            assignment readiness.
          </p>
        </div>
        <Link
          href="/fleet/assets/create"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add asset
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        <StatCard label="Available" value={stats.available} tone="green" />
        <StatCard label="Assigned" value={stats.assigned} tone="blue" />
        <StatCard label="Blocked" value={stats.blocked} tone="red" />
        <StatCard label="Other" value={stats.other} tone="neutral" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by code, plate, make, model..."
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
            label="Operational status"
            value={status}
            options={STATUS_FILTERS.map((s) => ({
              value: s,
              label: s === 'ALL' ? 'All' : operationalStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
          <FilterGroup
            label="Asset type"
            value={type}
            options={TYPE_FILTERS.map((t) => ({
              value: t,
              label: t === 'ALL' ? 'All' : assetTypeLabel(t),
            }))}
            onChange={(v) => setType(v as (typeof TYPE_FILTERS)[number])}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading assets…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No assets yet"
          description="Add your first asset to start tracking your fleet."
          action={
            <Link
              href="/fleet/assets/create"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add asset
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No assets match your filters"
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
          {filtered.map((asset) => {
            const opTone = operationalStatusTone(asset.operationalStatus);
            const lifeTone = lifecycleStatusTone(asset.lifecycleStatus);
            const Icon = TYPE_ICON[asset.assetType] ?? Truck;
            const name = assetDisplayName(asset);

            return (
              <Link
                key={asset.id}
                href={`/fleet/assets/${asset.id}`}
                className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-3 min-w-0">
                    <div
                      className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md ${opTone.bg} ${opTone.text}`}
                    >
                      <Icon className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950 truncate">
                          {asset.assetCode}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {assetTypeLabel(asset.assetType)}
                        </span>
                      </div>
                      <div className="text-xs text-ink-500 truncate">{name}</div>
                    </div>
                  </div>

                  <div className="flex flex-col items-end gap-1 flex-shrink-0">
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${opTone.bg} ${opTone.text}`}
                    >
                      <span className={`h-1.5 w-1.5 rounded-full ${opTone.dot}`} />
                      {operationalStatusLabel(asset.operationalStatus)}
                    </span>
                    <span
                      className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-medium ${lifeTone.bg} ${lifeTone.text}`}
                    >
                      <span className={`h-1 w-1 rounded-full ${lifeTone.dot}`} />
                      {lifecycleStatusLabel(asset.lifecycleStatus)}
                    </span>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  <div className="truncate">
                    <span className="text-ink-400">Plate:</span>{' '}
                    {asset.registrationReference || '—'}
                  </div>
                  <div className="truncate">
                    <span className="text-ink-400">Odo:</span>{' '}
                    {formatOdometer(asset.odometer)}
                  </div>
                  <div className="col-span-2 truncate font-mono text-[10px] text-ink-400">
                    {asset.branchId}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="font-mono text-[10px] text-ink-400 truncate max-w-[150px]">
                    {asset.id}
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

      {!loading && rows.length > 0 && (
        <p className="text-center text-xs text-ink-400">
          Showing {rows.length} asset{rows.length === 1 ? '' : 's'}.
        </p>
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
  tone: 'green' | 'blue' | 'red' | 'neutral';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    blue: 'bg-primary-bg text-primary',
    red: 'bg-red-bg text-red',
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