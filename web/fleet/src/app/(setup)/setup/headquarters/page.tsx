// src/app/(setup)/setup/headquarters/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  ChevronLeft,
  CheckCircle2,
  Loader2,
  MapPin,
  Building,
  Mail,
  Phone,
  Globe,
  FileText,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";

type FormData = {
  // Address
  street: string;
  city: string;
  state: string;
  postalCode: string;
  country: string;
  // Contact
  businessEmail: string;
  businessPhone: string;
  website: string;
  businessDescription: string;
};

const COUNTRIES = [
  { value: "NG", label: "Nigeria" },
  { value: "GH", label: "Ghana" },
  { value: "KE", label: "Kenya" },
  { value: "ZA", label: "South Africa" },
  { value: "EG", label: "Egypt" },
  { value: "MA", label: "Morocco" },
  { value: "OTHER", label: "Other" },
];

const NIGERIA_STATES = [
  "Abia", "Adamawa", "Akwa Ibom", "Anambra", "Bauchi", "Bayelsa", "Benue",
  "Borno", "Cross River", "Delta", "Ebonyi", "Edo", "Ekiti", "Enugu",
  "FCT", "Gombe", "Imo", "Jigawa", "Kaduna", "Kano", "Katsina", "Kebbi",
  "Kogi", "Kwara", "Lagos", "Nasarawa", "Niger", "Ogun", "Ondo", "Osun",
  "Oyo", "Plateau", "Rivers", "Sokoto", "Taraba", "Yobe", "Zamfara",
];

const inputClass = (hasError: boolean) =>
  `w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
    hasError
      ? "border-red focus:border-red focus:ring-2 focus:ring-red/10"
      : "border-line focus:border-primary focus:ring-2 focus:ring-primary/10"
  }`;

const REQUIRED_FIELDS: (keyof FormData)[] = [
  "street",
  "city",
  "state",
  "postalCode",
  "country",
  "businessEmail",
  "businessPhone",
];

function validateField(name: keyof FormData, value: string): string | undefined {
  switch (name) {
    case "street":
      if (!value.trim()) return "Street address is required";
      return undefined;
    case "city":
      if (!value.trim()) return "City is required";
      return undefined;
    case "state":
      if (!value.trim()) return "State is required";
      return undefined;
    case "postalCode":
      if (!value.trim()) return "Postal code is required";
      return undefined;
    case "country":
      if (!value) return "Country is required";
      return undefined;
    case "businessEmail":
      if (!value.trim()) return "Business email is required";
      if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value)) return "Enter a valid email address";
      return undefined;
    case "businessPhone":
      if (!value.trim()) return "Business phone is required";
      const phoneClean = value.replace(/[\s()-]/g, "");
      if (phoneClean.length < 7) return "Enter a valid phone number";
      return undefined;
    default:
      return undefined;
  }
}

function IconTile({ icon }: { icon: React.ReactNode }) {
  return (
    <div
      className="relative flex h-11 w-11 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br from-primary to-accent text-white ring-1 ring-white/20"
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
      <span className="relative [&>svg]:h-5 [&>svg]:w-5">{icon}</span>
    </div>
  );
}

