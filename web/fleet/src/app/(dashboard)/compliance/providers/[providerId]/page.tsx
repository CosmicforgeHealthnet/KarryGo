// src/app/(dashboard)/compliance/providers/[providerId]/page.tsx

"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  FileText,
  Calendar,
  ChevronDown,
  ChevronUp,
  AlertOctagon,
  Search,
  ListChecks,
  Info,
} from "lucide-react";

import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { providerService } from "@/lib/services/providerService";
import { complianceService, complianceStatusService } from "@/lib/services/safetyService";
import type { ProviderMembership } from "@/lib/types/provider";
import type { ComplianceRequirement, ComplianceStatus, Restriction } from "@/lib/types/safety";

// One row per requirement this provider was evaluated against: the status the
// compliance service recorded, labelled with the requirement definition.
type RequirementRow = {
  id: string;
  name: string;
  reference: string;
  category: string;
  status: string;
  dueDate: string | null;
  source: string;
  sourceReference: string;
  restrictionReference: string;
  restrictionImpact: boolean;
  reasons: string[];
  evaluatedAt: string;
};

const reasonList = (value: unknown): string[] => {
  if (Array.isArray(value)) return value.filter((v): v is string => typeof v === "string");
  if (value && typeof value === "object") return Object.keys(value as Record<string, unknown>);
  return [];
};

const toRow = (status: ComplianceStatus, requirement?: ComplianceRequirement): RequirementRow => ({
  id: status.id,
  name: requirement ? requirement.name || requirement.code : status.requirementId,
  reference: requirement?.code || status.requirementId,
  category: requirement?.requirementType || "",
  status: status.state,
  dueDate: status.expiresAt ?? status.dueAt,
  source: status.sourceType,
  sourceReference: status.sourceReference,
  restrictionReference: status.restrictionReference,
  restrictionImpact: Boolean(status.restrictionReference) || Boolean(requirement?.blocksOperation && status.state !== "COMPLIANT"),
  reasons: reasonList(status.reasonCodes),
  evaluatedAt: status.evaluatedAt,
});

const NEEDS_ACTION = ["NON_COMPLIANT", "RESTRICTED"];

// The worst state across the provider's requirements.
const overallOf = (rows: RequirementRow[]): string => {
  if (rows.length === 0) return "UNKNOWN";
  if (rows.some((r) => NEEDS_ACTION.includes(r.status) || r.status === "EXPIRED")) return "NON_COMPLIANT";
  if (rows.some((r) => r.status === "DUE_SOON")) return "DUE_SOON";
  if (rows.some((r) => r.status === "PENDING" || r.status === "UNKNOWN")) return "PENDING";
  return "COMPLIANT";
};

const REQUIREMENT_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  COMPLIANT: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  DUE_SOON: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <AlertTriangle className="h-3.5 w-3.5" />,
  },
  NON_COMPLIANT: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  PENDING: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  EXPIRED: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  RESTRICTED: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertOctagon className="h-3.5 w-3.5" />,
  },
  UNKNOWN: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

const CATEGORY_STYLES: Record<string, { bg: string; text: string }> = {
  DOCUMENT: { bg: "bg-blue-bg", text: "text-blue" },
  QUALIFICATION: { bg: "bg-purple-bg", text: "text-purple" },
  TRAINING: { bg: "bg-green-bg", text: "text-green" },
  INSPECTION: { bg: "bg-amber-bg", text: "text-amber" },
};

const RESTRICTION_SEVERITY_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  CRITICAL: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
  HIGH: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  MEDIUM: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  LOW: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const RESTRICTION_CLEARANCE_STYLES: Record<string, { bg: string; text: string }> = {
  ACTIVE: { bg: "bg-red-bg", text: "text-red" },
  CLEAR_PENDING: { bg: "bg-amber-bg", text: "text-amber" },
  CLEARED: { bg: "bg-green-bg", text: "text-green" },
  EXPIRED: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "COMPLIANT", "DUE_SOON", "PENDING", "EXPIRED", "NON_COMPLIANT", "RESTRICTED", "UNKNOWN"];

