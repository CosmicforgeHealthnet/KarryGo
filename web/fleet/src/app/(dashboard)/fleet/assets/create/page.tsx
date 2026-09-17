'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import {
  assetService,
  newAssetIdempotencyKey,
} from '@/lib/services/assetService';
import type { Asset } from '@/lib/types/asset';

const ASSET_TYPES = [
  'MOTORCYCLE',
  'VAN',
  'TRUCK',
  'CAR',
  'BIKE',
  'OTHER',
] as const;

const OWNERSHIP_TYPES = ['OWNED', 'LEASED', 'CONTRACTED'] as const;

export default function CreateAssetPage() {
  const router = useRouter();

  const [form, setForm] = useState({
    branchId: '',
    assetCode: '',
    assetType: 'MOTORCYCLE' as string,
    platformVehicleId: '',
    ownershipType: 'OWNED' as string,
    registrationReference: '',
    manufacturer: '',
    model: '',
    year: '',
    capacity: '',
    capacityUnit: 'KG',
    odometer: '',
    operatingHours: '',
  });

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created: Asset = await assetService.create(
        {
          branchId: form.branchId.trim(),
          assetCode: form.assetCode.trim(),
          assetType: form.assetType,
          platformVehicleId: form.platformVehicleId.trim() || undefined,
          ownershipType: form.ownershipType || undefined,
          registrationReference: form.registrationReference.trim() || undefined,
          manufacturer: form.manufacturer.trim() || undefined,
          model: form.model.trim() || undefined,
          year: form.year ? Number(form.year) : undefined,
          capacity: form.capacity ? Number(form.capacity) : undefined,
          capacityUnit: form.capacityUnit.trim() || undefined,
          odometer: form.odometer ? Number(form.odometer) : undefined,
          operatingHours: form.operatingHours
            ? Number(form.operatingHours)
            : undefined,
        },
        newAssetIdempotencyKey(),
      );
      router.push(`/fleet/assets/${created.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create asset.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/assets"
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Add asset</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Register a new fleet asset. Status is set automatically by the
            platform.
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
          <Field label="Branch ID" required>
            <input
              required
              value={form.branchId}
              onChange={(e) => setForm({ ...form, branchId: e.target.value })}
              placeholder="brn_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Asset code" required>
            <input
              required
              value={form.assetCode}
              onChange={(e) => setForm({ ...form, assetCode: e.target.value })}
              placeholder="BR-204"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Asset type" required>
            <select
              required
              value={form.assetType}
              onChange={(e) => setForm({ ...form, assetType: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {ASSET_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Ownership">
            <select
              value={form.ownershipType}
              onChange={(e) =>
                setForm({ ...form, ownershipType: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {OWNERSHIP_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Manufacturer">
            <input
              value={form.manufacturer}
              onChange={(e) => setForm({ ...form, manufacturer: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Model">
            <input
              value={form.model}
              onChange={(e) => setForm({ ...form, model: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Year">
            <input
              type="number"
              value={form.year}
              onChange={(e) => setForm({ ...form, year: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Registration reference">
            <input
              value={form.registrationReference}
              onChange={(e) =>
                setForm({ ...form, registrationReference: e.target.value })
              }
              placeholder="LAG-123-XY"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Field label="Capacity">
            <input
              type="number"
              step="0.1"
              value={form.capacity}
              onChange={(e) => setForm({ ...form, capacity: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Capacity unit">
            <input
              value={form.capacityUnit}
              onChange={(e) =>
                setForm({ ...form, capacityUnit: e.target.value })
              }
              placeholder="KG"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Platform vehicle ID">
            <input
              value={form.platformVehicleId}
              onChange={(e) =>
                setForm({ ...form, platformVehicleId: e.target.value })
              }
              placeholder="veh_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Odometer (km)">
            <input
              type="number"
              value={form.odometer}
              onChange={(e) => setForm({ ...form, odometer: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Operating hours">
            <input
              type="number"
              step="0.1"
              value={form.operatingHours}
              onChange={(e) =>
                setForm({ ...form, operatingHours: e.target.value })
              }
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
          {submitting ? 'Creating…' : 'Create asset'}
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