// src/app/(setup)/setup/capabilities/page.tsx
// Screen: ORG04 - Operating capabilities
// Redesigned to match the visual language of ORG03 (Company Profile):
// navy wave hero, shared 3D icon-tile recipe, real brand tokens only,
// minimal card shadows, fully responsive.

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  AlertTriangle,
  Bike,
  Truck,
  Warehouse,
  Snowflake,
  Globe,
  Lock,
  ChevronLeft,
} from "lucide-react";

// ==================== TYPES ====================

type Category = "Transport" | "Warehouse" | "Specialized" | "Support";

type Capability = {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  available: boolean;
  icon: React.ReactNode;
  category: Category;
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
    icon: <Bike />,
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
    icon: <Truck />,
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
    icon: <Warehouse />,
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
    icon: <Snowflake />,
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
    icon: <Truck />,
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
    icon: <Globe />,
    category: "Specialized",
    branchAvailability: ["All branches"],
    configurable: true,
  },
];

// ==================== STYLE MAPS ====================
// Reused from the Company Profile page so both screens read as one product.

const TILE_SIZE = {
  sm: "h-9 w-9 rounded-lg [&>svg]:h-4 [&>svg]:w-4",
  md: "h-11 w-11 rounded-xl [&>svg]:h-5 [&>svg]:w-5",
  lg: "h-12 w-12 rounded-xl [&>svg]:h-5 [&>svg]:w-5",
} as const;

const TILE_THEME = {
  brand: "from-primary to-accent shadow-primary/25",
  accent: "from-accent to-accent-hover shadow-accent/25",
  green: "from-green to-green shadow-green/25",
  amber: "from-amber to-amber shadow-amber/25",
  red: "from-red to-red shadow-red/25",
  ink: "from-ink-400 to-ink-600 shadow-ink-500/20",
} as const;

// No dedicated "purple"/"blue" tokens exist in the design system, so
// categories map onto the tokens that already exist rather than
// introducing arbitrary colors.
const CATEGORY_THEME: Record<Category, { tile: keyof typeof TILE_THEME; bg: string; text: string }> = {
  Transport: { tile: "brand", bg: "bg-primary-bg", text: "text-primary" },
  Warehouse: { tile: "amber", bg: "bg-amber-bg", text: "text-amber" },
  Specialized: { tile: "accent", bg: "bg-accent-bg", text: "text-accent" },
  Support: { tile: "green", bg: "bg-green-bg", text: "text-green" },
};

// ==================== SMALL COMPONENTS ====================

function IconTile({
  icon,
  theme = "brand",
  size = "md",
  muted = false,
}: {
  icon: React.ReactNode;
  theme?: keyof typeof TILE_THEME;
  size?: keyof typeof TILE_SIZE;
  muted?: boolean;
}) {
  if (muted) {
    return (
      <div className={`flex flex-shrink-0 items-center justify-center bg-surface-2 text-ink-400 ${TILE_SIZE[size]}`}>
        {icon}
      </div>
    );
  }

  return (
    <div
      className={`relative flex flex-shrink-0 items-center justify-center overflow-hidden bg-gradient-to-br text-white ring-1 ring-black/5 ${TILE_THEME[theme]} ${TILE_SIZE[size]}`}
      style={{
        boxShadow: "inset 0 1px 0 rgba(255,255,255,0.4), 0 4px 10px -4px rgba(0,0,0,0.22)",
      }}
    >
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background: "radial-gradient(120% 100% at 25% 12%, rgba(255,255,255,0.35), transparent 55%)",
          mixBlendMode: "overlay",
        }}
      />
      <span className="relative">{icon}</span>
    </div>
  );
}

