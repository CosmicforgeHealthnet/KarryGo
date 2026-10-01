'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { Clock } from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import { providerService } from '@/lib/services/providerService';
import { formatDateTime, statusLabel, statusTone } from '@/lib/utils/providerLabels';
import type { MembershipHistoryEntry } from '@/lib/types/provider';

export default function HistoryPage() {
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [entries, setEntries] = useState<MembershipHistoryEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!membershipId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    providerService
      .history(membershipId)
      .then((data) => {
        if (!cancelled) setEntries(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load history.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-12 text-center text-sm text-ink-500">
        Loading history…
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
        {error}
      </div>
    );
  }

  if (entries.length === 0) {
    return (
      <EmptyState
        image="/images/trips-empty.png"
        title="No history yet"
        description="Membership transitions will appear here as they occur."
      />
    );
  }

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="mb-4 flex items-center gap-2.5">
        <Clock className="h-5 w-5 text-ink-400" />
        <h3 className="text-sm font-semibold text-ink-950">Membership history</h3>
        <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
          {entries.length} {entries.length === 1 ? 'event' : 'events'}
        </span>
      </div>

      <ol className="space-y-0">
        {entries.map((entry, index) => {
          const isLast = index === entries.length - 1;
          const toTone = statusTone(entry.toStatus);

          return (
            <li key={entry.id} className="relative flex gap-3.5 pb-4 last:pb-0">
              {!isLast && (
                <div className="absolute left-[14px] top-[28px] bottom-0 w-px bg-line" />
              )}
              <div className={`flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full ${toTone.bg} ${toTone.text}`}>
                <span className={`h-2 w-2 rounded-full ${toTone.dot}`} />
              </div>
              <div className="flex-1 min-w-0 pt-0.5">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold text-ink-950">
                    {entry.fromStatus
                      ? `${statusLabel(entry.fromStatus)} → ${statusLabel(entry.toStatus)}`
                      : statusLabel(entry.toStatus)}
                  </span>
                </div>
                <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-ink-500">
                  <span>{formatDateTime(entry.occurredAt)}</span>
                  {entry.reasonCode && (
                    <>
                      <span className="text-ink-300">·</span>
                      <span className="font-mono">{entry.reasonCode}</span>
                    </>
                  )}
                  {entry.actorType && (
                    <>
                      <span className="text-ink-300">·</span>
                      <span>{entry.actorType}</span>
                    </>
                  )}
                </div>
              </div>
            </li>
          );
        })}
      </ol>
    </div>
  );
}