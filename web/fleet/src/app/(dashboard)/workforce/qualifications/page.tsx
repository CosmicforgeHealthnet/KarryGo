'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  CheckCircle2,
  FileText,
  Search,
  Shield,
  SlidersHorizontal,
  Users,
  X,
  XCircle,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { workforceService } from '@/lib/services/workforceService';
import {
  formatDate,
  qualificationStatusLabel,
  qualificationStatusTone,
} from '@/lib/utils/workforceLabels';
import type { Qualification } from '@/lib/types/workforce';

const STATUS_FILTERS = ['ALL', 'ACTIVE', 'INACTIVE', 'ARCHIVED'] as const;

export default function QualificationsPage() {
  const [rows, setRows] = useState<Qualification[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .listQualifications()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load qualifications.',
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
      const haystack = [row.code, row.name, row.id]
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
          <h1>Qualifications</h1>
          <p className="mt-1.5">
            Qualification definitions applied to workforce roles and services.
          </p>
        </div>
        <Link
          href="/workforce/qualifications/providers"
          className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <Users className="h-3.5 w-3.5" />
          Provider qualifications
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
            placeholder="Search by code or name..."
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
                {s === 'ALL' ? 'All' : qualificationStatusLabel(s)}
              </button>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading qualifications…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No qualifications yet"
          description="Qualification definitions will appear here once created."
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No qualifications match your filters"
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
          {filtered.map((q) => {
            const tone = qualificationStatusTone(q.status);
            return (
              <div
                key={q.id}
                className="rounded-lg border border-line bg-surface p-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-3 min-w-0">
                    <div
                      className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md ${tone.bg} ${tone.text}`}
                    >
                      <Shield className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-ink-950">
                        {q.name}
                      </div>
                      <div className="font-mono text-[10px] text-ink-400 truncate">
                        {q.code} · {q.id}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                    {qualificationStatusLabel(q.status)}
                  </span>
                </div>

                <div className="mt-3 space-y-1 text-xs text-ink-500">
                  <div className="truncate">
                    <span className="text-ink-400">Roles:</span>{' '}
                    {q.applicableRoles.length > 0
                      ? q.applicableRoles.join(', ')
                      : '—'}
                  </div>
                  <div className="truncate">
                    <span className="text-ink-400">Services:</span>{' '}
                    {q.applicableServices.length > 0
                      ? q.applicableServices.join(', ')
                      : '—'}
                  </div>
                  <div className="truncate">
                    <span className="text-ink-400">Assets:</span>{' '}
                    {q.applicableAssetTypes.length > 0
                      ? q.applicableAssetTypes.join(', ')
                      : '—'}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span
                    className={`flex items-center gap-1 text-[10px] font-medium ${q.evidenceRequired ? 'text-amber' : 'text-ink-400'}`}
                  >
                    {q.evidenceRequired ? (
                      <>
                        <CheckCircle2 className="h-3 w-3" />
                        Evidence required
                      </>
                    ) : (
                      <>
                        <XCircle className="h-3 w-3" />
                        Evidence optional
                      </>
                    )}
                  </span>
                  <span className="text-[10px] text-ink-400">
                    {formatDate(q.updatedAt)}
                  </span>
                </div>
              </div>
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