"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { Archive, ArrowLeft, Edit3, Pause, Play, Save } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { serviceCatalogService } from "@/lib/services/operationsService";
import type { ServiceCatalogItem } from "@/lib/types/operations";

type Command = "activate" | "deactivate" | "archive";

const splitList = (value: string) => value.split(",").map((v) => v.trim()).filter(Boolean);

const COMMAND_DONE: Record<Command, string> = {
  activate: "Service activated.",
  deactivate: "Service deactivated.",
  archive: "Service archived.",
};

export default function ServiceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const serviceId = params.serviceId as string;

  const [item, setItem] = useState<ServiceCatalogItem | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  const [editing, setEditing] = useState(false);
  const [edit, setEdit] = useState({
    name: "",
    description: "",
    capabilityReference: "",
    vehicleClasses: "",
    packageTypes: "",
    pricingPolicyReference: "",
    approvalPolicyReference: "",
    proofPolicyReference: "",
  });
  const [saving, setSaving] = useState(false);
  const [editError, setEditError] = useState<ApiErrorInfo | null>(null);

  // One key per command attempt, kept across retries until it succeeds.
  const [commandKeys, setCommandKeys] = useState<Record<Command, string>>(() => ({
    activate: newIdempotencyKey(),
    deactivate: newIdempotencyKey(),
    archive: newIdempotencyKey(),
  }));
  const [busy, setBusy] = useState<Command | null>(null);
  const [commandError, setCommandError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      setItem(await serviceCatalogService.get(serviceId));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "The service could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [serviceId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const startEdit = (current: ServiceCatalogItem) => {
    setEdit({
      name: current.name,
      description: current.description ?? "",
      capabilityReference: current.capabilityReference ?? "",
      vehicleClasses: (current.supportedVehicleClasses ?? []).join(", "),
      packageTypes: (current.supportedPackageTypes ?? []).join(", "),
      pricingPolicyReference: current.pricingPolicyReference ?? "",
      approvalPolicyReference: current.approvalPolicyReference ?? "",
      proofPolicyReference: current.proofPolicyReference ?? "",
    });
    setEditError(null);
    setEditing(true);
  };

  const saveEdit = async (e: FormEvent) => {
    e.preventDefault();
    if (!item || item.version == null) return;
    setSaving(true);
    setEditError(null);
    try {
      const updated = await serviceCatalogService.update(
        item.id,
        {
          name: edit.name.trim(),
          description: edit.description.trim(),
          capabilityReference: edit.capabilityReference.trim(),
          // The required-fields schema is kept exactly as stored.
          requiredFieldsSchema: item.requiredFieldsSchema ?? {},
          supportedVehicleClasses: splitList(edit.vehicleClasses),
          supportedPackageTypes: splitList(edit.packageTypes),
          pricingPolicyReference: edit.pricingPolicyReference.trim(),
          approvalPolicyReference: edit.approvalPolicyReference.trim(),
          proofPolicyReference: edit.proofPolicyReference.trim(),
        },
        item.version,
      );
      setItem(updated);
      setEditing(false);
      setNotice("Service updated.");
    } catch (err) {
      setEditError(describeApiError(err, "The service could not be saved."));
    } finally {
      setSaving(false);
    }
  };

  const run = async (command: Command) => {
    if (!item || item.version == null) return;
    setBusy(command);
    setCommandError(null);
    try {
      const updated = await serviceCatalogService.command(item.id, command, item.version, commandKeys[command]);
      setItem(updated);
      setCommandKeys((k) => ({ ...k, [command]: newIdempotencyKey() }));
      setNotice(COMMAND_DONE[command]);
    } catch (err) {
      setCommandError(describeApiError(err, "The service could not be updated."));
    } finally {
      setBusy(null);
    }
  };

  if (loading) return <Loading label="Loading service..." />;
  if (!item) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <Link href="/private-ops/services" className="text-sm text-primary">Back to services</Link>
      </div>
    );
  }

  const archived = item.status === "ARCHIVED";
  const schemaFields = Object.entries(item.requiredFieldsSchema ?? {});

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={item.name}
        description={`${item.code ?? ""} · ${humanize(item.category)}`}
        actions={
          <>
            {!archived && !editing && (
              <SecondaryButton onClick={() => startEdit(item)}>
                <Edit3 className="h-3.5 w-3.5" /> Edit service
              </SecondaryButton>
            )}
            <SecondaryButton onClick={() => router.push("/private-ops/services")}>
              <ArrowLeft className="h-3.5 w-3.5" /> Back
            </SecondaryButton>
          </>
        }
      />

      <ApiErrorNotice error={error} />
      <Notice message={notice} />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={item.status} />],
          ["Code", item.code ?? "—"],
          ["Category", humanize(item.category)],
          ["Branch", item.branchId || "All branches"],
          ["Capability reference", item.capabilityReference || "—"],
          ["Vehicle classes", (item.supportedVehicleClasses ?? []).join(", ") || "—"],
          ["Package types", (item.supportedPackageTypes ?? []).join(", ") || "—"],
          ["Pricing policy", item.pricingPolicyReference || "—"],
          ["Approval policy", item.approvalPolicyReference || "—"],
          ["Proof policy", item.proofPolicyReference || "—"],
          ["Updated", formatWhen(item.updatedAt)],
          ["Version", item.version ?? "—"],
        ]}
      />
      {item.description && <p className="rounded-xl border border-line bg-white p-4 text-sm text-ink-700">{item.description}</p>}

      {!archived && (
        <Panel title="Lifecycle" description="Only active services can be used on new orders.">
          <ApiErrorNotice error={commandError} />
          <div className="flex flex-wrap gap-2">
            {(item.status === "DRAFT" || item.status === "INACTIVE") && (
              <PrimaryButton onClick={() => run("activate")} disabled={busy !== null}>
                <Play className="h-3.5 w-3.5" /> {busy === "activate" ? "Activating..." : "Activate"}
              </PrimaryButton>
            )}
            {item.status === "ACTIVE" && (
              <SecondaryButton onClick={() => run("deactivate")} disabled={busy !== null}>
                <Pause className="h-3.5 w-3.5" /> {busy === "deactivate" ? "Deactivating..." : "Deactivate"}
              </SecondaryButton>
            )}
            <DangerButton onClick={() => run("archive")} disabled={busy !== null}>
              <Archive className="h-3.5 w-3.5" /> {busy === "archive" ? "Archiving..." : "Archive"}
            </DangerButton>
          </div>
        </Panel>
      )}

      {editing && (
        <Panel title="Edit service" onClose={() => setEditing(false)}>
          <form onSubmit={saveEdit} className="space-y-4">
            <ApiErrorNotice error={editError} />
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <Field label="Name" required>
                <input required value={edit.name} onChange={(e) => setEdit({ ...edit, name: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Capability reference">
                <input value={edit.capabilityReference} onChange={(e) => setEdit({ ...edit, capabilityReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Supported vehicle classes" hint="Comma separated.">
                <input value={edit.vehicleClasses} onChange={(e) => setEdit({ ...edit, vehicleClasses: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Supported package types" hint="Comma separated.">
                <input value={edit.packageTypes} onChange={(e) => setEdit({ ...edit, packageTypes: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Pricing policy reference">
                <input value={edit.pricingPolicyReference} onChange={(e) => setEdit({ ...edit, pricingPolicyReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Approval policy reference">
                <input value={edit.approvalPolicyReference} onChange={(e) => setEdit({ ...edit, approvalPolicyReference: e.target.value })} className={INPUT} />
              </Field>
              <Field label="Proof policy reference">
                <input value={edit.proofPolicyReference} onChange={(e) => setEdit({ ...edit, proofPolicyReference: e.target.value })} className={INPUT} />
              </Field>
              <div className="sm:col-span-2">
                <Field label="Description">
                  <textarea rows={3} value={edit.description} onChange={(e) => setEdit({ ...edit, description: e.target.value })} className={INPUT} />
                </Field>
              </div>
            </div>
            <PrimaryButton type="submit" disabled={saving}>
              <Save className="h-3.5 w-3.5" /> {saving ? "Saving..." : "Save service"}
            </PrimaryButton>
          </form>
        </Panel>
      )}

      <Panel title="Required order fields" description="Fields an order for this service must supply.">
        {schemaFields.length === 0 ? (
          <p className="text-sm text-ink-500">This service requires no extra fields.</p>
        ) : (
          <KeyValueGrid
            items={schemaFields.map(([name, descriptor]) => {
              const d = (descriptor ?? {}) as { type?: string; required?: boolean };
              return [name, `${humanize(d.type)}${d.required ? " · required" : ""}`] as [string, string];
            })}
          />
        )}
      </Panel>
    </div>
  );
}
