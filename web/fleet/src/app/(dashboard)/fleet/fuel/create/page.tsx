'use client';

import { useEffect, useMemo, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import {
  fuelService,
  newFuelIdempotencyKey,
} from '@/lib/services/fuelService';
import { assetService } from '@/lib/services/assetService';
import {
  fuelOrEnergyTypeLabel,
  fuelUnitLabel,
  validUnitsForType,
} from '@/lib/utils/fuelLabels';
import type { FuelRecord } from '@/lib/types/fuel';
import type { Asset } from '@/lib/types/asset';

const FUEL_TYPES = ['PETROL', 'DIESEL', 'ELECTRIC', 'GAS', 'OTHER'] as const;

export default function CreateFuelRecordPage() {
  const router = useRouter();

  const [assets, setAssets] = useState<Asset[]>([]);
  const [assetsLoading, setAssetsLoading] = useState(true);

  const [form, setForm] = useState({
    assetId: '',
    fuelOrEnergyType: 'DIESEL' as string,
    quantity: '',
    unit: 'L' as string,
    odometer: '',
    operatingHours: '',
    supplierReference: '',
    receiptMediaId: '',
    providerMembershipId: '',
    recordedAt: '',
  });

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAssetsLoading(true);
    assetService
      .list()
      .then((data) => {
        if (cancelled) return;
        setAssets(data);
        if (data.length > 0 && !form.assetId) {
          setForm((f) => ({ ...f, assetId: data[0].id }));
        }
      })
      .catch(() => {
        // Non-fatal — the user can still enter an asset ID manually.
      })
      .finally(() => {
        if (!cancelled) setAssetsLoading(false);
      });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const validUnits = useMemo(
    () => validUnitsForType(form.fuelOrEnergyType),
    [form.fuelOrEnergyType],
  );

  useEffect(() => {
    if (validUnits.length > 0 && !validUnits.includes(form.unit)) {
      setForm((f) => ({ ...f, unit: validUnits[0] }));
    }
  }, [validUnits, form.unit]);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created: FuelRecord = await fuelService.create(
        {
          assetId: form.assetId.trim(),
          providerMembershipId: form.providerMembershipId.trim() || undefined,
          fuelOrEnergyType: form.fuelOrEnergyType,
          quantity: Number(form.quantity),
          unit: form.unit,
          odometer: form.odometer ? Number(form.odometer) : undefined,
          operatingHours: form.operatingHours
            ? Number(form.operatingHours)
            : undefined,
          supplierReference: form.supplierReference.trim() || undefined,
          receiptMediaId: form.receiptMediaId.trim() || undefined,
          recordedAt: form.recordedAt || undefined,
        },
        newFuelIdempotencyKey(),
      );
      router.push(`/fleet/fuel/${created.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create fuel record.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/fuel"
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Add fuel record</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Records start as PENDING and require approval.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-4">
        <Field label="Asset" required>
          {assetsLoading ? (
            <div className="text-xs text-ink-500">Loading assets…</div>
          ) : assets.length > 0 ? (
            <select
              required
              value={form.assetId}
              onChange={(e) => setForm({ ...form, assetId: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {assets.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.assetCode} — {a.manufacturer} {a.model}
                </option>
              ))}
            </select>
          ) : (
            <input
              required
              value={form.assetId}
              onChange={(e) => setForm({ ...form, assetId: e.target.value })}
              placeholder="ast_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          )}
        </Field>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <Field label="Fuel type" required>
            <select
              required
              value={form.fuelOrEnergyType}
              onChange={(e) =>
                setForm({ ...form, fuelOrEnergyType: e.target.value })
              }
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {FUEL_TYPES.map((t) => (
                <option key={t} value={t}>
                  {fuelOrEnergyTypeLabel(t)}
                </option>
              ))}
            </select>
          </Field>
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
          <Field label="Unit" required>
            <select
              required
              value={form.unit}
              onChange={(e) => setForm({ ...form, unit: e.target.value })}
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            >
              {validUnits.map((u) => (
                <option key={u} value={u}>
                  {fuelUnitLabel(u)}
                </option>
              ))}
            </select>
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Odometer (km)">
            <input
              type="number"
              step="0.1"
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

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Supplier reference">
            <input
              value={form.supplierReference}
              onChange={(e) =>
                setForm({ ...form, supplierReference: e.target.value })
              }
              placeholder="TotalEnergies - Lekki"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Receipt media ID">
            <input
              value={form.receiptMediaId}
              onChange={(e) =>
                setForm({ ...form, receiptMediaId: e.target.value })
              }
              placeholder="med_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field label="Provider membership ID">
            <input
              value={form.providerMembershipId}
              onChange={(e) =>
                setForm({ ...form, providerMembershipId: e.target.value })
              }
              placeholder="pmb_..."
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
          <Field label="Recorded at">
            <input
              type="datetime-local"
              value={form.recordedAt}
              onChange={(e) => setForm({ ...form, recordedAt: e.target.value })}
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
          {submitting ? 'Creating…' : 'Create record'}
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