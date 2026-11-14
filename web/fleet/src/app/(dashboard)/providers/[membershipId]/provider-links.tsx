'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { Mail } from 'lucide-react';
import { ApiErrorNotice } from '@/components/features/ApiErrorNotice';
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from '@/lib/api/errors';
import { providerService } from '@/lib/services/providerService';
import type { ProviderMembership } from '@/lib/types/provider';
import type { ProviderBranchLink, ProviderServiceRoles, ProviderVehicle } from '@/lib/types/providerAdmin';

const humanize = (value?: string) =>
  value ? value.replace(/_/g, ' ').toLowerCase().replace(/^\w/, (c) => c.toUpperCase()) : '—';
const formatDate = (iso?: string) => (iso ? new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '—');

const PENDING = ['INVITED', 'PENDING_PROVIDER_ACCEPTANCE', 'PENDING_SETUP'];

// Re-sends the invitation or first-login notice of a provider who has not
// finished joining. Fleet never issues provider credentials; the provider
// signs in through their own provider app.
export function ResendLoginCard({ membership }: { membership: ProviderMembership }) {
  const [key, setKey] = useState(newIdempotencyKey);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [result, setResult] = useState<string | null>(null);

  if (!PENDING.includes(membership.status)) return null;

  const resend = async () => {
    setBusy(true);
    setError(null);
    try {
      const r = await providerService.resendLogin(membership.membershipId, key);
      setKey(newIdempotencyKey());
      setResult(
        `${r.notice || 'Sign-in details sent'} to ${r.maskedEmail || 'the provider'}. ${r.resendsRemaining} resend${r.resendsRemaining === 1 ? '' : 's'} left.`,
      );
    } catch (err) {
      setError(describeApiError(err, 'The sign-in details could not be re-sent.'));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold text-ink-950">Provider has not finished joining</h3>
          <p className="mt-0.5 text-xs text-ink-500">Send the invitation or first sign-in notice again.</p>
        </div>
        <button
          onClick={resend}
          disabled={busy}
          className="flex items-center gap-1.5 rounded-md border border-line bg-surface px-3 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2 disabled:opacity-50"
        >
          <Mail className="h-3.5 w-3.5" />
          {busy ? 'Sending…' : 'Resend sign-in details'}
        </button>
      </div>
      <ApiErrorNotice error={error} className="mt-3" />
      {result && <p className="mt-3 rounded-lg border border-green/30 bg-green-bg px-3 py-2 text-xs text-green">{result}</p>}
    </div>
  );
}

// Where the provider works and with what: branches, assigned vehicles and the
// service roles and capabilities in effect.
export function ProviderLinks({ membershipId }: { membershipId: string }) {
  const [branches, setBranches] = useState<ProviderBranchLink[]>([]);
  const [vehicles, setVehicles] = useState<ProviderVehicle[]>([]);
  const [roles, setRoles] = useState<ProviderServiceRoles | null>(null);
  const [history, setHistory] = useState(false);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    let cancelled = false;
    Promise.allSettled([providerService.branches(membershipId), providerService.serviceRoles(membershipId)]).then(([b, r]) => {
      if (cancelled) return;
      if (b.status === 'fulfilled') setBranches(b.value);
      if (r.status === 'fulfilled') setRoles(r.value);
      const failed = [b, r].find((x) => x.status === 'rejected') as PromiseRejectedResult | undefined;
      if (failed) setError(describeApiError(failed.reason, 'Some provider details could not be loaded.'));
    });
    return () => {
      cancelled = true;
    };
  }, [membershipId]);

  useEffect(() => {
    providerService
      .vehicles(membershipId, history)
      .then(setVehicles)
      .catch((e) => setError(describeApiError(e, 'Vehicles could not be loaded.')));
  }, [membershipId, history]);

  return (
    <div className="space-y-5">
      <ApiErrorNotice error={error} />
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 text-sm font-semibold text-ink-950">Branches</h3>
          {branches.length === 0 ? (
            <p className="text-xs text-ink-500">No branch linked.</p>
          ) : (
            <ul className="space-y-2">
              {branches.map((b) => (
                <li key={b.branchId} className="flex items-center justify-between gap-2 text-sm">
                  <span className="text-ink-950">
                    {b.name} <span className="font-mono text-[10px] text-ink-500">{b.code}</span>
                  </span>
                  <span className="text-[11px] text-ink-500">{b.sources.map((s) => (s === 'MEMBERSHIP_HOME_BRANCH' ? 'Home branch' : 'Workforce profile')).join(', ')}</span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 text-sm font-semibold text-ink-950">Service roles</h3>
          {!roles ? (
            <p className="text-xs text-ink-500">—</p>
          ) : (
            <>
              <p className="text-xs text-ink-600">
                {humanize(roles.serviceCategory)} · {humanize(roles.relationshipType)}
                {roles.workforceRole ? ` · ${roles.workforceRole}` : ''}
                {roles.employmentType ? ` · ${humanize(roles.employmentType)}` : ''}
              </p>
              <ul className="mt-2 space-y-1.5">
                {(roles.capabilities ?? []).map((c) => (
                  <li key={c.id} className="flex items-center justify-between gap-2 text-xs">
                    <span className="text-ink-950">
                      {humanize(c.serviceType)} · {c.vehicleClasses.join(', ') || 'any vehicle'}
                    </span>
                    <span className={c.effective ? 'font-semibold text-green' : 'text-ink-400'}>{c.effective ? 'In effect' : humanize(c.status)}</span>
                  </li>
                ))}
                {(roles.capabilities ?? []).length === 0 && <li className="text-xs text-ink-500">No capabilities recorded.</li>}
              </ul>
            </>
          )}
        </div>
      </div>

      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="mb-3 flex items-center justify-between">
          <h3 className="text-sm font-semibold text-ink-950">Vehicles</h3>
          <label className="flex items-center gap-1.5 text-xs text-ink-600">
            <input type="checkbox" checked={history} onChange={(e) => setHistory(e.target.checked)} /> Include past assignments
          </label>
        </div>
        {vehicles.length === 0 ? (
          <p className="text-xs text-ink-500">No vehicle assigned.</p>
        ) : (
          <ul className="divide-y divide-line">
            {vehicles.map((v) => (
              <li key={v.assignmentId} className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm">
                <Link href={`/fleet/assets/${v.assetId}`} className="text-ink-950 hover:text-primary">
                  {v.assetCode} · {[v.manufacturer, v.model].filter(Boolean).join(' ') || humanize(v.assetType)}
                  {v.registrationReference ? ` · ${v.registrationReference}` : ''}
                </Link>
                <span className="text-xs text-ink-500">
                  {humanize(v.assignmentType)} · {humanize(v.assignmentStatus)} · from {formatDate(v.startAt)}
                  {v.endAt ? ` to ${formatDate(v.endAt)}` : ''}
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}
