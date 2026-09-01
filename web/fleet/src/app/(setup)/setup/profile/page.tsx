// src/app/(setup)/setup/profile/page.tsx

"use client";

import { useState, useEffect } from "react";
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
  BadgeCheck,
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
} from "lucide-react";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { useAuthStore } from "@/lib/stores/authStore";
import { organizationService } from "@/lib/services/organizationService";

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
  DRAFT: "Draft",
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
      <p className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">{label}</p>
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
    <span className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-xs font-medium text-white/90 backdrop-blur-sm">
      {icon}
      <span className="truncate">{label}</span>
    </span>
  );
}

function IconTile({
  icon,
  theme = "brand",
}: {
  icon: React.ReactNode;
  theme?: "brand" | "accent";
}) {
  const gradient = theme === "brand" ? "from-primary to-accent" : "from-accent to-accent-hover";
  return (
    <div
      className={`relative flex h-11 w-11 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br text-white ring-1 ring-white/20 ${gradient}`}
      style={{
        boxShadow:
          "inset 0 1.5px 0 rgba(255,255,255,0.6), inset 0 -9px 14px rgba(0,0,0,0.3), 0 10px 18px -6px rgba(0,0,0,0.4)",
      }}
    >
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background: "radial-gradient(130% 110% at 25% 12%, rgba(255,255,255,0.55), transparent 55%)",
          mixBlendMode: "overlay",
        }}
      />
      <span
        className="relative [&>svg]:h-6 [&>svg]:w-6"
        style={{ filter: "drop-shadow(0 1.5px 1.5px rgba(0,0,0,0.35)) drop-shadow(0 -1px 0 rgba(255,255,255,0.25))" }}
      >
        {React.isValidElement(icon)
          ? React.cloneElement(icon as React.ReactElement<{ strokeWidth?: number }>, { strokeWidth: 2.25 })
          : icon}
      </span>
    </div>
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

