"use client";

import React, { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  Edit3,
  Building2,
  AlertCircle,
  MapPin,
  Landmark,
  FileText,
  Mail,
  Phone,
  User,
  Lock,
  Unlock,
  ChevronLeft,
  ChevronRight,
  Hash,
  Check,
  Building,
  Loader2,
  Save,
  X,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { organizationService } from "@/lib/services/organizationService";
import { StepFooter } from "@/components/onboarding/StepFooter";
import { nextPath, previousPath } from "../steps";
import type { Organization } from "@/lib/stores/organizationStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { IconTile } from "@/components/onboarding/IconTile";


type EditEligibility = "Editable" | "Locked" | "ReviewRequired";

type CompanyProfile = {
  organizationId: string;
  legalName: string;
  tradingName: string;
  status: string;
  countryCode: string;
  timezone: string;
  defaultCurrency: string;
  version: number;
};

const CURRENCY_SYMBOL: Record<string, string> = {
  NGN: "₦",
  GHS: "₵",
  KES: "K",
  ZAR: "R",
  EGP: "E",
  MAD: "D",
  USD: "$",
  EUR: "€",
};

const COUNTRY_NAMES: Record<string, string> = {
  NG: "Nigeria",
  GH: "Ghana",
  KE: "Kenya",
  ZA: "South Africa",
  EG: "Egypt",
  MA: "Morocco",
  OTHER: "Other",
};

const FLAG_STRIPES: Record<string, [string, string, string]> = {
  NG: ["#008751", "#ffffff", "#008751"],
  GH: ["#ce1126", "#fcd116", "#006b3f"],
  KE: ["#000000", "#bb0000", "#006600"],
  ZA: ["#007847", "#ffb612", "#001489"],
  EG: ["#ce1126", "#ffffff", "#000000"],
  MA: ["#c1272d", "#c1272d", "#006233"],
};

const VERTICAL_FLAGS = new Set(["NG"]);

const COUNTRIES = [
  { value: "NG", label: "Nigeria" },
  { value: "GH", label: "Ghana" },
  { value: "KE", label: "Kenya" },
  { value: "ZA", label: "South Africa" },
  { value: "EG", label: "Egypt" },
  { value: "MA", label: "Morocco" },
  { value: "OTHER", label: "Other" },
];

const TIMEZONES = [
  { value: "Africa/Lagos", label: "Africa/Lagos (GMT+1)" },
  { value: "Africa/Accra", label: "Africa/Accra (GMT+0)" },
  { value: "Africa/Nairobi", label: "Africa/Nairobi (GMT+3)" },
  { value: "Africa/Johannesburg", label: "Africa/Johannesburg (GMT+2)" },
  { value: "Africa/Cairo", label: "Africa/Cairo (GMT+2)" },
  { value: "Africa/Casablanca", label: "Africa/Casablanca (GMT+1)" },
];

const CURRENCIES = [
  { value: "NGN", label: "NGN (₦)" },
  { value: "GHS", label: "GHS (₵)" },
  { value: "KES", label: "KES (K)" },
  { value: "ZAR", label: "ZAR (R)" },
  { value: "EGP", label: "EGP (E)" },
  { value: "MAD", label: "MAD (D)" },
  { value: "USD", label: "USD ($)" },
  { value: "EUR", label: "EUR (€)" },
];

function FlagIcon({ countryCode }: { countryCode: string }) {
  const stripes = FLAG_STRIPES[countryCode] ?? ["#cbd5e1", "#e2e8f0", "#94a3b8"];
  const vertical = VERTICAL_FLAGS.has(countryCode);
  return (
    <svg
      width="18"
      height="13"
      viewBox="0 0 18 13"
      className="flex-shrink-0 rounded-[2px] ring-1 ring-black/10"
      aria-label={`${COUNTRY_NAMES[countryCode] || countryCode} flag`}
    >
      {vertical ? (
        <>
          <rect x="0" y="0" width="6" height="13" fill={stripes[0]} />
          <rect x="6" y="0" width="6" height="13" fill={stripes[1]} />
          <rect x="12" y="0" width="6" height="13" fill={stripes[2]} />
        </>
      ) : (
        <>
          <rect x="0" y="0" width="18" height="4.34" fill={stripes[0]} />
          <rect x="0" y="4.33" width="18" height="4.34" fill={stripes[1]} />
          <rect x="0" y="8.66" width="18" height="4.34" fill={stripes[2]} />
        </>
      )}
    </svg>
  );
}

const STATUS_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  DRAFT: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  PENDING: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  ACTIVE: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  SUSPENDED: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  VERIFYING: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
  APPROVED: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  REJECTED: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  CLOSED: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const STATUS_LABEL: Record<string, string> = {
  DRAFT: "Setup in progress",
  PENDING: "Pending Review",
  ACTIVE: "Active",
  SUSPENDED: "Suspended",
  VERIFYING: "Verifying",
  APPROVED: "Approved",
  REJECTED: "Rejected",
  CLOSED: "Closed",
};

function InfoField({
  label,
  value,
  icon,
  mono = false,
}: {
  label: string;
  value: React.ReactNode;
  icon?: React.ReactNode;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <p className="text-[13px] text-ink-500">{label}</p>
      <div className="mt-1 flex items-center gap-1.5">
        {icon}
        <p className={`truncate text-sm font-semibold text-ink-950 ${mono ? "font-mono tabular-nums" : ""}`}>
          {value || "—"}
        </p>
      </div>
    </div>
  );
}

function Chip({ icon, label }: { icon: React.ReactNode; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-surface/5 px-3 py-1.5 text-xs font-medium text-white/90 backdrop-blur-sm">
      {icon}
      <span className="truncate">{label}</span>
    </span>
  );
}


function EditBadge({ eligibility }: { eligibility: EditEligibility }) {
  const styles: Record<EditEligibility, { bg: string; text: string; icon: React.ReactNode; label: string }> = {
    Editable: { bg: "bg-green-bg", text: "text-green", icon: <Unlock className="h-3.5 w-3.5" />, label: "Editable" },
    Locked: { bg: "bg-red-bg", text: "text-red", icon: <Lock className="h-3.5 w-3.5" />, label: "Locked" },
    ReviewRequired: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" />, label: "Review Required" },
  };
  const style = styles[eligibility] || styles.Locked;

  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${style.bg} ${style.text}`}>
      {style.icon}
      {style.label}
    </span>
  );
}

const inputClass = (hasError: boolean) =>
  `w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
    hasError
      ? "border-red focus:border-red focus:ring-2 focus:ring-red/10"
      : "border-field-border focus:border-label-accent focus:ring-4 focus:ring-primary/15"
  }`;

function EditableField({
  label,
  value,
  onChange,
  type = "text",
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  type?: string;
  placeholder?: string;
}) {
  return (
    <div>
      <label className="text-[13px] font-semibold text-ink-900">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className={inputClass(false)}
      />
    </div>
  );
}

function EditableSelect({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string }[];
}) {
  return (
    <div>
      <label className="text-[13px] font-semibold text-ink-900">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={inputClass(false)}
      >
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>
            {opt.label}
          </option>
        ))}
      </select>
    </div>
  );
}

export default function CompanyProfilePage() {
  const router = useRouter();
  const { _hasHydrated } = useAuthStore();
  const { setOrganization } = useOrganizationStore();
  const { setStageComplete } = useOnboardingStore();

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CompanyProfile | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isEditing, setIsEditing] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [editData, setEditData] = useState<Partial<CompanyProfile> | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [isContinuing, setIsContinuing] = useState(false);

  const fetchProfile = async () => {
    if (!_hasHydrated) return;

    setLoading(true);
    setError(null);

    try {
      const response = await organizationService.getCurrentOrganization();
      setData(response);
      setOrganization(response);
    } catch (err: any) {
      if (err?.response?.status === 401) {
        document.cookie = "karrygo_has_session=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
        document.cookie = "karrygo_org_status=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;";
        router.replace("/sign-in");
        return;
      }
      const message = err?.response?.data?.error?.message || err?.message || "Failed to load company profile.";
      setError(message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchProfile();
  }, [_hasHydrated, router]);

  const handleEdit = () => {
    if (!data) return;
    setIsEditing(true);
    setEditData({ ...data });
    setSaveError(null);
    setSaveSuccess(false);
  };

  const handleCancelEdit = () => {
    setIsEditing(false);
    setEditData(null);
    setSaveError(null);
    setSaveSuccess(false);
  };

  const handleSave = async () => {
    if (!data || !editData) return;

    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      const updated = await organizationService.updateOrganization(data.organizationId, {
        legalName: editData.legalName || data.legalName,
        tradingName: editData.tradingName || data.tradingName || "",
        // The organization API does not return the registration number in
        // this summary response; omit it instead of overwriting it with "".
        countryCode: editData.countryCode || data.countryCode,
        timezone: editData.timezone || data.timezone,
        defaultCurrency: editData.defaultCurrency || data.defaultCurrency,
      }, data.version);

      setData(updated);
      setOrganization(updated);
      setSaveSuccess(true);
      setIsEditing(false);
      setEditData(null);

      setTimeout(() => {
        setSaveSuccess(false);
      }, 3000);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to update company.";
      setSaveError(message);
    } finally {
      setIsSaving(false);
    }
  };

  const handleContinue = async () => {
    if (!data) return;

    setIsContinuing(true);

    try {
      // Mark BUSINESS_PROFILE as complete
      await organizationService.markStageComplete(data.organizationId, 'BUSINESS_PROFILE');
      setStageComplete('BUSINESS_PROFILE');
    } catch (error) {
      console.error('Failed to mark stage complete:', error);
      // Still navigate even if marking fails
    } finally {
      setIsContinuing(false);
      router.push(nextPath("company"));
    }
  };

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
        <div className="space-y-2">
          <div className="h-7 w-40 animate-pulse rounded bg-surface-2 sm:w-48" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="mx-auto w-full max-w-5xl">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-red/20 bg-red-bg/30 px-4 py-16 text-center">
          <div className="rounded-full bg-red-bg p-4">
            <AlertCircle className="h-8 w-8 text-red" />
          </div>
          <h3 className="mt-4 text-[15px] font-semibold text-ink-950">Unable to load profile</h3>
          <p className="mt-1.5 text-sm text-ink-500">{error}</p>
          <button
            onClick={() => router.push("/setup/create-company")}
            className="btn btn-primary btn-md mt-5"
          >
            Go to Create Company
          </button>
        </div>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="mx-auto w-full max-w-5xl">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line px-4 py-16 text-center">
          <Building2 className="h-8 w-8 text-ink-400" />
          <h3 className="mt-3 text-sm font-semibold text-ink-950">No company found</h3>
          <p className="mt-1 text-sm text-ink-500">Create your company to get started.</p>
          <button
            onClick={() => router.push("/setup/create-company")}
            className="btn btn-primary btn-md mt-4"
          >
            Create Company
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.DRAFT;
  const statusLabel = STATUS_LABEL[data.status] || data.status;
  const editEligibility: EditEligibility = data.status === "DRAFT" ? "Editable" : "Locked";
  const isEditable = data.status === "DRAFT";

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h1 className="text-ink-950">Company details</h1>
          <p className="mt-1.5 text-ink-600">
            {isEditing
              ? "Update your company details, then save."
              : "Check these details are right. You can change them until you submit for review."}
          </p>
        </div>
        {!isEditing && isEditable && (
          <button onClick={handleEdit} className="btn btn-secondary btn-md self-start">
            <Edit3 />
            Edit details
          </button>
        )}
      </div>

      {/* Company banner: the company as it stands, from the saved record. */}
      <div className="relative overflow-hidden rounded-2xl bg-[#0B1F44] px-6 py-7 text-white ring-1 ring-white/5 sm:px-8">
        <div className="pointer-events-none absolute inset-0" aria-hidden="true">
          <div className="absolute -right-8 -top-16 h-56 w-56 rounded-full bg-white/10 blur-3xl" />
          <div className="absolute -bottom-16 right-16 h-44 w-72 rounded-full bg-[#00C2A8]/10 blur-3xl" />
          <svg className="absolute inset-0 h-full w-full opacity-70" preserveAspectRatio="none" viewBox="0 0 800 200">
            <path d="M0,140 C120,180 260,60 400,110 C540,160 660,70 800,120 L800,200 L0,200 Z" fill="white" fillOpacity="0.08" />
            <path d="M0,165 C160,105 320,195 480,135 C600,95 700,155 800,135 L800,200 L0,200 Z" fill="white" fillOpacity="0.12" />
          </svg>
        </div>
        <div className="relative flex items-start gap-4 sm:gap-5">
          <div className="flex h-14 w-14 flex-shrink-0 items-center justify-center rounded-2xl bg-white/10 text-[#64DBC9] ring-1 ring-white/10">
            <Building className="h-7 w-7" strokeWidth={1.5} />
          </div>
          <div className="min-w-0">
            <h2 className="truncate text-xl font-bold tracking-[-0.01em] text-white sm:text-2xl">{data.legalName}</h2>
            {data.tradingName && data.tradingName !== data.legalName && (
              <p className="mt-0.5 truncate text-sm text-white/70">Trading as {data.tradingName}</p>
            )}
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <span
                className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
              >
                {statusStyle.icon}
                {statusLabel}
              </span>
              <span className="flex items-center gap-1 text-xs text-white/70">
                <MapPin className="h-3.5 w-3.5" />
                {COUNTRY_NAMES[data.countryCode] || data.countryCode}
              </span>
            </div>
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-surface p-5 shadow-[0_1px_2px_rgba(11,31,68,0.05)] sm:p-6">
        <div className="grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2">
          {isEditing ? (
            <>
              <div className="sm:col-span-2">
                <EditableField
                  label="Legal name"
                  value={editData?.legalName || ""}
                  onChange={(val) => setEditData({ ...editData!, legalName: val })}
                  placeholder="e.g. Apex Logistics Limited"
                />
              </div>
              <EditableField
                label="Trading name"
                value={editData?.tradingName || ""}
                onChange={(val) => setEditData({ ...editData!, tradingName: val })}
                placeholder="e.g. Apex Logistics"
              />
              <EditableSelect
                label="Country"
                value={editData?.countryCode || "NG"}
                onChange={(val) => setEditData({ ...editData!, countryCode: val })}
                options={COUNTRIES}
              />
              <EditableSelect
                label="Timezone"
                value={editData?.timezone || "Africa/Lagos"}
                onChange={(val) => setEditData({ ...editData!, timezone: val })}
                options={TIMEZONES}
              />
              <EditableSelect
                label="Currency"
                value={editData?.defaultCurrency || "NGN"}
                onChange={(val) => setEditData({ ...editData!, defaultCurrency: val })}
                options={CURRENCIES}
              />
            </>
          ) : (
            <>
              <InfoField label="Legal name" value={data.legalName} />
              <InfoField label="Trading name" value={data.tradingName || "Not provided"} />
              <InfoField
                label="Country"
                value={COUNTRY_NAMES[data.countryCode] || data.countryCode}
                icon={<FlagIcon countryCode={data.countryCode} />}
              />
              <InfoField label="Timezone" value={data.timezone} />
              <InfoField label="Currency" value={data.defaultCurrency} />
            </>
          )}
        </div>

        {isEditing && (
          <div className="mt-6 flex flex-col gap-3 border-t border-line pt-5 sm:flex-row sm:items-center sm:justify-end">
            {saveError && (
              <p className="flex items-center gap-2 text-sm text-red sm:mr-auto">
                <AlertCircle className="h-4 w-4" />
                {saveError}
              </p>
            )}
            <button onClick={handleCancelEdit} disabled={isSaving} className="btn btn-secondary btn-md">
              Cancel
            </button>
            <button onClick={handleSave} disabled={isSaving} className="btn btn-primary btn-md">
              {isSaving ? "Saving…" : "Save changes"}
            </button>
          </div>
        )}
        {saveSuccess && !isEditing && (
          <p className="mt-5 flex items-center gap-2 border-t border-line pt-5 text-sm text-label-accent">
            <CheckCircle2 className="h-4 w-4" />
            Your changes are saved.
          </p>
        )}
      </div>

      <StepFooter
        onBack={() => router.push(previousPath("company"))}
        onPrimary={handleContinue}
        primaryLabel="Continue"
        primaryDisabled={isEditing}
        primaryLoading={isContinuing}
        loadingLabel="Opening next step…"
      />
    </div>
  );
}
