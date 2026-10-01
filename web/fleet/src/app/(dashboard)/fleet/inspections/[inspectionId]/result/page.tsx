'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  FileText,
  XCircle,
} from 'lucide-react';
import { inspectionService } from '@/lib/services/inspectionService';
import {
  formatDateTime,
  inspectionStatusLabel,
  inspectionStatusTone,
  overallResultLabel,
  overallResultTone,
} from '@/lib/utils/inspectionLabels';
import type { Inspection } from '@/lib/types/inspection';

export default function InspectionResultPage() {
  const params = useParams();
  const inspectionId = params.inspectionId as string;

  const [inspection, setInspection] = useState<Inspection | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!inspectionId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    inspectionService
      .getInspection(inspectionId)
      .then((data) => {
        if (!cancelled) setInspection(data);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load inspection result.',
        );
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [inspectionId]);

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

  if (!inspection) return null;

  const statusTone = inspectionStatusTone(inspection.status);
  const resultTone = overallResultTone(inspection.overallResult);
  const isFailed =
    inspection.overallResult === 'FAIL' ||
    inspection.overallResult === 'CRITICAL_FAIL';
  const isCritical = inspection.overallResult === 'CRITICAL_FAIL';
  const isPassed =
    inspection.overallResult === 'PASS' ||
    inspection.overallResult === 'PASS_WITH_ADVISORY';

  const defectSummaryText =
    inspection.defectSummary != null
      ? typeof inspection.defectSummary === 'string'
        ? inspection.defectSummary
        : JSON.stringify(inspection.defectSummary, null, 2)
      : '';

  const rows: { label: string; value: React.ReactNode }[] = [
    { label: 'Inspection ID', value: <span className="font-mono text-xs">{inspection.id}</span> },
    { label: 'Asset ID', value: <span className="font-mono text-xs">{inspection.assetId}</span> },
    { label: 'Template ID', value: <span className="font-mono text-xs">{inspection.templateId}</span> },
    { label: 'Inspector', value: <span className="font-mono text-xs">{inspection.inspectorMembershipId}</span> },
    { label: 'Assignment ID', value: inspection.assignmentId ? <span className="font-mono text-xs">{inspection.assignmentId}</span> : '—' },
    { label: 'Trip reference', value: inspection.tripReference || '—' },
    { label: 'Started at', value: formatDateTime(inspection.startedAt) },
    { label: 'Completed at', value: formatDateTime(inspection.completedAt) },
    { label: 'Version', value: <span className="font-mono text-xs">{inspection.version}</span> },
  ];

  return (
    <div className="mx-auto max-w-3xl space-y-5">
      <div className="flex items-center gap-3">
        <Link
          href="/fleet/inspections"
          className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2.5">
            <h1 className="truncate">Inspection result</h1>
            <span
              className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${statusTone.bg} ${statusTone.text}`}
            >
              <span className={`h-1.5 w-1.5 rounded-full ${statusTone.dot}`} />
              {inspectionStatusLabel(inspection.status)}
            </span>
            {inspection.overallResult && (
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-semibold ${resultTone.bg} ${resultTone.text}`}
              >
                {overallResultLabel(inspection.overallResult)}
              </span>
            )}
          </div>
          <p className="mt-0.5 font-mono text-xs text-ink-500">
            {inspection.id}
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {isPassed && (
        <div className="rounded-lg border border-green/20 bg-green-bg/20 p-4">
          <div className="flex items-start gap-3">
            <CheckCircle2 className="h-5 w-5 flex-shrink-0 text-green mt-0.5" />
            <div>
              <div className="text-sm font-semibold text-ink-950">
                {inspection.overallResult === 'PASS_WITH_ADVISORY'
                  ? 'Passed with advisory'
                  : 'Passed'}
              </div>
              <p className="mt-0.5 text-sm text-ink-600">
                The asset meets safety requirements. Advisory items are noted
                for future attention.
              </p>
            </div>
          </div>
        </div>
      )}

      {isFailed && !isCritical && (
        <div className="rounded-lg border border-amber/30 bg-amber-bg/20 p-4">
          <div className="flex items-start gap-3">
            <AlertCircle className="h-5 w-5 flex-shrink-0 text-amber mt-0.5" />
            <div>
              <div className="text-sm font-semibold text-ink-950">
                Failed
              </div>
              <p className="mt-0.5 text-sm text-ink-600">
                Non-critical defects found. A work order has been created to
                resolve them.
              </p>
            </div>
          </div>
        </div>
      )}

      {isCritical && (
        <div className="rounded-lg border border-red/30 bg-red-bg/20 p-4">
          <div className="flex items-start gap-3">
            <XCircle className="h-5 w-5 flex-shrink-0 text-red mt-0.5" />
            <div>
              <div className="text-sm font-semibold text-ink-950">
                Critical failure
              </div>
              <p className="mt-0.5 text-sm text-ink-600">
                The asset is blocked. A critical work order has been created
                automatically and must be resolved before the asset can return
                to service.
              </p>
            </div>
          </div>
        </div>
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

      {defectSummaryText && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-2 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <FileText className="h-4 w-4" />
            Defect summary
          </h3>
          <pre className="whitespace-pre-wrap break-words rounded-lg bg-surface-2 p-3 text-xs text-ink-700">
            {defectSummaryText}
          </pre>
        </div>
      )}

      {inspection.evidenceMediaIds.length > 0 && (
        <div className="rounded-lg border border-line bg-surface p-4">
          <h3 className="mb-3 flex items-center gap-2 text-sm font-semibold text-ink-950">
            <FileText className="h-4 w-4" />
            Evidence
          </h3>
          <div className="flex flex-wrap gap-2">
            {inspection.evidenceMediaIds.map((id) => (
              <span
                key={id}
                className="rounded-full bg-surface-2 px-3 py-1 font-mono text-xs text-ink-700"
              >
                {id}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}