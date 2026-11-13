'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { ArrowRight, Search } from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { controlTowerService } from '@/lib/services/controlTowerService';
import type { TrackingProjection } from '@/lib/types/controlTower';

const FRESHNESS_TONE: Record<string, string> = {
  FRESH: 'bg-green-bg text-green',
  STALE: 'bg-amber-bg text-amber',
  GAPPED: 'bg-red-bg text-red',
};

const humanize = (value: string) =>
  value ? value.replace(/_/g, ' ').toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : '—';

const formatWhen = (iso?: string) =>
  iso ? new Date(iso).toLocaleString(undefined, { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }) : '—';

// Trips the company's providers are running, as last reported by the
// execution services.
export default function TripsPage() {
  const [rows, setRows] = useState<TrackingProjection[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  useEffect(() => {
    let cancelled = false;
    controlTowerService
      .listTrips({ limit: 100 })
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (!cancelled) setError(err?.response?.data?.error?.message ?? 'Failed to load trips.');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return rows;
    return rows.filter((r) =>
      [r.canonicalTripReference, r.vehicleOrTruckReference, r.providerId, r.operationalStatus].join(' ').toLowerCase().includes(q),
    );
  }, [rows, search]);

  return (
    <div className="space-y-5">
      <div>
        <h1>Trips</h1>
        <p className="mt-1.5">Live and recent trips run by your providers, with their latest reported position and status.</p>
      </div>

      {error && <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">{error}</div>}

      <div className="flex flex-wrap items-center gap-2.5">
        <div className="flex h-10 w-full items-center gap-2 rounded-lg border border-line bg-surface px-3 sm:max-w-xs">
          <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-500" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by trip, vehicle or status..."
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
        <EmptyState
          image="/images/trips-empty.png"
          title="No trips yet"
          description="Trips appear here once your providers start work."
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-line bg-surface">
          <div className="overflow-x-auto">
            <table className="w-full min-w-[760px] border-collapse text-sm">
              <thead>
                <tr className="bg-surface-1">
                  {['Trip', 'Service', 'Vehicle', 'Status', 'Signal', 'Last update', ''].map((h) => (
                    <th key={h} className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">
                      {h}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {filtered.map((t) => (
                  <tr key={t.canonicalTripReference} className="border-b border-line hover:bg-surface-2">
                    <td className="px-4 py-3 font-mono text-xs text-ink-950">{t.canonicalTripReference}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{humanize(t.sourceService.replace(/^driver-|-service$/g, ''))}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{t.vehicleOrTruckReference || '—'}</td>
                    <td className="px-4 py-3 text-xs text-ink-600">{humanize(t.operationalStatus)}</td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${FRESHNESS_TONE[t.freshnessStatus] ?? 'bg-surface-2 text-ink-500'}`}>
                        {humanize(t.freshnessStatus)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-500">{formatWhen(t.sourceTimestamp)}</td>
                    <td className="px-4 py-3 text-right">
                      <Link
                        href={`/operations/trips/${encodeURIComponent(t.canonicalTripReference)}`}
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
    </div>
  );
}
