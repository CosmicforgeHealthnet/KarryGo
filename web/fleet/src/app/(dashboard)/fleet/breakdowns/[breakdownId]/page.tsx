'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  AlertTriangle,
  ArrowLeft,
  MapPin,
  Truck,
} from 'lucide-react';
import { breakdownService } from '@/lib/services/breakdownService';
import {
  breakdownStatusLabel,
  breakdownStatusTone,
  formatDateTime,
  formatDuration,
  rescueStatusLabel,
  rescueStatusTone,
  severityLabel,
  severityTone,
  tripImpactLabel,
  tripImpactTone,
} from '@/lib/utils/breakdownLabels';
import type { Breakdown } from '@/lib/types/breakdown';

export default function BreakdownDetailPage() {
  const params = useParams();
  const breakdownId = params.breakdownId as string;

  const [breakdown, setBreakdown] = useState<Breakdown | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!breakdownId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    breakdownService
      .get(breakdownId)
      .then((data) => {
        if (!cancelled) setBreakdown(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load breakdown.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [breakdownId]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading breakdown…
      </div>
    );
  }

  if (error && !breakdown) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/breakdowns"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to breakdowns
        </Link>
      </div>
    );
  }

  if (!breakdown) return null;

  const severityToneCls = severityTone(breakdown.severity);
  const statusToneCls = breakdownStatusTone(breakdown.status);
  const rescueToneCls = rescueStatusTone(breakdown.rescueStatus);
  const impactToneCls = tripImpactTone(breakdown.tripImpact);
  const isActive =
    breakdown.status === 'OPEN' || breakdown.status === 'IN_PROGRESS';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Breakdown ID', value: <span className="font-mono text-xs">{breakdown.id}</span> },
    { label: 'Asset ID', value: <span className="font-mono text-xs">{breakdown.assetId}</span> },
    {
      label: 'Provider membership',
      value: breakdown.providerMembershipId ? (
        <span className="font-mono text-xs">{breakdown.providerMembershipId}</span>
      ) : (
        '—'
      ),
    },
    { label: 'Trip reference', value: breakdown.tripReference || '—' },
    { label: 'Location', value: breakdown.locationReference || '—' },
    { label: 'Occurred at', value: formatDateTime(breakdown.occurredAt) },
    { label: 'Downtime', value: formatDuration(breakdown.occurredAt, null) },
    { label: 'Towing reference', value: breakdown.towingReference || '—' },
    {
      label: 'Replacement asset',
      value: breakdown.replacementAssetId ? (
        <span className="font-mono text-xs">{breakdown.replacementAssetId}</span>
      ) : (
        '—'
      ),
    },
    { label: 'Created', value: formatDateTime(breakdown.createdAt) },
    { label: 'Updated', value: formatDateTime(breakdown.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{breakdown.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/breakdowns"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{breakdown.id}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${severityToneCls.bg} ${severityToneCls.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${severityToneCls.dot}`} />
                {severityLabel(breakdown.severity)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusToneCls.bg} ${statusToneCls.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusToneCls.dot}`} />
                {breakdownStatusLabel(breakdown.status)}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {breakdown.assetId}
            </p>
          </div>
        </div>
        {isActive && (
          <Link
            href={`/fleet/breakdowns/${breakdown.id}/edit`}
            className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Truck className="h-3.5 w-3.5" />
            Update
          </Link>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="grid grid-cols-2 gap-3.5 sm:grid-cols-4">
        <MiniCard label="Trip impact" value={tripImpactLabel(breakdown.tripImpact)} tone={impactToneCls} />
        <MiniCard label="Rescue" value={rescueStatusLabel(breakdown.rescueStatus)} tone={rescueToneCls} />
        <MiniCard
          label="Towing"
          value={breakdown.towingReference ? 'Required' : 'Not required'}
          tone={
            breakdown.towingReference
              ? { bg: 'bg-amber-bg', text: 'text-amber', dot: 'bg-amber' }
              : { bg: 'bg-surface-2', text: 'text-ink-500', dot: 'bg-ink-400' }
          }
        />
        <MiniCard
          label="Replacement"
          value={breakdown.replacementAssetId ? 'Assigned' : 'None'}
          tone={
            breakdown.replacementAssetId
              ? { bg: 'bg-primary-bg', text: 'text-primary', dot: 'bg-primary' }
              : { bg: 'bg-surface-2', text: 'text-ink-500', dot: 'bg-ink-400' }
          }
        />
      </div>

      {(breakdown.locationReference || breakdown.tripReference) && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <MapPin className="h-4 w-4" />
            Context
          </h3>
          <dl className="space-y-2">
            {breakdown.locationReference && (
              <div className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0">
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                  Location
                </dt>
                <dd className="text-sm text-ink-950">{breakdown.locationReference}</dd>
              </div>
            )}
            {breakdown.tripReference && (
              <div className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0">
                <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                  Trip reference
                </dt>
                <dd className="font-mono text-xs text-ink-950">
                  {breakdown.tripReference}
                </dd>
              </div>
            )}
          </dl>
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">Details</h3>
        <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
          {rows.map((r) => (
            <div
              key={r.label}
              className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0"
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {r.label}
              </dt>
              <dd className="text-sm text-ink-950 text-right truncate">
                {r.value}
              </dd>
            </div>
          ))}
        </dl>
      </div>
    </div>
  );
}

function MiniCard({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone: { bg: string; text: string; dot: string };
}) {
  return (
    <div className="rounded-lg border border-line bg-surface p-3.5">
      <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">
        {label}
      </div>
      <div className="mt-1.5">
        <span
          className={`inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-[11px] font-semibold ${tone.bg} ${tone.text}`}
        >
          <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
          {value}
        </span>
      </div>
    </div>
  );
}