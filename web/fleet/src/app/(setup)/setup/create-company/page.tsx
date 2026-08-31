// src/app/(setup)/setup/create-company/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
} from "lucide-react";

// ==================== TYPES ====================

type CompanyFormData = {
  legalName: string;
  tradingName: string;
  registrationReference: string;
  country: string;
  companyType: string;
  timezone: string;
  defaultCurrency: string;
  ownerName: string;
  ownerEmail: string;
  ownerPhone: string;
};

// ==================== CONSTANTS ====================

const COUNTRIES = ["Nigeria", "Ghana", "Kenya", "South Africa", "Egypt", "Morocco", "Other"];
const COMPANY_TYPES = ["Private Limited", "Public Limited", "Sole Proprietorship", "Partnership", "LLC", "Other"];
const TIMEZONES = ["Africa/Lagos", "Africa/Accra", "Africa/Nairobi", "Africa/Johannesburg", "Africa/Cairo", "Africa/Casablanca"];
const CURRENCIES = ["NGN", "GHS", "KES", "ZAR", "EGP", "MAD", "USD", "EUR"];

export default function CreateCompanyPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(false);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [formData, setFormData] = useState<CompanyFormData>({
    legalName: "",
    tradingName: "",
    registrationReference: "",
    country: "Nigeria",
    companyType: "Private Limited",
    timezone: "Africa/Lagos",
    defaultCurrency: "NGN",
    ownerName: "",
    ownerEmail: "",
    ownerPhone: "",
  });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!formData.legalName.trim()) newErrors.legalName = "Legal name is required";
    if (!formData.registrationReference.trim()) newErrors.registrationReference = "Registration reference is required";
    if (!formData.ownerName.trim()) newErrors.ownerName = "Owner name is required";
    if (!formData.ownerEmail.trim()) newErrors.ownerEmail = "Owner email is required";
    else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(formData.ownerEmail)) {
      newErrors.ownerEmail = "Invalid email address";
    }
    if (!formData.ownerPhone.trim()) newErrors.ownerPhone = "Owner phone is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setIsSubmitting(true);
    setLoading(true);

    await new Promise((resolve) => setTimeout(resolve, 1500));

    setLoading(false);
    setIsSubmitting(false);

    toast("Company created successfully! (mock)");
    router.push("/setup/profile");
  };

  const handleCancel = () => {
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
          <span className="text-ink-700 font-medium">Create Company</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Create Company
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Legal name, trading name, registration reference, country, company type, timezone, default currency and owner information.
        </p>
      </div>

      {/* ===== FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
            {/* Legal Name */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Legal Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.legalName}
                onChange={(e) => setFormData((prev) => ({ ...prev, legalName: e.target.value }))}
                placeholder="e.g. Apex Logistics Limited"
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
                value={formData.tradingName}
                onChange={(e) => setFormData((prev) => ({ ...prev, tradingName: e.target.value }))}
                placeholder="e.g. Apex Logistics"
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              />
            </div>

            {/* Registration Reference */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Registration Reference <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.registrationReference}
                onChange={(e) => setFormData((prev) => ({ ...prev, registrationReference: e.target.value }))}
                placeholder="e.g. RC-123456"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.registrationReference ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.registrationReference && <p className="mt-1 text-sm text-red">{errors.registrationReference}</p>}
            </div>

            {/* Country */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Country
              </label>
              <select
                value={formData.country}
                onChange={(e) => setFormData((prev) => ({ ...prev, country: e.target.value }))}
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
                value={formData.companyType}
                onChange={(e) => setFormData((prev) => ({ ...prev, companyType: e.target.value }))}
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
                value={formData.timezone}
                onChange={(e) => setFormData((prev) => ({ ...prev, timezone: e.target.value }))}
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
                value={formData.defaultCurrency}
                onChange={(e) => setFormData((prev) => ({ ...prev, defaultCurrency: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {CURRENCIES.map((c) => (
                  <option key={c} value={c}>{c}</option>
                ))}
              </select>
            </div>

            {/* Owner Name */}
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Owner Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.ownerName}
                onChange={(e) => setFormData((prev) => ({ ...prev, ownerName: e.target.value }))}
                placeholder="Full name of the company owner"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.ownerName ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.ownerName && <p className="mt-1 text-sm text-red">{errors.ownerName}</p>}
            </div>

            {/* Owner Email */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Owner Email <span className="text-red">*</span>
              </label>
              <input
                type="email"
                value={formData.ownerEmail}
                onChange={(e) => setFormData((prev) => ({ ...prev, ownerEmail: e.target.value }))}
                placeholder="owner@company.com"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.ownerEmail ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.ownerEmail && <p className="mt-1 text-sm text-red">{errors.ownerEmail}</p>}
            </div>

            {/* Owner Phone */}
            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Owner Phone <span className="text-red">*</span>
              </label>
              <input
                type="tel"
                value={formData.ownerPhone}
                onChange={(e) => setFormData((prev) => ({ ...prev, ownerPhone: e.target.value }))}
                placeholder="+234 800 000 0000"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.ownerPhone ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.ownerPhone && <p className="mt-1 text-sm text-red">{errors.ownerPhone}</p>}
            </div>
          </div>

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleSubmit}
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
                  Creating...
                </>
              ) : (
                <>
                  <Save className="h-4 w-4" />
                  Create Company
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
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}