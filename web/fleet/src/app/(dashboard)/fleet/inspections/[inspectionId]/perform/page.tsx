'use client';

import { useEffect, useMemo, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import {
  ArrowLeft,
  CheckCircle2,
  Save,
  XCircle,
} from 'lucide-react';
import {
  inspectionService,
  newInspectionIdempotencyKey,
} from '@/lib/services/inspectionService';
import type {
  Answer,
  ChecklistItem,
  Inspection,
  InspectionTemplate,
} from '@/lib/types/inspection';

type LocalAnswer = {
  passed: boolean | null;
  advisory: boolean;
  evidenceMediaIds: string;
};

export default function PerformInspectionPage() {
  const params = useParams();
  const router = useRouter();
  const inspectionId = params.inspectionId as string;

  const [inspection, setInspection] = useState<Inspection | null>(null);
  const [template, setTemplate] = useState<InspectionTemplate | null>(null);
  const [answers, setAnswers] = useState<Record<string, LocalAnswer>>({});
  const [defectSummary, setDefectSummary] = useState('');

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!inspectionId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    inspectionService
      .getInspection(inspectionId)
      .then(async (data) => {
        if (cancelled) return;
        setInspection(data);
        const tpl = await inspectionService.getTemplate(data.templateId);
        if (cancelled) return;
        setTemplate(tpl);

        const initial: Record<string, LocalAnswer> = {};
        for (const item of tpl.checklistItems) {
          initial[item.code] = {
            passed: null,
            advisory: false,
            evidenceMediaIds: '',
          };
        }
        setAnswers(initial);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load inspection.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [inspectionId]);

  const items = template?.checklistItems ?? [];

  const validation = useMemo(() => {
    const missingRequired: string[] = [];
    const missingEvidence: string[] = [];

    for (const item of items) {
      const a = answers[item.code];
      if (!a) continue;

      if (item.required && a.passed === null) {
        missingRequired.push(item.code);
      }
      if (item.evidenceRequired && a.passed === false) {
        const count = a.evidenceMediaIds
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean).length;
        if (count === 0) missingEvidence.push(item.code);
      }
    }
    return { missingRequired, missingEvidence };
  }, [items, answers]);

  const canSubmit =
    validation.missingRequired.length === 0 &&
    validation.missingEvidence.length === 0 &&
    inspection != null &&
    items.length > 0;

  function setAnswer(code: string, patch: Partial<LocalAnswer>) {
    setAnswers((prev) => ({
      ...prev,
      [code]: { ...prev[code], ...patch },
    }));
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (!inspection || !template) return;
    setSubmitting(true);
    setError(null);

    try {
      const payload: Answer[] = items.map((item) => {
        const a = answers[item.code];
        const evidence = a.evidenceMediaIds
          .split(',')
          .map((s) => s.trim())
          .filter(Boolean);
        return {
          itemCode: item.code,
          passed: a.passed === true,
          advisory: a.advisory,
          evidenceMediaIds: evidence,
        };
      });

      await inspectionService.completeInspection(
        inspection.id,
        {
          expectedVersion: inspection.version,
          answers: payload,
          defectSummary: defectSummary.trim() || undefined,
        },
        newInspectionIdempotencyKey(),
      );

      router.push(`/fleet/inspections/${inspection.id}/result`);
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to complete inspection.',
      );
    } finally {
      setSubmitting(false);
    }
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading inspection…
      </div>
    );
  }

  if (error && !inspection) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/inspections"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to inspections
        </Link>
      </div>
    );
  }

  if (!inspection || !template) return null;

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/inspections"
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <h1>Perform inspection</h1>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {inspection.id} · {template.name}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      <div className="rounded-lg border border-line bg-surface-2 p-3 text-xs text-ink-600">
        Answer every required item. A failed critical item blocks the asset and
        creates a work order automatically.
      </div>

      <form onSubmit={handleSubmit} className="space-y-4">
        {items.map((item) => (
          <ItemCard
            key={item.code}
            item={item}
            answer={answers[item.code]}
            onChange={(patch) => setAnswer(item.code, patch)}
          />
        ))}

        <div className="rounded-lg border border-line bg-surface p-4">
          <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
            Defect summary (optional)
          </label>
          <textarea
            rows={3}
            value={defectSummary}
            onChange={(e) => setDefectSummary(e.target.value)}
            placeholder="Notes about defects found"
            className="w-full resize-none rounded-lg border border-line bg-surface px-3 py-2.5 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </div>

        {(validation.missingRequired.length > 0 ||
          validation.missingEvidence.length > 0) && (
          <div className="rounded-lg border border-amber/30 bg-amber-bg/20 px-4 py-3 text-xs text-amber">
            {validation.missingRequired.length > 0 && (
              <div>
                Missing answers for:{' '}
                <span className="font-mono">
                  {validation.missingRequired.join(', ')}
                </span>
              </div>
            )}
            {validation.missingEvidence.length > 0 && (
              <div>
                Missing evidence for:{' '}
                <span className="font-mono">
                  {validation.missingEvidence.join(', ')}
                </span>
              </div>
            )}
          </div>
        )}

        <button
          type="submit"
          disabled={!canSubmit || submitting}
          className="flex h-10 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
        >
          <Save className="h-4 w-4" />
          {submitting ? 'Submitting…' : 'Complete inspection'}
        </button>
      </form>
    </div>
  );
}

