"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { ArrowLeft, FileUp, Plus, RefreshCw, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DataTable,
  EmptyBlock,
  Loading,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatWhen,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { orderImportService } from "@/lib/services/operationsService";
import { structureService } from "@/lib/services/structureService";
import type { OrderImport } from "@/lib/types/operations";
import type { Branch } from "@/lib/types/structure";

const EMPTY_FORM = { branchId: "", mediaFileId: "", fileFormat: "CSV" };

// Bulk order imports: a CSV or JSON file is validated row by row and the
// valid rows become draft orders.
export default function OrderImportsPage() {
  const router = useRouter();
  const [rows, setRows] = useState<OrderImport[]>([]);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY_FORM);
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setRows(await orderImportService.list());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Imports could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
    structureService.listBranches().then(setBranches).catch(() => setBranches([]));
  }, [load]);

  const branchName = (id: string) => branches.find((b) => b.id === id)?.name ?? id;

  const create = async (e: FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const created = await orderImportService.create(
        { branchId: form.branchId, mediaFileId: form.mediaFileId.trim(), fileFormat: form.fileFormat },
        key,
      );
      setKey(newIdempotencyKey());
      router.push(`/private-ops/orders/import/${created.ID}`);
    } catch (err) {
      setFormError(describeApiError(err, "The import could not be started."));
    } finally {
      setSaving(false);
    }
  };

  if (loading && rows.length === 0 && !error) return <Loading label="Loading imports..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Order imports"
        description="Create many orders at once from a CSV or JSON file."
        actions={
          <>
            <SecondaryButton onClick={load}>
              <RefreshCw className="h-3 w-3" /> Refresh
            </SecondaryButton>
            <SecondaryButton onClick={() => router.push("/private-ops/orders")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Orders
            </SecondaryButton>
            <PrimaryButton
              onClick={() => {
                setForm(EMPTY_FORM);
                setFormError(null);
                setShowCreate(true);
              }}
            >
              <Plus className="h-3.5 w-3.5" /> New import
            </PrimaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />

      <StatGrid
        items={[
          { label: "Loaded", value: rows.length },
          { label: "In progress", value: rows.filter((i) => ["UPLOADED", "VALIDATING", "READY", "IMPORTING"].includes(i.Status)).length, tone: "amber" },
          { label: "Completed", value: rows.filter((i) => i.Status === "COMPLETED" || i.Status === "PARTIALLY_COMPLETED").length, tone: "green" },
          { label: "Failed", value: rows.filter((i) => i.Status === "FAILED" || i.Status === "VALIDATION_FAILED").length, tone: "red" },
        ]}
      />

      {showCreate && (
        <Panel title="New import" description="Reference a file already stored in the media service." onClose={() => setShowCreate(false)}>
          <form onSubmit={create} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
              <Field label="Branch" required>
                <select required value={form.branchId} onChange={(e) => setForm({ ...form, branchId: e.target.value })} className={INPUT}>
                  <option value="">Choose a branch</option>
                  {branches.filter((b) => b.status === "ACTIVE").map((b) => <option key={b.id} value={b.id}>{b.name}</option>)}
                </select>
              </Field>
              <Field label="Media file ID" required>
                <input required value={form.mediaFileId} onChange={(e) => setForm({ ...form, mediaFileId: e.target.value })} className={INPUT} />
              </Field>
              <Field label="File format" required>
                <select value={form.fileFormat} onChange={(e) => setForm({ ...form, fileFormat: e.target.value })} className={INPUT}>
                  <option value="CSV">CSV</option>
                  <option value="JSON">JSON</option>
                </select>
              </Field>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Starting..." : "Start import"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      {rows.length === 0 ? (
        <EmptyBlock icon={<FileUp className="h-8 w-8" />} title="No imports yet" description="Start an import to create orders in bulk." />
      ) : (
        <DataTable
          rows={rows}
          rowKey={(i) => i.ID}
          onRowClick={(i) => router.push(`/private-ops/orders/import/${i.ID}`)}
          columns={[
            { header: "Import", cell: (i) => <span className="font-mono text-ink-950">{i.FileName || i.ID}</span> },
            { header: "Branch", cell: (i) => branchName(i.BranchID) },
            { header: "Format", cell: (i) => i.FileFormat },
            { header: "Rows", cell: (i) => `${i.ImportedRows} imported · ${i.InvalidRows + i.FailedRows} failed · ${i.TotalRows} total` },
            { header: "Status", cell: (i) => <StatusPill status={i.Status} /> },
            { header: "Created", cell: (i) => formatWhen(i.CreatedAt) },
          ]}
        />
      )}
    </div>
  );
}
