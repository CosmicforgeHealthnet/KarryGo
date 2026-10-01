'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import {
  Plus,
  Search,
  SlidersHorizontal,
  X,
  ArrowRight,
  Package,
  Truck,
  Car,
  GitBranch,
  Clock,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { providerService } from '@/lib/services/providerService';
import {
  onboardingSourceLabel,
  relationshipTypeLabel,
  serviceCategoryLabel,
  statusLabel,
  statusTone,
  formatDate,
} from '@/lib/utils/providerLabels';
import type {
  MembershipStatus,
  OnboardingSource,
  ProviderMembership,
  RelationshipType,
  ServiceCategory,
} from '@/lib/types/provider';

const STATUSES: MembershipStatus[] = [
  'INVITED',
  'PENDING_PROVIDER_ACCEPTANCE',
  'PENDING_SETUP',
  'ACTIVE',
  'SUSPENDED',
  'ENDING',
  'REMOVED',
  'DECLINED',
  'EXPIRED',
  'CANCELLED',
];

const CATEGORIES: ServiceCategory[] = ['DISPATCH', 'TAXI', 'HAULING'];
const SOURCES: OnboardingSource[] = ['ORGANIZATION_SPONSORED', 'EXISTING_PROVIDER_LINKED'];
const RELATIONSHIPS: RelationshipType[] = ['EMPLOYEE', 'CONTRACTOR', 'PARTNER'];

const CATEGORY_ICON = {
  DISPATCH: Package,
  TAXI: Car,
  HAULING: Truck,
} as const;