function ItemCard({
  item,
  answer,
  onChange,
}: {
  item: ChecklistItem;
  answer: LocalAnswer | undefined;
  onChange: (patch: Partial<LocalAnswer>) => void;
}) {
  const a = answer ?? {
    passed: null,
    advisory: false,
    evidenceMediaIds: '',
  };

  return (
    <div className="rounded-lg border border-line bg-surface p-4">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-semibold text-ink-950">
          {item.label ?? item.code}
        </span>
        <span className="font-mono text-[10px] text-ink-400">{item.code}</span>
        {item.critical && (
          <span className="rounded-full bg-red-bg px-2 py-0.5 text-[9px] font-semibold text-red">
            Critical
          </span>
        )}
        {item.required && (
          <span className="rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">
            Required
          </span>
        )}
        {item.evidenceRequired && (
          <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
            Evidence required on fail
          </span>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <button
          type="button"
          onClick={() => onChange({ passed: true })}
          className={`flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-semibold transition-colors ${
            a.passed === true
              ? 'border-green/40 bg-green-bg text-green'
              : 'border-line bg-surface text-ink-600 hover:bg-surface-2'
          }`}
        >
          <CheckCircle2 className="h-3.5 w-3.5" />
          Pass
        </button>
        <button
          type="button"
          onClick={() => onChange({ passed: false })}
          className={`flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-xs font-semibold transition-colors ${
            a.passed === false
              ? 'border-red/40 bg-red-bg text-red'
              : 'border-line bg-surface text-ink-600 hover:bg-surface-2'
          }`}
        >
          <XCircle className="h-3.5 w-3.5" />
          Fail
        </button>

        {a.passed === true && (
          <label className="flex items-center gap-2 rounded-md border border-line bg-surface px-3 py-1.5 text-xs text-ink-600">
            <input
              type="checkbox"
              checked={a.advisory}
              onChange={(e) => onChange({ advisory: e.target.checked })}
              className="h-3.5 w-3.5 rounded border-line"
            />
            Advisory
          </label>
        )}
      </div>

      {a.passed === false && (
        <div className="mt-3">
          <label className="mb-1.5 block text-xs font-medium uppercase tracking-wide text-ink-500">
            Evidence media IDs (comma separated)
            {item.evidenceRequired && <span className="ml-1 text-red">*</span>}
          </label>
          <input
            value={a.evidenceMediaIds}
            onChange={(e) => onChange({ evidenceMediaIds: e.target.value })}
            placeholder="med_..., med_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </div>
      )}
    </div>
  );
}