// src/app/(dashboard)/settings/security/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Save,
  Shield,
  Clock,
} from "lucide-react";

// ==================== TYPES ====================

type SecuritySetting = {
  id: string;
  name: string;
  description: string;
  category: "Authentication" | "Session" | "Access" | "Documents" | "Audit" | "Retention";
  value: string | boolean | number;
  type: "toggle" | "select" | "number";
  options?: string[];
  defaultValue: string | boolean | number;
  platformMin?: string;
  platformMax?: string;
  isOverride: boolean;
  canOverride: boolean;
};

// ==================== MOCK DATA ====================

const MOCK_SECURITY_SETTINGS: SecuritySetting[] = [
  {
    id: "SS-001",
    name: "Sensitive Action Verification",
    description: "Require fresh verification for high-risk actions.",
    category: "Authentication",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-002",
    name: "Verification Freshness Window",
    description: "How long a verification remains valid.",
    category: "Authentication",
    value: "15 minutes",
    type: "select",
    options: ["5 minutes", "15 minutes", "30 minutes", "1 hour", "2 hours"],
    defaultValue: "15 minutes",
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-003",
    name: "Session Timeout",
    description: "Maximum session idle time before automatic logout.",
    category: "Session",
    value: "8 hours",
    type: "select",
    options: ["1 hour", "4 hours", "8 hours", "12 hours", "24 hours"],
    defaultValue: "8 hours",
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-004",
    name: "Provider Access Policy",
    description: "Allow providers to access organization-managed features.",
    category: "Access",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-005",
    name: "Restricted Document Controls",
    description: "Require additional verification for restricted documents.",
    category: "Documents",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: true,
  },
  {
    id: "SS-006",
    name: "Audit Capture",
    description: "Enable detailed audit logging.",
    category: "Audit",
    value: true,
    type: "toggle",
    defaultValue: true,
    isOverride: false,
    canOverride: false,
  },
  {
    id: "SS-007",
    name: "Audit Retention Period",
    description: "How long audit logs are retained.",
    category: "Audit",
    value: "90 days",
    type: "select",
    options: ["30 days", "90 days", "180 days", "365 days", "3 years", "7 years"],
    defaultValue: "90 days",
    isOverride: false,
    canOverride: false,
    platformMin: "90 days",
  },
];

type CategoryColor = {
  bg: string;
  text: string;
};

const CATEGORY_COLORS: Record<string, CategoryColor> = {
  Authentication: { bg: "bg-blue-bg", text: "text-blue" },
  Session: { bg: "bg-purple-bg", text: "text-purple" },
  Access: { bg: "bg-green-bg", text: "text-green" },
  Documents: { bg: "bg-amber-bg", text: "text-amber" },
  Audit: { bg: "bg-red-bg", text: "text-red" },
  Retention: { bg: "bg-surface-2", text: "text-ink-500" },
};

export default function SecuritySettingsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<SecuritySetting[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isSaving, setIsSaving] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SECURITY_SETTINGS);
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
      prev.map((setting) =>
        setting.id === id && setting.type === "toggle"
          ? { ...setting, value: !setting.value }
          : setting
      )
    );
  };

  const handleSelect = (id: string, value: string) => {
    setData((prev) =>
      prev.map((setting) =>
        setting.id === id && setting.type === "select"
          ? { ...setting, value }
          : setting
      )
    );
  };

  const handleNumberChange = (id: string, value: number) => {
    setData((prev) =>
      prev.map((setting) =>
        setting.id === id && setting.type === "number"
          ? { ...setting, value }
          : setting
      )
    );
  };

  const handleSave = async () => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setIsSaving(false);
    toast("Security settings updated successfully! (mock)");
  };

  const handleReset = () => {
    setData(MOCK_SECURITY_SETTINGS);
    toast("Settings reset to default (mock)");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading security settings...</p>
      </div>
    );
  }

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Security Settings
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Sensitive-action verification requirement, freshness window, session policy, provider access policy, restricted-document controls, audit capture and retention preference.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
        </div>
      </div>

      {/* ===== SETTINGS LIST ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="divide-y divide-line">
          {data.map((setting) => {
            const categoryStyle = CATEGORY_COLORS[setting.category] || CATEGORY_COLORS.Retention;

            return (
              <div key={setting.id} className="p-4 hover:bg-surface-2 transition-colors">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex-1 min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium text-ink-950">{setting.name}</span>
                      <span className={`rounded-full px-2 py-0.5 text-[9px] font-medium ${categoryStyle.bg} ${categoryStyle.text}`}>
                        {setting.category}
                      </span>
                      {!setting.canOverride && (
                        <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                          Platform Default
                        </span>
                      )}
                    </div>
                    <div className="mt-0.5 text-xs text-ink-500">{setting.description}</div>
                    {setting.platformMin && (
                      <div className="mt-0.5 text-[10px] text-ink-400">
                        Platform minimum: {setting.platformMin}
                      </div>
                    )}
                  </div>
                  <div className="flex-shrink-0">
                    {setting.type === "toggle" ? (
                      <button
                        onClick={() => handleToggle(setting.id)}
                        disabled={!setting.canOverride}
                        className={`relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full transition-colors ${
                          setting.value ? "bg-primary" : "bg-ink-300"
                        } ${!setting.canOverride ? "opacity-50 cursor-not-allowed" : ""}`}
                      >
                        <span
                          className={`inline-block h-5 w-5 transform rounded-full bg-white shadow transition-transform ${
                            setting.value ? "translate-x-5" : "translate-x-0.5"
                          } mt-0.5`}
                        />
                      </button>
                    ) : setting.type === "select" ? (
                      <select
                        value={setting.value as string}
                        onChange={(e) => handleSelect(setting.id, e.target.value)}
                        disabled={!setting.canOverride}
                        className={`rounded-xl border border-line px-3 py-1.5 text-sm text-ink-950 outline-none focus:border-primary ${
                          !setting.canOverride ? "opacity-50 cursor-not-allowed" : ""
                        }`}
                      >
                        {setting.options?.map((opt) => (
                          <option key={opt} value={opt}>
                            {opt}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        type="number"
                        value={typeof setting.value === 'number' ? setting.value : 0}
                        onChange={(e) => {
                          const numValue = parseFloat(e.target.value);
                          if (!isNaN(numValue)) {
                            handleNumberChange(setting.id, numValue);
                          }
                        }}
                        disabled={!setting.canOverride}
                        className={`rounded-xl border border-line px-3 py-1.5 text-sm text-ink-950 outline-none focus:border-primary ${
                          !setting.canOverride ? "opacity-50 cursor-not-allowed" : ""
                        }`}
                      />
                    )}
                  </div>
                </div>
              </div>
            );
          })}
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
              Save Settings
            </>
          )}
        </button>
        <button
          onClick={handleReset}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Reset to Defaults
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          SEC-01 · Security Settings
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