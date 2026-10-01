'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import {
  maintenanceService,
  newMaintenanceIdempotencyKey,
} from '@/lib/services/maintenanceService';
import type { MaintenanceSchedule } from '@/lib/types/maintenance';

const TRIGGER_TYPES = [
  'DATE',
  'MILEAGE',
  'OPERATING_HOURS',
  'COMBINED',
] as const;

export default function CreateSchedulePage() {
  const router = useRouter();

  const [form, setForm] = useState({
    assetId: '',
    triggerType: 'DATE' as string,
    intervalValue: '',
    nextDueDate: '',
    nextDueOdometer: '',
    nextDueHours: '',
    critical: false,
  });

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created: MaintenanceSchedule = await maintenanceService.createSchedule(
        {
          assetId: form.assetId.trim(),
          triggerType: form.triggerType,
          intervalValue: form.intervalValue ? Number(form.intervalValue) : undefined,
          nextDueDate: form.nextDueDate || undefined,
          nextDueOdometer: form.nextDueOdometer
            ? Number(form.nextDueOdometer)
            : undefined,
          nextDueHours: form.nextDueHours ? Number(form.nextDueHours) : undefined,
          critical: form.critical,
        },
        newMaintenanceIdempotencyKey(),
      );
      router.push(`/fleet/maintenance/schedules/${created.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create schedule.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/maintenance/schedules"
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Create schedule</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            The platform triggers work orders automatically when the schedule
            is due.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Asset ID" required>
          <input
            required
            value={form.assetId}
            onChange={(e) => setForm({ ...form, assetId: e.target.value })}
            placeholder="ast_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Trigger type" required>
          <select
            required
            value={form.triggerType}
            onChange={(e) => setForm({ ...form, triggerType: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          >
            {TRIGGER_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Field label="Next due date">
            <input
              type="date"
              value={form.nextDueDate}
              onChange={(e) => setForm({ ...form, nextDueDate: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Next due odometer (km)">
            <input
              type="number"
              step="0.1"
              value={form.nextDueOdometer}
              onChange={(e) =>
                setForm({ ...form, nextDueOdometer: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Next due hours">
            <input
              type="number"
              step="0.1"
              value={form.nextDueHours}
              onChange={(e) => setForm({ ...form, nextDueHours: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <Field label="Interval value (optional)">
          <input
            type="number"
            step="0.1"
            value={form.intervalValue}
            onChange={(e) => setForm({ ...form, intervalValue: e.target.value })}
            placeholder="5000"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <label className="flex items-center gap-2.5 rounded-lg border border-line bg-surface px-3 py-2.5">
          <input
            type="checkbox"
            checked={form.critical}
            onChange={(e) => setForm({ ...form, critical: e.target.checked })}
            className="h-4 w-4 rounded border-line text-primary focus:ring-primary/20"
          />
          <span className="text-sm text-ink-950">
            Critical — blocks the asset when overdue
          </span>
        </label>

        <button
          type="submit"
          disabled={submitting}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <Plus className="h-4 w-4" />
          {submitting ? 'Creating…' : 'Create schedule'}
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