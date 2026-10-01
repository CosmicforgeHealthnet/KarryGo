'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import { partService, newPartIdempotencyKey } from '@/lib/services/partService';
import { formatNumber } from '@/lib/utils/partLabels';
import type { Part } from '@/lib/types/part';

const MOVEMENT_TYPES = [
  'RECEIVE',
  'ISSUE',
  'ADJUST',
  'TRANSFER_IN',
  'TRANSFER_OUT',
  'RETURN',
] as const;

export default function AddMovementPage() {
  const params = useParams();
  const router = useRouter();
  const partId = params.partId as string;

  const [part, setPart] = useState<Part | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    movementType: 'RECEIVE' as string,
    quantity: '',
    reason: '',
    locationReference: '',
    workOrderId: '',
  });

  useEffect(() => {
    if (!partId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    partService
      .get(partId)
      .then((data) => {
        if (!cancelled) setPart(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load part.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [partId]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!part) return;
    setSubmitting(true);
    setError(null);
    try {
      await partService.createMovement(
        {
          partId: part.id,
          movementType: form.movementType,
          quantity: Number(form.quantity),
          reason: form.reason.trim(),
          locationReference: form.locationReference.trim() || undefined,
          workOrderId: form.workOrderId.trim() || undefined,
        },
        newPartIdempotencyKey(),
      );
      router.push(`/fleet/parts/${part.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to record movement.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading part…
      </div>
    );
  }

  if (error && !part) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/parts"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to parts
        </Link>
      </div>
    );
  }

  if (!part) return null;

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/fleet/parts/${part.id}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Record stock movement</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            {part.name} · <span className="font-mono text-xs">{part.sku}</span>
          </p>
        </div>
      </div>

      <div className="rounded-lg border border-line bg-surface-2 p-3 text-xs text-ink-600">
        Current stock:{' '}
        <span className="font-semibold text-ink-950">
          {formatNumber(part.currentQuantityProjection)} {part.stockUnit}
        </span>
        {' · '}
        Reorder level: {formatNumber(part.reorderLevel)} {part.stockUnit}
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Movement type" required>
          <select
            required
            value={form.movementType}
            onChange={(e) => setForm({ ...form, movementType: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            {MOVEMENT_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Quantity" required>
            <input
              required
              type="number"
              step="0.01"
              min="0.01"
              value={form.quantity}
              onChange={(e) => setForm({ ...form, quantity: e.target.value })}
              placeholder="1"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Location reference">
            <input
              value={form.locationReference}
              onChange={(e) =>
                setForm({ ...form, locationReference: e.target.value })
              }
              placeholder="warehouse-1"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <Field label="Reason" required>
          <input
            required
            value={form.reason}
            onChange={(e) => setForm({ ...form, reason: e.target.value })}
            placeholder="Purchase order PO-2026-0800"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Work order ID (optional)">
          <input
            value={form.workOrderId}
            onChange={(e) => setForm({ ...form, workOrderId: e.target.value })}
            placeholder="wor_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <button
          type="submit"
          disabled={submitting}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <Plus className="h-4 w-4" />
          {submitting ? 'Recording…' : 'Record movement'}
        </button>
      </form>
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