'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  FileText,
  Shield,
  X,
} from 'lucide-react';
import {
  maintenanceService,
  newMaintenanceIdempotencyKey,
} from '@/lib/services/maintenanceService';
import {
  formatDateTime,
  workOrderPriorityLabel,
  workOrderPriorityTone,
  workOrderSourceLabel,
  workOrderStatusLabel,
  workOrderStatusTone,
} from '@/lib/utils/maintenanceLabels';
import type { WorkOrder } from '@/lib/types/maintenance';

export default function ReturnToServicePage() {
  const params = useParams();
  const router = useRouter();
  const workOrderId = params.workOrderId as string;

  const [workOrder, setWorkOrder] = useState<WorkOrder | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [reason, setReason] = useState('');
  const [evidenceMediaIds, setEvidenceMediaIds] = useState('');
  const [submitting, setSubmitting] = useState(false);

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
        if (data.returnToServiceStatus === 'APPROVED') {
          setError('This work order has already been returned to service.');
        }
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

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!workOrder) return;
    setSubmitting(true);
    setError(null);
    try {
      const mediaIds = evidenceMediaIds
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);

      await maintenanceService.returnToService(
        workOrder.id,
        {
          expectedVersion: workOrder.version,
          reason: reason.trim(),
          evidenceMediaIds: mediaIds,
        },
        newMaintenanceIdempotencyKey(),
      );
      router.push(`/fleet/work-orders/${workOrder.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to return asset to service.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading work order…
      </div>
    );
  }

  if (!workOrder) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error ?? 'Work order not found.'}
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

  const statusTone = workOrderStatusTone(workOrder.status);
  const priorityTone = workOrderPriorityTone(workOrder.priority);
  const canSubmit =
    workOrder.status === 'COMPLETED' &&
    workOrder.returnToServiceStatus !== 'APPROVED';

  const mediaCount = evidenceMediaIds
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean).length;

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/fleet/work-orders/${workOrder.id}`}
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Return to service</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {workOrder.id}
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-primary/20 bg-primary-bg/20 p-4">
        <div className="flex items-start gap-3">
          <Shield className="h-5 w-5 flex-shrink-0 text-primary mt-0.5" />
          <div>
            <div className="text-sm font-semibold text-ink-950">
              Return-to-service decision
            </div>
            <p className="mt-0.5 text-sm text-ink-600">
              Approving clears the safety block on the asset and marks it
              available again. Evidence media is required.
            </p>
          </div>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface p-4">
        <h3 className="mb-3 text-sm font-semibold text-ink-950">
          Work order summary
        </h3>
        <dl className="space-y-2">
          <Row
            label="Status"
            value={
              <span
                className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
                {workOrderStatusLabel(workOrder.status)}
              </span>
            }
          />
          <Row
            label="Priority"
            value={
              <span
                className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${priorityTone.bg} ${priorityTone.text}`}
              >
                {workOrderPriorityLabel(workOrder.priority)}
              </span>
            }
          />
          <Row label="Source" value={workOrderSourceLabel(workOrder.sourceType)} />
          <Row
            label="Asset"
            value={<span className="font-mono text-xs">{workOrder.assetId}</span>}
          />
          <Row label="Diagnosis" value={workOrder.diagnosis || '—'} />
          <Row label="Required work" value={workOrder.requiredWork || '—'} />
          <Row label="Completed at" value={formatDateTime(workOrder.completedAt)} />
        </dl>
      </div>

      {!canSubmit && (
        <div className="rounded-lg border border-amber/30 bg-amber-bg/20 px-4 py-3 text-sm text-amber">
          This work order cannot be returned to service — it must be completed
          and its return-to-service status must not already be approved.
        </div>
      )}

      {canSubmit && (
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field label="Reason" required>
            <textarea
              required
              rows={3}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              placeholder="Why is the asset being returned to service?"
              className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          <Field label="Evidence media IDs (comma separated)" required>
            <input
              required
              value={evidenceMediaIds}
              onChange={(e) => setEvidenceMediaIds(e.target.value)}
              placeholder="med_..., med_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>

          {mediaCount > 0 && (
            <div className="flex items-center gap-2 rounded-lg border border-line bg-surface-2 px-3 py-2 text-xs text-ink-600">
              <FileText className="h-3.5 w-3.5" />
              {mediaCount} evidence file{mediaCount === 1 ? '' : 's'} attached
            </div>
          )}

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={submitting || mediaCount === 0}
              className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              <CheckCircle2 className="h-4 w-4" />
              {submitting ? 'Submitting…' : 'Approve return to service'}
            </button>
            <Link
              href={`/fleet/work-orders/${workOrder.id}`}
              className="flex h-10 items-center gap-2 rounded-lg border border-line bg-surface px-4 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              <X className="h-4 w-4" />
              Cancel
            </Link>
          </div>
        </form>
      )}
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

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
        {required && <span className="ml-1 text-red">*</span>}
      </label>
      {children}
    </div>
  );
}