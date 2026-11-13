// src/app/(dashboard)/providers/[membershipId]/work-setup/page.tsx

"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  Truck,
  Package,
  MapPin,
  CheckCircle2,
  Edit3,
  Plus,
  X,
  Save,
  Briefcase,
  Shield,
  Lock,
} from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { providerService } from "@/lib/services/providerService";
import { structureService } from "@/lib/services/structureService";
import { serviceCategoryLabel, formatDate } from "@/lib/utils/providerLabels";
import type { ProviderCapability, ProviderMembership, ProviderPolicy } from "@/lib/types/provider";
import type { Branch } from "@/lib/types/structure";

const ASSIGNMENT_MODE_LABEL: Record<string, string> = {
  OFFER: "Offer, provider accepts or declines",
  BROADCAST: "Broadcast to eligible providers",
  DIRECTED_OFFER: "Directed offer to this provider",
};

const VISIBILITY_LABEL: Record<string, string> = {
  ORGANIZATION: "Visible to the whole organization",
  BRANCH: "Visible to the home branch only",
};

type CapabilityRow = {
  serviceType: string;
  vehicleClasses: string;
  operatingZones: string;
  qualifications: string;
  validUntil: string;
};

type PolicyForm = {
  assignmentMode: string;
  visibilityPolicy: string;
  shiftRestrictions: string;
  zoneRestrictions: string;
  workloadRestrictions: string;
};

const splitList = (value: string) =>
  value
    .split(",")
    .map((v) => v.trim())
    .filter(Boolean);

const toCapabilityRow = (c: ProviderCapability): CapabilityRow => ({
  serviceType: c.serviceType,
  vehicleClasses: (c.vehicleClasses ?? []).join(", "),
  operatingZones: (c.operatingZones ?? []).join(", "),
  qualifications: (c.qualifications ?? []).join(", "),
  validUntil: c.validUntil ? c.validUntil.slice(0, 10) : "",
});

const toJSON = (value: Record<string, unknown> | null | undefined) =>
  value && Object.keys(value).length > 0 ? JSON.stringify(value, null, 2) : "";

const toPolicyForm = (p: ProviderPolicy | null): PolicyForm => ({
  assignmentMode: p?.assignmentMode || "OFFER",
  visibilityPolicy: p?.visibilityPolicy || "ORGANIZATION",
  shiftRestrictions: toJSON(p?.shiftRestrictions),
  zoneRestrictions: toJSON(p?.zoneRestrictions),
  workloadRestrictions: toJSON(p?.workloadRestrictions),
});

const parseRestrictions = (label: string, raw: string): Record<string, unknown> => {
  if (raw.trim() === "") return {};
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    throw new Error(`${label} must be valid JSON.`);
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error(`${label} must be a JSON object.`);
  }
  return parsed as Record<string, unknown>;
};

const titleCase = (value: string) =>
  value.replace(/[_.]+/g, " ").toLowerCase().replace(/\b\w/g, (m) => m.toUpperCase()).trim();

function RestrictionList({ value }: { value: Record<string, unknown> | null | undefined }) {
  const entries = Object.entries(value ?? {});
  if (entries.length === 0) return <span className="text-ink-400">None</span>;
  return (
    <span>
      {entries
        .map(([k, v]) => `${titleCase(k)}: ${typeof v === "object" ? JSON.stringify(v) : String(v)}`)
        .join(" · ")}
    </span>
  );
}

