// src/app/(setup)/setup/create-company/page.tsx

"use client";

import React, { useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  ChevronLeft,
  Building2,
  User,
  MapPin,
  Globe2,
  Landmark,
  Clock,
  Hash,
  Mail,
  Phone,
  AlertCircle,
  CheckCircle2,
} from "lucide-react";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";
import { useAuthStore } from "@/lib/stores/authStore";

type CompanyFormData = {
  legalName: string;
  tradingName: string;
  registrationNumber: string;
  countryCode: string;
  organizationType: string;
  timezone: string;
  defaultCurrency: string;
  ownerName: string;
  ownerEmail: string;
  ownerPhone: string;
};

type FieldName = keyof CompanyFormData;

const COUNTRIES = [
  { value: "NG", label: "Nigeria" },
  { value: "GH", label: "Ghana" },
  { value: "KE", label: "Kenya" },
  { value: "ZA", label: "South Africa" },
  { value: "EG", label: "Egypt" },
  { value: "MA", label: "Morocco" },
  { value: "OTHER", label: "Other" },
];

const COMPANY_TYPES = [
  { value: "PRIVATE_LIMITED", label: "Private Limited" },
  { value: "PUBLIC_LIMITED", label: "Public Limited" },
  { value: "SOLE_PROPRIETORSHIP", label: "Sole Proprietorship" },
  { value: "PARTNERSHIP", label: "Partnership" },
  { value: "LLC", label: "LLC" },
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

const REQUIRED_FIELDS: FieldName[] = [
  "legalName",
  "registrationNumber",
  "ownerName",
  "ownerEmail",
  "ownerPhone",
];

const INITIAL_FORM: CompanyFormData = {
  legalName: "",
  tradingName: "",
  registrationNumber: "",
  countryCode: "NG",
  organizationType: "PRIVATE_LIMITED",
  timezone: "Africa/Lagos",
  defaultCurrency: "NGN",
  ownerName: "",
  ownerEmail: "",
  ownerPhone: "",
};

function validateField(name: FieldName, value: string): string | undefined {
  switch (name) {
    case "legalName":
      if (!value.trim()) return "Legal name is required";
      if (value.trim().length < 2) return "Legal name must be at least 2 characters";
      return undefined;
    case "registrationNumber":
      if (!value.trim()) return "Registration number is required";
      if (value.trim().length < 3) return "Registration number must be at least 3 characters";
      return undefined;
    case "ownerName":
      if (!value.trim()) return "Owner name is required";
      if (value.trim().length < 2) return "Owner name must be at least 2 characters";
      return undefined;
    case "ownerEmail":
      if (!value.trim()) return "Owner email is required";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) return "Enter a valid email address";
      return undefined;
    case "ownerPhone":
      if (!value.trim()) return "Owner phone is required";
      const phoneClean = value.replace(/[\s()-]/g, "");
      if (phoneClean.length < 7) return "Enter a valid phone number";
      return undefined;
    default:
      return undefined;
  }
}

