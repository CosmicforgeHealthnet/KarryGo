'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, ExternalLink, UserPlus } from 'lucide-react';
import {
  assetService,
  newAssetIdempotencyKey,
} from '@/lib/services/assetService';
import type { Assignment } from '@/lib/types/asset';

const ASSIGNMENT_TYPES = ['PRIMARY', 'SECONDARY', 'TEMPORARY'] as const;

export default function AssetAssignmentsPage() {
  const params = useParams();
  const assetId = params.assetId as string;

  const [form, setForm] = useState({
    providerMembershipId: '',
    shiftId: '',
    assignmentType: 'PRIMARY' as string,
    startAt: '',
    endAt: '',
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<Assignment | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const result = await assetService.assign(
        assetId,
        {
          providerMembershipId: form.providerMembershipId.trim(),
          shiftId: form.shiftId.trim() || undefined,
          assignmentType: form.assignmentType || undefined,
          startAt: form.startAt || undefined,
          endAt: form.endAt || undefined,
        },
        newAssetIdempotencyKey(),
      );
      setCreated(result);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create assignment.',
      );
    } finally {
      setBusy(false);
    }
  }

  if (created) {
    return (
      <div className="mx-auto max-w-2xl space-y-5">
        <Link
          href={`/fleet/assets/${assetId}`}
          className="inline-flex items-center gap-1.5 text-xs font-medium text-ink-500 transition-colors hover:text-ink-950"
        >
          <ArrowLeft className="h-3.5 w-3.5" />
          Back to asset
        </Link>

        <div className="rounded-lg border border-green/20 bg-green-bg/30 p-5">
          <h2 className="text-sm font-semibold text-ink-950">
            Assignment created
          </h2>
          <p className="mt-1 text-sm text-ink-700">
            The asset is now assigned to provider membership{' '}
            <span className="font-mono text-xs">
              {created.providerMembershipId}
            </span>
            .
          </p>
          <dl className="mt-4 space-y-2">
            <Row label="Assignment ID" value={created.id} mono />
            <Row label="Status" value={created.status} />
            <Row label="Type" value={created.assignmentType || '—'} />
            <Row label="Start" value={created.startAt} />
            <Row label="End" value={created.endAt ?? '—'} />
          </dl>
          <div className="mt-4 flex flex-wrap gap-2">
            <Link
              href="/operations/marketplace/assignments"
              className="flex items-center gap-1.5 rounded-md border border-line bg-surface px-3.5 py-1.5 text-xs font-medium text-ink-700 transition-colors hover:bg-surface-2"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              View all assignments
            </Link>
            <Link
              href={`/fleet/assets/${assetId}`}
              className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover"
            >
              Back to asset
            </Link>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/fleet/assets/${assetId}`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Assign asset</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">{assetId}</p>
        </div>
      </div>

      <div className="rounded-lg border border-line bg-surface-2 p-3 text-xs text-ink-600">
        Assigning reserves the asset for a provider membership. The asset must
        be <span className="font-semibold">AVAILABLE</span> and the provider
        membership must be <span className="font-semibold">ACTIVE</span>.
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={submit} className="space-y-4">
        <Field label="Provider membership ID" required>
          <input
            required
            value={form.providerMembershipId}
            onChange={(e) =>
              setForm({ ...form, providerMembershipId: e.target.value })
            }
            placeholder="pmb_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Shift ID (optional)">
          <input
            value={form.shiftId}
            onChange={(e) => setForm({ ...form, shiftId: e.target.value })}
            placeholder="shf_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Assignment type">
          <select
            value={form.assignmentType}
            onChange={(e) =>
              setForm({ ...form, assignmentType: e.target.value })
            }
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            {ASSIGNMENT_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Start at (optional)">
            <input
              type="datetime-local"
              value={form.startAt}
              onChange={(e) => setForm({ ...form, startAt: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="End at (optional)">
            <input
              type="datetime-local"
              value={form.endAt}
              onChange={(e) => setForm({ ...form, endAt: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <button
          type="submit"
          disabled={busy}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <UserPlus className="h-4 w-4" />
          {busy ? 'Assigning…' : 'Assign asset'}
        </button>
      </form>
    </div>
  );
}

function Row({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line py-1.5 last:border-0">
      <dt className="text-xs font-medium uppercase tracking-wide text-ink-500">
        {label}
      </dt>
      <dd className={`text-sm text-ink-950 ${mono ? 'font-mono text-xs' : ''}`}>
        {value}
      </dd>
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