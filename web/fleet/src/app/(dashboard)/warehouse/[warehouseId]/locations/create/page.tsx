'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import { locationTypeLabel } from '@/lib/utils/warehouseLabels';
import type { StorageLocation } from '@/lib/types/warehouse';

const LOCATION_TYPES = [
  'ZONE',
  'AISLE',
  'BAY',
  'BIN',
  'DOCK',
  'STAGING',
  'QUARANTINE',
  'COLD',
  'HAZMAT',
  'OTHER',
] as const;

export default function CreateLocationPage() {
  const params = useParams();
  const router = useRouter();
  const warehouseId = params.warehouseId as string;

  const [siblings, setSiblings] = useState<StorageLocation[]>([]);
  const [loading, setLoading] = useState(true);

  const [form, setForm] = useState({
    code: '',
    name: '',
    locationType: 'ZONE' as string,
    parentLocationId: '',
    capacity: '',
    capacityUnit: 'UNIT',
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!warehouseId) return;
    let cancelled = false;
    setLoading(true);

    warehouseService
      .listLocations(warehouseId)
      .then((data) => {
        if (!cancelled) setSiblings(data);
      })
      .catch(() => {
        // Non-fatal — parent list is optional.
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [warehouseId]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created: StorageLocation = await warehouseService.createLocation(
        warehouseId,
        {
          code: form.code.trim().toUpperCase(),
          name: form.name.trim(),
          locationType: form.locationType,
          parentLocationId: form.parentLocationId || undefined,
          capacity: form.capacity ? Number(form.capacity) : undefined,
          capacityUnit: form.capacityUnit.trim() || undefined,
        },
        newWarehouseIdempotencyKey(),
      );
      router.push(`/warehouse/${warehouseId}/storage`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create location.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href={`/warehouse/${warehouseId}/storage`}
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Add storage location</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Create a zone, aisle, bay, bin or controlled location.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Code" required>
            <input
              required
              value={form.code}
              onChange={(e) => setForm({ ...form, code: e.target.value })}
              placeholder="A-01"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 uppercase outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Type" required>
            <select
              required
              value={form.locationType}
              onChange={(e) =>
                setForm({ ...form, locationType: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {LOCATION_TYPES.map((t) => (
                <option key={t} value={t}>
                  {locationTypeLabel(t)}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <Field label="Name" required>
          <input
            required
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder="Aisle A1"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Parent location">
          {loading ? (
            <div className="text-xs text-ink-500">Loading locations…</div>
          ) : siblings.length > 0 ? (
            <select
              value={form.parentLocationId}
              onChange={(e) =>
                setForm({ ...form, parentLocationId: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              <option value="">None (top-level)</option>
              {siblings.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.code} — {s.name} ({locationTypeLabel(s.locationType)})
                </option>
              ))}
            </select>
          ) : (
            <input
              value={form.parentLocationId}
              onChange={(e) =>
                setForm({ ...form, parentLocationId: e.target.value })
              }
              placeholder="loc_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          )}
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Capacity">
            <input
              type="number"
              step="0.01"
              value={form.capacity}
              onChange={(e) => setForm({ ...form, capacity: e.target.value })}
              placeholder="500"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Capacity unit">
            <input
              value={form.capacityUnit}
              onChange={(e) =>
                setForm({ ...form, capacityUnit: e.target.value })
              }
              placeholder="UNIT, PALLET, BIN"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <button
          type="submit"
          disabled={submitting}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <Plus className="h-4 w-4" />
          {submitting ? 'Creating…' : 'Create location'}
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