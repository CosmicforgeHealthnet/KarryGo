'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useSearchParams } from 'next/navigation';
import {
  ArrowLeft,
  FileText,
  Search,
  SlidersHorizontal,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { workforceService } from '@/lib/services/workforceService';
import {
  formatDate,
  providerQualificationStatusLabel,
  providerQualificationStatusTone,
} from '@/lib/utils/workforceLabels';
import type { ProviderQualification } from '@/lib/types/workforce';

const STATUS_FILTERS = [
  'ALL',
  'PENDING_VERIFICATION',
  'VERIFIED',
  'REJECTED',
  'REVOKED',
] as const;

export default function ProviderQualificationsPage() {
  const searchParams = useSearchParams();
  const membershipId = searchParams.get('membershipId') ?? '';

  const [rows, setRows] = useState<ProviderQualification[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [searched, setSearched] = useState(false);

  const [search, setSearch] = useState(membershipId);
  const [status, setStatus] = useState<(typeof STATUS_FILTERS)[number]>('ALL');
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    if (membershipId) {
      load(membershipId);
    }
  }, [membershipId]);

  async function load(id: string) {
    setLoading(true);
    setError(null);
    setSearched(true);
    try {
      const data = await workforceService.listProviderQualifications(id);
      setRows(data);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to load provider qualifications.',
      );
      setRows([]);
    } finally {
      setLoading(false);
    }
  }

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    return rows.filter((row) => {
      if (status !== 'ALL' && row.status !== status) return false;
      if (q === '') return true;
      const haystack = [
        row.id,
        row.qualificationId,
        row.verificationReference,
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
        <div className="flex items-center gap-3">
          <Link
            href="/workforce/qualifications"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Provider Qualifications</h1>
            <p className="mt-0.5 text-sm text-ink-500">
              Look up qualifications held by a specific provider membership.
            </p>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="space-y-3 rounded-lg border border-line bg-surface p-4">
        <span className="block text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
          Membership ID
        </span>
        <div className="flex flex-wrap gap-2">
          <div className="flex h-10 min-w-[240px] flex-1 items-center gap-2 rounded-lg border border-line bg-surface px-3">
            <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="pmb_..."
              className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
            />
          </div>
          <button
            onClick={() => load(search.trim())}
            disabled={!search.trim() || loading}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            {loading ? 'Loading…' : 'Load'}
          </button>
        </div>
      </div>

      {searched && rows.length > 0 && (
        <>
          <div className="flex flex-wrap items-center gap-2.5">
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
                    {s === 'ALL'
                      ? 'All'
                      : providerQualificationStatusLabel(s)}
                  </button>
                ))}
              </div>
            </div>
          )}

          <div className="overflow-x-auto rounded-lg border border-line bg-surface">
            <table className="w-full min-w-[800px] border-collapse text-left">
              <thead>
                <tr className="border-b border-line bg-surface-2">
                  <Th>Qualification</Th>
                  <Th>Status</Th>
                  <Th>Issued</Th>
                  <Th>Expires</Th>
                  <Th>Verified</Th>
                  <Th className="text-right">Version</Th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((row) => {
                  const tone = providerQualificationStatusTone(row.status);
                  return (
                    <tr
                      key={row.id}
                      className="border-b border-line last:border-0 hover:bg-surface-2"
                    >
                      <Td>
                        <div className="font-mono text-xs text-ink-600">
                          {row.qualificationId}
                        </div>
                        <div className="font-mono text-[10px] text-ink-400">
                          {row.id}
                        </div>
                      </Td>
                      <Td>
                        <span
                          className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                        >
                          <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                          {providerQualificationStatusLabel(row.status)}
                        </span>
                      </Td>
                      <Td>{formatDate(row.issuedAt)}</Td>
                      <Td>{formatDate(row.expiresAt)}</Td>
                      <Td>
                        <span className="text-xs text-ink-500">
                          {row.verifiedBy || '—'}
                        </span>
                      </Td>
                      <Td className="text-right">
                        <span className="font-mono text-xs text-ink-500">
                          {row.version}
                        </span>
                      </Td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      )}

      {searched && !loading && rows.length === 0 && !error && (
        <EmptyState
          image="/images/trips-empty.png"
          title="No qualifications on file"
          description="This membership has no qualifications recorded."
        />
      )}

      {!searched && (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-line bg-surface px-4 py-14 text-center">
          <div className="rounded-full bg-surface-2 p-4">
            <FileText className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-sm font-semibold text-ink-950">
            Enter a membership ID
          </h3>
          <p className="mt-1 max-w-sm text-sm text-ink-500">
            Provider qualifications are looked up per membership. Enter a
            membership ID above to load its records.
          </p>
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