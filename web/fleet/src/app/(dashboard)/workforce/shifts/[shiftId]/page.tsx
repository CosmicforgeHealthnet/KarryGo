'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  Calendar,
  CheckCircle2,
  Clock,
  Send,
  UserPlus,
  Users,
  X,
} from 'lucide-react';
import {
  newWorkforceIdempotencyKey,
  workforceService,
} from '@/lib/services/workforceService';
import {
  formatDateTime,
  formatTimeRange,
  shiftStatusLabel,
  shiftStatusTone,
} from '@/lib/utils/workforceLabels';
import type { Shift, ShiftAssignment } from '@/lib/types/workforce';

export default function ShiftDetailPage() {
  const params = useParams();
  const router = useRouter();
  const shiftId = params.shiftId as string;

  const [shift, setShift] = useState<Shift | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [showAssign, setShowAssign] = useState(false);
  const [assignForm, setAssignForm] = useState({
    membershipId: '',
    assignmentType: 'PRIMARY',
  });

  useEffect(() => {
    if (!shiftId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .getShift(shiftId)
      .then((data) => {
        if (!cancelled) setShift(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load shift.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [shiftId]);

  async function refresh() {
    if (!shift) return;
    const updated = await workforceService.getShift(shift.id);
    setShift(updated);
  }

  async function handlePublish() {
    if (!shift) return;
    setBusy(true);
    setError(null);
    try {
      await workforceService.publishShift(
        shift.id,
        shift.version,
        newWorkforceIdempotencyKey(),
      );
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to publish shift.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleCancel() {
    if (!shift) return;
    setBusy(true);
    setError(null);
    try {
      await workforceService.cancelShift(
        shift.id,
        shift.version,
        newWorkforceIdempotencyKey(),
      );
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to cancel shift.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleAssign() {
    if (!shift) return;
    setBusy(true);
    setError(null);
    try {
      await workforceService.assignShift(
        shift.id,
        {
          membershipId: assignForm.membershipId.trim(),
          assignmentType: assignForm.assignmentType,
        },
        newWorkforceIdempotencyKey(),
      );
      setShowAssign(false);
      setAssignForm({ membershipId: '', assignmentType: 'PRIMARY' });
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to assign shift.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading shift…
      </div>
    );
  }

  if (error && !shift) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/workforce/shifts"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to shifts
        </Link>
      </div>
    );
  }

  if (!shift) return null;

  const tone = shiftStatusTone(shift.status);
  const canEdit = shift.status === 'DRAFT';
  const canPublish = shift.status === 'DRAFT';
  const canCancel =
    shift.status === 'DRAFT' || shift.status === 'PUBLISHED';
  const canAssign =
    shift.status === 'DRAFT' || shift.status === 'PUBLISHED';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Shift ID', value: <span className="font-mono text-xs">{shift.id}</span> },
    { label: 'Name', value: shift.name || '—' },
    { label: 'Role', value: shift.role || '—' },
    { label: 'Branch', value: <span className="font-mono text-xs">{shift.branchId}</span> },
    { label: 'Start', value: formatDateTime(shift.startAt) },
    { label: 'End', value: formatDateTime(shift.endAt) },
    { label: 'Timezone', value: shift.timezone },
    { label: 'Capacity', value: shift.requiredCapacity },
    { label: 'Planner', value: shift.plannerUserId || '—' },
    { label: 'Created', value: formatDateTime(shift.createdAt) },
    { label: 'Updated', value: formatDateTime(shift.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{shift.version}</span> },
  ];

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
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{shift.name || 'Shift'}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                {shiftStatusLabel(shift.status)}
              </span>
            </div>
            <p className="mt-0.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-ink-500">
              <span className="flex items-center gap-1.5">
                <Calendar className="h-3.5 w-3.5" />
                {formatTimeRange(shift.startAt, shift.endAt)}
              </span>
              <span className="flex items-center gap-1.5">
                <Users className="h-3.5 w-3.5" />
                {shift.requiredCapacity} slots
              </span>
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canEdit && (
            <Link
              href={`/workforce/shifts/${shift.id}/edit`}
              className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Edit
            </Link>
          )}
          {canPublish && (
            <button
              onClick={handlePublish}
              disabled={busy}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              <Send className="h-3.5 w-3.5" />
              Publish
            </button>
          )}
          {canCancel && (
            <button
              onClick={handleCancel}
              disabled={busy}
              className="flex h-10 items-center gap-1.5 rounded-lg border border-red/30 bg-red-bg px-3.5 text-sm font-semibold text-red transition-colors hover:bg-red-bg/70 disabled:opacity-60"
            >
              <X className="h-3.5 w-3.5" />
              Cancel
            </button>
          )}
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {showAssign && (
        <div className="space-y-3 rounded-lg border border-line bg-surface p-4">
          <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
            <UserPlus className="h-4 w-4" />
            Assign to shift
          </h3>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Membership ID
              </label>
              <input
                value={assignForm.membershipId}
                onChange={(e) =>
                  setAssignForm({ ...assignForm, membershipId: e.target.value })
                }
                placeholder="pmb_..."
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </div>
            <div>
              <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
                Assignment type
              </label>
              <select
                value={assignForm.assignmentType}
                onChange={(e) =>
                  setAssignForm({
                    ...assignForm,
                    assignmentType: e.target.value,
                  })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              >
                <option value="PRIMARY">Primary</option>
                <option value="SECONDARY">Secondary</option>
                <option value="TEMPORARY">Temporary</option>
              </select>
            </div>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleAssign}
              disabled={busy || !assignForm.membershipId.trim()}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              {busy ? 'Assigning…' : 'Assign'}
            </button>
            <button
              onClick={() => setShowAssign(false)}
              disabled={busy}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {canAssign && !showAssign && (
        <button
          onClick={() => setShowAssign(true)}
          className="flex h-9 items-center gap-2 rounded-lg border border-line bg-surface px-3.5 text-xs font-semibold text-ink-700 transition-colors hover:bg-surface-2"
        >
          <UserPlus className="h-3.5 w-3.5" />
          Assign someone
        </button>
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