function validateAll(data: CompanyFormData): Partial<Record<FieldName, string>> {
  const errors: Partial<Record<FieldName, string>> = {};
  for (const field of REQUIRED_FIELDS) {
    const message = validateField(field, data[field]);
    if (message) errors[field] = message;
  }
  return errors;
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

function SectionCard({
  icon,
  theme,
  title,
  description,
  children,
}: {
  icon: React.ReactNode;
  theme?: "brand" | "accent";
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-line bg-white p-5">
      <div className="flex items-start gap-3">
        <IconTile icon={icon} theme={theme} />
        <div>
          <h3 className="text-sm font-semibold text-ink-950">{title}</h3>
          <p className="text-xs text-ink-500">{description}</p>
        </div>
      </div>
      <div className="mt-5 grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2">{children}</div>
    </div>
  );
}

function Field({
  label,
  required,
  error,
  touched,
  icon,
  span2,
  children,
}: {
  label: string;
  required?: boolean;
  error?: string;
  touched?: boolean;
  icon?: React.ReactNode;
  span2?: boolean;
  children: React.ReactNode;
}) {
  const showError = touched && !!error;
  return (
    <div className={span2 ? "sm:col-span-2" : ""}>
      <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
        {icon}
        {label}
        {required && <span className="text-red">*</span>}
      </label>
      <div className="mt-1">{children}</div>
      {showError && (
        <p className="mt-1 flex items-center gap-1 text-xs text-red">
          <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
          {error}
        </p>
      )}
    </div>
  );
}

const inputClass = (hasError: boolean) =>
  `w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
    hasError
      ? "border-red focus:border-red focus:ring-2 focus:ring-red/10"
      : "border-line focus:border-primary focus:ring-2 focus:ring-primary/10"
  }`;

export default function CreateCompanyPage() {
  const router = useRouter();
  const { setOrganizationId, updateCompanyData, setLoading, setError } = useOnboardingStore();
  const { accessToken, _hasHydrated } = useAuthStore();

  const [formData, setFormData] = useState<CompanyFormData>(INITIAL_FORM);
  const [touched, setTouched] = useState<Partial<Record<FieldName, boolean>>>({});
  const [errors, setErrors] = useState<Partial<Record<FieldName, string>>>({});
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitSuccess, setSubmitSuccess] = useState(false);

  const completedRequired = useMemo(
    () => REQUIRED_FIELDS.filter((field) => !validateField(field, formData[field])).length,
    [formData]
  );

  const setField = (name: FieldName, value: string) => {
    setFormData((prev) => ({ ...prev, [name]: value }));
    const error = validateField(name, value);
    setErrors((prev) => {
      if (error) {
        return { ...prev, [name]: error };
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const handleBlur = (name: FieldName) => {
    setTouched((prev) => ({ ...prev, [name]: true }));
    const error = validateField(name, formData[name]);
    setErrors((prev) => {
      if (error) {
        return { ...prev, [name]: error };
      }
      const next = { ...prev };
      delete next[name];
      return next;
    });
  };

  const handleSubmit = async () => {
    const allErrors = validateAll(formData);
    setErrors(allErrors);
    setTouched(
      REQUIRED_FIELDS.reduce((acc, field) => ({ ...acc, [field]: true }), {} as Record<FieldName, boolean>)
    );
    if (Object.keys(allErrors).length > 0) {
      setSubmitError("Please fix all highlighted fields before continuing.");
      return;
    }

    if (!_hasHydrated) {
      setSubmitError("Loading your session, please try again.");
      return;
    }

    setSubmitError(null);
    setIsSubmitting(true);
    setLoading(true);

    try {
      if (!accessToken) {
        throw new Error("You must be signed in to create a company.");
      }

      const response = await organizationService.createOrganization({
        legalName: formData.legalName.trim(),
        tradingName: formData.tradingName.trim(),
        registrationNumber: formData.registrationNumber.trim(),
        countryCode: formData.countryCode,
        organizationType: formData.organizationType,
        timezone: formData.timezone,
        defaultCurrency: formData.defaultCurrency,
        ownerName: formData.ownerName.trim(),
      });

      setOrganizationId(response.organizationId);
      updateCompanyData({
        legalName: formData.legalName.trim(),
        tradingName: formData.tradingName.trim(),
        registrationNumber: formData.registrationNumber.trim(),
        countryCode: formData.countryCode,
        organizationType: formData.organizationType,
        timezone: formData.timezone,
        defaultCurrency: formData.defaultCurrency,
        ownerName: formData.ownerName.trim(),
        ownerEmail: formData.ownerEmail.trim(),
        ownerPhone: formData.ownerPhone.trim(),
      });

      setSubmitSuccess(true);
      setSubmitError(null);

      setTimeout(() => {
        router.push("/setup/profile");
      }, 1000);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to create company. Please try again.";
      setSubmitError(message);
      setSubmitSuccess(false);
    } finally {
      setIsSubmitting(false);
      setLoading(false);
    }
  };

  const handleBack = () => router.push("/setup");

  const isComplete = completedRequired === REQUIRED_FIELDS.length;
  const hasErrors = Object.values(errors).some((e) => !!e);
  const isReady = _hasHydrated && !isSubmitting && !submitSuccess && isComplete && !hasErrors;

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 pb-24 sm:space-y-8">
      {/* Header */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Create Company</span>
        </p>
        <h1 className="mt-1 text-ink-950">Create Company</h1>
        <p className="mt-1.5 text-ink-600">
          Tell us about your company and who we should contact about it. You can update most of this later.
        </p>
      </div>

      {/* Status Strip */}
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

      {/* Legal Identity */}
      <SectionCard
        icon={<Building2 />}
        theme="brand"
        title="Legal Identity"
        description="How your company is registered"
      >
        <Field label="Legal Name" required error={errors.legalName} touched={touched.legalName} span2>
          <input
            type="text"
            value={formData.legalName}
            onChange={(e) => setField("legalName", e.target.value)}
            onBlur={() => handleBlur("legalName")}
            placeholder="e.g. Apex Logistics Limited"
            className={inputClass(!!(touched.legalName && errors.legalName))}
          />
        </Field>

        <Field label="Trading Name">
          <input
            type="text"
            value={formData.tradingName}
            onChange={(e) => setField("tradingName", e.target.value)}
            placeholder="e.g. Apex Logistics"
            className={inputClass(false)}
          />
        </Field>

        <Field
          label="Registration Number"
          required
          error={errors.registrationNumber}
          touched={touched.registrationNumber}
          icon={<Hash className="h-3 w-3 text-ink-400" />}
        >
          <input
            type="text"
            value={formData.registrationNumber}
            onChange={(e) => setField("registrationNumber", e.target.value)}
            onBlur={() => handleBlur("registrationNumber")}
            placeholder="e.g. RC-123456"
            className={`${inputClass(!!(touched.registrationNumber && errors.registrationNumber))} font-mono`}
          />
        </Field>
      </SectionCard>

      {/* Regional Settings */}
      <SectionCard
        icon={<Globe2 />}
        theme="accent"
        title="Regional Settings"
        description="Where you operate and how we format things for you"
      >
        <Field label="Country" icon={<MapPin className="h-3 w-3 text-ink-400" />}>
          <select
            value={formData.countryCode}
            onChange={(e) => setField("countryCode", e.target.value)}
            className={inputClass(false)}
          >
            {COUNTRIES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Company Type" icon={<Landmark className="h-3 w-3 text-ink-400" />}>
          <select
            value={formData.organizationType}
            onChange={(e) => setField("organizationType", e.target.value)}
            className={inputClass(false)}
          >
            {COMPANY_TYPES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Timezone" icon={<Clock className="h-3 w-3 text-ink-400" />}>
          <select
            value={formData.timezone}
            onChange={(e) => setField("timezone", e.target.value)}
            className={inputClass(false)}
          >
            {TIMEZONES.map((t) => (
              <option key={t.value} value={t.value}>
                {t.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Default Currency">
          <select
            value={formData.defaultCurrency}
            onChange={(e) => setField("defaultCurrency", e.target.value)}
            className={inputClass(false)}
          >
            {CURRENCIES.map((c) => (
              <option key={c.value} value={c.value}>
                {c.label}
              </option>
            ))}
          </select>
        </Field>
      </SectionCard>

      {/* Owner Contact */}
      <SectionCard
        icon={<User />}
        theme="brand"
        title="Owner Contact"
        description="Who we reach out to about this company"
      >
        <Field label="Full Name" required error={errors.ownerName} touched={touched.ownerName} span2>
          <input
            type="text"
            value={formData.ownerName}
            onChange={(e) => setField("ownerName", e.target.value)}
            onBlur={() => handleBlur("ownerName")}
            placeholder="Full name of the company owner"
            className={inputClass(!!(touched.ownerName && errors.ownerName))}
          />
        </Field>

        <Field
          label="Email"
          required
          error={errors.ownerEmail}
          touched={touched.ownerEmail}
          icon={<Mail className="h-3 w-3 text-ink-400" />}
        >
          <input
            type="email"
            value={formData.ownerEmail}
            onChange={(e) => setField("ownerEmail", e.target.value)}
            onBlur={() => handleBlur("ownerEmail")}
            placeholder="owner@company.com"
            className={inputClass(!!(touched.ownerEmail && errors.ownerEmail))}
          />
        </Field>

        <Field
          label="Phone"
          required
          error={errors.ownerPhone}
          touched={touched.ownerPhone}
          icon={<Phone className="h-3 w-3 text-ink-400" />}
        >
          <input
            type="tel"
            value={formData.ownerPhone}
            onChange={(e) => setField("ownerPhone", e.target.value)}
            onBlur={() => handleBlur("ownerPhone")}
            placeholder="080 000 0000"
            className={`${inputClass(!!(touched.ownerPhone && errors.ownerPhone))} font-mono`}
          />
        </Field>
      </SectionCard>

      {/* Submit Error */}
      {submitError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {submitError}
        </div>
      )}

      {/* Submit Success */}
      {submitSuccess && (
        <div className="flex items-center gap-2 rounded-xl border border-green/20 bg-green-bg/40 px-4 py-3 text-sm text-green">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          Company created successfully! Redirecting to profile...
        </div>
      )}

      {/* Navigation */}
      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          disabled={isSubmitting || submitSuccess}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2 disabled:opacity-50"
        >
          <ChevronLeft className="h-4 w-4" />
          Back to Onboarding
        </button>
        <button
          onClick={handleSubmit}
          disabled={!isReady}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            !isReady
              ? "cursor-not-allowed bg-ink-400"
              : "bg-primary hover:bg-primary-hover"
          }`}
        >
          {isSubmitting ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Creating company...
            </>
          ) : submitSuccess ? (
            <>
              <CheckCircle2 className="h-4 w-4" />
              Created!
            </>
          ) : (
            <>
              <Save className="h-4 w-4" />
              Create Company
            </>
          )}
        </button>
      </div>

      {/* Sticky Mobile Action Bar */}
      <div className="fixed inset-x-0 bottom-0 z-10 border-t border-line bg-white/90 px-4 py-3 backdrop-blur-xl sm:hidden">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-3">
          <span className="flex items-center gap-1.5 text-xs text-ink-500">
            {isComplete && !hasErrors ? (
              <>
                <CheckCircle2 className="h-3.5 w-3.5 text-green" />
                Ready to create
              </>
            ) : (
              <>{REQUIRED_FIELDS.length - completedRequired} field(s) left</>
            )}
          </span>
          <button
            onClick={handleSubmit}
            disabled={!isReady}
            className={`inline-flex items-center gap-2 rounded-lg px-5 py-2 text-sm font-medium text-white ${
              !isReady
                ? "bg-ink-400"
                : "bg-primary hover:bg-primary-hover"
            }`}
          >
            {isSubmitting ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            Create
          </button>
        </div>
      </div>
    </div>
  );
}