'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { AlertCircle, ArrowLeft, Save } from 'lucide-react';
import { workforceService } from '@/lib/services/workforceService';
import type { Shift } from '@/lib/types/workforce';

export default function EditShiftPage() {
  const params = useParams();
  const router = useRouter();
  const shiftId = params.shiftId as string;

  const [shift, setShift] = useState<Shift | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    name: '',
    startAt: '',
    endAt: '',
  });

  useEffect(() => {
    if (!shiftId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    workforceService
      .getShift(shiftId)
      .then((data) => {
        if (cancelled) return;
        setShift(data);
        setForm({
          name: data.name ?? '',
          startAt: toLocalInput(data.startAt),
          endAt: toLocalInput(data.endAt),
        });
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

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!shift) return;
    setSubmitting(true);
    setError(null);
    try {
      await workforceService.updateShift(shift.id, {
        expectedVersion: shift.version,
        name: form.name.trim() || undefined,
        startAt: form.startAt
          ? new Date(form.startAt).toISOString()
          : undefined,
        endAt: form.endAt ? new Date(form.endAt).toISOString() : undefined,
      });
      router.push(`/workforce/shifts/${shift.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update shift.',
      );
    } finally {
      setSubmitting(false);
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

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/workforce/shifts/${shift.id}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Edit shift</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">{shift.id}</p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {shift.status !== 'DRAFT' && (
        <div className="flex items-start gap-2 rounded-lg border border-amber/30 bg-amber-bg/20 px-3 py-2 text-xs text-amber">
          <AlertCircle className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
          <p>
            Only draft shifts are editable. This shift is{' '}
            <span className="font-semibold">{shift.status}</span>.
          </p>
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Name">
          <input
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Start at">
            <input
              type="datetime-local"
              value={form.startAt}
              onChange={(e) => setForm({ ...form, startAt: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="End at">
            <input
              type="datetime-local"
              value={form.endAt}
              onChange={(e) => setForm({ ...form, endAt: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="flex gap-2">
          <button
            type="submit"
            disabled={submitting || shift.status !== 'DRAFT'}
            className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {submitting ? 'Saving…' : 'Save changes'}
          </button>
          <Link
            href={`/workforce/shifts/${shift.id}`}
            className="flex h-10 items-center gap-2 rounded-lg border border-line bg-surface px-4 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Cancel
          </Link>
        </div>
      </form>
    </div>
  );
}

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (!Number.isFinite(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
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