'use client';

import { useEffect, useState } from 'react';
import { useParams } from 'next/navigation';
import { AlertCircle, CheckCircle2, ShieldAlert } from 'lucide-react';
import { providerService } from '@/lib/services/providerService';
import {
  formatDate,
  onboardingSourceLabel,
  providerServiceLabel,
  relationshipTypeLabel,
  serviceCategoryLabel,
  statusLabel,
  statusTone,
} from '@/lib/utils/providerLabels';
import type { ProviderMembership } from '@/lib/types/provider';

export default function ProviderOverviewPage() {
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [membership, setMembership] = useState<ProviderMembership | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [confirmSuspend, setConfirmSuspend] = useState(false);
  const [suspending, setSuspending] = useState(false);

  useEffect(() => {
    if (!membershipId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    providerService
      .get(membershipId)
      .then((data) => {
        if (!cancelled) setMembership(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load provider.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  async function handleSuspend() {
    if (!membership) return;
    setSuspending(true);
    setError(null);
    try {
      await providerService.suspend(membership.membershipId, membership.version);
      const refreshed = await providerService.get(membership.membershipId);
      setMembership(refreshed);
      setConfirmSuspend(false);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to suspend provider.',
      );
    } finally {
      setSuspending(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-12 text-center text-sm text-ink-500">
        Loading provider…
      </div>
    );
  }

  if (error && !membership) {
    return (
      <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
        {error}
      </div>
    );
  }

  if (!membership) return null;

  const tone = statusTone(membership.status);
  const canSuspend = membership.status === 'ACTIVE';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Membership ID', value: <span className="font-mono text-xs">{membership.membershipId}</span> },
    { label: 'Status', value: <span className={`font-semibold ${tone.text}`}>{statusLabel(membership.status)}</span> },
    { label: 'Service category', value: serviceCategoryLabel(membership.serviceCategory) },
    { label: 'Owning service', value: providerServiceLabel(membership.providerService) },
    { label: 'Relationship', value: relationshipTypeLabel(membership.relationshipType) },
    { label: 'Onboarding source', value: onboardingSourceLabel(membership.onboardingSource) },
    { label: 'Home branch', value: membership.homeBranchId ?? '—' },
    { label: 'Provider ID', value: membership.providerId ? <span className="font-mono text-xs">{membership.providerId}</span> : <span className="text-ink-400">Not yet linked</span> },
    { label: 'Joined', value: formatDate(membership.joinedAt) },
    { label: 'Version', value: String(membership.version) },
  ];

  return (
    <div className="space-y-5">
      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">Membership</h3>
        <dl className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          {rows.map((row) => (
            <div
              key={row.label}
              className="flex items-center justify-between gap-3 border-b border-line py-2 last:border-0"
            >
              <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
                {row.label}
              </dt>
              <dd className="text-sm text-ink-950 text-right truncate">
                {row.value}
              </dd>
            </div>
          ))}
        </dl>
      </div>

      {canSuspend && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-amber-bg p-2">
              <ShieldAlert className="h-4 w-4 text-amber" />
            </div>
            <div className="flex-1">
              <h3 className="text-sm font-semibold text-ink-950">Suspend membership</h3>
              <p className="mt-0.5 text-xs text-ink-500">
                Revokes organization matching eligibility in the owning provider
                service. The provider identity itself is not affected.
              </p>

              {!confirmSuspend ? (
                <button
                  onClick={() => setConfirmSuspend(true)}
                  className="mt-3 flex items-center gap-2 rounded-md border border-amber/30 bg-amber-bg px-3.5 py-1.5 text-xs font-semibold text-amber transition-colors hover:bg-amber-bg/70"
                >
                  <ShieldAlert className="h-3.5 w-3.5" />
                  Suspend
                </button>
              ) : (
                <div className="mt-3 flex flex-wrap items-center gap-2">
                  <span className="flex items-center gap-1 text-xs text-ink-600">
                    <AlertCircle className="h-3.5 w-3.5 text-amber" />
                    Are you sure?
                  </span>
                  <button
                    onClick={handleSuspend}
                    disabled={suspending}
                    className="rounded-md bg-amber px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-amber/90 disabled:opacity-50"
                  >
                    {suspending ? 'Suspending…' : 'Confirm suspend'}
                  </button>
                  <button
                    onClick={() => setConfirmSuspend(false)}
                    disabled={suspending}
                    className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {membership.status === 'SUSPENDED' && (
        <div className="flex items-center gap-2 rounded-lg border border-line bg-surface-2 px-4 py-3 text-sm text-ink-600">
          <CheckCircle2 className="h-4 w-4 text-green" />
          This membership is currently suspended.
        </div>
      )}
    </div>
  );
}