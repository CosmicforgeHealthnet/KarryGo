// src/app/(setup)/setup/capabilities/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  Bike,
  Truck,
  Warehouse,
  Snowflake,
  Globe,
  Lock,
  ChevronLeft,
  CheckCircle2,
  Loader2,
  Package,
  Car,
  Ship,
  FileText,
  Building,
  Briefcase,
  Users,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";

type Capability = {
  id: string;
  name: string;
  icon: React.ReactNode;
  enabled: boolean;
  status: string;
};

// Only display metadata — no "available", no "requiresReview", no "category"
const CAPABILITY_METADATA: Record<string, { name: string; icon: React.ReactNode }> = {
  PACKAGE_DELIVERY: { name: "Package Delivery", icon: <Package /> },
  TAXI: { name: "Taxi", icon: <Car /> },
  HAULING: { name: "Hauling", icon: <Truck /> },
  COURIER_LAST_MILE: { name: "Courier (Last Mile)", icon: <Bike /> },
  PASSENGER_TRANSPORT: { name: "Passenger Transport", icon: <Users /> },
  TRUCKING: { name: "Trucking", icon: <Truck /> },
  HEAVY_HAUL: { name: "Heavy Haul", icon: <Truck /> },
  FREIGHT_FORWARDING: { name: "Freight Forwarding", icon: <Ship /> },
  CROSS_BORDER: { name: "Cross-Border", icon: <Globe /> },
  COLD_CHAIN: { name: "Cold Chain", icon: <Snowflake /> },
  MEDICAL_LOGISTICS: { name: "Medical Logistics", icon: <FileText /> },
  AGRICULTURE_FOOD: { name: "Agriculture & Food", icon: <Building /> },
  MOVING_RELOCATION: { name: "Moving & Relocation", icon: <Briefcase /> },
  TOWING_RECOVERY: { name: "Towing & Recovery", icon: <Truck /> },
  WAREHOUSING: { name: "Warehousing", icon: <Warehouse /> },
  FULFILMENT: { name: "Fulfilment", icon: <Package /> },
  PRIVATE_LOGISTICS: { name: "Private Logistics", icon: <Truck /> },
};

const CATEGORIES: Record<string, string> = {
  PACKAGE_DELIVERY: "Transport",
  TAXI: "Transport",
  HAULING: "Transport",
  COURIER_LAST_MILE: "Transport",
  PASSENGER_TRANSPORT: "Transport",
  TRUCKING: "Transport",
  HEAVY_HAUL: "Specialized",
  FREIGHT_FORWARDING: "Specialized",
  CROSS_BORDER: "Specialized",
  COLD_CHAIN: "Specialized",
  MEDICAL_LOGISTICS: "Specialized",
  AGRICULTURE_FOOD: "Specialized",
  MOVING_RELOCATION: "Support",
  TOWING_RECOVERY: "Support",
  WAREHOUSING: "Warehouse",
  FULFILMENT: "Warehouse",
  PRIVATE_LOGISTICS: "Support",
};

const CATEGORY_THEME: Record<string, { bg: string; text: string }> = {
  Transport: { bg: "bg-primary-bg", text: "text-primary" },
  Warehouse: { bg: "bg-amber-bg", text: "text-amber" },
  Specialized: { bg: "bg-accent-bg", text: "text-accent" },
  Support: { bg: "bg-green-bg", text: "text-green" },
};

function IconTile({ icon, muted = false }: { icon: React.ReactNode; muted?: boolean }) {
  if (muted) {
    return (
      <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-surface-2 text-ink-400">
        {icon}
      </div>
    );
  }
  return (
    <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg bg-primary text-white">
      {icon}
    </div>
  );
}

