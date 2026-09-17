'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, Calendar, Save } from 'lucide-react';
import { maintenanceService } from '@/lib/services/maintenanceService';
import {
  formatDate,
  formatDateTime,
  formatNumber,
  scheduleStatusLabel,
  scheduleStatusTone,
  triggerTypeLabel,
} from '@/lib/utils/maintenanceLabels';
import type { MaintenanceSchedule } from '@/lib/types/maintenance';

export default function ScheduleDetailPage() {
  const params = useParams();
  const scheduleId = params.scheduleId as string;

  const [schedule, setSchedule] = useState<MaintenanceSchedule | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [editing, setEditing] = useState(false);
  const [form, setForm] = useState({
    nextDueDate: '',
    nextDueOdometer: '',
    nextDueHours: '',
    status: '',
  });
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!scheduleId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    maintenanceService
      .getSchedule(scheduleId)
      .then((data) => {
        if (cancelled) return;
        setSchedule(data);
        setForm({
          nextDueDate: data.nextDueDate ? data.nextDueDate.slice(0, 10) : '',
          nextDueOdometer:
            data.nextDueOdometer != null ? String(data.nextDueOdometer) : '',
          nextDueHours: data.nextDueHours != null ? String(data.nextDueHours) : '',
          status: data.status ?? '',
        });
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load schedule.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [scheduleId]);

  async function handleSave(e: FormEvent) {
    e.preventDefault();
    if (!schedule) return;
    setSaving(true);
    setError(null);
    try {
      const updated = await maintenanceService.updateSchedule(schedule.id, {
        version: schedule.version,
        nextDueDate: form.nextDueDate || null,
        nextDueOdometer: form.nextDueOdometer ? Number(form.nextDueOdometer) : null,
        nextDueHours: form.nextDueHours ? Number(form.nextDueHours) : null,
        status: form.status || undefined,
      });
      setSchedule(updated);
      setEditing(false);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to update schedule.',
      );
    } finally {
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading schedule…
      </div>
    );
  }

  if (error && !schedule) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/maintenance/schedules"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to schedules
        </Link>
      </div>
    );
  }

  if (!schedule) return null;

  const tone = scheduleStatusTone(schedule.status);

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Schedule ID', value: <span className="font-mono text-xs">{schedule.id}</span> },
    { label: 'Asset ID', value: <span className="font-mono text-xs">{schedule.assetId}</span> },
    { label: 'Trigger', value: triggerTypeLabel(schedule.triggerType) },
    { label: 'Interval', value: formatNumber(schedule.intervalValue) },
    { label: 'Next due date', value: formatDate(schedule.nextDueDate) },
    { label: 'Next due odometer', value: formatNumber(schedule.nextDueOdometer) },
    { label: 'Next due hours', value: formatNumber(schedule.nextDueHours) },
    { label: 'Critical', value: schedule.critical ? 'Yes' : 'No' },
    { label: 'Created', value: formatDateTime(schedule.createdAt) },
    { label: 'Updated', value: formatDateTime(schedule.updatedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{schedule.version}</span> },
  ];

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/maintenance/schedules"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2.5">
              <h1 className="truncate">
                {triggerTypeLabel(schedule.triggerType)}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${tone.bg} ${tone.text}`}
              >
                <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                {scheduleStatusLabel(schedule.status)}
              </span>
              {schedule.critical && (
                <span className="rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                  Critical
                </span>
              )}
            </div>
            <p className="mt-0.5 font-mono text-xs text-ink-500">
              {schedule.assetId}
            </p>
          </div>
        </div>
        {!editing && (
          <button
            onClick={() => setEditing(true)}
            className="flex h-10 items-center gap-1.5 rounded-lg border border-line bg-surface px-3.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <Calendar className="h-3.5 w-3.5" />
            Reschedule
          </button>
        )}
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {editing && (
        <form
          onSubmit={handleSave}
          className="space-y-4 rounded-lg border border-line bg-surface p-4"
        >
          <h3 className="text-sm font-semibold text-ink-950">Update schedule</h3>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
            <Field label="Next due date">
              <input
                type="date"
                value={form.nextDueDate}
                onChange={(e) => setForm({ ...form, nextDueDate: e.target.value })}
                className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
              />
            </Field>
            <Field label="Next due odometer">
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

          <Field label="Status">
            <select
              value={form.status}
              onChange={(e) => setForm({ ...form, status: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              <option value="">Unchanged</option>
              <option value="ACTIVE">Active</option>
              <option value="PAUSED">Paused</option>
              <option value="COMPLETE">Complete</option>
              <option value="ARCHIVED">Archived</option>
            </select>
          </Field>

          <div className="flex gap-2">
            <button
              type="submit"
              disabled={saving}
              className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
            >
              <Save className="h-4 w-4" />
              {saving ? 'Saving…' : 'Save'}
            </button>
            <button
              type="button"
              onClick={() => setEditing(false)}
              disabled={saving}
              className="rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
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