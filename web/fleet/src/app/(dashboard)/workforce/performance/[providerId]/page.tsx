'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  Activity,
  ArrowLeft,
  BarChart3,
  Calendar,
  TrendingUp,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { workforceService } from '@/lib/services/workforceService';
import { formatDate, formatDateTime } from '@/lib/utils/workforceLabels';
import type { PerformanceSnapshot } from '@/lib/types/workforce';

export default function ProviderPerformancePage() {
  const params = useParams();
  const membershipId = params.providerId as string;

  const [rows, setRows] = useState<PerformanceSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<string | null>(null);

  useEffect(() => {
    if (!membershipId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .listPerformance(membershipId)
      .then((data) => {
        if (!cancelled) setRows(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load performance snapshots.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  const latest = useMemo(() => rows[0] ?? null, [rows]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading performance snapshots…
      </div>
    );
  }

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/workforce/shifts"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <h1 className="truncate">Performance</h1>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {membershipId}
            </p>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {latest && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="mb-3 flex items-center justify-between">
            <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
              <TrendingUp className="h-4 w-4" />
              Latest snapshot
            </h3>
            <span className="text-xs text-ink-500">
              {formatDate(latest.periodStart)} – {formatDate(latest.periodEnd)}
            </span>
          </div>

          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {Object.entries(latest.calculatedMetrics).map(([key, value]) => (
              <MetricTile key={key} label={key} value={value} />
            ))}
          </div>

          <dl className="mt-4 grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
            <Row label="Snapshot ID" value={<span className="font-mono text-xs">{latest.id}</span>} />
            <Row label="Formula version" value={latest.formulaVersion} />
            <Row label="Generation status" value={latest.generationStatus} />
            <Row label="Generated at" value={formatDateTime(latest.generatedAt)} />
            <Row label="Version" value={<span className="font-mono text-xs">{latest.version}</span>} />
          </dl>
        </div>
      )}

      {rows.length === 0 ? (
        <EmptyState
          image="/images/vehicles-empty.png"
          title="No performance snapshots"
          description="Performance snapshots are generated per membership and period."
        />
      ) : (
        <div className="space-y-2">
          {rows.map((row) => {
            const isExpanded = expanded === row.id;
            return (
              <div
                key={row.id}
                className="rounded-lg border border-line bg-surface"
              >
                <button
                  onClick={() => setExpanded(isExpanded ? null : row.id)}
                  className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors hover:bg-surface-2"
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                      <Calendar className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-ink-950 truncate">
                        {formatDate(row.periodStart)} – {formatDate(row.periodEnd)}
                      </div>
                      <div className="font-mono text-[10px] text-ink-400 truncate">
                        {row.id} · {row.formulaVersion}
                      </div>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <span
                      className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        row.generationStatus === 'COMPLETE'
                          ? 'bg-green-bg text-green'
                          : row.generationStatus === 'FAILED'
                          ? 'bg-red-bg text-red'
                          : 'bg-amber-bg text-amber'
                      }`}
                    >
                      {row.generationStatus}
                    </span>
                    <BarChart3 className="h-4 w-4 text-ink-400" />
                  </div>
                </button>

                {isExpanded && (
                  <div className="border-t border-line p-4 space-y-4">
                    <div>
                      <h4 className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
                        <Activity className="h-3.5 w-3.5" />
                        Calculated metrics
                      </h4>
                      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                        {Object.entries(row.calculatedMetrics).map(
                          ([key, value]) => (
                            <MetricTile key={key} label={key} value={value} />
                          ),
                        )}
                      </div>
                    </div>

                    {Object.keys(row.metricInputs).length > 0 && (
                      <div>
                        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
                          Metric inputs
                        </h4>
                        <pre className="max-h-60 overflow-auto rounded-lg bg-surface-2 p-3 text-[11px] text-ink-700">
                          {JSON.stringify(row.metricInputs, null, 2)}
                        </pre>
                      </div>
                    )}

                    {Object.keys(row.explanation).length > 0 && (
                      <div>
                        <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-ink-500">
                          Explanation
                        </h4>
                        <pre className="max-h-60 overflow-auto rounded-lg bg-surface-2 p-3 text-[11px] text-ink-700">
                          {JSON.stringify(row.explanation, null, 2)}
                        </pre>
                      </div>
                    )}

                    <dl className="grid grid-cols-1 gap-x-6 gap-y-2 sm:grid-cols-2">
                      <Row label="Generated" value={formatDateTime(row.generatedAt)} />
                      <Row label="Version" value={<span className="font-mono text-xs">{row.version}</span>} />
                    </dl>
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function MetricTile({
  label,
  value,
}: {
  label: string;
  value: unknown;
}) {
  const display =
    typeof value === 'number' || typeof value === 'string'
      ? String(value)
      : JSON.stringify(value);
  return (
    <div className="rounded-lg border border-line bg-surface-2 p-3">
      <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500 truncate">
        {label}
      </div>
      <div className="mt-1 text-sm font-semibold text-ink-950 truncate">
        {display}
      </div>
    </div>
  );
}

function Row({
  label,
  value,
}: {
  label: string;
  value: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0">
      <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </dt>
      <dd className="text-sm text-ink-950 text-right truncate">{value}</dd>
    </div>
  );
}