export default function ProviderCompliancePage() {
  const params = useParams();
  const router = useRouter();
  const providerId = params.providerId as string;

  const [loading, setLoading] = useState(true);
  const [provider, setProvider] = useState<ProviderMembership | null>(null);
  const [requirements, setRequirements] = useState<RequirementRow[] | null>(null);
  const [restrictions, setRestrictions] = useState<Restriction[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [loadedAt, setLoadedAt] = useState(0);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [statusFilter, setStatusFilter] = useState("All");
  const [categoryFilter, setCategoryFilter] = useState("All");
  const [search, setSearch] = useState("");
  const [expandedRequirements, setExpandedRequirements] = useState<Set<string>>(new Set());
  const [showRestrictions, setShowRestrictions] = useState(true);
  const [showRequirements, setShowRequirements] = useState(true);

  const load = useCallback(async () => {
    try {
      const [statuses, definitions] = await Promise.all([
        complianceStatusService.forProvider(providerId),
        complianceStatusService.requirements().catch(() => [] as ComplianceRequirement[]),
      ]);
      setRequirements(statuses.map((st) => toRow(st, definitions.find((d) => d.id === st.requirementId))));
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Provider compliance could not be loaded."));
    } finally {
      setLoadedAt(Date.now());
      setSecondsAgo(0);
      setLoading(false);
    }
    providerService.get(providerId).then(setProvider).catch(() => setProvider(null));
    complianceService
      .listRestrictions({ subjectType: "PROVIDER", subjectReference: providerId })
      .then(setRestrictions)
      .catch(() => setRestrictions([]));
  }, [providerId]);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading provider compliance data...</p>
      </div>
    );
  }

  if (!requirements) {
    return (
      <div className="space-y-4">
      <ApiErrorNotice error={error} />
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Shield className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Provider not found</h3>
        <p className="mt-1 text-sm text-ink-500">No compliance data available for this provider.</p>
        <button
          onClick={() => router.push("/compliance")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Compliance
        </button>
      </div>
      </div>
    );
  }

  const data = {
    providerId,
    requirements,
    restrictions,
    overallStatus: overallOf(requirements),
    complianceScore: requirements.length === 0 ? 0 : Math.round((requirements.filter((r) => r.status === "COMPLIANT").length / requirements.length) * 100),
    lastAssessment: requirements.reduce<string | null>((latest, r) => (!latest || r.evaluatedAt > latest ? r.evaluatedAt : latest), null),
    summary: {
      total: requirements.length,
      compliant: requirements.filter((r) => r.status === "COMPLIANT").length,
      warning: requirements.filter((r) => r.status === "DUE_SOON").length,
      nonCompliant: requirements.filter((r) => NEEDS_ACTION.includes(r.status)).length,
      pending: requirements.filter((r) => r.status === "PENDING" || r.status === "UNKNOWN").length,
      expired: requirements.filter((r) => r.status === "EXPIRED").length,
    },
  };
  const categories = ["All", ...Array.from(new Set(requirements.map((r) => r.category).filter(Boolean)))];
  const providerLabel = provider?.employeeNumber || providerId;

  const toggleRequirement = (id: string) => {
    const newSet = new Set(expandedRequirements);
    if (newSet.has(id)) {
      newSet.delete(id);
    } else {
      newSet.add(id);
    }
    setExpandedRequirements(newSet);
  };

  const filteredRequirements = data.requirements.filter((req) => {
    const matchesStatus = statusFilter === "All" || req.status === statusFilter;
    const matchesCategory = categoryFilter === "All" || req.category === categoryFilter;
    const matchesSearch =
      search.trim() === "" ||
      req.name.toLowerCase().includes(search.toLowerCase()) ||
      req.reference.toLowerCase().includes(search.toLowerCase());
    return matchesStatus && matchesCategory && matchesSearch;
  });

  const overallStatusStyle = ({
    COMPLIANT: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-5 w-5" /> },
    DUE_SOON: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-5 w-5" /> },
    NON_COMPLIANT: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-5 w-5" /> },
    PENDING: { bg: "bg-blue-bg", text: "text-blue", icon: <Clock className="h-5 w-5" /> },
  } as Record<string, { bg: string; text: string; icon: React.ReactNode }>)[data.overallStatus] || { bg: "bg-surface-2", text: "text-ink-500", icon: <Shield className="h-5 w-5" /> };

  const formatDate = (date: string | null) => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  const formatTime = (date: string | null) => {
    if (!date) return "Never";
    const diffMs = loadedAt - new Date(date).getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  const getExpiryStatus = (dueDate: string | null) => {
    if (!dueDate) return { label: "No expiry", class: "text-ink-500" };
    const due = new Date(dueDate);
    const diffDays = Math.ceil((due.getTime() - loadedAt) / 86400000);

    if (diffDays < 0) return { label: "Expired", class: "text-red" };
    if (diffDays <= 7) return { label: `${diffDays} days`, class: "text-red" };
    if (diffDays <= 30) return { label: `${diffDays} days`, class: "text-amber" };
    return { label: `${diffDays} days`, class: "text-green" };
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/compliance")}
              className="hover:text-primary transition-colors"
            >
              Compliance
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{providerLabel}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Provider Compliance
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            {[provider ? humanize(provider.serviceCategory) : null, provider ? humanize(provider.relationshipType) : null, provider?.homeBranchId ? `${provider.homeBranchId} branch` : null]
              .filter(Boolean)
              .join(" · ") || providerId}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          <button
            onClick={load}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
        </div>
      </div>

      <ApiErrorNotice error={error} />

      {/* ===== OVERVIEW CARD ===== */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-4">
        {/* Status */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center gap-3">
            <div className={`rounded-full p-2.5 ${overallStatusStyle.bg}`}>
              {overallStatusStyle.icon}
            </div>
            <div>
              <div className="text-sm text-ink-500">Overall Status</div>
              <div className={`text-lg font-semibold ${overallStatusStyle.text}`}>
                {humanize(data.overallStatus)}
              </div>
            </div>
          </div>
          <div className="mt-3 flex items-center gap-2 text-xs text-ink-500">
            <Clock className="h-3.5 w-3.5" />
            <span>Last assessment: {formatTime(data.lastAssessment)}</span>
          </div>
        </div>

        {/* Compliance Score */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
          <div className="flex items-center justify-between">
            <div>
              <div className="text-sm text-ink-500">Requirements Met</div>
              <div className="text-2xl font-bold text-ink-950">{data.complianceScore}%</div>
            </div>
            <div className="relative h-14 w-14">
              <svg className="h-14 w-14 -rotate-90 transform" viewBox="0 0 56 56">
                <circle
                  cx="28"
                  cy="28"
                  r="22"
                  fill="none"
                  stroke="#E5E7EB"
                  strokeWidth="6"
                />
                <circle
                  cx="28"
                  cy="28"
                  r="22"
                  fill="none"
                  stroke={data.complianceScore >= 80 ? "#139a5d" : data.complianceScore >= 60 ? "#d97706" : "#df2f3f"}
                  strokeWidth="6"
                  strokeDasharray={`${data.complianceScore * 1.382} 138.2`}
                  strokeLinecap="round"
                />
              </svg>
              <span className="absolute inset-0 flex items-center justify-center text-[9px] font-bold text-ink-950">
                {data.complianceScore}%
              </span>
            </div>
          </div>
        </div>

        {/* Summary Stats */}
        <div className="rounded-2xl border border-line bg-white p-5 shadow-sm col-span-2">
          <div className="text-sm text-ink-500 mb-2">Requirements Summary</div>
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
            <div className="text-center">
              <div className="text-lg font-bold text-ink-950">{data.summary.total}</div>
              <div className="text-[10px] text-ink-500">Total</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-green">{data.summary.compliant}</div>
              <div className="text-[10px] text-ink-500">Compliant</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-amber">{data.summary.warning}</div>
              <div className="text-[10px] text-ink-500">Due Soon</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-red">{data.summary.nonCompliant}</div>
              <div className="text-[10px] text-ink-500">Non-Compliant</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-blue">{data.summary.pending}</div>
              <div className="text-[10px] text-ink-500">Pending</div>
            </div>
            <div className="text-center">
              <div className="text-lg font-bold text-red">{data.summary.expired}</div>
              <div className="text-[10px] text-ink-500">Expired</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== RESTRICTIONS SECTION ===== */}
      {data.restrictions.length > 0 && (
        <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
          <button
            onClick={() => setShowRestrictions(!showRestrictions)}
            className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
          >
            <div className="flex items-center gap-3">
              <div className="rounded-full bg-red-bg p-2">
                <AlertOctagon className="h-4 w-4 text-red" />
              </div>
              <div>
                <span className="font-semibold text-ink-950">Active Restrictions</span>
                <span className="ml-2 rounded-full bg-red-bg px-2 py-0.5 text-[10px] font-semibold text-red">
                  {data.restrictions.filter((r) => r.state === "ACTIVE").length} active
                </span>
              </div>
            </div>
            {showRestrictions ? (
              <ChevronUp className="h-4 w-4 text-ink-400" />
            ) : (
              <ChevronDown className="h-4 w-4 text-ink-400" />
            )}
          </button>

          {showRestrictions && (
            <div className="border-t border-line p-4">
              <div className="space-y-3">
                {data.restrictions.map((restriction) => {
                  const severityStyle = RESTRICTION_SEVERITY_STYLES[restriction.severity] || RESTRICTION_SEVERITY_STYLES.MEDIUM;
                  const clearanceStyle = RESTRICTION_CLEARANCE_STYLES[restriction.state] || RESTRICTION_CLEARANCE_STYLES.ACTIVE;

                  return (
                    <div
                      key={restriction.id}
                      className="rounded-xl border border-line p-4 hover:bg-surface-2 transition-colors"
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                              {humanize(restriction.severity)}
                            </span>
                            <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${clearanceStyle.bg} ${clearanceStyle.text}`}>
                              {humanize(restriction.state)}
                            </span>
                            <span className="text-[10px] text-ink-500">{humanize(restriction.sourceType)}</span>
                          </div>
                          <div className="mt-1 font-medium text-ink-950">{humanize(restriction.reasonCode)}</div>
                          <div className="mt-0.5 text-xs text-ink-500">
                            Source: {restriction.sourceReference || humanize(restriction.sourceType)} · Effective: {formatDate(restriction.effectiveAt)}
                          </div>
                          {restriction.clearanceReason && (
                            <div className="mt-2 flex items-start gap-1.5 text-xs text-amber">
                              <Info className="h-3.5 w-3.5 flex-shrink-0 mt-0.5" />
                              <span>{restriction.clearanceReason}</span>
                            </div>
                          )}
                        </div>
                        {restriction.state === "ACTIVE" && (
                          <button
                            onClick={() => router.push("/compliance/restrictions")}
                            className="flex-shrink-0 rounded-lg border border-line px-3 py-1.5 text-[11px] font-medium text-ink-700 hover:bg-white transition-colors"
                          >
                            Resolve
                          </button>
                        )}
                      </div>
                    </div>
                  );
                })}
              </div>
            </div>
          )}
        </div>
      )}

      {/* ===== REQUIREMENTS SECTION ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowRequirements(!showRequirements)}
          className="flex w-full items-center justify-between p-4 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-2">
              <ListChecks className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-semibold text-ink-950">Compliance Requirements</span>
              <span className="ml-2 text-sm text-ink-500">
                {filteredRequirements.length} of {data.requirements.length}
              </span>
            </div>
          </div>
          {showRequirements ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showRequirements && (
          <div className="border-t border-line">
            {/* Filters */}
            <div className="flex flex-wrap items-center gap-2 p-4 border-b border-line">
              <div className="flex flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-3.5 py-2 shadow-sm">
                <Search className="h-4 w-4 text-ink-400" />
                <input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder="Search requirements..."
                  className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
                />
              </div>

              <div className="flex flex-wrap gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setStatusFilter(status)}
                    className={`rounded-full px-3 py-1 text-[10px] font-semibold transition ${
                      statusFilter === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status === "All" ? "All" : humanize(status)}
                  </button>
                ))}
              </div>

              <div className="flex flex-wrap gap-1.5">
                {categories.map((category) => (
                  <button
                    key={category}
                    onClick={() => setCategoryFilter(category)}
                    className={`rounded-full px-3 py-1 text-[10px] font-semibold transition ${
                      categoryFilter === category
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {category === "All" ? "All" : humanize(category)}
                  </button>
                ))}
              </div>

              {(statusFilter !== "All" || categoryFilter !== "All" || search !== "") && (
                <button
                  onClick={() => {
                    setStatusFilter("All");
                    setCategoryFilter("All");
                    setSearch("");
                  }}
                  className="rounded-lg border border-line px-3 py-1 text-[10px] font-medium text-ink-600 hover:bg-white transition-colors"
                >
                  Clear all
                </button>
              )}
            </div>

            {/* Requirement List */}
            {filteredRequirements.length === 0 ? (
              <div className="py-12 text-center">
                <div className="inline-block rounded-full bg-surface-2 p-4">
                  <ListChecks className="h-6 w-6 text-ink-400" />
                </div>
                <p className="mt-3 text-sm text-ink-500">No requirements match your filters.</p>
              </div>
            ) : (
              <div className="divide-y divide-line">
                {filteredRequirements.map((req) => {
                  const statusStyle = REQUIREMENT_STATUS_STYLES[req.status] || REQUIREMENT_STATUS_STYLES.UNKNOWN;
                  const categoryStyle = CATEGORY_STYLES[req.category] || { bg: "bg-surface-2", text: "text-ink-600" };
                  const expiryStatus = getExpiryStatus(req.dueDate);
                  const isExpanded = expandedRequirements.has(req.id);

                  return (
                    <div key={req.id} className="transition-colors hover:bg-surface-2/50">
                      <button
                        onClick={() => toggleRequirement(req.id)}
                        className="flex w-full items-center gap-3 p-4 text-left"
                      >
                        <div className="flex-shrink-0">
                          {statusStyle.icon}
                        </div>

                        <div className="flex-1 min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <span className="font-medium text-ink-950">{req.name}</span>
                            <span className="text-[10px] text-ink-500 font-mono">{req.reference}</span>
                            {req.category && (
                              <span className={`rounded-full px-2 py-0.5 text-[9px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                                {humanize(req.category)}
                              </span>
                            )}
                          </div>
                          <div className="mt-0.5 flex flex-wrap items-center gap-3 text-xs text-ink-500">
                            <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
                              {humanize(req.status)}
                            </span>
                            {req.dueDate && (
                              <span className={`flex items-center gap-1 ${expiryStatus.class}`}>
                                <Calendar className="h-3 w-3" />
                                Due: {formatDate(req.dueDate)} ({expiryStatus.label})
                              </span>
                            )}
                            {req.restrictionImpact && (
                              <span className="flex items-center gap-1 text-red">
                                <AlertCircle className="h-3 w-3" />
                                Blocks work
                              </span>
                            )}
                          </div>
                        </div>

                        <div className="flex-shrink-0">
                          <button className="rounded-lg border border-line px-3 py-1 text-[10px] font-medium text-ink-700 hover:bg-white transition-colors">
                            {isExpanded ? "Collapse" : "Expand"}
                          </button>
                        </div>
                      </button>

                      {isExpanded && (
                        <div className="border-t border-line px-4 pb-4 pt-3 bg-surface-2/30">
                          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Source</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.source ? humanize(req.source) : <span className="text-ink-400">Not recorded</span>}
                              </div>
                              {req.sourceReference && <div className="mt-1 font-mono text-xs text-ink-500">{req.sourceReference}</div>}
                            </div>

                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Restriction</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.restrictionReference ? (
                                  <span className="flex items-center gap-1 font-mono text-red">
                                    <FileText className="h-4 w-4" />
                                    {req.restrictionReference}
                                  </span>
                                ) : (
                                  <span className="text-ink-400">No restriction</span>
                                )}
                              </div>
                            </div>

                            <div>
                              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Reasons</div>
                              <div className="mt-1 text-sm text-ink-950">
                                {req.reasons.length > 0 ? req.reasons.map(humanize).join(", ") : <span className="text-ink-400">None recorded</span>}
                              </div>
                              <div className="mt-1 text-xs text-ink-500">Evaluated {formatTime(req.evaluatedAt)}</div>
                            </div>
                          </div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          CMP-03 · Provider Compliance · {data.providerId}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · Last evaluation {formatTime(data.lastAssessment)}
        </span>
      </div>
    </div>
  );
}