'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  ArrowRight,
  Droplets,
  Plus,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { fuelService } from '@/lib/services/fuelService';
import {
  approvalStatusLabel,
  approvalStatusTone,
  formatDateTime,
  formatNumber,
  fuelOrEnergyTypeLabel,
  fuelUnitLabel,
} from '@/lib/utils/fuelLabels';
import type { FuelRecord } from '@/lib/types/fuel';

const STATUS_FILTERS = ['ALL', 'PENDING', 'APPROVED', 'REJECTED'] as const;
const TYPE_FILTERS = ['ALL', 'PETROL', 'DIESEL', 'ELECTRIC', 'GAS', 'OTHER'] as const;

export default function FuelRecordsPage() {
  const [rows, setRows] = useState<FuelRecord[]>([]);
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

    fuelService
      .list()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load fuel records.',
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
    let pending = 0;
    let approved = 0;
    let totalQuantity = 0;
    for (const r of rows) {
      if (r.approvalStatus === 'PENDING') pending++;
      if (r.approvalStatus === 'APPROVED') approved++;
      if (r.approvalStatus === 'APPROVED') totalQuantity += r.quantity;
    }
    return { pending, approved, totalQuantity };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.approvalStatus !== status) return false;
      if (type !== 'ALL' && row.fuelOrEnergyType !== type) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.assetId,
        row.supplierReference,
        row.providerMembershipId,
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
          <h1>Fuel Records</h1>
          <p className="mt-1.5">
            Fuel and energy records with approval workflow.
          </p>
        </div>
        <Link
          href="/fleet/fuel/create"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add record
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Approved" value={stats.approved} tone="green" />
        <StatCard label="Pending" value={stats.pending} tone="amber" />
        <StatCard label="Approved quantity" value={stats.totalQuantity} tone="blue" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, supplier..."
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
              label: s === 'ALL' ? 'All' : approvalStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
          <FilterGroup
            label="Fuel type"
            value={type}
            options={TYPE_FILTERS.map((t) => ({
              value: t,
              label: t === 'ALL' ? 'All' : fuelOrEnergyTypeLabel(t),
            }))}
            onChange={(v) => setType(v as (typeof TYPE_FILTERS)[number])}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading fuel records…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No fuel records yet"
          description="Fuel and energy records will appear here as they are added."
          action={
            <Link
              href="/fleet/fuel/create"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add record
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No fuel records match your filters"
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
          {filtered.map((row) => {
            const tone = approvalStatusTone(row.approvalStatus);
            const isPending = row.approvalStatus === 'PENDING';
            const isRejected = row.approvalStatus === 'REJECTED';

            return (
              <Link
                key={row.id}
                href={`/fleet/fuel/${row.id}`}
                className={`group rounded-lg border p-4 transition-colors ${
                  isRejected
                    ? 'border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20'
                    : isPending
                    ? 'border-amber/30 bg-amber-bg/10 hover:border-amber/50 hover:bg-amber-bg/20'
                    : 'border-line bg-surface hover:border-ink-400 hover:bg-surface-2'
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-semibold text-ink-950">
                        {row.id}
                      </span>
                      <span
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                        {approvalStatusLabel(row.approvalStatus)}
                      </span>
                    </div>
                    <div className="mt-0.5 font-mono text-[11px] text-ink-400 truncate">
                      {row.assetId}
                    </div>
                  </div>
                </div>

                <div className="mt-3 flex items-baseline gap-2">
                  <Droplets className="h-4 w-4 flex-shrink-0 text-ink-400" />
                  <span className="text-lg font-bold text-ink-950">
                    {formatNumber(row.quantity)}
                  </span>
                  <span className="text-xs text-ink-500">
                    {fuelUnitLabel(row.unit)}{' '}
                    {fuelOrEnergyTypeLabel(row.fuelOrEnergyType)}
                  </span>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  {row.odometer != null && (
                    <div className="truncate">
                      <span className="text-ink-400">Odo:</span>{' '}
                      {formatNumber(row.odometer)}
                    </div>
                  )}
                  {row.operatingHours != null && (
                    <div className="truncate">
                      <span className="text-ink-400">Hrs:</span>{' '}
                      {formatNumber(row.operatingHours)}
                    </div>
                  )}
                  <div className="col-span-2 truncate">
                    <span className="text-ink-400">Supplier:</span>{' '}
                    {row.supplierReference || '—'}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="text-[10px] text-ink-400">
                    {formatDateTime(row.recordedAt)}
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
          <span className="text-sm font-bold">
            {Number.isFinite(value)
              ? value.toLocaleString(undefined, { maximumFractionDigits: 0 })
              : value}
          </span>
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