function Toggle({ checked, onChange }: { checked: boolean; onChange: () => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      onClick={onChange}
      className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors ${
        checked ? "bg-primary" : "bg-line"
      }`}
    >
      <span
        className={`inline-block h-5 w-5 transform rounded-full bg-white shadow-sm transition-transform ${
          checked ? "translate-x-5" : "translate-x-0.5"
        }`}
      />
    </button>
  );
}

export default function CapabilitiesPage() {
  const router = useRouter();
  const { _hasHydrated } = useAuthStore();
  const { organization } = useOrganizationStore();
  const { setStageComplete } = useOnboardingStore();

  const [loading, setLoading] = useState(true);
  const [capabilities, setCapabilities] = useState<Capability[]>([]);
  const [isSaving, setIsSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!_hasHydrated) return;

    const fetchCapabilities = async () => {
      if (!organization?.organizationId) {
        setLoading(false);
        return;
      }

      try {
        const statuses = await organizationService.getCapabilities(organization.organizationId);

        const merged: Capability[] = Object.keys(CAPABILITY_METADATA).map((id) => {
          const status = statuses[id] || "NOT_REQUESTED";
          const enabled = status === "REQUESTED" || status === "APPROVED";

          return {
            id,
            name: CAPABILITY_METADATA[id].name,
            icon: CAPABILITY_METADATA[id].icon,
            enabled,
            status,
          };
        });

        setCapabilities(merged);
      } catch (err) {
        console.error("Failed to fetch capabilities:", err);
        const fallback: Capability[] = Object.keys(CAPABILITY_METADATA).map((id) => ({
          id,
          name: CAPABILITY_METADATA[id].name,
          icon: CAPABILITY_METADATA[id].icon,
          enabled: false,
          status: "NOT_REQUESTED",
        }));
        setCapabilities(fallback);
      } finally {
        setLoading(false);
      }
    };

    fetchCapabilities();
  }, [_hasHydrated, organization?.organizationId]);

  const handleToggle = (id: string) => {
    setCapabilities((prev) =>
      prev.map((cap) => (cap.id === id ? { ...cap, enabled: !cap.enabled } : cap))
    );
  };

  const handleSave = async () => {
    if (!organization?.organizationId) {
      setSaveError("Organization not found.");
      return;
    }

    setIsSaving(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      const enabledIds = capabilities.filter((c) => c.enabled).map((c) => c.id);

      await organizationService.updateCapabilities(organization.organizationId, enabledIds);

      setStageComplete("capabilities");

      setSaveSuccess(true);
      setTimeout(() => {
        router.push("/setup/headquarters");
      }, 1000);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to save capabilities.";
      setSaveError(message);
    } finally {
      setIsSaving(false);
    }
  };

  const handleBack = () => router.push("/setup/authorized-representative");

  const enabledCount = capabilities.filter((c) => c.enabled).length;

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
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
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Operating Capabilities</span>
        </p>
        <h1 className="mt-1 text-ink-950">Operating Capabilities</h1>
        <p className="mt-1.5 text-ink-600">
          Enable the logistics capabilities your company will offer.
        </p>
      </div>

      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
        <p className="flex-shrink-0 text-sm font-medium text-ink-700">
          <span className="font-mono tabular-nums text-ink-950">{enabledCount}</span>
          <span className="text-ink-400"> / {capabilities.length}</span> capabilities enabled
        </p>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-2 sm:max-w-xs">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${(enabledCount / capabilities.length) * 100}%` }}
          />
        </div>
      </div>

      <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
        <div className="flex items-start gap-4">
          <IconTile icon={<Package />} />
          <div>
            <h2 className="text-[15px] font-semibold text-ink-950">Select Capabilities</h2>
            <p className="text-sm text-ink-500">
              Toggle the logistics services your company will provide.
            </p>
          </div>
        </div>

        <div className="mt-6 grid grid-cols-1 gap-x-6 gap-y-5 sm:grid-cols-2">
          {capabilities.map((cap) => {
            const category = CATEGORIES[cap.id] || "Support";
            const theme = CATEGORY_THEME[category] || CATEGORY_THEME.Support;

            return (
              <div
                key={cap.id}
                className={`rounded-xl border p-4 transition-colors ${
                  cap.enabled ? "border-primary/30 bg-primary-bg/40" : "border-line bg-surface"
                }`}
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex min-w-0 items-start gap-3">
                    <IconTile icon={cap.icon} muted={!cap.enabled} />
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-semibold text-ink-950">{cap.name}</span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[9px] font-semibold uppercase tracking-wide ${theme.bg} ${theme.text}`}
                        >
                          {category}
                        </span>
                      </div>
                    </div>
                  </div>
                  <Toggle checked={cap.enabled} onChange={() => handleToggle(cap.id)} />
                </div>
              </div>
            );
          })}
        </div>
      </div>

      {saveError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <span>{saveError}</span>
        </div>
      )}

      {saveSuccess && (
        <div className="flex items-center gap-2 rounded-xl border border-green/20 bg-green-bg/40 px-4 py-3 text-sm text-green">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          Capabilities saved successfully! Redirecting...
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
          onClick={handleSave}
          disabled={isSaving || saveSuccess}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            isSaving || saveSuccess
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