export default function ProvidersPage() {
  const [rows, setRows] = useState<ProviderMembership[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<MembershipStatus | 'ALL'>('ALL');
  const [category, setCategory] = useState<ServiceCategory | 'ALL'>('ALL');
  const [source, setSource] = useState<OnboardingSource | 'ALL'>('ALL');
  const [relationship, setRelationship] = useState<RelationshipType | 'ALL'>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    providerService
      .list()
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to load providers.';
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const stats = useMemo(() => {
    const counts: Record<MembershipStatus, number> = {
      INVITED: 0,
      PENDING_PROVIDER_ACCEPTANCE: 0,
      PENDING_SETUP: 0,
      ACTIVE: 0,
      SUSPENDED: 0,
      ENDING: 0,
      REMOVED: 0,
      DECLINED: 0,
      EXPIRED: 0,
      CANCELLED: 0,
    };
    for (const row of rows) counts[row.status] = (counts[row.status] ?? 0) + 1;
    return counts;
  }, [rows]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (category !== 'ALL' && row.serviceCategory !== category) return false;
      if (source !== 'ALL' && row.onboardingSource !== source) return false;
      if (relationship !== 'ALL' && row.relationshipType !== relationship) return false;
      if (q === '') return true;
      const haystack = [
        row.membershipId,
        row.providerId ?? '',
        row.homeBranchId ?? '',
      ]
        .join(' ')
        .toLowerCase();
      return haystack.includes(q);
    });
  }, [rows, search, status, category, source, relationship]);

  const hasActiveFilters =
    status !== 'ALL' ||
    category !== 'ALL' ||
    source !== 'ALL' ||
    relationship !== 'ALL' ||
    search !== '';

  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setCategory('ALL');
    setSource('ALL');
    setRelationship('ALL');
  };

  return (
    <div className="space-y-5">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1>Managed Providers</h1>
          <p className="mt-1.5">
            Organization-linked Dispatch, Taxi and Hauling providers with
            membership state and onboarding source.
          </p>
        </div>
        <Link
          href="/providers/add"
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Add provider
        </Link>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {/* Stats */}
      <div className="grid grid-cols-2 gap-3.5 lg:grid-cols-4">
        <StatCard label="Active" value={stats.ACTIVE} tone="green" />
        <StatCard label="Pending" value={stats.INVITED + stats.PENDING_PROVIDER_ACCEPTANCE + stats.PENDING_SETUP} tone="amber" />
        <StatCard label="Suspended" value={stats.SUSPENDED} tone="amber" />
        <StatCard label="Removed" value={stats.REMOVED + stats.DECLINED + stats.EXPIRED + stats.CANCELLED} tone="red" />
      </div>

      {/* Toolbar */}
      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by membership or provider ID..."
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

      {/* Filter drawer */}
      {showFilters && (
        <div className="space-y-4 rounded-lg border border-line bg-surface p-4">
          <FilterGroup
            label="Status"
            value={status}
            options={STATUSES.map((s) => ({ value: s, label: statusLabel(s) }))}
            onChange={(v) => setStatus(v as MembershipStatus | 'ALL')}
          />
          <FilterGroup
            label="Service category"
            value={category}
            options={CATEGORIES.map((c) => ({ value: c, label: serviceCategoryLabel(c) }))}
            onChange={(v) => setCategory(v as ServiceCategory | 'ALL')}
          />
          <FilterGroup
            label="Onboarding"
            value={source}
            options={SOURCES.map((s) => ({ value: s, label: onboardingSourceLabel(s) }))}
            onChange={(v) => setSource(v as OnboardingSource | 'ALL')}
          />
          <FilterGroup
            label="Relationship"
            value={relationship}
            options={RELATIONSHIPS.map((r) => ({ value: r, label: relationshipTypeLabel(r) }))}
            onChange={(v) => setRelationship(v as RelationshipType | 'ALL')}
          />
        </div>
      )}

      {/* List */}
      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading providers…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No providers yet"
          description="Add a provider to invite or link them to your organization."
          action={
            <Link
              href="/providers/add"
              className="inline-flex items-center justify-center rounded-lg bg-primary-bg px-4 py-2 text-xs font-semibold text-primary transition-colors hover:bg-primary/10"
            >
              Add provider
            </Link>
          }
        />
      ) : filtered.length === 0 ? (
        <EmptyState
          image="/images/trips-empty.png"
          title="No providers match your filters"
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
            const tone = statusTone(row.status);
            const Icon = CATEGORY_ICON[row.serviceCategory] ?? Package;

            return (
              <Link
                key={row.membershipId}
                href={`/providers/${row.membershipId}`}
                className="group rounded-lg border border-line bg-surface p-4 transition-colors hover:border-ink-400 hover:bg-surface-2"
              >
                <div className="flex items-start gap-3">
                  <div className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md ${tone.bg} ${tone.text}`}>
                    <Icon className="h-[18px] w-[18px]" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold text-ink-950">
                        {serviceCategoryLabel(row.serviceCategory)}
                      </span>
                      <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}>
                        <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                        {statusLabel(row.status)}
                      </span>
                    </div>
                    <div className="font-mono text-[10px] text-ink-400 truncate">
                      {row.membershipId}
                    </div>
                  </div>
                </div>

                <div className="mt-3 grid grid-cols-2 gap-1.5 text-xs text-ink-500">
                  <div className="flex items-center gap-1.5">
                    <GitBranch className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">
                      {relationshipTypeLabel(row.relationshipType)}
                    </span>
                  </div>
                  <div className="flex items-center gap-1.5">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{formatDate(row.joinedAt)}</span>
                  </div>
                  <div className="col-span-2 truncate">
                    {onboardingSourceLabel(row.onboardingSource)}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="font-mono text-[10px] text-ink-400 truncate max-w-[150px]">
                    {row.providerId ?? 'Not yet linked'}
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
          Showing the latest {rows.length} membership{rows.length === 1 ? '' : 's'}.
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
        <FilterChip active={value === 'ALL'} onClick={() => onChange('ALL')}>
          All
        </FilterChip>
        {options.map((o) => (
          <FilterChip
            key={o.value}
            active={value === o.value}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </FilterChip>
        ))}
      </div>
    </div>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      className={`rounded-full px-3 py-1 text-xs font-semibold transition-colors whitespace-nowrap ${
        active
          ? 'bg-primary text-white'
          : 'bg-surface-2 text-ink-600 hover:bg-surface-3'
      }`}
    >
      {children}
    </button>
  );
}