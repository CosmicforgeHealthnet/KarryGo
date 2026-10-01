'use client';

import { useEffect, useState, type FormEvent } from 'react';
import Link from 'next/link';
import { useParams } from 'next/navigation';
import { ArrowLeft, FileText, Plus, X } from 'lucide-react';
import EmptyState from '@/components/EmptyState';
import {
  assetService,
  newAssetIdempotencyKey,
} from '@/lib/services/assetService';
import {
  formatDate,
  verificationStatusLabel,
  verificationStatusTone,
} from '@/lib/utils/assetLabels';
import type { AssetDocument } from '@/lib/types/asset';

export default function AssetDocumentsPage() {
  const params = useParams();
  const assetId = params.assetId as string;

  const [docs, setDocs] = useState<AssetDocument[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [showForm, setShowForm] = useState(false);

  const load = () => {
    if (!assetId) return;
    setLoading(true);
    setError(null);
    assetService
      .listDocuments(assetId)
      .then(setDocs)
      .catch((err) => {
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Failed to load documents.',
        );
      })
      .finally(() => setLoading(false));
  };

  useEffect(load, [assetId]);

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link
            href={`/fleet/assets/${assetId}`}
            className="flex h-9 w-9 items-center justify-center rounded-lg border border-line bg-surface text-ink-500 transition-colors hover:bg-surface-2"
          >
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-ink-950">
              Documents
            </h1>
            <p className="mt-0.5 font-mono text-xs text-ink-500">{assetId}</p>
          </div>
        </div>
        <button
          onClick={() => setShowForm((v) => !v)}
          className="flex h-10 items-center gap-1.5 rounded-lg bg-primary px-3.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover"
        >
          <Plus className="h-3.5 w-3.5" />
          Attach document
        </button>
      </div>

      {error && (
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
      )}

      {showForm && (
        <AttachForm
          assetId={assetId}
          onCancel={() => setShowForm(false)}
          onCreated={() => {
            setShowForm(false);
            load();
          }}
        />
      )}

      {loading ? (
        <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
          Loading documents…
        </div>
      ) : docs.length === 0 ? (
        <EmptyState
          image="/images/documents-empty.png"
          title="No documents yet"
          description="Attach registration, insurance, or inspection documents to this asset."
        />
      ) : (
        <div className="overflow-x-auto rounded-lg border border-line bg-surface">
          <table className="w-full min-w-[760px] border-collapse text-left">
            <thead>
              <tr className="border-b border-line bg-surface-2">
                <Th>Type</Th>
                <Th>Media ID</Th>
                <Th>Issued</Th>
                <Th>Expires</Th>
                <Th>Verification</Th>
                <Th className="text-right">Version</Th>
              </tr>
            </thead>
            <tbody>
              {docs.map((doc) => {
                const tone = verificationStatusTone(doc.verificationStatus);
                return (
                  <tr
                    key={doc.id}
                    className="border-b border-line last:border-0 hover:bg-surface-2"
                  >
                    <Td>
                      <span className="text-sm font-medium text-ink-950">
                        {doc.documentType || '—'}
                      </span>
                    </Td>
                    <Td>
                      <span className="font-mono text-xs text-ink-500">
                        {doc.mediaFileId}
                      </span>
                    </Td>
                    <Td>{formatDate(doc.issuedAt)}</Td>
                    <Td>{formatDate(doc.expiresAt)}</Td>
                    <Td>
                      <span
                        className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${tone.bg} ${tone.text}`}
                      >
                        <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
                        {verificationStatusLabel(doc.verificationStatus)}
                      </span>
                    </Td>
                    <Td className="text-right">
                      <span className="font-mono text-xs text-ink-500">
                        {doc.version}
                      </span>
                    </Td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function AttachForm({
  assetId,
  onCancel,
  onCreated,
}: {
  assetId: string;
  onCancel: () => void;
  onCreated: () => void;
}) {
  const [form, setForm] = useState({
    mediaFileId: '',
    documentType: '',
    issuedAt: '',
    expiresAt: '',
  });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await assetService.addDocument(
        assetId,
        {
          mediaFileId: form.mediaFileId.trim(),
          documentType: form.documentType.trim(),
          issuedAt: form.issuedAt || undefined,
          expiresAt: form.expiresAt || undefined,
        },
        newAssetIdempotencyKey(),
      );
      onCreated();
    } catch (err: any) {
      setError(
        err?.response?.data?.error?.message ??
          err?.message ??
          'Failed to attach document.',
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
        <h3 className="flex items-center gap-2 text-sm font-semibold text-ink-950">
          <FileText className="h-4 w-4" />
          Attach document
        </h3>
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

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <Field label="Media file ID" required>
          <input
            required
            value={form.mediaFileId}
            onChange={(e) => setForm({ ...form, mediaFileId: e.target.value })}
            placeholder="med_..."
            className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Document type" required>
          <input
            required
            value={form.documentType}
            onChange={(e) => setForm({ ...form, documentType: e.target.value })}
            placeholder="INSURANCE"
            className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Issued at">
          <input
            type="date"
            value={form.issuedAt}
            onChange={(e) => setForm({ ...form, issuedAt: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
        <Field label="Expires at">
          <input
            type="date"
            value={form.expiresAt}
            onChange={(e) => setForm({ ...form, expiresAt: e.target.value })}
            className="w-full rounded-lg border border-line bg-surface px-3 py-2 text-sm text-ink-950 outline-none focus:border-primary focus:ring-2 focus:ring-primary/20"
          />
        </Field>
      </div>

      <button
        type="submit"
        disabled={busy}
        className="rounded-md bg-primary px-3.5 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60"
      >
        {busy ? 'Attaching…' : 'Attach'}
      </button>
    </form>
  );
}

function Th({
  children,
  className = '',
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <th
      className={`px-4 py-3 text-[10px] font-semibold uppercase tracking-[0.06em] text-ink-500 whitespace-nowrap ${className}`}
    >
      {children}
    </th>
  );
}

function Td({
  children,
  className = '',
}: {
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <td className={`px-4 py-3.5 text-sm text-ink-700 ${className}`}>
      {children}
    </td>
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