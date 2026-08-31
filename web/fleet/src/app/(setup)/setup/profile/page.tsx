// src/app/(setup)/setup/profile/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  X,
  CheckCircle2,
  Edit3,
  Lock,
  Eye,
  Save,
  Building2,
} from "lucide-react";

// ==================== TYPES ====================

type CompanyProfile = {
  legalName: string;
  tradingName: string;
  registrationReference: string;
  country: string;
  companyType: string;
  timezone: string;
  defaultCurrency: string;
  status: "Active" | "Pending" | "Suspended" | "Verifying" | "Approved";
  version: number;
  editEligibility: "Editable" | "Locked" | "ReviewRequired";
  operatingContactName: string;
  operatingContactEmail: string;
  operatingContactPhone: string;
  operatingAddress: string;
  website: string;
  industry: string;
  taxId: string;
};

// ==================== CONSTANTS ====================

const COUNTRIES = ["Nigeria", "Ghana", "Kenya", "South Africa", "Egypt", "Morocco", "Other"];
const COMPANY_TYPES = ["Private Limited", "Public Limited", "Sole Proprietorship", "Partnership", "LLC", "Other"];
const TIMEZONES = ["Africa/Lagos", "Africa/Accra", "Africa/Nairobi", "Africa/Johannesburg", "Africa/Cairo", "Africa/Casablanca"];
const CURRENCIES = ["NGN", "GHS", "KES", "ZAR", "EGP", "MAD", "USD", "EUR"];
const INDUSTRIES = ["Logistics & Transportation", "Supply Chain", "Warehousing", "Distribution", "Manufacturing", "Retail", "Other"];

// ==================== MOCK DATA ====================

const MOCK_PROFILE: CompanyProfile = {
  legalName: "Apex Logistics Limited",
  tradingName: "Apex Logistics",
  registrationReference: "RC-123456",
  country: "Nigeria",
  companyType: "Private Limited",
  timezone: "Africa/Lagos",
  defaultCurrency: "NGN",
  status: "Pending",
  version: 1,
  editEligibility: "Editable",
  operatingContactName: "Favour Daniel",
  operatingContactEmail: "favour@apexlogistics.com",
  operatingContactPhone: "+234 800 000 0000",
  operatingAddress: "42 Adebayo Street, Ikeja, Lagos, Nigeria",
  website: "https://apexlogistics.com",
  industry: "Logistics & Transportation",
  taxId: "TAX-2026-001",
};

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Active: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-4 w-4" /> },
  Suspended: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-4 w-4" /> },
  Verifying: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-4 w-4" /> },
  Approved: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
};

type EligibilityStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const ELIGIBILITY_STYLES: Record<string, EligibilityStyle> = {
  Editable: { bg: "bg-green-bg", text: "text-green", icon: <Edit3 className="h-4 w-4" /> },
  Locked: { bg: "bg-red-bg", text: "text-red", icon: <Lock className="h-4 w-4" /> },
  ReviewRequired: { bg: "bg-amber-bg", text: "text-amber", icon: <Eye className="h-4 w-4" /> },
};

