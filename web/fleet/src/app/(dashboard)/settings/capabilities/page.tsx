"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { Power, PowerOff, RefreshCw } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
  DataTable,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatGrid,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { SPECIALIZED_CAPABILITY_CODES, specializedService, type Capability } from "@/lib/services/specializedService";

type Row = { code: string; capability?: Capability };
type Pending = { code: string; enable: boolean; version?: number };

// Specialized logistics capabilities (spec 20.1): which specialized modules
// (cold chain, medical, heavy haulage...) this organization uses.
export default function CapabilityConfigurationPage() {
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [pending, setPending] = useState<Pending | null>(null);
  const [reason, setReason] = useState("");
  const [key, setKey] = useState(newIdempotencyKey);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      setCapabilities(await specializedService.listCapabilities());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Capabilities could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const rows: Row[] = SPECIALIZED_CAPABILITY_CODES.map((code) => ({
    code,
    capability: capabilities.find((c) => c.CapabilityCode === code),
  }));

  const open = (row: Row, enable: boolean) => {
    setPending({ code: row.code, enable, version: row.capability?.Version });
    setReason("");
    setKey(newIdempotencyKey());
    setFormError(null);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!pending) return;
    setSaving(true);
    setFormError(null);
    try {
      const updated = await specializedService.setCapability(pending.code, pending.enable, reason.trim(), pending.version, key);
      setCapabilities((list) => [...list.filter((c) => c.CapabilityCode !== updated.CapabilityCode), updated]);
      setNotice(`${humanize(pending.code)} ${pending.enable ? "enabled" : "disabled"}.`);
      setPending(null);
    } catch (err) {
      setFormError(describeApiError(err, "The capability could not be changed."));
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <Loading label="Loading capabilities..." />;

  return (
    <div className="space-y-5">
      <PageHeader
        title="Capability configuration"
        description="Turn on the specialized logistics modules your company runs. A module not offered in this deployment cannot be enabled."
        actions={
          <SecondaryButton onClick={load}>
            <RefreshCw className="h-3 w-3" /> Refresh
          </SecondaryButton>
        }
      />

      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <StatGrid
        items={[
          { label: "Enabled", value: capabilities.filter((c) => c.Status === "ENABLED").length, tone: "green" },
          { label: "Disabled", value: capabilities.filter((c) => c.Status === "DISABLED").length },
          { label: "Never configured", value: rows.filter((r) => !r.capability).length },
        ]}
      />

      {pending && (
        <Panel
          title={`${pending.enable ? "Enable" : "Disable"} ${humanize(pending.code)}`}
          description="The reason is kept in the capability history."
          onClose={() => setPending(null)}
        >
          <form onSubmit={submit} className="space-y-4">
            <ApiErrorNotice error={formError} />
            <Field label="Reason" required>
              <textarea required rows={2} value={reason} onChange={(e) => setReason(e.target.value)} className={INPUT} />
            </Field>
            {pending.enable ? (
              <PrimaryButton type="submit" disabled={saving}>
                <Power className="h-3.5 w-3.5" /> {saving ? "Enabling..." : "Enable capability"}
              </PrimaryButton>
            ) : (
              <DangerButton type="submit" disabled={saving}>
                <PowerOff className="h-3.5 w-3.5" /> {saving ? "Disabling..." : "Disable capability"}
              </DangerButton>
            )}
          </form>
        </Panel>
      )}

      <DataTable
        rows={rows}
        rowKey={(r) => r.code}
        minWidth={720}
        columns={[
          { header: "Capability", cell: (r) => <span className="font-medium text-ink-950">{humanize(r.code)}</span> },
          {
            header: "Status",
            cell: (r) => (r.capability ? <StatusPill status={r.capability.Status} tone={r.capability.Status === "ENABLED" ? "green" : "gray"} /> : <StatusPill label="Not configured" tone="gray" />),
          },
          { header: "Enabled", cell: (r) => formatWhen(r.capability?.EnabledAt) },
          { header: "Disabled", cell: (r) => formatWhen(r.capability?.DisabledAt) },
          {
            header: "Action",
            cell: (r) =>
              r.capability?.Status === "ENABLED" ? (
                <SecondaryButton onClick={() => open(r, false)}>
                  <PowerOff className="h-3 w-3" /> Disable
                </SecondaryButton>
              ) : (
                <SecondaryButton onClick={() => open(r, true)}>
                  <Power className="h-3 w-3" /> Enable
                </SecondaryButton>
              ),
          },
        ]}
      />
    </div>
  );
}