function Toggle({ checked, disabled, onChange }: { checked: boolean; disabled?: boolean; onChange: () => void }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={onChange}
      className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors ${
        disabled ? "cursor-not-allowed bg-surface-2" : checked ? "bg-primary" : "bg-line"
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

// ==================== MAIN PAGE ====================

export default function CapabilitiesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<Capability[]>([]);
  const [isSaving, setIsSaving] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_CAPABILITIES);
      setLoading(false);
    };
    loadData();
  }, []);

  const handleToggle = (id: string) => {
    setData((prev) => prev.map((cap) => (cap.id === id ? { ...cap, enabled: !cap.enabled } : cap)));
  };

  const handleSave = async () => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1000));
    setIsSaving(false);
    toast("Capabilities updated successfully! (mock)");
    router.push("/setup/profile");
  };

  const handleBack = () => router.push("/setup/profile");

  const enabledCount = data.filter((c) => c.enabled).length;

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
        <div className="space-y-2">
          <div className="h-7 w-48 animate-pulse rounded bg-surface-2 sm:w-56" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="h-32 animate-pulse rounded-2xl bg-surface-2 sm:h-36" />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-32 animate-pulse rounded-2xl bg-surface-2" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      {/* ===== HEADER ===== */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Operating Capabilities</span>
        </p>
        <h1 className="mt-1 text-ink-950">Operating Capabilities</h1>
        <p className="mt-1.5 text-ink-600">Enable the logistics capabilities your company will offer.</p>
      </div>

      {/* ===== STATUS STRIP ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
        <p className="flex-shrink-0 text-sm font-medium text-ink-700">
          <span className="font-mono tabular-nums text-ink-950">{enabledCount}</span>
          <span className="text-ink-400"> / {data.length}</span> capabilities enabled
        </p>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-2 sm:max-w-xs">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${(enabledCount / data.length) * 100}%` }}
          />
        </div>
      </div>

      {/* ===== CAPABILITIES GRID ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-6">
        {data.map((cap) => {
          const theme = CATEGORY_THEME[cap.category];
          const isOn = cap.enabled && cap.available;

          return (
            <div
              key={cap.id}
              className={`rounded-2xl border p-5 transition-colors ${
                isOn ? "border-primary/30 bg-primary-bg/40" : "border-line bg-surface"
              } ${!cap.available ? "opacity-70" : ""}`}
            >
              <div className="flex items-start justify-between gap-3">
                <div className="flex min-w-0 items-start gap-3">
                  <IconTile icon={cap.icon} theme={theme.tile} muted={!cap.available} />
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="text-sm font-semibold text-ink-950 sm:text-base">{cap.name}</span>
                      <span
                        className={`rounded-full px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${theme.bg} ${theme.text}`}
                      >
                        {cap.category}
                      </span>
                    </div>
                    <p className="mt-0.5 text-xs text-ink-500 sm:text-sm">{cap.description}</p>
                  </div>
                </div>
                <Toggle checked={cap.enabled} disabled={!cap.available} onChange={() => handleToggle(cap.id)} />
              </div>

             {!cap.available ? (
  <div className="mt-3 flex items-center gap-1.5 border-t border-line pt-3 text-xs text-ink-500">
    <Lock className="h-3.5 w-3.5" />
    Not available for your company yet
  </div>
) : null}
            </div>
          );
        })}
      </div>

      {/* ===== TAXI NOTICE ===== */}
      <div className="rounded-2xl border border-amber/20 bg-amber-bg/40 p-5">
        <div className="flex items-start gap-3">
          <IconTile icon={<AlertTriangle />} theme="amber" size="md" />
          <div className="min-w-0">
            <h4 className="text-sm font-semibold text-amber">Taxi Production Not Available</h4>
            <p className="mt-1 text-sm text-amber/80">
              Taxi may exist as a configuration/data reference, but production execution is not available in Release 1.
            </p>
          </div>
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 transition hover:bg-surface-2"
        >
          <ChevronLeft className="h-4 w-4" />
          Back
        </button>
        <button
          onClick={handleSave}
          disabled={isSaving}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            isSaving ? "cursor-not-allowed bg-ink-400" : "bg-primary hover:bg-primary-hover"
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
              Save 
            </>
          )}
        </button>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}