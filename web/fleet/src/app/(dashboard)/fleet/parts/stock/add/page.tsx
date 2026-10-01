'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
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

export default function AddStockMovementPage() {
  const router = useRouter();

  const [parts, setParts] = useState<Part[]>([]);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    partId: '',
    movementType: 'RECEIVE' as string,
    quantity: '',
    reason: '',
    locationReference: '',
    workOrderId: '',
  });

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);
    partService
      .list()
      .then((data) => {
        if (cancelled) return;
        setParts(data);
        if (data.length > 0 && !form.partId) {
          setForm((f) => ({ ...f, partId: data[0].id }));
        }
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load parts.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const selected = parts.find((p) => p.id === form.partId);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await partService.createMovement(
        {
          partId: form.partId,
          movementType: form.movementType,
          quantity: Number(form.quantity),
          reason: form.reason.trim(),
          locationReference: form.locationReference.trim() || undefined,
          workOrderId: form.workOrderId.trim() || undefined,
        },
        newPartIdempotencyKey(),
      );
      router.push('/fleet/parts/stock');
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

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/parts/stock"
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Record stock movement</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Adjust stock levels for any part.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading parts…
        </div>
      ) : parts.length === 0 ? (
        <div className="rounded-lg border border-line bg-surface p-6 text-center">
          <p className="text-sm text-ink-600">
            No parts exist yet. Create a part before recording stock movements.
          </p>
          <Link
            href="/fleet/parts/create"
            className="mt-3 inline-flex items-center gap-1.5 rounded-lg bg-primary px-3.5 py-2 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
          >
            <Plus className="h-3.5 w-3.5" />
            Add part
          </Link>
        </div>
      ) : (
        <form onSubmit={handleSubmit} className="space-y-4">
          <Field label="Part" required>
            <select
              required
              value={form.partId}
              onChange={(e) => setForm({ ...form, partId: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {parts.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} ({p.sku})
                </option>
              ))}
            </select>
          </Field>

          {selected && (
            <div className="rounded-lg border border-line bg-surface-2 p-3 text-xs text-ink-600">
              Current stock:{' '}
              <span className="font-semibold text-ink-950">
                {formatNumber(selected.currentQuantityProjection)}{' '}
                {selected.stockUnit}
              </span>
              {' · '}
              Reorder level: {formatNumber(selected.reorderLevel)}{' '}
              {selected.stockUnit}
            </div>
          )}

          <Field label="Movement type" required>
            <select
              required
              value={form.movementType}
              onChange={(e) =>
                setForm({ ...form, movementType: e.target.value })
              }
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
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="Location reference">
              <input
                value={form.locationReference}
                onChange={(e) =>
                  setForm({ ...form, locationReference: e.target.value })
                }
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
          </div>

          <Field label="Reason" required>
            <input
              required
              value={form.reason}
              onChange={(e) => setForm({ ...form, reason: e.target.value })}
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
      )}
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