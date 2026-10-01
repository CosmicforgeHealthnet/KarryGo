'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Clock,
  Shield,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { maintenanceService } from '@/lib/services/maintenanceService';
import {
  formatDateTime,
  workOrderPriorityLabel,
  workOrderPriorityTone,
  workOrderSourceLabel,
  workOrderStatusLabel,
  workOrderStatusTone,
} from '@/lib/utils/maintenanceLabels';
import type { WorkOrder } from '@/lib/types/maintenance';

const STATUS_FILTERS = [
  'ALL',
  'DRAFT',
  'PENDING_APPROVAL',
  'APPROVED',
  'IN_PROGRESS',
  'COMPLETED',
  'CANCELLED',
] as const;

const PRIORITY_FILTERS = ['ALL', 'CRITICAL', 'HIGH', 'MEDIUM', 'LOW'] as const;

export default function WorkOrdersPage() {
  const [rows, setRows] = useState<WorkOrder[]>([]);
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

    maintenanceService
      .listWorkOrders()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load work orders.',
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
    let open = 0;
    let critical = 0;
    let completed = 0;
    for (const r of rows) {
      if (r.status === 'IN_PROGRESS' || r.status === 'APPROVED') open++;
      if (r.priority === 'CRITICAL') critical++;
      if (r.status === 'COMPLETED') completed++;
    }
    return { open, critical, completed };
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (priority !== 'ALL' && row.priority !== priority) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.assetId,
        row.sourceReference,
        row.diagnosis,
        row.requiredWork,
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status, priority]);

  const hasActiveFilters =
    status !== 'ALL' || priority !== 'ALL' || search.trim() !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setPriority('ALL');
  };

  return (
    <div className="space-y-5">
      <div>
        <h1>Work Orders</h1>
        <p className="mt-1.5">
          Maintenance and repair work orders across the fleet. Created
          automatically from inspections, breakdowns and maintenance schedules.
        </p>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-3 gap-3.5">
        <StatCard label="Open" value={stats.open} tone="amber" />
        <StatCard label="Critical" value={stats.critical} tone="red" />
        <StatCard label="Completed" value={stats.completed} tone="green" />
      </div>

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by ID, asset, diagnosis..."
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
              label: s === 'ALL' ? 'All' : workOrderStatusLabel(s),
            }))}
            onChange={(v) => setStatus(v as (typeof STATUS_FILTERS)[number])}
          />
          <FilterGroup
            label="Priority"
            value={priority}
            options={PRIORITY_FILTERS.map((p) => ({
              value: p,
              label: p === 'ALL' ? 'All' : workOrderPriorityLabel(p),
            }))}
            onChange={(v) => setPriority(v as (typeof PRIORITY_FILTERS)[number])}
          />
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading work orders…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No work orders yet"
          description="Work orders are created automatically from inspections, breakdowns and maintenance schedules."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No work orders match your filters"
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
            const statusTone = workOrderStatusTone(row.status);
            const priorityTone = workOrderPriorityTone(row.priority);
            const isCritical =
              row.priority === 'CRITICAL' || row.priority === 'HIGH';
            const isOpen =
              row.status === 'IN_PROGRESS' ||
              row.status === 'APPROVED' ||
              row.status === 'PENDING_APPROVAL';

            return (
              <Link
                key={row.id}
                href={`/fleet/work-orders/${row.id}`}
                className={`group rounded-lg border p-4 transition-colors ${
                  isCritical && isOpen
                    ? 'border-red/30 bg-red-bg/10 hover:border-red/50 hover:bg-red-bg/20'
                    : isOpen
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
                        className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${priorityTone.bg} ${priorityTone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${priorityTone.dot}`} />
                        {workOrderPriorityLabel(row.priority)}
                      </span>
                    </div>
                    <div className="mt-0.5 font-mono text-[11px] text-ink-400 truncate">
                      {row.assetId}
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusTone.bg} ${statusTone.text}`}
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                    {workOrderStatusLabel(row.status)}
                  </span>
                </div>

                <p className="mt-2 line-clamp-2 text-sm text-ink-700">
                  {row.diagnosis || row.requiredWork || '—'}
                </p>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  <div className="flex items-center gap-1.5">
                    <Shield className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">
                      {workOrderSourceLabel(row.sourceType)}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">
                      {row.completedAt
                        ? formatDateTime(row.completedAt)
                        : formatDateTime(row.createdAt)}
                    </span>
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="text-[10px] text-ink-400 truncate max-w-[150px]">
                    {row.sourceReference || '—'}
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
  tone: 'green' | 'amber' | 'red';
}) {
  const cls = {
    green: 'bg-green-bg text-green',
    amber: 'bg-amber-bg text-amber',
    red: 'bg-red-bg text-red',
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