'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, Save } from 'lucide-react';
import { partService } from '@/lib/services/partService';
import type { Part } from '@/lib/types/part';

const STATUSES = ['ACTIVE', 'INACTIVE', 'ARCHIVED'] as const;

export default function EditPartPage() {
  const params = useParams();
  const router = useRouter();
  const partId = params.partId as string;

  const [part, setPart] = useState<Part | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [form, setForm] = useState({
    name: '',
    reorderLevel: '',
    status: '',
  });

  useEffect(() => {
    if (!partId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    partService
      .get(partId)
      .then((data) => {
        if (cancelled) return;
        setPart(data);
        setForm({
          name: data.name ?? '',
          reorderLevel:
            data.reorderLevel != null ? String(data.reorderLevel) : '',
          status: data.status ?? '',
        });
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
      await partService.update(part.id, {
        version: part.version,
        name: form.name.trim() || undefined,
        reorderLevel: form.reorderLevel ? Number(form.reorderLevel) : undefined,
        status: form.status || undefined,
      });
      router.push(`/fleet/parts/${part.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update part.',
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
          <h1>Edit part</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">{part.sku}</p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
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

        <Field label="Reorder level">
          <input
            type="number"
            step="0.01"
            value={form.reorderLevel}
            onChange={(e) => setForm({ ...form, reorderLevel: e.target.value })}
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
            href={`/fleet/parts/${part.id}`}
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