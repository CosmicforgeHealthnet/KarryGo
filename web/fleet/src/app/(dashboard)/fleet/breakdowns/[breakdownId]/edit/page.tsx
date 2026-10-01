'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, Save } from 'lucide-react';
import { breakdownService } from '@/lib/services/breakdownService';
import type { Breakdown } from '@/lib/types/breakdown';

const RESCUE_STATUSES = [
  'NOT_REQUIRED',
  'REQUESTED',
  'DISPATCHED',
  'ON_SITE',
  'COMPLETED',
  'FAILED',
] as const;

const STATUSES = ['OPEN', 'IN_PROGRESS', 'RESOLVED', 'CLOSED'] as const;

export default function EditBreakdownPage() {
  const params = useParams();
  const router = useRouter();
  const breakdownId = params.breakdownId as string;

  const [breakdown, setBreakdown] = useState<Breakdown | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    rescueStatus: '',
    towingReference: '',
    replacementAssetId: '',
    status: '',
  });

  useEffect(() => {
    if (!breakdownId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    breakdownService
      .get(breakdownId)
      .then((data) => {
        if (cancelled) return;
        setBreakdown(data);
        setForm({
          rescueStatus: data.rescueStatus ?? '',
          towingReference: data.towingReference ?? '',
          replacementAssetId: data.replacementAssetId ?? '',
          status: data.status ?? '',
        });
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load breakdown.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [breakdownId]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!breakdown) return;
    setSubmitting(true);
    setError(null);
    try {
      await breakdownService.update(breakdown.id, {
        expectedVersion: breakdown.version,
        rescueStatus: form.rescueStatus || undefined,
        towingReference: form.towingReference.trim() || undefined,
        replacementAssetId: form.replacementAssetId.trim() || undefined,
        status: form.status || undefined,
      });
      router.push(`/fleet/breakdowns/${breakdown.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update breakdown.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading breakdown…
      </div>
    );
  }

  if (error && !breakdown) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/breakdowns"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to breakdowns
        </Link>
      </div>
    );
  }

  if (!breakdown) return null;

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/fleet/breakdowns/${breakdown.id}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Update breakdown</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {breakdown.id}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Rescue status">
          <select
            value={form.rescueStatus}
            onChange={(e) => setForm({ ...form, rescueStatus: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            <option value="">Unchanged</option>
            {RESCUE_STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Towing reference">
          <input
            value={form.towingReference}
            onChange={(e) => setForm({ ...form, towingReference: e.target.value })}
            placeholder="tow_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Replacement asset ID">
          <input
            value={form.replacementAssetId}
            onChange={(e) =>
              setForm({ ...form, replacementAssetId: e.target.value })
            }
            placeholder="ast_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Status">
          <select
            value={form.status}
            onChange={(e) => setForm({ ...form, status: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            <option value="">Unchanged</option>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </Field>

        <div className="flex gap-2">
          <button
            type="submit"
            disabled={submitting}
            className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {submitting ? 'Saving…' : 'Save changes'}
          </button>
          <Link
            href={`/fleet/breakdowns/${breakdown.id}`}
            className="flex h-10 items-center gap-2 rounded-lg border border-line bg-surface px-4 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            Cancel
          </Link>
        </div>
      </form>
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