export default function CompanyProfilePage() {
  const router = useRouter();
  const { organizationId } = useOnboardingStore();
  const { accessToken, _hasHydrated } = useAuthStore();

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<CompanyProfile | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    async function fetchProfile() {
      if (!_hasHydrated) {
        return;
      }

      if (!organizationId) {
        setError("No organization found. Please create a company first.");
        setLoading(false);
        return;
      }

      setLoading(true);
      setError(null);

      try {
        const response = await organizationService.getCurrentOrganization();
        setData(response.data);
      } catch (err: any) {
        const message = err?.response?.data?.error?.message || err?.message || "Failed to load company profile.";
        setError(message);
      } finally {
        setLoading(false);
      }
    }

    fetchProfile();
  }, [organizationId, _hasHydrated]);

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
            className="mt-5 rounded-lg bg-primary px-5 py-2 text-[13px] font-semibold text-white transition hover:bg-primary/90"
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
            className="mt-4 rounded-lg bg-primary px-4 py-1.5 text-sm font-medium text-white hover:bg-primary-hover"
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

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <p className="text-xs font-medium text-ink-500">
            Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
            <span className="text-ink-700">Company Profile</span>
          </p>
          <h1 className="mt-1 text-ink-950">Company Profile</h1>
          <p className="mt-1.5 text-ink-600">
            Review the company details submitted during onboarding.
          </p>
        </div>
      </div>

      {/* Hero Card */}
      <div className="relative overflow-hidden rounded-xl bg-gradient-to-br from-[#2D2A8C] via-[#3B2FA0] to-[#4C3CC4] px-6 sm:px-8 py-7 text-white">
        <div className="pointer-events-none absolute inset-0">
          <div className="absolute -right-8 -top-16 h-56 w-56 rounded-full bg-white/10 blur-3xl" />
          <div className="absolute right-16 -bottom-16 h-44 w-72 rounded-full bg-indigo-300/10 blur-3xl" />
          <svg className="absolute inset-0 h-full w-full opacity-70" preserveAspectRatio="none" viewBox="0 0 800 200">
            <path d="M0,140 C120,180 260,60 400,110 C540,160 660,70 800,120 L800,200 L0,200 Z" fill="white" fillOpacity="0.08" />
            <path d="M0,165 C160,105 320,195 480,135 C600,95 700,155 800,135 L800,200 L0,200 Z" fill="white" fillOpacity="0.12" />
          </svg>
        </div>

        <div className="relative flex flex-col lg:flex-row lg:items-start lg:justify-between gap-6">
          <div className="flex min-w-0 items-start gap-4 sm:gap-5">
            <div
              className="relative flex h-16 w-16 flex-shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-primary to-accent text-white ring-1 ring-white/20"
              style={{
                boxShadow:
                  "inset 0 1.5px 0 rgba(255,255,255,0.6), inset 0 -9px 14px rgba(0,0,0,0.3), 0 10px 18px -6px rgba(0,0,0,0.5)",
              }}
            >
              <span
                aria-hidden
                className="pointer-events-none absolute inset-0"
                style={{
                  background: "radial-gradient(130% 110% at 25% 12%, rgba(255,255,255,0.55), transparent 55%)",
                  mixBlendMode: "overlay",
                }}
              />
              <Building className="relative h-8 w-8" strokeWidth={1.5} />
            </div>

            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="truncate text-xl font-bold tracking-[-0.01em] text-white sm:text-2xl">
                  {data.legalName}
                </h2>
                <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-white/10 ring-4 ring-green-500/30 shadow-[inset_0_1px_1px_rgba(255,255,255,0.4),0_2px_6px_rgba(0,0,0,0.25)]">
                  <Check className="h-[14px] w-[14px] text-green-500" strokeWidth={3} />
                </span>
              </div>

              <div className="mt-1.5 flex flex-wrap items-center gap-2">
                <span
                  className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
                >
                  {statusStyle.icon}
                  {statusLabel}
                </span>
                <span className="flex items-center gap-1 text-xs text-white/60">
                  <MapPin className="h-3.5 w-3.5" />
                  {COUNTRY_NAMES[data.countryCode] || data.countryCode}
                </span>
              </div>

              <div className="mt-4 flex flex-wrap gap-2">
                <Chip icon={<Landmark className="h-3.5 w-3.5" />} label={data.organizationId.slice(0, 16) + "..."} />
              </div>
            </div>
          </div>

          <div className="flex w-full flex-shrink-0 gap-6 border-t border-white/10 pt-5 lg:w-auto lg:border-t-0 lg:pt-0">
            <div>
              <p className="text-xs uppercase tracking-[0.06em] text-white/50">Version</p>
              <p className="mt-1 font-mono text-lg font-semibold tabular-nums text-white">v{data.version}</p>
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.06em] text-white/50">Edit Status</p>
              <EditBadge eligibility={editEligibility} />
            </div>
          </div>
        </div>
      </div>

      {/* Info Grid */}
      <div className="grid grid-cols-1 gap-4 sm:gap-6 lg:grid-cols-3">
        {/* Company Information */}
        <div className="rounded-lg border border-line bg-white p-5 lg:col-span-2">
          <div className="flex items-start justify-between gap-4">
            <div className="flex items-start gap-3">
              <IconTile icon={<FileText />} theme="brand" />
              <div>
                <h3 className="text-sm font-semibold text-ink-950">Company Information</h3>
                <p className="text-xs text-ink-500">As submitted during Create Company</p>
              </div>
            </div>
            {editEligibility === "Editable" && (
              <button className="flex-shrink-0 text-ink-400 hover:text-ink-700">
                <Edit3 className="h-4 w-4" />
              </button>
            )}
          </div>

          <div className="mt-5 grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
            <InfoField label="Legal Name" value={data.legalName} />
            <InfoField label="Trading Name" value={data.tradingName || "—"} />
            <InfoField
              label="Registration Reference"
              value="—"
              icon={<Hash className="h-3.5 w-3.5 text-ink-400" />}
              mono
            />
            <InfoField
              label="Country"
              value={COUNTRY_NAMES[data.countryCode] || data.countryCode}
              icon={<FlagIcon countryCode={data.countryCode} />}
            />
            <InfoField label="Company Type" value="—" />
            <InfoField label="Timezone" value={data.timezone} icon={<Clock className="h-3.5 w-3.5 text-ink-400" />} />
            <InfoField
              label="Default Currency"
              value={data.defaultCurrency}
              icon={
                <span className="flex h-3.5 w-3.5 items-center justify-center rounded-full bg-green text-[8px] font-bold leading-none text-white">
                  {CURRENCY_SYMBOL[data.defaultCurrency] ?? "$"}
                </span>
              }
            />
          </div>
        </div>

        {/* Status & Version */}
        <div className="rounded-lg border border-line bg-white p-5">
          <div className="flex items-start gap-3">
            <IconTile icon={<User />} theme="accent" />
            <div>
              <h3 className="text-sm font-semibold text-ink-950">Status</h3>
              <p className="text-xs text-ink-500">Current company state</p>
            </div>
          </div>

          <div className="mt-5 space-y-4">
            <div>
              <p className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Status</p>
              <span
                className={`mt-1 inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
              >
                {statusStyle.icon}
                {statusLabel}
              </span>
            </div>
            <div>
              <p className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Organization ID</p>
              <p className="mt-1 break-all font-mono text-xs text-ink-600">{data.organizationId}</p>
            </div>
            <div>
              <p className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">Version</p>
              <p className="mt-1 font-mono text-sm font-semibold text-ink-950">v{data.version}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Navigation */}
      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={() => router.push("/setup/create-company")}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2"
        >
          <ChevronLeft className="h-4 w-4" />
          Previous Step
        </button>
        <button
          onClick={() => router.push("/setup/capabilities")}
          className="inline-flex items-center justify-center gap-2 rounded-lg bg-primary px-6 py-2 text-sm font-medium text-white hover:bg-primary-hover"
        >
          Continue
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}