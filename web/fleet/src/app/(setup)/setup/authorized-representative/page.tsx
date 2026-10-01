"use client";

import React, { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  User,
  Mail,
  Phone,
  Briefcase,
  FileText,
  Loader2,
  Save,
  UserCheck,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService, type Representative } from "@/lib/services/organizationService";

type FormData = {
  fullName: string;
  roleTitle: string;
  email: string;
  phone: string;
  authorityType: string;
};

const AUTHORITY_TYPES = [
  { value: "LEGAL_REPRESENTATIVE", label: "Legal Representative" },
  { value: "BENEFICIAL_OWNER", label: "Beneficial Owner" },
  { value: "DIRECTOR", label: "Director" },
  { value: "CEO", label: "CEO / Managing Director" },
  { value: "CFO", label: "CFO / Finance Director" },
  { value: "COMPANY_SECRETARY", label: "Company Secretary" },
  { value: "AUTHORIZED_SIGNATORY", label: "Authorized Signatory" },
  { value: "OTHER", label: "Other" },
];

const STATUS_LABEL: Record<string, string> = {
  DRAFT: "Draft",
  PENDING: "Pending Review",
  ACTIVE: "Active",
  SUSPENDED: "Suspended",
  APPROVED: "Approved",
  REJECTED: "Rejected",
  CLOSED: "Closed",
};

const STATUS_STYLES: Record<string, { bg: string; text: string }> = {
  DRAFT: { bg: "bg-amber-bg", text: "text-amber" },
  PENDING: { bg: "bg-amber-bg", text: "text-amber" },
  ACTIVE: { bg: "bg-green-bg", text: "text-green" },
  SUSPENDED: { bg: "bg-red-bg", text: "text-red" },
  APPROVED: { bg: "bg-green-bg", text: "text-green" },
  REJECTED: { bg: "bg-red-bg", text: "text-red" },
  CLOSED: { bg: "bg-surface-2", text: "text-ink-500" },
};

const inputClass = (hasError: boolean) =>
  `w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
    hasError
      ? "border-red focus:border-red focus:ring-2 focus:ring-red/10"
      : "border-line focus:border-primary focus:ring-2 focus:ring-primary/10"
  }`;

const REQUIRED_FIELDS: (keyof FormData)[] = ["fullName", "roleTitle", "email", "phone", "authorityType"];

function validateField(name: keyof FormData, value: string): string | undefined {
  switch (name) {
    case "fullName":
      if (!value.trim()) return "Full name is required";
      if (value.trim().length < 2) return "Full name must be at least 2 characters";
      return undefined;
    case "roleTitle":
      if (!value.trim()) return "Role title is required";
      return undefined;
    case "email":
      if (!value.trim()) return "Email is required";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) return "Enter a valid email address";
      return undefined;
    case "phone":
      if (!value.trim()) return "Phone number is required";
      const phoneClean = value.replace(/[\s()-]/g, "");
      if (phoneClean.length < 7) return "Enter a valid phone number";
      return undefined;
    case "authorityType":
      if (!value) return "Authority type is required";
      return undefined;
    default:
      return undefined;
  }
}

