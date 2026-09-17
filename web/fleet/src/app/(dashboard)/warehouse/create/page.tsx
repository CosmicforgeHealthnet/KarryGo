'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { ArrowLeft, Plus } from 'lucide-react';
import {
  warehouseService,
  newWarehouseIdempotencyKey,
} from '@/lib/services/warehouseService';
import type { Warehouse } from '@/lib/types/warehouse';

export default function CreateWarehousePage() {
  const router = useRouter();

  const [form, setForm] = useState({
    branchId: '',
    code: '',
    name: '',
    siteReference: '',
    addressReference: '',
    timezone: 'Africa/Lagos',
  });
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      const created: Warehouse = await warehouseService.createWarehouse(
        {
          branchId: form.branchId.trim(),
          code: form.code.trim().toUpperCase(),
          name: form.name.trim(),
          siteReference: form.siteReference.trim() || undefined,
          addressReference: form.addressReference.trim(),
          timezone: form.timezone.trim(),
        },
        newWarehouseIdempotencyKey(),
      );
      router.push(`/warehouse/${created.id}`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create warehouse.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/warehouse"
          className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Add warehouse</h1>
          <p className="mt-0.5 text-sm text-ink-500">
            Register a new warehouse site.
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
          <Field label="Code" required>
            <input
              required
              value={form.code}
              onChange={(e) => setForm({ ...form, code: e.target.value })}
              placeholder="WH-IKJ"
              className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 uppercase outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
            />
          </Field>
        </div>

        <Field label="Name" required>
          <input
            required
            value={form.name}
            onChange={(e) => setForm({ ...form, name: e.target.value })}
            placeholder="Ikeja Central Warehouse"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Site reference">
          <input
            value={form.siteReference}
            onChange={(e) => setForm({ ...form, siteReference: e.target.value })}
            placeholder="site_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Address reference" required>
          <input
            required
            value={form.addressReference}
            onChange={(e) =>
              setForm({ ...form, addressReference: e.target.value })
            }
            placeholder="addr_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <Field label="Timezone" required>
          <input
            required
            value={form.timezone}
            onChange={(e) => setForm({ ...form, timezone: e.target.value })}
            placeholder="Africa/Lagos"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>

        <button
          type="submit"
          disabled={submitting}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <Plus className="h-4 w-4" />
          {submitting ? 'Creating…' : 'Create warehouse'}
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