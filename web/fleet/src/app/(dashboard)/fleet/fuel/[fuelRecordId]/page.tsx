'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  ArrowLeft,
  Check,
  Droplets,
  X,
} from 'lucide-react';
import {
  fuelService,
  newFuelIdempotencyKey,
} from '@/lib/services/fuelService';
import {
  approvalStatusLabel,
  approvalStatusTone,
  formatDateTime,
  formatNumber,
  fuelOrEnergyTypeLabel,
  fuelUnitLabel,
} from '@/lib/utils/fuelLabels';
import type { FuelRecord } from '@/lib/types/fuel';

export default function FuelRecordDetailPage() {
  const params = useParams();
  const router = useRouter();
  const fuelRecordId = params.fuelRecordId as string;

  const [record, setRecord] = useState<FuelRecord | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [action, setAction] = useState<'approve' | 'reject' | null>(null);
  const [reason, setReason] = useState('');

  useEffect(() => {
    if (!fuelRecordId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    fuelService
      .get(fuelRecordId)
      .then((data) => {
        if (!cancelled) setRecord(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load fuel record.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [fuelRecordId]);

  async function runAction() {
    if (!record || !action) return;
    setBusy(true);
    setError(null);
    try {
      if (action === 'approve') {
        await fuelService.approve(
          record.id,
          { expectedVersion: record.version, reason: reason.trim() || undefined },
          newFuelIdempotencyKey(),
        );
      } else {
        await fuelService.reject(
          record.id,
          { expectedVersion: record.version, reason: reason.trim() || undefined },
          newFuelIdempotencyKey(),
        );
      }
      const refreshed = await fuelService.get(record.id);
      setRecord(refreshed);
      setAction(null);
      setReason('');
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Action failed.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading fuel record…
      </div>
    );
  }

  if (error && !record) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/fuel"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to fuel records
        </Link>
      </div>
    );
  }

  if (!record) return null;

  const tone = approvalStatusTone(record.approvalStatus);
  const canDecide = record.approvalStatus === 'PENDING';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Record ID', value: <span className="font-mono text-xs">{record.id}</span> },
    { label: 'Asset ID', value: <span className="font-mono text-xs">{record.assetId}</span> },
    {
      label: 'Provider membership',
      value: record.providerMembershipId ? (
        <span className="font-mono text-xs">{record.providerMembershipId}</span>
      ) : (
        '—'
      ),
    },
    { label: 'Fuel / energy', value: fuelOrEnergyTypeLabel(record.fuelOrEnergyType) },
    {
      label: 'Quantity',
      value: `${formatNumber(record.quantity)} ${fuelUnitLabel(record.unit)}`,
    },
    { label: 'Odometer', value: formatNumber(record.odometer) },
    { label: 'Operating hours', value: formatNumber(record.operatingHours) },
    {
      label: 'Supplier reference',
      value: record.supplierReference || '—',
    },
    {
      label: 'Receipt media',
      value: record.receiptMediaId ? (
        <span className="font-mono text-xs">{record.receiptMediaId}</span>
      ) : (
        '—'
      ),
    },
    { label: 'Recorded at', value: formatDateTime(record.recordedAt) },
    { label: 'Approved by', value: record.approvedBy || '—' },
    { label: 'Created', value: formatDateTime(record.createdAt) },
    { label: 'Updated', value: formatDateTime(record.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{record.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/fuel"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{record.id}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                {approvalStatusLabel(record.approvalStatus)}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {record.assetId}
            </p>
          </div>
        </div>

        {canDecide && !action && (
          <div className="flex flex-wrap items-center gap-2">
            <button
              onClick={() => setAction('approve')}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-green px-3.5 text-sm font-semibold text-white transition-colors hover:bg-green/90"
            >
              <Check className="h-3.5 w-3.5" />
              Approve
            </button>
            <button
              onClick={() => setAction('reject')}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-red px-3.5 text-sm font-semibold text-white transition-colors hover:bg-red/90"
            >
              <X className="h-3.5 w-3.5" />
              Reject
            </button>
          </div>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-md bg-primary-bg text-primary">
            <Droplets className="h-5 w-5" />
          </div>
          <div>
            <div className="text-2xl font-bold text-ink-950">
              {formatNumber(record.quantity)}{' '}
              <span className="text-sm font-normal text-ink-500">
                {fuelUnitLabel(record.unit)}
              </span>
            </div>
            <div className="text-sm text-ink-500">
              {fuelOrEnergyTypeLabel(record.fuelOrEnergyType)}
            </div>
          </div>
        </div>
      </div>

      {action && (
        <div
          className={`rounded-lg border p-4 ${
            action === 'approve'
              ? 'border-green/30 bg-green-bg/20'
              : 'border-red/30 bg-red-bg/20'
          }`}
        >
          <h3 className="text-sm font-semibold text-ink-950">
            {action === 'approve' ? 'Approve fuel record?' : 'Reject fuel record?'}
          </h3>
          <p className="mt-0.5 text-xs text-ink-600">
            {action === 'approve'
              ? 'Approving confirms the record is accurate and complete.'
              : 'Rejecting marks the record as not accepted. A reason is recommended.'}
          </p>

          <div className="mt-3">
            <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
              Reason {action === 'reject' && <span className="text-red">*</span>}
            </label>
            <textarea
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder={
                action === 'approve'
                  ? 'Optional note'
                  : 'Why is this record being rejected?'
              }
              className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </div>

          <div className="mt-3 flex gap-2">
            <button
              onClick={runAction}
              disabled={busy || (action === 'reject' && !reason.trim())}
              className={`rounded-md px-3.5 py-1.5 text-xs font-semibold text-white transition-colors disabled:opacity-60 ${
                action === 'approve'
                  ? 'bg-green hover:bg-green/90'
                  : 'bg-red hover:bg-red/90'
              }`}
            >
              {busy
                ? 'Working…'
                : action === 'approve'
                ? 'Confirm approve'
                : 'Confirm reject'}
            </button>
            <button
              onClick={() => {
                setAction(null);
                setReason('');
              }}
              disabled={busy}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
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