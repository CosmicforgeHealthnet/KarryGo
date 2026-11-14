"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Plus, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { DataTable, Notice, Panel, PrimaryButton, SecondaryButton, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { privateOpsService } from "@/lib/services/operationsService";
import { specializedService, type Capability } from "@/lib/services/specializedService";
import type { CapabilitySchema, SchemaField, SpecializedRecord } from "@/lib/types/operations";

// Typed value for one schema field from what the person typed. Object fields
// are entered as JSON.
function fieldValue(field: SchemaField, raw: string): unknown {
  const v = raw.trim();
  if (v === "") return undefined;
  switch (field.kind) {
    case "integer":
      return Number.parseInt(v, 10);
    case "number":
      return Number(v);
    case "boolean":
      return v === "true";
    case "stringArray":
    case "qualificationReferences":
      return v.split(",").map((s) => s.trim()).filter(Boolean);
    case "object":
    case "objectArray":
      return JSON.parse(v);
    case "timestamp":
      return new Date(v).toISOString();
    default:
      return v;
  }
}

// Capability-specific data stored as a typed extension of the order (cold
// chain, hazardous goods, ...). The form is built from the capability's
// published schema; the server validates against the same schema.
export function SpecializedDataPanel({ orderId }: { orderId: string }) {
  const [records, setRecords] = useState<SpecializedRecord[]>([]);
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [capability, setCapability] = useState("");
  const [schema, setSchema] = useState<CapabilitySchema | null>(null);
  const [recordType, setRecordType] = useState("");
  const [values, setValues] = useState<Record<string, string>>({});
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setRecords(await privateOpsService.listSpecializedData(orderId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Specialized data could not be loaded."));
    }
  }, [orderId]);

  useEffect(() => {
    Promise.resolve().then(load);
    specializedService
      .listCapabilities()
      .then((caps) => setCapabilities(caps.filter((c) => c.Status === "ENABLED")))
      .catch(() => setCapabilities([]));
  }, [load]);

  const chooseCapability = async (code: string) => {
    setCapability(code);
    setSchema(null);
    setRecordType("");
    setValues({});
    if (!code) return;
    try {
      const s = await privateOpsService.capabilitySchema(code);
      setSchema(s);
      setRecordType(s.recordTypes[0]?.recordType ?? "");
    } catch (e) {
      setFormError(describeApiError(e, "The capability schema could not be loaded."));
    }
  };

  const fields = schema?.recordTypes.find((r) => r.recordType === recordType)?.fields ?? [];

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!schema) return;
    setSaving(true);
    setFormError(null);
    try {
      const payload: Record<string, unknown> = {};
      for (const f of fields) {
        const value = fieldValue(f, values[f.name] ?? "");
        if (value !== undefined) payload[f.name] = value;
      }
      await privateOpsService.createSpecializedData(orderId, { capabilityCode: schema.capabilityCode, recordType, schemaVersion: schema.version, payload }, key);
      setKey(newIdempotencyKey());
      setShowForm(false);
      setNotice("Specialized data saved.");
      await load();
    } catch (err) {
      setFormError(
        err instanceof SyntaxError ? { message: "An object field is not valid JSON." } : describeApiError(err, "The specialized data could not be saved."),
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Panel
      title="Specialized data"
      description="Capability-specific details validated against the capability schema."
      actions={
        !showForm &&
        capabilities.length > 0 && (
          <SecondaryButton onClick={() => setShowForm(true)}>
            <Plus className="h-3.5 w-3.5" /> Add data
          </SecondaryButton>
        )
      }
    >
      <ApiErrorNotice error={error} />
      <Notice message={notice} />
      {showForm && (
        <form onSubmit={submit} className="space-y-4 rounded-xl border border-line p-4">
          <ApiErrorNotice error={formError} />
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <Field label="Capability" required>
              <select required value={capability} onChange={(e) => chooseCapability(e.target.value)} className={INPUT}>
                <option value="">Choose a capability</option>
                {capabilities.map((c) => <option key={c.CapabilityCode} value={c.CapabilityCode}>{humanize(c.CapabilityCode)}</option>)}
              </select>
            </Field>
            {schema && (
              <Field label="Record type" required hint={`Schema ${schema.schemaId}`}>
                <select value={recordType} onChange={(e) => setRecordType(e.target.value)} className={INPUT}>
                  {schema.recordTypes.map((r) => <option key={r.recordType} value={r.recordType}>{humanize(r.recordType)}</option>)}
                </select>
              </Field>
            )}
            {fields.map((f) => (
              <Field key={f.name} label={humanize(f.name)} required={f.required} hint={f.kind === "object" || f.kind === "objectArray" ? "JSON" : f.kind}>
                {f.enum ? (
                  <select required={f.required} value={values[f.name] ?? ""} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })} className={INPUT}>
                    <option value="">—</option>
                    {f.enum.map((v) => <option key={v} value={v}>{humanize(v)}</option>)}
                  </select>
                ) : f.kind === "boolean" ? (
                  <select value={values[f.name] ?? ""} onChange={(e) => setValues({ ...values, [f.name]: e.target.value })} className={INPUT}>
                    <option value="">—</option>
                    <option value="true">Yes</option>
                    <option value="false">No</option>
                  </select>
                ) : (
                  <input
                    required={f.required}
                    type={f.kind === "timestamp" ? "datetime-local" : "text"}
                    inputMode={f.kind === "integer" || f.kind === "number" ? "decimal" : undefined}
                    value={values[f.name] ?? ""}
                    onChange={(e) => setValues({ ...values, [f.name]: e.target.value })}
                    className={INPUT}
                  />
                )}
              </Field>
            ))}
          </div>
          <div className="flex gap-2">
            <PrimaryButton type="submit" disabled={saving || !schema}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save data"}
            </PrimaryButton>
            <SecondaryButton onClick={() => setShowForm(false)}>Cancel</SecondaryButton>
          </div>
        </form>
      )}
      {records.length === 0 ? (
        <p className="text-sm text-ink-500">No specialized data recorded for this order.</p>
      ) : (
        <DataTable
          minWidth={640}
          rows={records}
          rowKey={(r) => r.ID}
          columns={[
            { header: "Capability", cell: (r) => humanize(r.CapabilityCode) },
            { header: "Record", cell: (r) => humanize(r.RecordType) },
            { header: "Schema", cell: (r) => r.SchemaID || `v${r.SchemaVersion}` },
            { header: "Status", cell: (r) => <StatusPill status={r.Status} /> },
            { header: "Recorded", cell: (r) => formatWhen(r.CreatedAt) },
          ]}
        />
      )}
    </Panel>
  );
}
