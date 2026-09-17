'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { usePathname, useParams } from 'next/navigation';
import { Layers, GitBranch, Clock } from 'lucide-react';
import { providerService } from '@/lib/services/providerService';
import {
  serviceCategoryLabel,
  relationshipTypeLabel,
  statusLabel,
  statusTone,
} from '@/lib/utils/providerLabels';
import type { ProviderMembership } from '@/lib/types/provider';

const TABS = [
  { label: 'Overview',     segment: '',             icon: Layers },
  { label: 'Provisioning', segment: 'provisioning', icon: GitBranch },
  { label: 'History',      segment: 'history',      icon: Clock },
];

export default function ProviderDetailLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const pathname = usePathname();
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [membership, setMembership] = useState<ProviderMembership | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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
        const message =
          err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to load provider.';
        setError(message);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  const basePath = `/providers/${membershipId}`;

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading provider…
      </div>
    );
  }

  if (error || !membership) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error ?? 'Provider not found.'}
        </div>
        <Link
          href="/providers"
          className="inline-flex items-center justify-center rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          Back to providers
        </Link>
      </div>
    );
  }

  const tone = statusTone(membership.status);

  return (
    <div className="space-y-5">
      <div>
        <div className="flex flex-wrap items-center gap-2.5">
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            {serviceCategoryLabel(membership.serviceCategory)} provider
          </h1>
          <span className="font-mono text-sm text-ink-400">
            {membership.membershipId}
          </span>
          <span
            className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
          >
            <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
            {statusLabel(membership.status)}
          </span>
        </div>
        <p className="mt-0.5 text-sm text-ink-500">
          {relationshipTypeLabel(membership.relationshipType)}
          {membership.homeBranchId ? ` · ${membership.homeBranchId}` : ''}
        </p>
      </div>

      <div className="flex gap-1 border-b border-line overflow-x-auto scrollbar-hide">
        {TABS.map((tab) => {
          const href = tab.segment ? `${basePath}/${tab.segment}` : basePath;
          const active = pathname === href;
          const Icon = tab.icon;

          return (
            <Link
              key={tab.label}
              href={href}
              className={`flex items-center gap-1.5 px-3.5 py-2.5 text-sm font-medium whitespace-nowrap transition-colors ${
                active
                  ? 'text-primary border-b-2 border-primary'
                  : 'text-ink-600 hover:text-ink-950'
              }`}
            >
              <Icon className="h-4 w-4" />
              {tab.label}
            </Link>
          );
        })}
      </div>

      <div>{children}</div>
    </div>
  );
}