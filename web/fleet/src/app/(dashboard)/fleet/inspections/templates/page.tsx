'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import {
  ArrowLeft,
  FileText,
  Plus,
  X,
} from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import {
  inspectionService,
  newInspectionIdempotencyKey,
} from '@/lib/services/inspectionService';
import {
  formatDateTime,
  templateStatusLabel,
  templateStatusTone,
} from '@/lib/utils/inspectionLabels';
import type { InspectionTemplate } from '@/lib/types/inspection';

export default function InspectionTemplatesPage() {
  const [rows, setRows] = useState<InspectionTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);

  const load = () => {
    setLoading(true);
    setError(null);
    inspectionService
      .listTemplates()
      .then(setRows)
      .catch((err) => {
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load templates.',
        );
      })
      .finally(() => setLoading(false));
  };

  useEffect(load, []);

  return (
    <div className="space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-center gap-3">
          <Link
            href="/fleet/inspections"
            className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1>Inspection Templates</h1>
            <p className="mt-0.5 text-sm text-ink-500">
              Reusable checklists that define what inspectors must verify.
            </p>
          </div>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          New template
        </button>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {showForm && (
        <TemplateForm
          onCancel={() => setShowForm(false)}
          onCreated={() => {
            setShowForm(false);
            load();
          }}
        />
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading templates…
        </div>
      ) : rows.length === 0 ? (
        <EmptyState
          image="/images/reports-empty.png"
          title="No templates yet"
          description="Create a template to standardize inspections across your fleet."
        />
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {rows.map((tpl) => {
            const tone = templateStatusTone(tpl.status);
            return (
              <div
                key={tpl.id}
                className="rounded-lg border border-line bg-surface p-4"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-3 min-w-0">
                    <div className="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-md bg-primary-bg text-primary">
                      <FileText className="h-[18px] w-[18px]" />
                    </div>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-ink-950">
                        {tpl.name}
                      </div>
                      <div className="font-mono text-[10px] text-ink-400 truncate">
                        {tpl.id}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${tone.bg} ${tone.text}`}
                  >
                    <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                    {templateStatusLabel(tpl.status)}
                  </span>
                </div>

                <div className="mt-3 space-y-1 text-xs text-ink-500">
                  <div>
                    <span className="text-ink-400">Items:</span>{' '}
                    {tpl.checklistItems.length}
                  </div>
                  <div className="truncate">
                    <span className="text-ink-400">Asset types:</span>{' '}
                    {tpl.assetTypes.length > 0 ? tpl.assetTypes.join(', ') : '—'}
                  </div>
                  <div className="truncate">
                    <span className="text-ink-400">Service types:</span>{' '}
                    {tpl.serviceTypes.length > 0 ? tpl.serviceTypes.join(', ') : '—'}
                  </div>
                </div>

                <div className="mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <span className="text-[10px] text-ink-400">
                    {formatDateTime(tpl.updatedAt)}
                  </span>
                  <span className="font-mono text-[10px] text-ink-400">
                    v{tpl.version}
                  </span>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

function TemplateForm({
  onCancel,
  onCreated,
}: {
  onCancel: () => void;
  onCreated: () => void;
}) {
  const [name, setName] = useState('');
  const [assetTypes, setAssetTypes] = useState('');
  const [serviceTypes, setServiceTypes] = useState('');
  const [items, setItems] = useState('vehicle_safety');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const checklistItems = items
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
        .map((code) => ({
          code,
          required: true,
          evidenceRequired: true,
          critical: true,
        }));

      await inspectionService.createTemplate(
        {
          name: name.trim(),
          assetTypes: assetTypes
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
          serviceTypes: serviceTypes
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
          checklistItems,
          criticalityRules: { criticalFailure: 'BLOCK_ASSET' },
        },
        newInspectionIdempotencyKey(),
      );
      onCreated();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to create template.',
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={submit}
      className="space-y-4 rounded-lg border border-line bg-surface p-4"
    >
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-ink-950">New template</h3>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md p-1 text-ink-400 hover:bg-surface-2 hover:text-ink-700"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      {error && (
        <div className="rounded-md border border-red/30 bg-red-bg px-3 py-2 text-xs text-red">
          {error}
        </div>
      )}

      <Field label="Template name" required>
        <input
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Motorcycle Safety Check"
          className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </Field>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <Field label="Asset types (comma separated)">
          <input
            value={assetTypes}
            onChange={(e) => setAssetTypes(e.target.value)}
            placeholder="MOTORCYCLE,VAN"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Service types (comma separated)">
          <input
            value={serviceTypes}
            onChange={(e) => setServiceTypes(e.target.value)}
            placeholder="DISPATCH,HAULING"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
      </div>

      <Field label="Checklist item codes (comma separated)" required>
        <input
          required
          value={items}
          onChange={(e) => setItems(e.target.value)}
          placeholder="brakes, tyres, lights"
          className="w-full rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
        />
      </Field>

      <button
        type="submit"
        disabled={busy}
        className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
      >
        {busy ? 'Creating…' : 'Create template'}
      </button>
    </form>
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