export default function HeadquartersPage() {
  const router = useRouter();
  const { _hasHydrated } = useAuthStore();
  const { organization } = useOrganizationStore();
  const { setStageComplete } = useOnboardingStore();

  const [loading, setLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [formData, setFormData] = useState<FormData>({
    street: "",
    city: "",
    state: "",
    postalCode: "",
    country: "NG",
    businessEmail: "",
    businessPhone: "",
    website: "",
    businessDescription: "",
  });

  const [touched, setTouched] = useState<Partial<Record<keyof FormData, boolean>>>({});
  const [errors, setErrors] = useState<Partial<Record<keyof FormData, string>>>({});

  const completedRequired = REQUIRED_FIELDS.filter((field) => !validateField(field, formData[field])).length;

  useEffect(() => {
    if (!_hasHydrated) return;

    const fetchProfile = async () => {
      if (!organization?.organizationId) {
        setLoading(false);
        return;
      }

      try {
        const profile = await organizationService.getProfile(organization.organizationId);
        if (profile) {
          setFormData({
            street: profile.registeredAddress?.street || "",
            city: profile.registeredAddress?.city || "",
            state: profile.registeredAddress?.state || "",
            postalCode: profile.registeredAddress?.postalCode || "",
            country: profile.registeredAddress?.country || "NG",
            businessEmail: profile.businessEmail || "",
            businessPhone: profile.businessPhone || "",
            website: profile.website || "",
            businessDescription: profile.businessDescription || "",
          });
        }
      } catch (err) {
        console.error("Failed to fetch profile:", err);
      } finally {
        setLoading(false);
      }
    };

    fetchProfile();
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

    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      await organizationService.updateProfile(organization.organizationId, {
        registeredAddress: {
          street: formData.street.trim(),
          city: formData.city.trim(),
          state: formData.state.trim(),
          postalCode: formData.postalCode.trim(),
          country: formData.country,
        },
        operatingAddress: {
          street: formData.street.trim(),
          city: formData.city.trim(),
          state: formData.state.trim(),
          postalCode: formData.postalCode.trim(),
          country: formData.country,
        },
        businessEmail: formData.businessEmail.trim() || null,
        businessPhone: formData.businessPhone.trim() || null,
        website: formData.website.trim() || null,
        businessDescription: formData.businessDescription.trim() || null,
      });

      setStageComplete("headquarters");

      setSaveSuccess(true);
      setTimeout(() => {
        router.push("/setup/business-docs");
      }, 1000);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to save headquarters information.";
      setSaveError(message);
    } finally {
      setIsSaving(false);
    }
  };

  const handleBack = () => router.push("/setup/capabilities");

  const isComplete = completedRequired === REQUIRED_FIELDS.length;
  const hasErrors = Object.values(errors).some((e) => !!e);
  const canSubmit = !isSaving && !saveSuccess && isComplete && !hasErrors;

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-4xl space-y-6">
        <div className="space-y-2">
          <div className="h-7 w-48 animate-pulse rounded bg-surface-2 sm:w-56" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-4xl space-y-6">
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Headquarters</span>
        </p>
        <h1 className="mt-1 text-ink-950">Headquarters</h1>
        <p className="mt-1.5 text-ink-600">
          Provide your company's registered address and contact information.
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
          <IconTile icon={<MapPin />} />
          <div>
            <h2 className="text-[15px] font-semibold text-ink-950">Registered Address</h2>
            <p className="text-sm text-ink-500">
              This is the official address of your company.
            </p>
          </div>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2">
          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <MapPin className="h-3 w-3 text-ink-400" />
              Street Address <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="text"
                value={formData.street}
                onChange={(e) => setField("street", e.target.value)}
                onBlur={() => handleBlur("street")}
                placeholder="123 Marina Road, Suite 4B"
                className={inputClass(!!(touched.street && errors.street))}
              />
            </div>
            {touched.street && errors.street && (
              <p className="mt-1 text-xs text-red">{errors.street}</p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              City <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="text"
                value={formData.city}
                onChange={(e) => setField("city", e.target.value)}
                onBlur={() => handleBlur("city")}
                placeholder="Lagos"
                className={inputClass(!!(touched.city && errors.city))}
              />
            </div>
            {touched.city && errors.city && (
              <p className="mt-1 text-xs text-red">{errors.city}</p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              State <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <select
                value={formData.state}
                onChange={(e) => setField("state", e.target.value)}
                onBlur={() => handleBlur("state")}
                className={inputClass(!!(touched.state && errors.state))}
              >
                <option value="">Select state...</option>
                {NIGERIA_STATES.map((state) => (
                  <option key={state} value={state}>
                    {state}
                  </option>
                ))}
              </select>
            </div>
            {touched.state && errors.state && (
              <p className="mt-1 text-xs text-red">{errors.state}</p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              Postal Code <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="text"
                value={formData.postalCode}
                onChange={(e) => setField("postalCode", e.target.value)}
                onBlur={() => handleBlur("postalCode")}
                placeholder="101241"
                className={inputClass(!!(touched.postalCode && errors.postalCode))}
              />
            </div>
            {touched.postalCode && errors.postalCode && (
              <p className="mt-1 text-xs text-red">{errors.postalCode}</p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Globe className="h-3 w-3 text-ink-400" />
              Country <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <select
                value={formData.country}
                onChange={(e) => setField("country", e.target.value)}
                onBlur={() => handleBlur("country")}
                className={inputClass(!!(touched.country && errors.country))}
              >
                {COUNTRIES.map((c) => (
                  <option key={c.value} value={c.value}>
                    {c.label}
                  </option>
                ))}
              </select>
            </div>
            {touched.country && errors.country && (
              <p className="mt-1 text-xs text-red">{errors.country}</p>
            )}
          </div>
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
        <div className="flex items-start gap-4">
          <IconTile icon={<Mail />} />
          <div>
            <h2 className="text-[15px] font-semibold text-ink-950">Contact Information</h2>
            <p className="text-sm text-ink-500">
              How customers and partners can reach your company.
            </p>
          </div>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2">
          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Mail className="h-3 w-3 text-ink-400" />
              Business Email <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="email"
                value={formData.businessEmail}
                onChange={(e) => setField("businessEmail", e.target.value)}
                onBlur={() => handleBlur("businessEmail")}
                placeholder="info@company.com"
                className={inputClass(!!(touched.businessEmail && errors.businessEmail))}
              />
            </div>
            {touched.businessEmail && errors.businessEmail && (
              <p className="mt-1 text-xs text-red">{errors.businessEmail}</p>
            )}
          </div>

          <div>
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Phone className="h-3 w-3 text-ink-400" />
              Business Phone <span className="text-red">*</span>
            </label>
            <div className="mt-1">
              <input
                type="tel"
                value={formData.businessPhone}
                onChange={(e) => setField("businessPhone", e.target.value)}
                onBlur={() => handleBlur("businessPhone")}
                placeholder="+234 800 000 0000"
                className={inputClass(!!(touched.businessPhone && errors.businessPhone))}
              />
            </div>
            {touched.businessPhone && errors.businessPhone && (
              <p className="mt-1 text-xs text-red">{errors.businessPhone}</p>
            )}
          </div>

          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <Globe className="h-3 w-3 text-ink-400" />
              Website
            </label>
            <div className="mt-1">
              <input
                type="url"
                value={formData.website}
                onChange={(e) => setField("website", e.target.value)}
                placeholder="www.company.com"
                className={inputClass(false)}
              />
            </div>
          </div>

          <div className="sm:col-span-2">
            <label className="flex items-center gap-1 text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
              <FileText className="h-3 w-3 text-ink-400" />
              Business Description
            </label>
            <div className="mt-1">
              <textarea
                value={formData.businessDescription}
                onChange={(e) => setField("businessDescription", e.target.value)}
                rows={3}
                placeholder="Describe what your company does..."
                className={inputClass(false)}
              />
            </div>
          </div>
        </div>
      </div>

      {saveError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          {saveError}
        </div>
      )}

      {saveSuccess && (
        <div className="flex items-center gap-2 rounded-xl border border-green/20 bg-green-bg/40 px-4 py-3 text-sm text-green">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          Headquarters saved successfully! Redirecting...
        </div>
      )}

      <div className="flex flex-col items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          disabled={isSaving || saveSuccess}
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
          {isSaving ? (
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
              Save & Continue
            </>
          )}
        </button>
      </div>
    </div>
  );
}