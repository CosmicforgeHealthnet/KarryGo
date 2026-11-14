'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowRight, Search } from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { describeApiError, type ApiErrorInfo } from '@/lib/api/errors';
import { tripService } from '@/lib/services/operationsService';
import type { CorporateTrip } from '@/lib/types/operations';

const FRESHNESS_TONE: Record<string, string> = {
  FRESH: 'bg-green-bg text-green',
  CURRENT: 'bg-green-bg text-green',
  STALE: 'bg-amber-bg text-amber',
  GAPPED: 'bg-red-bg text-red',
};

const humanize = (value: string) =>
  value ? value.replace(/_/g, ' ').toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : '—';

const formatWhen = (iso?: string) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : '—';

const PAGE_SIZE = 50;

// Trips the company's providers run, with their lifecycle status as the
// execution services report it. Live position is on each trip's page.
export default function TripsPage() {
  const [rows, setRows] = useState<CorporateTrip[]>([]);
  const [nextOffset, setNextOffset] = useState<number | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [search, setSearch] = useState('');

  const loadPage = useCallback(async (offset: number) => {
    try {
      const page = await tripService.list({ limit: PAGE_SIZE, offset });
      setRows((r) => (offset === 0 ? page.items : [...r, ...page.items]));
      setNextOffset(page.hasMore ? (page.nextOffset ?? offset + page.items.length) : null);
      setError(null);
    } catch (err) {
      setError(describeApiError(err, 'Failed to load trips.'));
    }
  }, []);

  useEffect(() => {
    Promise.resolve()
      .then(() => loadPage(0))
      .finally(() => setLoading(false));
  }, [loadPage]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) =>
      [r.tripId, r.platformTripId, r.executingProviderId, r.tripStatus, r.serviceType, r.assignmentId ?? ''].join(' ').toLowerCase().includes(q),
    );
  }, [rows, search]);

  return (
    <div className="space-y-5">
      <div>
        <h1>Trips</h1>
        <p className="mt-1.5">Live and recent trips run by your providers, with their current lifecycle status.</p>
      </div>

      <ApiErrorNotice error={error} />

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by trip, provider or status..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-500"
          />
        </div>
        <span className="ml-auto whitespace-nowrap text-sm text-ink-500">
          {filtered.length} of {rows.length}
        </span>
      </div>

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">Loading trips…</div>
      ) : rows.length === 0 ? (
        <EmptyState image="/images/trips-empty.png" title="No trips yet" description="Trips appear here once your providers start work." />
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-1">
                  {['Trip', 'Service', 'Provider', 'Status', 'Signal', 'Last event', ''].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((t) => (
                  <tr key={t.tripId} className="border-b border-line hover:bg-surface-2">
                    <td className="px-4 py-3 font-mono text-xs text-ink-950">{t.platformTripId || t.tripId}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{humanize(t.serviceType || t.sourceService.replace(/^driver-|-service$/g, ''))}</td>
                    <td className="px-4 py-3 font-mono text-xs text-ink-600">{t.executingProviderId || '—'}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{humanize(t.tripStatus)}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${FRESHNESS_TONE[t.freshnessState] ?? 'bg-surface-2 text-ink-500'}`}>
                        {humanize(t.freshnessState)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatWhen(t.lastEventAt ?? t.updatedAt)}</td>
                    <td className="px-4 py-3 text-right">
                      <Link
                        href={`/operations/trips/${encodeURIComponent(t.tripId)}`}
                        className="inline-flex items-center gap-1 text-xs font-medium text-ink-400 hover:text-primary"
                      >
                        View <ArrowRight className="h-3.5 w-3.5" />
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {nextOffset != null && (
        <div className="flex justify-center">
          <button
            onClick={async () => {
              setLoadingMore(true);
              await loadPage(nextOffset);
              setLoadingMore(false);
            }}
            disabled={loadingMore}
            className="rounded-lg border border-line bg-surface px-4 py-2 text-xs font-medium text-ink-600 hover:bg-surface-2 disabled:opacity-60"
          >
            {loadingMore ? 'Loading…' : 'Load more trips'}
          </button>
        </div>
      )}
    </div>
  );
}
