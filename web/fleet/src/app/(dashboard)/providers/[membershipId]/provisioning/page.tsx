'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { providerService } from '@/lib/services/providerService';
import {
  provisioningOverallLabel,
  provisioningOverallTone,
  provisioningStepLabel,
} from '@/lib/utils/providerLabels';
import type { ProvisioningStatus } from '@/lib/types/provider';

const STEP_KEYS = [
  'providerResolutionStatus',
  'providerIdentityStatus',
  'associationStatus',
  'subWalletStatus',
  'notificationStatus',
] as const;

export default function ProvisioningPage() {
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [status, setStatus] = useState<ProvisioningStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!membershipId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    providerService
      .provisioningStatus(membershipId)
      .then((data) => {
        if (!cancelled) setStatus(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load provisioning status.',
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
        Loading provisioning status…
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

  if (!status) return null;

  const overall = provisioningOverallTone(status.overallStatus);

  return (
    <div className="space-y-5">
      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-ink-950">Overall status</h3>
          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${overall.bg} ${overall.text}`}>
            <span className={`h-1.5 w-1.5 rounded-full ${overall.dot}`} />
            {provisioningOverallLabel(status.overallStatus)}
          </span>
        </div>
      </div>

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-4 text-sm font-semibold text-ink-950">Steps</h3>
        <ol className="space-y-0">
          {STEP_KEYS.map((key, index) => {
            const value = status[key];
            const isLast = index === STEP_KEYS.length - 1;
            return (
              <li key={key} className="relative flex gap-3.5 pb-4 last:pb-0">
                {!isLast && (
                  <div className="absolute left-[14px] top-[28px] bottom-0 w-px bg-line" />
                )}
                <div className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-full bg-surface-2 text-[11px] font-bold text-ink-500">
                  {index + 1}
                </div>
                <div className="flex-1 min-w-0 pt-0.5">
                  <div className="text-sm font-semibold text-ink-950">
                    {provisioningStepLabel(key)}
                  </div>
                  <div className="font-mono text-[11px] text-ink-500 mt-0.5">
                    {value || '—'}
                  </div>
                </div>
              </li>
            );
          })}
        </ol>
      </div>
    </div>
  );
}