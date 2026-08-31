// src/app/(setup)/setup/capabilities/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  AlertTriangle,
  Briefcase,
  Clock,
  Bike,
  Truck,
  Warehouse,
  Snowflake,
  Globe,
} from "lucide-react";

// ==================== TYPES ====================

type Capability = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  available: boolean;
  icon: React.ReactNode;
  category: "Transport" | "Warehouse" | "Specialized" | "Support";
  branchAvailability: string[];
  configurable: boolean;
};

// ==================== MOCK DATA ====================

const MOCK_CAPABILITIES: Capability[] = [
  {
    id: "dispatch",
    name: "Dispatch",
    description: "Last-mile delivery and courier services.",
    enabled: true,
    available: true,
    icon: <Bike className="h-5 w-5" />,
    category: "Transport",
    branchAvailability: ["All branches"],
    configurable: true,
  },
  {
    id: "hauling",
    name: "Hauling",
    description: "FTL/LTL trucking and freight services.",
    enabled: true,
    available: true,
    icon: <Truck className="h-5 w-5" />,
    category: "Transport",
    branchAvailability: ["All branches"],
    configurable: true,
  },
  {
    id: "warehouse",
    name: "Warehouse",
    description: "Warehouse storage and fulfillment services.",
    enabled: false,
    available: false,
    icon: <Warehouse className="h-5 w-5" />,
    category: "Warehouse",
    branchAvailability: ["Ikeja", "Lekki"],
    configurable: true,
  },
  {
    id: "cold-chain",
    name: "Cold Chain",
    description: "Temperature-controlled logistics.",
    enabled: false,
    available: false,
    icon: <Snowflake className="h-5 w-5" />,
    category: "Specialized",
    branchAvailability: ["Ikeja"],
    configurable: true,
  },
  {
    id: "heavy-transport",
    name: "Heavy Transport",
    description: "Specialized heavy equipment transport.",
    enabled: false,
    available: false,
    icon: <Truck className="h-5 w-5" />,
    category: "Specialized",
    branchAvailability: ["Apapa"],
    configurable: true,
  },
  {
    id: "cross-border",
    name: "Cross-Border",
    description: "Freight forwarding and customs clearance.",
    enabled: false,
    available: false,
    icon: <Globe className="h-5 w-5" />,
    category: "Specialized",
    branchAvailability: ["All branches"],
    configurable: true,
  },
];

type CategoryStyle = {
  bg: string;
  text: string;
};

const CATEGORY_STYLES: Record<string, CategoryStyle> = {
  Transport: { bg: "bg-blue-bg", text: "text-blue" },
  Warehouse: { bg: "bg-amber-bg", text: "text-amber" },
  Specialized: { bg: "bg-purple-bg", text: "text-purple" },
  Support: { bg: "bg-green-bg", text: "text-green" },
};

export default function CapabilitiesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<Capability[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isSaving, setIsSaving] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_CAPABILITIES);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleToggle = (id: string) => {
    setData((prev) =>
      prev.map((cap) =>
        cap.id === id ? { ...cap, enabled: !cap.enabled } : cap
      )
    );
  };

  const handleSave = async () => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1000));
    setIsSaving(false);
    toast("Capabilities updated successfully! (mock)");
    router.push("/setup/profile");
  };

  const handleBack = () => {
    router.push("/setup");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading capabilities...</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button onClick={handleBack} className="hover:text-primary transition-colors">
            Onboarding
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Operating Capabilities</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Operating Capabilities
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Enable the logistics capabilities your company will offer.
        </p>
      </div>

      {/* ===== CAPABILITIES GRID ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {data.map((cap) => {
          const categoryStyle = CATEGORY_STYLES[cap.category] || CATEGORY_STYLES.Transport;

          return (
            <div
              key={cap.id}
              className={`rounded-2xl border p-5 transition-all ${
                cap.enabled
                  ? "border-primary/30 bg-primary-bg/10 shadow-sm"
                  : "border-line bg-white hover:bg-surface-2"
              }`}
            >
              <div className="flex items-start justify-between">
                <div className="flex items-center gap-3">
                  <div
                    className={`rounded-full p-2 ${
                      cap.enabled ? "bg-primary-bg text-primary" : "bg-surface-2 text-ink-400"
                    }`}
                  >
                    {cap.icon}
                  </div>
                  <div>
                    <div className="flex items-center gap-2">
                      <span className="font-semibold text-ink-950">{cap.name}</span>
                      <span
                        className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${categoryStyle.bg} ${categoryStyle.text}`}
                      >
                        {cap.category}
                      </span>
                    </div>
                    <div className="text-xs text-ink-500">{cap.description}</div>
                  </div>
                </div>
                <button
                  onClick={() => handleToggle(cap.id)}
                  className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors ${
                    cap.enabled ? "bg-primary" : "bg-ink-300"
                  }`}
                >
                  <span
                    className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                      cap.enabled ? "translate-x-5" : "translate-x-0.5"
                    } mt-0.5`}
                  />
                </button>
              </div>

              {cap.enabled && (
                <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-3">
                  <span className="text-xs text-ink-500">Available at:</span>
                  {cap.branchAvailability.map((branch) => (
                    <span
                      key={branch}
                      className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-600"
                    >
                      {branch}
                    </span>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </div>

      {/* ===== TAXI NOTICE ===== */}
      <div className="rounded-2xl border border-amber/20 bg-amber-bg/30 p-5 shadow-sm">
        <div className="flex items-start gap-3">
          <div className="rounded-full bg-amber-bg p-2 flex-shrink-0">
            <AlertTriangle className="h-5 w-5 text-amber" />
          </div>
          <div>
            <h4 className="text-sm font-semibold text-amber">Taxi Production Not Available</h4>
            <p className="mt-1 text-sm text-amber/80">
              Taxi may exist as configuration/data reference but production execution is not available in Release 1.
            </p>
          </div>
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={handleSave}
          disabled={isSaving}
          className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
            isSaving
              ? "bg-ink-400 cursor-not-allowed"
              : "bg-primary shadow-primary/20 hover:bg-primary/90"
          }`}
        >
          {isSaving ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : (
            <>
              <Save className="h-4 w-4" />
              Save Capabilities
            </>
          )}
        </button>
        <button
          onClick={handleBack}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Back
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Briefcase className="h-4 w-4" />
          ORG-04 · Operating Capabilities
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}