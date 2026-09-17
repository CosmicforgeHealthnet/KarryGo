'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  ArrowLeft,
  Layers,
  MapPin,
  Plus,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { warehouseService } from '@/lib/services/warehouseService';
import {
  formatNumber,
  locationStatusLabel,
  locationStatusTone,
  locationTypeLabel,
} from '@/lib/utils/warehouseLabels';
import type { StorageLocation } from '@/lib/types/warehouse';

const TYPE_FILTERS = [
  'ALL',
  'ZONE',
  'AISLE',
  'BAY',
  'BIN',
  'DOCK',
  'STAGING',
  'QUARANTINE',
  'COLD',
  'HAZMAT',
  'OTHER',
] as const;

const STATUS_FILTERS = ['ALL', 'ACTIVE', 'INACTIVE', 'MAINTENANCE'] as const;

export default function StoragePage() {
  const params = useParams();
  const warehouseId = params.warehouseId as string;

  const [rows, setRows] = useState<StorageLocation[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [type, setType] = useState<(typeof TYPE_FILTERS)[number]>('ALL');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    if (!warehouseId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    warehouseService
      .listLocations(warehouseId)
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load storage locations.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [warehouseId]);

  const stats = useMemo(() => {
    let active = 0;
    let inactive = 0;
    let maintenance = 0;
    for (const r of rows) {
      if (r.status === 'ACTIVE') active++;
      else if (r.status === 'INACTIVE') inactive++;
      else if (r.status === 'MAINTENANCE') maintenance++;
    }
    return { active, inactive, maintenance };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (type !== 'ALL' && row.locationType !== type) return false;
      if (status !== 'ALL' && row.status !== status) return false;
      if (q === '') return true;
      const haystack = [
        row.code,
        row.name,
        row.id,
        row.parentLocationId,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, type, status]);

  const hasActiveFilters =
    type !== 'ALL' || status !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setType('ALL');
    setStatus('ALL');
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href={`/warehouse/${warehouseId}`}
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Storage Locations</h1>
            <p className="mt-0.5 text-sm text-ink-500">
              Zones, aisles, bays, bins and controlled locations.
            </p>
          </div>
        </div>
        <Link
          href={`/warehouse/${warehouseId}/locations/create`}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add location
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Active" value={stats.active} tone="green" />
        <StatCard label="Inactive" value={stats.inactive} tone="neutral" />
        <StatCard label="Maintenance" value={stats.maintenance} tone="amber" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by code, name or ID..."
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
            label="Type"
            value={type}
            options={TYPE_FILTERS.map((t) => ({
              value: t,
              label: t === 'ALL' ? 'All' : locationTypeLabel(t),
            }))}
            onChange={(v) => setType(v as (typeof TYPE_FILTERS)[number])}
          />
          <FilterGroup
            label="Status"
            value={status}
            options={STATUS_FILTERS.map((s) => ({
              value: s,
              label: s === 'ALL' ? 'All' : locationStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading locations…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No storage locations yet"
          description="Add a location to start organizing inventory within this warehouse."
          action={
            <Link
              href={`/warehouse/${warehouseId}/locations/create`}
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add location
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No locations match your filters"
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
                <Th>Location</Th>
                <Th>Type</Th>
                <Th>Parent</Th>
                <Th>Capacity</Th>
                <Th>Status</Th>
                <Th className="text-right">Version</Th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((loc) => {
                const tone = locationStatusTone(loc.status);
                return (
                  <tr
                    key={loc.id}
                    className="border-b border-line last:border-0 hover:bg-surface-2"
                  >
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                          <Layers className="h-4 w-4" />
                        </div>
                        <div className="min-w-0">
                          <div className="text-sm font-medium text-ink-950 truncate">
                            {loc.name}
                          </div>
                          <div className="flex items-center gap-2 text-[10px] text-ink-400">
                            <span className="font-mono">{loc.code}</span>
                            <span>·</span>
                            <span className="font-mono truncate">
                              {loc.id}
                            </span>
                          </div>
                        </div>
                      </div>
                    </Td>
                    <Td>
                      <span className="rounded-full bg-surface-2 px-2.5 py-0.5 text-[10px] font-medium text-ink-600">
                        {locationTypeLabel(loc.locationType)}
                      </span>
                    </Td>
                    <Td>
                      {loc.parentLocationId ? (
                        <span className="flex items-center gap-1.5 font-mono text-xs text-ink-500">
                          <MapPin className="h-3.5 w-3.5 flex-shrink-0" />
                          {loc.parentLocationId}
                        </span>
                      ) : (
                        <span className="text-xs text-ink-400">—</span>
                      )}
                    </Td>
                    <Td>
                      {loc.capacity != null ? (
                        <>
                          <span className="text-sm font-medium text-ink-950">
                            {formatNumber(loc.capacity)}
                          </span>{' '}
                          <span className="text-xs text-ink-500">
                            {loc.capacityUnit}
                          </span>
                        </>
                      ) : (
                        <span className="text-xs text-ink-400">Unlimited</span>
                      )}
                    </Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                        {locationStatusLabel(loc.status)}
                      </span>
                    </Td>
                    <Td className="text-right">
                      <span className="font-mono text-xs text-ink-500">
                        {loc.version}
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