// src/app/(dashboard)/settings/retention/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Database,
  RefreshCw,
  Play,
  Check,
  X,
  Edit3,
  AlertCircle,
  Clock,
  ArrowUp,
} from "lucide-react";

// ==================== TYPES ====================

type RetentionClass = {
  id: string;
  name: string;
  description: string;
  platformMin: string;
  companyEffective: string;
  overrideStatus: "Default" | "Extended";
  canOverride: boolean;
  recordsAffected: number;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_RETENTION_CLASSES: RetentionClass[] = [
  {
    id: "RC-001",
    name: "Regulatory",
    description: "Records required for regulatory compliance.",
    platformMin: "5 years",
    companyEffective: "7 years",
    overrideStatus: "Extended",
    canOverride: true,
    recordsAffected: 1243,
    tags: ["Regulatory", "Compliance"],
  },
  {
    id: "RC-002",
    name: "Financial",
    description: "Financial and accounting records.",
    platformMin: "5 years",
    companyEffective: "5 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 892,
    tags: ["Financial", "Accounting"],
  },
  {
    id: "RC-003",
    name: "Operational",
    description: "Operational logs and trip records.",
    platformMin: "3 years",
    companyEffective: "3 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 5671,
    tags: ["Operational", "Logs"],
  },
  {
    id: "RC-004",
    name: "Staff",
    description: "Staff and employment records.",
    platformMin: "3 years",
    companyEffective: "3 years",
    overrideStatus: "Default",
    canOverride: true,
    recordsAffected: 234,
    tags: ["Staff", "HR"],
  },
  {
    id: "RC-005",
    name: "Provider",
    description: "Provider and contractor records.",
    platformMin: "3 years",
    companyEffective: "5 years",
    overrideStatus: "Extended",
    canOverride: true,
    recordsAffected: 156,
    tags: ["Provider", "Contractor"],
  },
  {
    id: "RC-006",
    name: "Audit",
    description: "Audit and compliance review records.",
    platformMin: "7 years",
    companyEffective: "7 years",
    overrideStatus: "Default",
    canOverride: false,
    recordsAffected: 456,
    tags: ["Audit", "Compliance"],
  },
];

type OverrideStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const OVERRIDE_STYLES: Record<string, OverrideStyle> = {
  Default: { bg: "bg-surface-2", text: "text-ink-500", icon: <Check className="h-3.5 w-3.5" /> },
  Extended: { bg: "bg-amber-bg", text: "text-amber", icon: <ArrowUp className="h-3.5 w-3.5" /> },
};

export default function RetentionPoliciesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<RetentionClass[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editValue, setEditValue] = useState<string>("");
  const [isSaving, setIsSaving] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_RETENTION_CLASSES);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleEdit = (id: string, currentValue: string) => {
    setEditingId(id);
    setEditValue(currentValue);
  };

  const handleSave = async (id: string) => {
    setIsSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 800));
    setData((prev) =>
      prev.map((item) =>
        item.id === id
          ? { ...item, companyEffective: editValue, overrideStatus: editValue === item.platformMin ? "Default" : "Extended" }
          : item
      )
    );
    setEditingId(null);
    setEditValue("");
    setIsSaving(false);
    toast("Retention policy updated (mock)");
  };

  const handleCancel = () => {
    setEditingId(null);
    setEditValue("");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading retention policies...</p>
      </div>
    );
  }

  const goToRunSetup = () => {
    router.push("/settings/retention/run");
  };

  // Stats
  const extendedCount = data.filter((d) => d.overrideStatus === "Extended").length;
  const totalRecords = data.reduce((acc, d) => acc + d.recordsAffected, 0);

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Retention Policies
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Retention classes, platform minimum, company effective duration and override status.
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
          <button
            onClick={goToRunSetup}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Play className="h-3.5 w-3.5" />
            Run Retention
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Classes</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Extended</div>
          <div className="text-lg font-bold text-amber">{extendedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Records Affected</div>
          <div className="text-lg font-bold text-ink-950">{totalRecords.toLocaleString()}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Platform Defaults</div>
          <div className="text-lg font-bold text-green">{data.length - extendedCount}</div>
        </div>
      </div>

      {/* ===== POLICIES TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Class</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Description</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Platform Min</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Company Effective</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Records</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
              </tr>
            </thead>
            <tbody>
              {data.map((item) => {
                const overrideStyle = OVERRIDE_STYLES[item.overrideStatus] || OVERRIDE_STYLES.Default;

                return (
                  <tr key={item.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950">{item.name}</div>
                      {item.tags.map((tag) => (
                        <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500 mr-1">
                          {tag}
                        </span>
                      ))}
                    </td>
                    <td className="px-4 py-3 text-ink-600 text-xs">{item.description}</td>
                    <td className="px-4 py-3 text-ink-600 text-xs font-mono">{item.platformMin}</td>
                    <td className="px-4 py-3">
                      {editingId === item.id ? (
                        <div className="flex items-center gap-2">
                          <input
                            type="text"
                            value={editValue}
                            onChange={(e) => setEditValue(e.target.value)}
                            className="rounded-lg border border-line px-2 py-1 text-sm text-ink-950 outline-none focus:border-primary w-24"
                          />
                          <button
                            onClick={() => handleSave(item.id)}
                            disabled={isSaving}
                            className="text-primary hover:text-primary/80 transition-colors"
                          >
                            <Check className="h-4 w-4" />
                          </button>
                          <button
                            onClick={handleCancel}
                            className="text-ink-400 hover:text-ink-600 transition-colors"
                          >
                            <X className="h-4 w-4" />
                          </button>
                        </div>
                      ) : (
                        <span className="font-medium text-ink-950">{item.companyEffective}</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${overrideStyle.bg} ${overrideStyle.text}`}>
                        {overrideStyle.icon}
                        {item.overrideStatus}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-600 text-xs">{item.recordsAffected.toLocaleString()}</td>
                    <td className="px-4 py-3 text-right">
                      {item.canOverride && editingId !== item.id && (
                        <button
                          onClick={() => handleEdit(item.id, item.companyEffective)}
                          className="text-ink-400 hover:text-primary transition-colors"
                        >
                          <Edit3 className="h-4 w-4" />
                        </button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== NOTE ===== */}
      <div className="rounded-xl border border-amber/20 bg-amber-bg/30 p-4">
        <div className="flex items-start gap-2">
          <AlertCircle className="h-4 w-4 text-amber flex-shrink-0 mt-0.5" />
          <p className="text-xs text-amber/80">
            <span className="font-semibold">Note:</span> Company may extend but not shorten platform minimum retention periods. Legal holds override retention policies.
          </p>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Database className="h-4 w-4" />
          SEC-07 · Retention Policies
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.length} classes
        </span>
      </div>
    </div>
  );
}

function toast(message: string) {
  console.log(message);
}