function IconTile({ icon, theme = "brand" }: { icon: React.ReactNode; theme?: "brand" | "accent" }) {
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

export default function AuthorizedRepresentativePage() {
  const router = useRouter();
  const { _hasHydrated } = useAuthStore();
  const { organization } = useOrganizationStore();
  const { setStageComplete } = useOnboardingStore();

  const [loading, setLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [existingReps, setExistingReps] = useState<Representative[]>([]);

  const [formData, setFormData] = useState<FormData>({
    fullName: "",
    roleTitle: "",
    email: "",
    phone: "",
    authorityType: "",
  });

  const [touched, setTouched] = useState<Partial<Record<keyof FormData, boolean>>>({});
  const [errors, setErrors] = useState<Partial<Record<keyof FormData, string>>>({});

  const completedRequired = REQUIRED_FIELDS.filter((field) => !validateField(field, formData[field])).length;

  useEffect(() => {
    if (!_hasHydrated) return;

    const fetchData = async () => {
      if (!organization?.organizationId) {
        setLoading(false);
        return;
      }

      try {
        const reps = await organizationService.listRepresentatives(organization.organizationId);
        
        // ✅ SAFETY: ensure reps is always an array
        const repsArray = Array.isArray(reps) ? reps : [];
        setExistingReps(repsArray);

        // If there's an existing representative, pre-fill the form
        if (repsArray.length > 0) {
          const rep = repsArray[0];
          setFormData({
            fullName: rep.fullName || "",
            roleTitle: rep.roleTitle || "",
            email: rep.email || "",
            phone: rep.phone || "",
            authorityType: rep.authorityType || "",
          });
        }
      } catch (err) {
        console.error("Failed to fetch representatives:", err);
        setExistingReps([]);
      } finally {
        setLoading(false);
      }
    };

    fetchData();
  }, [_hasHydrated, organization?.organizationId]);

  const setField = (name: keyof FormData, value: string) => {
    setFormData((prev) => ({ ...prev, [name]: value }));
    const err = validateField(name, value);
    setErrors((prev) => {
      if (err) {
        return { ...prev, [name]: err };
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const handleBlur = (name: keyof FormData) => {
    setTouched((prev) => ({ ...prev, [name]: true }));
    const err = validateField(name, formData[name]);
    setErrors((prev) => {
      if (err) {
        return { ...prev, [name]: err };
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const handleSubmit = async () => {
    // Validate all fields
    const allErrors: Partial<Record<keyof FormData, string>> = {};
    REQUIRED_FIELDS.forEach((field) => {
      const err = validateField(field, formData[field]);
      if (err) allErrors[field] = err;
    });
    setErrors(allErrors);
    setTouched(
      REQUIRED_FIELDS.reduce((acc, field) => ({ ...acc, [field]: true }), {} as Record<keyof FormData, boolean>)
    );

    if (Object.keys(allErrors).length > 0) {
      setSaveError("Please fix all highlighted fields before continuing.");
      return;
    }

    if (!organization?.organizationId) {
      setSaveError("Organization not found.");
      return;
    }

    setIsSubmitting(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      // If existing representative, update; otherwise create
      if (existingReps.length > 0) {
        await organizationService.updateRepresentative(
          organization.organizationId,
          existingReps[0].id,
          {
            fullName: formData.fullName.trim(),
            roleTitle: formData.roleTitle.trim(),
            email: formData.email.trim(),
            phone: formData.phone.trim(),
            authorityType: formData.authorityType,
          }
        );
      } else {
        await organizationService.createRepresentative(organization.organizationId, {
          fullName: formData.fullName.trim(),
          roleTitle: formData.roleTitle.trim(),
          email: formData.email.trim(),
          phone: formData.phone.trim(),
          authorityType: formData.authorityType,
          isOnboardingOwner: true,
        });
      }

      // Mark the stage as complete in the store
      setStageComplete('authorized-representative');

      setSaveSuccess(true);
      setTimeout(() => {
        router.push('/setup/capabilities');
      }, 1000);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to save representative information.";
      setSaveError(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleBack = () => router.push('/setup/profile');

  const isComplete = completedRequired === REQUIRED_FIELDS.length;
  const hasErrors = Object.values(errors).some((e) => !!e);
  const canSubmit = !isSubmitting && !saveSuccess && isComplete && !hasErrors;

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-4xl space-y-6">
        <div className="space-y-2">
          <div className="h-7 w-56 animate-pulse rounded bg-surface-2" />
          <div className="h-4 w-96 animate-pulse rounded bg-surface-2" />
        </div>
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="mx-auto w-full max-w-4xl">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-red/20 bg-red-bg/30 px-4 py-16 text-center">
          <div className="rounded-full bg-red-bg p-4">
            <AlertCircle className="h-8 w-8 text-red" />
          </div>
          <h3 className="mt-4 text-[15px] font-semibold text-ink-950">Unable to load page</h3>
          <p className="mt-1.5 text-sm text-ink-500">{error}</p>
          <button
            onClick={() => router.push("/setup")}
            className="mt-5 rounded-lg bg-primary px-5 py-2 text-[13px] font-semibold text-white transition hover:bg-primary/90"
          >
            Go to Onboarding Home
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Authorized Representative</span>
        </p>
        <h1 className="mt-1 text-ink-950">Authorized Representative</h1>
        <p className="mt-1.5 text-ink-600">
          Provide details of the authorized representative with legal authority for your organization.
        </p>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
        <p className="flex-shrink-0 text-sm font-medium text-ink-700">
          <span className="font-mono tabular-nums text-ink-950">{completedRequired}</span>
          <span className="text-ink-400"> / {REQUIRED_FIELDS.length}</span> required fields complete
        </p>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-2 sm:max-w-xs">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${(completedRequired / REQUIRED_FIELDS.length) * 100}%` }}
          />
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
        <div className="flex items-start gap-4">
          <IconTile icon={<UserCheck />} theme="brand" />
          <div>
            <h2 className="text-[15px] font-semibold text-ink-950">Representative Details</h2>
            <p className="text-sm text-ink-500">
              This person must have legal authority to act on behalf of the organization.
            </p>
          </div>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <User className="h-3 w-3 text-ink-400" />
              Full Name <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="text"
                value={formData.fullName}
                onChange={(e) => setField("fullName", e.target.value)}
                onBlur={() => handleBlur("fullName")}
                placeholder="e.g. John Doe"
                className={inputClass(!!(touched.fullName && errors.fullName))}
              />
            </div>
            {touched.fullName && errors.fullName && (
              <p className="mt-1 flex items-center gap-1 text-xs text-red">
                <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
                {errors.fullName}
              </p>
            )}
          </div>

          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Briefcase className="h-3 w-3 text-ink-400" />
              Role Title <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="text"
                value={formData.roleTitle}
                onChange={(e) => setField("roleTitle", e.target.value)}
                onBlur={() => handleBlur("roleTitle")}
                placeholder="e.g. Chief Executive Officer"
                className={inputClass(!!(touched.roleTitle && errors.roleTitle))}
              />
            </div>
            {touched.roleTitle && errors.roleTitle && (
              <p className="mt-1 flex items-center gap-1 text-xs text-red">
                <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
                {errors.roleTitle}
              </p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Mail className="h-3 w-3 text-ink-400" />
              Email <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="email"
                value={formData.email}
                onChange={(e) => setField("email", e.target.value)}
                onBlur={() => handleBlur("email")}
                placeholder="representative@company.com"
                className={inputClass(!!(touched.email && errors.email))}
              />
            </div>
            {touched.email && errors.email && (
              <p className="mt-1 flex items-center gap-1 text-xs text-red">
                <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
                {errors.email}
              </p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Phone className="h-3 w-3 text-ink-400" />
              Phone <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="tel"
                value={formData.phone}
                onChange={(e) => setField("phone", e.target.value)}
                onBlur={() => handleBlur("phone")}
                placeholder="+234 800 000 0000"
                className={inputClass(!!(touched.phone && errors.phone))}
              />
            </div>
            {touched.phone && errors.phone && (
              <p className="mt-1 flex items-center gap-1 text-xs text-red">
                <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
                {errors.phone}
              </p>
            )}
          </div>

          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <FileText className="h-3 w-3 text-ink-400" />
              Authority Type <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <select
                value={formData.authorityType}
                onChange={(e) => setField("authorityType", e.target.value)}
                onBlur={() => handleBlur("authorityType")}
                className={inputClass(!!(touched.authorityType && errors.authorityType))}
              >
                <option value="">Select authority type...</option>
                {AUTHORITY_TYPES.map((type) => (
                  <option key={type.value} value={type.value}>
                    {type.label}
                  </option>
                ))}
              </select>
            </div>
            {touched.authorityType && errors.authorityType && (
              <p className="mt-1 flex items-center gap-1 text-xs text-red">
                <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
                {errors.authorityType}
              </p>
            )}
          </div>
        </div>
      </div>

      {saveError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {saveError}
        </div>
      )}

      {saveSuccess && (
        <div className="flex items-center gap-2 rounded-xl border border-green/20 bg-green-bg/40 px-4 py-3 text-sm text-green">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          Representative saved successfully! Redirecting...
        </div>
      )}

      <div className="flex flex-col items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          disabled={isSubmitting || saveSuccess}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2 disabled:opacity-50"
        >
          <ChevronLeft className="h-4 w-4" />
          Previous Step
        </button>
        <button
          onClick={handleSubmit}
          disabled={!canSubmit}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            !canSubmit
              ? "cursor-not-allowed bg-ink-400"
              : "bg-primary hover:bg-primary-hover"
          }`}
        >
          {isSubmitting ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : saveSuccess ? (
            <>
              <CheckCircle2 className="h-4 w-4" />
              Saved!
            </>
          ) : (
            <>
              <Save className="h-4 w-4" />
              {existingReps.length > 0 ? "Update" : "Save"} & Continue
            </>
          )}
        </button>
      </div>
    </div>
  );
}