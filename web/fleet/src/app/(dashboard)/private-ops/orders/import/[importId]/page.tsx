"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, RefreshCw, RotateCcw, XCircle } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import {
  DangerButton,
  DataTable,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { orderImportService } from "@/lib/services/operationsService";
import type { OrderImport, OrderImportRow } from "@/lib/types/operations";

const TERMINAL = ["COMPLETED", "PARTIALLY_COMPLETED", "FAILED", "CANCELLED", "VALIDATION_FAILED"];
const ROW_FILTERS = ["ALL", "VALID", "INVALID", "IMPORTED", "FAILED"];

export default function OrderImportDetailPage() {
  const params = useParams();
  const router = useRouter();
  const importId = params.importId as string;

  const [job, setJob] = useState<OrderImport | null>(null);
  const [rows, setRows] = useState<OrderImportRow[]>([]);
  const [rowFilter, setRowFilter] = useState("ALL");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [keys, setKeys] = useState(() => ({ cancel: newIdempotencyKey(), replay: newIdempotencyKey() }));
  const [busy, setBusy] = useState<"cancel" | "replay" | null>(null);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      const detail = await orderImportService.get(importId);
      setJob(detail.import);
      setRows(detail.rows ?? []);
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The import could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [importId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const cancel = async () => {
    if (!job) return;
    setBusy("cancel");
    setActionError(null);
    try {
      setJob(await orderImportService.cancel(job.ID, job.Version, keys.cancel));
      setKeys((k) => ({ ...k, cancel: newIdempotencyKey() }));
      setNotice("Import cancelled.");
    } catch (err) {
      setActionError(describeApiError(err, "The import could not be cancelled."));
    } finally {
      setBusy(null);
    }
  };

  const replay = async () => {
    if (!job) return;
    setBusy("replay");
    setActionError(null);
    try {
      await orderImportService.replay(job.ID, keys.replay);
      setKeys((k) => ({ ...k, replay: newIdempotencyKey() }));
      setNotice("Import queued again.");
      await load();
    } catch (err) {
      setActionError(describeApiError(err, "The import could not be replayed."));
    } finally {
      setBusy(null);
    }
  };

  const filtered = useMemo(() => (rowFilter === "ALL" ? rows : rows.filter((r) => r.Status === rowFilter)), [rows, rowFilter]);

  if (loading) return <Loading label="Loading import..." />;
  if (!job) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/private-ops/orders/import" className="text-sm text-primary">Back to imports</Link>
      </div>
    );
  }

  const terminal = TERMINAL.includes(job.Status);

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Import ${job.FileName || job.ID}`}
        description={`Started ${formatWhen(job.CreatedAt)} · ${job.FileFormat}`}
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/private-ops/orders/import")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </SecondaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <StatGrid
        items={[
          { label: "Total rows", value: job.TotalRows },
          { label: "Imported", value: job.ImportedRows, tone: "green" },
          { label: "Invalid", value: job.InvalidRows, tone: "amber" },
          { label: "Failed", value: job.FailedRows, tone: "red" },
        ]}
      />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={job.Status} />],
          ["Branch", job.BranchID],
          ["Media file", job.MediaFileID],
          ["Valid rows", job.ValidRows],
          ["Attempts", job.Attempts],
          ["Completed", formatWhen(job.CompletedAt)],
          ["Cancelled", formatWhen(job.CancelledAt)],
          ["Error report", job.ErrorReportMediaID || "—"],
          ["Version", job.Version],
        ]}
      />

      {(!terminal || job.Status === "FAILED") && (
        <Panel title="Actions">
          <ApiErrorNotice error={actionError} />
          <div className="flex flex-wrap gap-2">
            {job.Status === "FAILED" && (
              <SecondaryButton onClick={replay} disabled={busy !== null}>
                <RotateCcw className="h-3.5 w-3.5" /> {busy === "replay" ? "Queueing..." : "Replay import"}
              </SecondaryButton>
            )}
            {!terminal && (
              <DangerButton onClick={cancel} disabled={busy !== null}>
                <XCircle className="h-3.5 w-3.5" /> {busy === "cancel" ? "Cancelling..." : "Cancel import"}
              </DangerButton>
            )}
          </div>
        </Panel>
      )}

      <Panel
        title="Row outcomes"
        actions={
          <select value={rowFilter} onChange={(e) => setRowFilter(e.target.value)} className="h-9 rounded-xl border border-line bg-white/70 px-3 text-xs text-ink-600 shadow-sm" aria-label="Row status">
            {ROW_FILTERS.map((s) => <option key={s} value={s}>{s === "ALL" ? "All rows" : humanize(s)}</option>)}
          </select>
        }
      >
        {filtered.length === 0 ? (
          <p className="text-sm text-ink-500">{rows.length === 0 ? "No rows have been processed yet." : "No rows match this filter."}</p>
        ) : (
          <DataTable
            minWidth={720}
            rows={filtered}
            rowKey={(r) => r.ID}
            onRowClick={(r) => {
              if (r.GeneratedOrderID) router.push(`/private-ops/orders/${r.GeneratedOrderID}`);
            }}
            columns={[
              { header: "Row", cell: (r) => r.RowNumber },
              { header: "Reference", cell: (r) => <span className="font-mono">{r.RowReference || "—"}</span> },
              { header: "Status", cell: (r) => <StatusPill status={r.Status} /> },
              { header: "Order", cell: (r) => <span className="font-mono">{r.GeneratedOrderID || "—"}</span> },
              { header: "Problem", cell: (r) => (r.SafeErrorMessage ? `${r.SafeErrorMessage}${r.SafeErrorCode ? ` (${r.SafeErrorCode})` : ""}` : "—") },
            ]}
          />
        )}
      </Panel>
    </div>
  );
}
