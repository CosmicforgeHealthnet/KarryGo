'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  Clock,
  FileText,
  Shield,
  Wrench,
  X,
} from 'lucide-react';
import {
  maintenanceService,
  newMaintenanceIdempotencyKey,
} from '@/lib/services/maintenanceService';
import {
  formatDateTime,
  formatDuration,
  returnToServiceLabel,
  returnToServiceTone,
  workOrderPriorityLabel,
  workOrderPriorityTone,
  workOrderSourceLabel,
  workOrderStatusLabel,
  workOrderStatusTone,
} from '@/lib/utils/maintenanceLabels';
import type { WorkOrder } from '@/lib/types/maintenance';

export default function WorkOrderDetailPage() {
  const params = useParams();
  const router = useRouter();
  const workOrderId = params.workOrderId as string;

  const [workOrder, setWorkOrder] = useState<WorkOrder | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({ diagnosis: '', requiredWork: '' });

  const [completing, setCompleting] = useState(false);
  const [completeForm, setCompleteForm] = useState({
    requiredWork: '',
    evidenceMediaIds: '',
  });

  useEffect(() => {
    if (!workOrderId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    maintenanceService
      .getWorkOrder(workOrderId)
      .then((data) => {
        if (cancelled) return;
        setWorkOrder(data);
        setForm({
          diagnosis: data.diagnosis ?? '',
          requiredWork: data.requiredWork ?? '',
        });
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load work order.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [workOrderId]);

  async function refresh() {
    if (!workOrder) return;
    const updated = await maintenanceService.getWorkOrder(workOrder.id);
    setWorkOrder(updated);
    setForm({
      diagnosis: updated.diagnosis ?? '',
      requiredWork: updated.requiredWork ?? '',
    });
  }

  async function handleApprove() {
    if (!workOrder) return;
    setBusy(true);
    setError(null);
    try {
      await maintenanceService.approveWorkOrder(
        workOrder.id,
        { expectedVersion: workOrder.version },
        newMaintenanceIdempotencyKey(),
      );
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to approve work order.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleComplete(e: FormEvent) {
    e.preventDefault();
    if (!workOrder) return;
    setBusy(true);
    setError(null);
    try {
      const evidenceMediaIds = completeForm.evidenceMediaIds
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);

      await maintenanceService.completeWorkOrder(
        workOrder.id,
        {
          expectedVersion: workOrder.version,
          requiredWork: completeForm.requiredWork.trim() || undefined,
          evidenceMediaIds,
        },
        newMaintenanceIdempotencyKey(),
      );
      setCompleting(false);
      setCompleteForm({ requiredWork: '', evidenceMediaIds: '' });
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to complete work order.',
      );
    } finally {
      setBusy(false);
    }
  }

  async function handleSaveEdit(e: FormEvent) {
    e.preventDefault();
    if (!workOrder) return;
    setBusy(true);
    setError(null);
    try {
      await maintenanceService.updateWorkOrder(workOrder.id, {
        expectedVersion: workOrder.version,
        diagnosis: form.diagnosis.trim() || undefined,
        requiredWork: form.requiredWork.trim() || undefined,
      });
      setEditing(false);
      await refresh();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update work order.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading work order…
      </div>
    );
  }

  if (error && !workOrder) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/work-orders"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to work orders
        </Link>
      </div>
    );
  }

  if (!workOrder) return null;

  const statusTone = workOrderStatusTone(workOrder.status);
  const priorityTone = workOrderPriorityTone(workOrder.priority);
  const rtsTone = returnToServiceTone(workOrder.returnToServiceStatus);

  const canApprove =
    workOrder.status === 'DRAFT' || workOrder.status === 'PENDING_APPROVAL';
  const canComplete =
    workOrder.status === 'APPROVED' || workOrder.status === 'IN_PROGRESS';
  const canReturnToService =
    workOrder.status === 'COMPLETED' &&
    workOrder.returnToServiceStatus !== 'APPROVED';
  const canEdit = workOrder.status !== 'COMPLETED' && workOrder.status !== 'CANCELLED';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Work order ID', value: <span className="font-mono text-xs">{workOrder.id}</span> },
    { label: 'Asset ID', value: <span className="font-mono text-xs">{workOrder.assetId}</span> },
    { label: 'Source', value: workOrderSourceLabel(workOrder.sourceType) },
    { label: 'Source reference', value: <span className="font-mono text-xs">{workOrder.sourceReference || '—'}</span> },
    { label: 'Diagnosis', value: workOrder.diagnosis || '—' },
    { label: 'Required work', value: workOrder.requiredWork || '—' },
    { label: 'Approved by', value: workOrder.approvedBy || '—' },
    { label: 'Approved at', value: formatDateTime(workOrder.approvedAt) },
    { label: 'Started at', value: formatDateTime(workOrder.startedAt) },
    { label: 'Completed at', value: formatDateTime(workOrder.completedAt) },
    { label: 'Downtime start', value: formatDateTime(workOrder.downtimeStart) },
    { label: 'Downtime end', value: formatDateTime(workOrder.downtimeEnd) },
    {
      label: 'Downtime',
      value: formatDuration(workOrder.downtimeStart, workOrder.downtimeEnd),
    },
    { label: 'Created', value: formatDateTime(workOrder.createdAt) },
    { label: 'Updated', value: formatDateTime(workOrder.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{workOrder.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/work-orders"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">{workOrder.id}</h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${priorityTone.bg} ${priorityTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${priorityTone.dot}`} />
                {workOrderPriorityLabel(workOrder.priority)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {workOrderStatusLabel(workOrder.status)}
              </span>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-medium ${rtsTone.bg} ${rtsTone.text}`}
              >
                RTS: {returnToServiceLabel(workOrder.returnToServiceStatus)}
              </span>
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {workOrder.assetId}
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canEdit && !editing && (
            <button
              onClick={() => setEditing(true)}
              className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              <Wrench className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
          {canApprove && (
            <button
              onClick={handleApprove}
              disabled={busy}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              Approve
            </button>
          )}
          {canComplete && !completing && (
            <button
              onClick={() => setCompleting(true)}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              Complete
            </button>
          )}
          {canReturnToService && (
            <Link
              href={`/fleet/work-orders/${workOrder.id}/return-to-service`}
              className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              <Shield className="h-3.5 w-3.5" />
              Return to service
            </Link>
          )}
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {editing && (
        <form
          onSubmit={handleSaveEdit}
          className="space-y-4 rounded-lg border border-line bg-surface p-4"
        >
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-ink-950">
              Edit work order
            </h3>
            <button
              type="button"
              onClick={() => setEditing(false)}
              className="rounded-md p-1 text-ink-400 hover:bg-surface-2 hover:text-ink-700"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <Field label="Diagnosis">
            <textarea
              rows={3}
              value={form.diagnosis}
              onChange={(e) => setForm({ ...form, diagnosis: e.target.value })}
              className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <Field label="Required work">
            <textarea
              rows={3}
              value={form.requiredWork}
              onChange={(e) => setForm({ ...form, requiredWork: e.target.value })}
              className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={busy}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              {busy ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              disabled={busy}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
        </form>
      )}

      {completing && (
        <form
          onSubmit={handleComplete}
          className="space-y-4 rounded-lg border border-line bg-surface p-4"
        >
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-semibold text-ink-950">
              Complete work order
            </h3>
            <button
              type="button"
              onClick={() => setCompleting(false)}
              className="rounded-md p-1 text-ink-400 hover:bg-surface-2 hover:text-ink-700"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <Field label="Work summary">
            <textarea
              rows={3}
              value={completeForm.requiredWork}
              onChange={(e) =>
                setCompleteForm({ ...completeForm, requiredWork: e.target.value })
              }
              placeholder="What was actually done"
              className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <Field label="Evidence media IDs (comma separated)">
            <input
              value={completeForm.evidenceMediaIds}
              onChange={(e) =>
                setCompleteForm({
                  ...completeForm,
                  evidenceMediaIds: e.target.value,
                })
              }
              placeholder="med_..., med_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={busy}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              {busy ? 'Completing…' : 'Complete'}
            </button>
            <button
              type="button"
              onClick={() => setCompleting(false)}
              disabled={busy}
              className="rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              Cancel
            </button>
          </div>
        </form>
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

      {workOrder.evidenceMediaIds.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <FileText className="h-4 w-4" />
            Evidence
          </h3>
          <div className="flex flex-wrap gap-2">
            {workOrder.evidenceMediaIds.map((id) => (
              <span
                key={id}
                className="rounded-full bg-surface-2 px-3 py-1 font-mono text-xs text-ink-700"
              >
                {id}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </label>
      {children}
    </div>
  );
}