export default function WorkSetupTab() {
  const params = useParams();
  const membershipId = params.membershipId as string;

  const [loading, setLoading] = useState(true);
  const [membership, setMembership] = useState<ProviderMembership | null>(null);
  const [capabilities, setCapabilities] = useState<ProviderCapability[]>([]);
  const [policy, setPolicy] = useState<ProviderPolicy | null>(null);
  const [branches, setBranches] = useState<Branch[]>([]);
  const [loadError, setLoadError] = useState<ApiErrorInfo | null>(null);

  const [isEditing, setIsEditing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [capabilityRows, setCapabilityRows] = useState<CapabilityRow[]>([]);
  const [policyForm, setPolicyForm] = useState<PolicyForm>(toPolicyForm(null));

  const load = useCallback(async () => {
    try {
      const [m, caps, pol, allBranches] = await Promise.all([
        providerService.get(membershipId),
        providerService.capabilities(membershipId),
        providerService.policy(membershipId),
        structureService.listBranches().catch(() => [] as Branch[]),
      ]);
      setMembership(m);
      setCapabilities(caps);
      setPolicy(pol);
      setBranches(allBranches);
      setLoadError(null);
    } catch (e) {
      setLoadError(describeApiError(e, "The provider's work setup could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [membershipId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  const startEditing = () => {
    setCapabilityRows(capabilities.map(toCapabilityRow));
    setPolicyForm(toPolicyForm(policy));
    setSaveError(null);
    setNotice(null);
    setIsEditing(true);
  };

  const updateRow = (index: number, patch: Partial<CapabilityRow>) =>
    setCapabilityRows(capabilityRows.map((row, i) => (i === index ? { ...row, ...patch } : row)));

  const save = async () => {
    setSaveError(null);
    let restrictions;
    try {
      restrictions = {
        shiftRestrictions: parseRestrictions("Shift restrictions", policyForm.shiftRestrictions),
        zoneRestrictions: parseRestrictions("Zone restrictions", policyForm.zoneRestrictions),
        workloadRestrictions: parseRestrictions("Workload restrictions", policyForm.workloadRestrictions),
      };
    } catch (e) {
      setSaveError({ message: (e as Error).message });
      return;
    }
    setSaving(true);
    try {
      const [caps, pol] = await Promise.all([
        providerService.replaceCapabilities(
          membershipId,
          capabilityRows.map((row) => ({
            serviceType: row.serviceType,
            vehicleClasses: splitList(row.vehicleClasses),
            operatingZones: splitList(row.operatingZones),
            qualifications: splitList(row.qualifications),
            validUntil: row.validUntil ? new Date(`${row.validUntil}T23:59:59Z`).toISOString() : null,
          })),
        ),
        providerService.replacePolicy(membershipId, {
          assignmentMode: policyForm.assignmentMode,
          visibilityPolicy: policyForm.visibilityPolicy,
          ...restrictions,
        }),
      ]);
      setCapabilities(caps);
      setPolicy(pol);
      setIsEditing(false);
      setNotice("Work setup saved.");
    } catch (e) {
      setSaveError(describeApiError(e, "The work setup could not be saved."));
      // One of the two may have saved; show what the server now holds.
      await load();
    } finally {
      setSaving(false);
    }
  };

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-12 text-center text-sm text-ink-500">
        Loading work setup…
      </div>
    );
  }

  if (!membership) {
    return <ApiErrorNotice error={loadError ?? { message: "Provider not found." }} />;
  }

  const serviceType = membership.serviceCategory;
  const homeBranch = membership.homeBranchId
    ? branches.find((b) => b.id === membership.homeBranchId)
    : undefined;
  const vehicleClasses = [...new Set(capabilities.flatMap((c) => c.vehicleClasses ?? []))];
  const zones = [...new Set(capabilities.flatMap((c) => c.operatingZones ?? []))];

  return (
    <div className="space-y-5">
      {/* ── Actions ── */}
      <div className="flex justify-end gap-2">
        {isEditing && (
          <button
            onClick={() => setIsEditing(false)}
            disabled={saving}
            className="flex items-center gap-2 rounded-lg border border-line bg-white px-4 py-2.5 text-sm font-semibold text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </button>
        )}
        <button
          onClick={isEditing ? save : startEditing}
          disabled={saving}
          className={`flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm font-semibold transition-colors disabled:opacity-60 ${
            isEditing
              ? "bg-primary text-white hover:bg-primary-hover"
              : "border border-line bg-white text-ink-700 hover:bg-surface-2"
          }`}
        >
          {isEditing ? (
            <>
              <Save className="h-4 w-4" />
              {saving ? "Saving…" : "Save Changes"}
            </>
          ) : (
            <>
              <Edit3 className="h-4 w-4" />
              Edit Setup
            </>
          )}
        </button>
      </div>

      <ApiErrorNotice error={saveError ?? loadError} />
      {notice && (
        <div className="rounded-lg border border-green/30 bg-green-bg px-4 py-2 text-sm text-green">{notice}</div>
      )}

      {/* ── Grid ── */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Capabilities */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Briefcase className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Service Capabilities</h3>
          </div>
          <div className="space-y-2.5">
            {!isEditing && capabilities.length === 0 && (
              <p className="text-sm text-ink-500">No capabilities recorded for this provider yet.</p>
            )}
            {!isEditing &&
              capabilities.map((c, index) => (
                <div
                  key={c.id ?? index}
                  className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-4 py-3"
                >
                  <div className="flex items-center gap-3 min-w-0">
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-primary-bg text-primary">
                      <Package className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-ink-950">{serviceCategoryLabel(c.serviceType)}</div>
                      <div className="truncate text-xs text-ink-500">
                        {c.qualifications?.length ? c.qualifications.join(", ") : "No qualifications listed"}
                        {c.validUntil ? ` · until ${formatDate(c.validUntil)}` : ""}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                      c.status === "ACTIVE" ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                    }`}
                  >
                    {c.status ? titleCase(c.status) : "—"}
                  </span>
                </div>
              ))}
            {isEditing &&
              capabilityRows.map((row, index) => (
                <div key={index} className="space-y-3 rounded-lg border border-line bg-surface-2/50 p-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm font-semibold text-ink-950">{serviceCategoryLabel(row.serviceType)}</span>
                    <button
                      type="button"
                      title="Remove"
                      onClick={() => setCapabilityRows(capabilityRows.filter((_, i) => i !== index))}
                      className="text-ink-400 transition-colors hover:text-red"
                    >
                      <X className="h-4 w-4" />
                    </button>
                  </div>
                  <Field label="Vehicle classes" hint="Separate with commas.">
                    <input value={row.vehicleClasses} onChange={(e) => updateRow(index, { vehicleClasses: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Operating zones" hint="Separate with commas.">
                    <input value={row.operatingZones} onChange={(e) => updateRow(index, { operatingZones: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Qualifications" hint="Separate with commas.">
                    <input value={row.qualifications} onChange={(e) => updateRow(index, { qualifications: e.target.value })} className={INPUT} />
                  </Field>
                  <Field label="Valid until">
                    <input type="date" value={row.validUntil} onChange={(e) => updateRow(index, { validUntil: e.target.value })} className={INPUT} />
                  </Field>
                </div>
              ))}
            {isEditing && (
              <button
                type="button"
                onClick={() =>
                  setCapabilityRows([
                    ...capabilityRows,
                    { serviceType, vehicleClasses: "", operatingZones: "", qualifications: "", validUntil: "" },
                  ])
                }
                className="flex w-full items-center justify-center gap-2 rounded-lg border-2 border-dashed border-line py-3 text-xs font-medium text-ink-500 transition-colors hover:border-primary hover:text-primary"
              >
                <Plus className="h-4 w-4" />
                Add capability
              </button>
            )}
          </div>
        </div>

        {/* Branch Assignment */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <MapPin className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Branch Assignment</h3>
          </div>
          <div className="space-y-2.5">
            <div className="flex items-center justify-between rounded-lg border border-line bg-surface-2/50 px-4 py-3">
              <div>
                <div className="text-sm font-semibold text-ink-950">
                  {homeBranch?.name ?? membership.homeBranchId ?? "No home branch"}
                </div>
                <div className="text-xs text-ink-500">Home branch</div>
              </div>
              {homeBranch && (
                <span
                  className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                    homeBranch.status === "ACTIVE" ? "bg-green-bg text-green" : "bg-surface-2 text-ink-500"
                  }`}
                >
                  {titleCase(homeBranch.status)}
                </span>
              )}
            </div>
            <Link
              href={`/providers/${encodeURIComponent(membershipId)}`}
              className="inline-block text-xs font-medium text-primary transition-colors hover:text-primary-hover"
            >
              Change on the overview →
            </Link>
          </div>
        </div>

        {/* Vehicle Classes */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Truck className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Vehicle Classes and Zones</h3>
          </div>
          {vehicleClasses.length === 0 && zones.length === 0 ? (
            <p className="text-sm text-ink-500">None recorded. They are set with each capability.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {vehicleClasses.map((v) => (
                <span key={`v-${v}`} className="rounded-full bg-green-bg px-3 py-1.5 text-xs font-semibold text-green">
                  {v}
                </span>
              ))}
              {zones.map((z) => (
                <span key={`z-${z}`} className="flex items-center gap-1 rounded-full bg-surface-2 px-3 py-1.5 text-xs font-medium text-ink-600">
                  <MapPin className="h-3 w-3" />
                  {z}
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Work Policy */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-center gap-2.5 mb-4">
            <Shield className="h-5 w-5 text-primary" />
            <h3 className="text-sm font-semibold text-ink-950">Work Policy</h3>
          </div>
          <div className="space-y-3">
            {/* A platform invariant, never editable. */}
            <div className="flex items-center gap-3 text-sm">
              <Lock className="h-4 w-4 text-ink-400" />
              <span className="text-ink-700">The provider can always decline company work</span>
            </div>
            {!isEditing ? (
              <>
                <div className="flex items-center gap-3 text-sm">
                  <CheckCircle2 className="h-4 w-4 text-green" />
                  <span className="text-ink-700">
                    {ASSIGNMENT_MODE_LABEL[policy?.assignmentMode ?? ""] ?? policy?.assignmentMode ?? "—"}
                  </span>
                </div>
                <div className="flex items-center gap-3 text-sm">
                  <CheckCircle2 className="h-4 w-4 text-green" />
                  <span className="text-ink-700">
                    {VISIBILITY_LABEL[policy?.visibilityPolicy ?? ""] ?? policy?.visibilityPolicy ?? "—"}
                  </span>
                </div>
                <div className="text-xs text-ink-500">
                  <div>Shift: <RestrictionList value={policy?.shiftRestrictions} /></div>
                  <div>Zones: <RestrictionList value={policy?.zoneRestrictions} /></div>
                  <div>Workload: <RestrictionList value={policy?.workloadRestrictions} /></div>
                </div>
              </>
            ) : (
              <div className="space-y-3">
                <Field label="Assignment mode">
                  <select
                    value={policyForm.assignmentMode}
                    onChange={(e) => setPolicyForm({ ...policyForm, assignmentMode: e.target.value })}
                    className={INPUT}
                  >
                    {Object.entries(ASSIGNMENT_MODE_LABEL).map(([value, label]) => (
                      <option key={value} value={value}>{label}</option>
                    ))}
                  </select>
                </Field>
                <Field label="Visibility">
                  <select
                    value={policyForm.visibilityPolicy}
                    onChange={(e) => setPolicyForm({ ...policyForm, visibilityPolicy: e.target.value })}
                    className={INPUT}
                  >
                    {Object.entries(VISIBILITY_LABEL).map(([value, label]) => (
                      <option key={value} value={value}>{label}</option>
                    ))}
                  </select>
                </Field>
                <Field label="Shift restrictions" hint="A JSON object, or leave blank for none.">
                  <textarea
                    rows={2}
                    value={policyForm.shiftRestrictions}
                    onChange={(e) => setPolicyForm({ ...policyForm, shiftRestrictions: e.target.value })}
                    className={`${INPUT} font-mono text-xs`}
                  />
                </Field>
                <Field label="Zone restrictions" hint="A JSON object, or leave blank for none.">
                  <textarea
                    rows={2}
                    value={policyForm.zoneRestrictions}
                    onChange={(e) => setPolicyForm({ ...policyForm, zoneRestrictions: e.target.value })}
                    className={`${INPUT} font-mono text-xs`}
                  />
                </Field>
                <Field label="Workload restrictions" hint="A JSON object, or leave blank for none.">
                  <textarea
                    rows={2}
                    value={policyForm.workloadRestrictions}
                    onChange={(e) => setPolicyForm({ ...policyForm, workloadRestrictions: e.target.value })}
                    className={`${INPUT} font-mono text-xs`}
                  />
                </Field>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