export default function CompanyProfilePage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<CompanyProfile | null>(null);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isEditing, setIsEditing] = useState<boolean>(false);
  const [formData, setFormData] = useState<CompanyProfile | null>(null);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_PROFILE);
      setFormData(MOCK_PROFILE);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading company profile...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Building2 className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Profile not found</h3>
        <p className="mt-1 text-sm text-ink-500">No company profile data found.</p>
        <button
          onClick={() => router.push("/setup")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Onboarding
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Pending;
  const eligibilityStyle = ELIGIBILITY_STYLES[data.editEligibility] || ELIGIBILITY_STYLES.Locked;

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!formData?.legalName.trim()) newErrors.legalName = "Legal name is required";
    if (!formData?.operatingContactName.trim()) newErrors.operatingContactName = "Contact name is required";
    if (!formData?.operatingContactEmail.trim()) newErrors.operatingContactEmail = "Contact email is required";
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.operatingContactEmail)) {
      newErrors.operatingContactEmail = "Invalid email address";
    }
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSave = async () => {
    if (!validateForm() || !formData) return;

    setIsSubmitting(true);
    setLoading(true);

    await new Promise((resolve) => setTimeout(resolve, 1200));

    setData(formData);
    setLoading(false);
    setIsSubmitting(false);
    setIsEditing(false);

    toast("Profile updated successfully! (mock)");
  };

  const handleCancel = () => {
    setIsEditing(false);
    setFormData(data);
    setErrors({});
  };

  const handleBack = () => {
    router.push("/setup");
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button
            onClick={() => router.push("/setup")}
            className="hover:text-primary transition-colors"
          >
            Onboarding
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Company Profile</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Company Profile
        </h1>
        <div className="mt-1.5 flex flex-wrap items-center gap-2">
          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
            {statusStyle.icon}
            {data.status}
          </span>
          <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${eligibilityStyle.bg} ${eligibilityStyle.text}`}>
            {eligibilityStyle.icon}
            {data.editEligibility}
          </span>
          <span className="text-[10px] text-ink-500 font-mono">v{data.version}</span>
        </div>
      </div>

      {/* ===== PROFILE FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          {isEditing ? (
            // Edit Mode
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              {/* Legal Name */}
              <div className="sm:col-span-2">
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Legal Name <span className="text-red">*</span>
                </label>
                <input
                  type="text"
                  value={formData?.legalName || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, legalName: e.target.value } : null)}
                  className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                    errors.legalName ? "border-red focus:border-red" : "border-line focus:border-primary"
                  }`}
                />
                {errors.legalName && <p className="mt-1 text-sm text-red">{errors.legalName}</p>}
              </div>

              {/* Trading Name */}
              <div className="sm:col-span-2">
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Trading Name
                </label>
                <input
                  type="text"
                  value={formData?.tradingName || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, tradingName: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Registration Reference */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Registration Reference
                </label>
                <input
                  type="text"
                  value={formData?.registrationReference || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, registrationReference: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Country */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Country
                </label>
                <select
                  value={formData?.country || "Nigeria"}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, country: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                >
                  {COUNTRIES.map((c) => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
              </div>

              {/* Company Type */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Company Type
                </label>
                <select
                  value={formData?.companyType || "Private Limited"}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, companyType: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                >
                  {COMPANY_TYPES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>

              {/* Timezone */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Timezone
                </label>
                <select
                  value={formData?.timezone || "Africa/Lagos"}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, timezone: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                >
                  {TIMEZONES.map((t) => (
                    <option key={t} value={t}>{t}</option>
                  ))}
                </select>
              </div>

              {/* Default Currency */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Default Currency
                </label>
                <select
                  value={formData?.defaultCurrency || "NGN"}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, defaultCurrency: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                >
                  {CURRENCIES.map((c) => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
              </div>

              {/* Operating Contact Name */}
              <div className="sm:col-span-2">
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Operating Contact Name <span className="text-red">*</span>
                </label>
                <input
                  type="text"
                  value={formData?.operatingContactName || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, operatingContactName: e.target.value } : null)}
                  className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                    errors.operatingContactName ? "border-red focus:border-red" : "border-line focus:border-primary"
                  }`}
                />
                {errors.operatingContactName && <p className="mt-1 text-sm text-red">{errors.operatingContactName}</p>}
              </div>

              {/* Operating Contact Email */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Operating Contact Email <span className="text-red">*</span>
                </label>
                <input
                  type="email"
                  value={formData?.operatingContactEmail || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, operatingContactEmail: e.target.value } : null)}
                  className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                    errors.operatingContactEmail ? "border-red focus:border-red" : "border-line focus:border-primary"
                  }`}
                />
                {errors.operatingContactEmail && <p className="mt-1 text-sm text-red">{errors.operatingContactEmail}</p>}
              </div>

              {/* Operating Contact Phone */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Operating Contact Phone
                </label>
                <input
                  type="tel"
                  value={formData?.operatingContactPhone || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, operatingContactPhone: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Operating Address */}
              <div className="sm:col-span-2">
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Operating Address
                </label>
                <input
                  type="text"
                  value={formData?.operatingAddress || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, operatingAddress: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Website */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Website
                </label>
                <input
                  type="text"
                  value={formData?.website || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, website: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Industry */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Industry
                </label>
                <select
                  value={formData?.industry || "Logistics & Transportation"}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, industry: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                >
                  {INDUSTRIES.map((i) => (
                    <option key={i} value={i}>{i}</option>
                  ))}
                </select>
              </div>

              {/* Tax ID */}
              <div>
                <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                  Tax ID
                </label>
                <input
                  type="text"
                  value={formData?.taxId || ""}
                  onChange={(e) => setFormData((prev) => prev ? { ...prev, taxId: e.target.value } : null)}
                  className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
              </div>

              {/* Save/Cancel Buttons */}
              <div className="sm:col-span-2 mt-2 flex gap-3">
                <button
                  onClick={handleSave}
                  disabled={isSubmitting}
                  className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                    isSubmitting
                      ? "bg-ink-400 cursor-not-allowed"
                      : "bg-primary shadow-primary/20 hover:bg-primary/90"
                  }`}
                >
                  {isSubmitting ? (
                    <>
                      <RefreshCw className="h-4 w-4 animate-spin" />
                      Saving...
                    </>
                  ) : (
                    <>
                      <Save className="h-4 w-4" />
                      Save Changes
                    </>
                  )}
                </button>
                <button
                  onClick={handleCancel}
                  className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            // View Mode
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Legal Name</div>
                <div className="mt-1 text-sm font-medium text-ink-950">{data.legalName}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Trading Name</div>
                <div className="mt-1 text-sm text-ink-950">{data.tradingName || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Registration Reference</div>
                <div className="mt-1 text-sm text-ink-950">{data.registrationReference}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Country</div>
                <div className="mt-1 text-sm text-ink-950">{data.country}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Company Type</div>
                <div className="mt-1 text-sm text-ink-950">{data.companyType}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Timezone</div>
                <div className="mt-1 text-sm text-ink-950">{data.timezone}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Default Currency</div>
                <div className="mt-1 text-sm text-ink-950">{data.defaultCurrency}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Version</div>
                <div className="mt-1 text-sm font-mono text-ink-950">v{data.version}</div>
              </div>
              <div className="sm:col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Operating Contact</div>
                <div className="mt-1 text-sm text-ink-950">{data.operatingContactName}</div>
                <div className="text-sm text-ink-600">{data.operatingContactEmail} · {data.operatingContactPhone}</div>
              </div>
              <div className="sm:col-span-2">
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Operating Address</div>
                <div className="mt-1 text-sm text-ink-950">{data.operatingAddress || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Website</div>
                <div className="mt-1 text-sm text-ink-950">{data.website || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Industry</div>
                <div className="mt-1 text-sm text-ink-950">{data.industry}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Tax ID</div>
                <div className="mt-1 text-sm text-ink-950">{data.taxId || "—"}</div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
                <div className="mt-1">
                  <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                    {statusStyle.icon}
                    {data.status}
                  </span>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}