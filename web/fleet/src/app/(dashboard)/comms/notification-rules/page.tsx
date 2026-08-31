// src/app/(dashboard)/comms/notification-rules/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Bell,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  Search,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Download,
  Tag,
  Info,
  AlertOctagon,
  ToggleLeft,
  ToggleRight,
  Smartphone,
  MessageSquare,
  Mail,
  Globe,
} from "lucide-react";

// ==================== TYPES ====================

type NotificationRule = {
  id: string;
  reference: string;
  name: string;
  description: string;
  event: "TripAssigned" | "TripStarted" | "TripCompleted" | "TripCancelled" | "IncidentReported" | "IncidentResolved" | "SafetyConcern" | "SLAWarning" | "SLABreached" | "ProviderDeclined" | "ProviderAvailable" | "MaintenanceDue" | "DocumentExpiring" | "ComplianceBlock" | "WalletAlert" | "Custom";
  eventCategory: "Operations" | "Safety" | "Compliance" | "Finance" | "Fleet" | "Workforce";
  recipients: string[];
  channels: ("Email" | "SMS" | "Push" | "InApp" | "Webhook")[];
  severity: "Urgent" | "High" | "Medium" | "Low" | "Info";
  enabled: boolean;
  quietHours: {
    enabled: boolean;
    start: string;
    end: string;
    timezone: string;
  } | null;
  branch: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  lastTriggered: string | null;
  triggerCount: number;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_RULES: NotificationRule[] = [
  {
    id: "NR-001",
    reference: "NR-2026-001",
    name: "Trip Assignment Alert",
    description: "Notify dispatchers and providers when a trip is assigned.",
    event: "TripAssigned",
    eventCategory: "Operations",
    recipients: ["Dispatchers", "Branch Managers", "Assigned Provider"],
    channels: ["Push", "InApp", "SMS"],
    severity: "Medium",
    enabled: true,
    quietHours: null,
    branch: "All branches",
    createdAt: "2026-06-01T09:00:00Z",
    updatedAt: "2026-07-15T14:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: "2026-08-22T13:45:00Z",
    triggerCount: 284,
    tags: ["Operations", "Trip"],
  },
  {
    id: "NR-002",
    reference: "NR-2026-002",
    name: "Critical Incident Alert",
    description: "Immediate notification for critical safety incidents.",
    event: "IncidentReported",
    eventCategory: "Safety",
    recipients: ["Safety Manager", "Operations Director", "Branch Manager"],
    channels: ["SMS", "Push", "Email"],
    severity: "Urgent",
    enabled: true,
    quietHours: {
      enabled: false,
      start: "22:00",
      end: "06:00",
      timezone: "Africa/Lagos",
    },
    branch: "All branches",
    createdAt: "2026-06-15T10:00:00Z",
    updatedAt: "2026-07-20T11:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: "2026-08-22T06:30:00Z",
    triggerCount: 12,
    tags: ["Safety", "Critical", "Incident"],
  },
  {
    id: "NR-003",
    reference: "NR-2026-003",
    name: "SLA Breach Warning",
    description: "Alert when a trip is approaching or has breached SLA.",
    event: "SLAWarning",
    eventCategory: "Operations",
    recipients: ["Dispatchers", "Account Managers"],
    channels: ["InApp", "Push"],
    severity: "High",
    enabled: true,
    quietHours: {
      enabled: true,
      start: "20:00",
      end: "07:00",
      timezone: "Africa/Lagos",
    },
    branch: "All branches",
    createdAt: "2026-06-20T14:00:00Z",
    updatedAt: "2026-08-01T09:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: "2026-08-22T12:15:00Z",
    triggerCount: 156,
    tags: ["SLA", "Operations"],
  },
  {
    id: "NR-004",
    reference: "NR-2026-004",
    name: "Provider Availability Alert",
    description: "Notify when a provider becomes available or goes offline.",
    event: "ProviderAvailable",
    eventCategory: "Operations",
    recipients: ["Dispatchers"],
    channels: ["InApp"],
    severity: "Low",
    enabled: false,
    quietHours: null,
    branch: "All branches",
    createdAt: "2026-07-01T08:00:00Z",
    updatedAt: "2026-08-10T16:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: null,
    triggerCount: 0,
    tags: ["Providers", "Availability"],
  },
  {
    id: "NR-005",
    reference: "NR-2026-005",
    name: "Document Expiry Alert",
    description: "Notify compliance team when documents are expiring.",
    event: "DocumentExpiring",
    eventCategory: "Compliance",
    recipients: ["Compliance Team", "Provider Managers"],
    channels: ["Email", "InApp"],
    severity: "High",
    enabled: true,
    quietHours: {
      enabled: false,
      start: "00:00",
      end: "00:00",
      timezone: "Africa/Lagos",
    },
    branch: "All branches",
    createdAt: "2026-06-25T11:00:00Z",
    updatedAt: "2026-08-15T10:00:00Z",
    createdBy: "O. Adebayo",
    lastTriggered: "2026-08-20T09:00:00Z",
    triggerCount: 42,
    tags: ["Compliance", "Documents", "Expiry"],
  },
  {
    id: "NR-006",
    reference: "NR-2026-006",
    name: "Compliance Block Alert",
    description: "Alert when a provider or asset is blocked due to compliance issues.",
    event: "ComplianceBlock",
    eventCategory: "Compliance",
    recipients: ["Compliance Manager", "Operations Manager"],
    channels: ["SMS", "Email", "Push"],
    severity: "Urgent",
    enabled: true,
    quietHours: {
      enabled: false,
      start: "22:00",
      end: "06:00",
      timezone: "Africa/Lagos",
    },
    branch: "All branches",
    createdAt: "2026-07-10T09:00:00Z",
    updatedAt: "2026-08-01T14:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: "2026-08-20T10:30:00Z",
    triggerCount: 8,
    tags: ["Compliance", "Block", "Urgent"],
  },
  {
    id: "NR-007",
    reference: "NR-2026-007",
    name: "Maintenance Due Alert",
    description: "Notify fleet team when maintenance is due or overdue.",
    event: "MaintenanceDue",
    eventCategory: "Fleet",
    recipients: ["Fleet Manager", "Workshop Team"],
    channels: ["Email", "InApp"],
    severity: "Medium",
    enabled: true,
    quietHours: {
      enabled: true,
      start: "18:00",
      end: "06:00",
      timezone: "Africa/Lagos",
    },
    branch: "All branches",
    createdAt: "2026-07-15T13:00:00Z",
    updatedAt: "2026-08-05T10:00:00Z",
    createdBy: "M. Bello",
    lastTriggered: "2026-08-21T08:00:00Z",
    triggerCount: 67,
    tags: ["Fleet", "Maintenance"],
  },
  {
    id: "NR-008",
    reference: "NR-2026-008",
    name: "Provider Decline Notification",
    description: "Alert when a provider declines an assignment with safety concerns.",
    event: "ProviderDeclined",
    eventCategory: "Safety",
    recipients: ["Safety Manager", "Dispatcher Lead"],
    channels: ["InApp", "Email"],
    severity: "High",
    enabled: true,
    quietHours: null,
    branch: "All branches",
    createdAt: "2026-08-01T10:00:00Z",
    updatedAt: "2026-08-15T14:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: "2026-08-21T14:30:00Z",
    triggerCount: 5,
    tags: ["Safety", "Decline", "Provider"],
  },
  {
    id: "NR-009",
    reference: "NR-2026-009",
    name: "Wallet Balance Alert",
    description: "Notify finance when wallet balance falls below threshold.",
    event: "WalletAlert",
    eventCategory: "Finance",
    recipients: ["Finance Manager", "Organization Admin"],
    channels: ["Email", "SMS"],
    severity: "High",
    enabled: false,
    quietHours: null,
    branch: "All branches",
    createdAt: "2026-08-10T15:00:00Z",
    updatedAt: "2026-08-15T11:00:00Z",
    createdBy: "F. Daniel",
    lastTriggered: null,
    triggerCount: 0,
    tags: ["Finance", "Wallet"],
  },
];

type EventCategoryStyle = {
  bg: string;
  text: string;
};

const EVENT_CATEGORY_STYLES: Record<string, EventCategoryStyle> = {
  Operations: { bg: "bg-blue-bg", text: "text-blue" },
  Safety: { bg: "bg-red-bg", text: "text-red" },
  Compliance: { bg: "bg-purple-bg", text: "text-purple" },
  Finance: { bg: "bg-green-bg", text: "text-green" },
  Fleet: { bg: "bg-amber-bg", text: "text-amber" },
  Workforce: { bg: "bg-surface-2", text: "text-ink-500" },
};

type SeverityStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const SEVERITY_STYLES: Record<string, SeverityStyle> = {
  Urgent: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  High: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Medium: { bg: "bg-blue-bg", text: "text-blue", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Low: { bg: "bg-surface-2", text: "text-ink-500", icon: <Info className="h-3.5 w-3.5" /> },
  Info: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
};

const CHANNEL_ICONS: Record<string, React.ReactNode> = {
  Email: <Mail className="h-3.5 w-3.5" />,
  SMS: <Smartphone className="h-3.5 w-3.5" />,
  Push: <Bell className="h-3.5 w-3.5" />,
  InApp: <MessageSquare className="h-3.5 w-3.5" />,
  Webhook: <Globe className="h-3.5 w-3.5" />,
};

const CATEGORY_FILTERS = ["All", "Operations", "Safety", "Compliance", "Finance", "Fleet", "Workforce"];
const SEVERITY_FILTERS = ["All", "Urgent", "High", "Medium", "Low", "Info"];
const STATUS_FILTERS = ["All", "Enabled", "Disabled"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function NotificationRulesPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<NotificationRule[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [search, setSearch] = useState<string>("");
  const [selectedCategory, setSelectedCategory] = useState<string>("All");
  const [selectedSeverity, setSelectedSeverity] = useState<string>("All");
  const [selectedStatus, setSelectedStatus] = useState<string>("All");
  const [selectedBranch, setSelectedBranch] = useState<string>("All branches");
  const [showFilters, setShowFilters] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_RULES);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.reference.toLowerCase().includes(search.toLowerCase()) ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.description.toLowerCase().includes(search.toLowerCase()) ||
      item.event.toLowerCase().includes(search.toLowerCase());

    const matchesCategory = selectedCategory === "All" || item.eventCategory === selectedCategory;
    const matchesSeverity = selectedSeverity === "All" || item.severity === selectedSeverity;
    const matchesStatus = selectedStatus === "All" || (selectedStatus === "Enabled" ? item.enabled : !item.enabled);
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesCategory && matchesSeverity && matchesStatus && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading notification rules...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedCategory !== "All" ||
    selectedSeverity !== "All" ||
    selectedStatus !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedCategory("All");
    setSelectedSeverity("All");
    setSelectedStatus("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedCategory !== "All" ? 1 : 0) +
    (selectedSeverity !== "All" ? 1 : 0) +
    (selectedStatus !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatTime = (date: string | null): string => {
    if (!date) return "Never";
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  // Stats
  const totalEnabled = data.filter((d) => d.enabled).length;
  const totalUrgent = data.filter((d) => d.severity === "Urgent").length;
  const totalOperations = data.filter((d) => d.eventCategory === "Operations").length;

  const goToRuleDetail = (id: string) => {
    router.push(`/comms/notification-rules/${id}`);
  };

  const goToCreateRule = () => {
    router.push("/comms/notification-rules/create");
  };

  const toggleRule = (id: string) => {
    setData(data.map(rule => 
      rule.id === id ? { ...rule, enabled: !rule.enabled } : rule
    ));
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Notification Rules
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Event, recipients, channel, severity, quiet hours/timezone and state. Rules control intent, not direct delivery.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
          <button
            onClick={goToCreateRule}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Create Rule
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Enabled</div>
          <div className="text-lg font-bold text-green">{totalEnabled}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Urgent</div>
          <div className="text-lg font-bold text-red">{totalUrgent}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Operations</div>
          <div className="text-lg font-bold text-blue">{totalOperations}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, reference, event..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const categories = ["All", "Operations", "Safety", "Compliance", "Finance", "Fleet", "Workforce"];
            const currentIndex = categories.indexOf(selectedCategory);
            const nextIndex = (currentIndex + 1) % categories.length;
            setSelectedCategory(categories[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedCategory}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const severities = ["All", "Urgent", "High", "Medium", "Low", "Info"];
            const currentIndex = severities.indexOf(selectedSeverity);
            const nextIndex = (currentIndex + 1) % severities.length;
            setSelectedSeverity(severities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <AlertTriangle className="h-4 w-4 text-ink-400" />
          {selectedSeverity}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const statuses = ["All", "Enabled", "Disabled"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Bell className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {filterCount}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Branch</div>
              <div className="flex gap-1.5 flex-wrap">
                {BRANCHES.map((branch) => (
                  <button
                    key={branch}
                    onClick={() => setSelectedBranch(branch)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedBranch === branch
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {branch}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== RULES TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Bell className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No notification rules found</h3>
          <p className="mt-1 text-sm text-ink-500">No rules match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-2xl border border-line bg-white shadow-sm">
          <div className="overflow-x-auto">
            <table className="w-full border-collapse text-sm min-w-[1100px]">
              <thead>
                <tr className="bg-surface-2/70">
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Rule</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Category</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Severity</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Channels</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Last Trigger</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const categoryStyle = EVENT_CATEGORY_STYLES[item.eventCategory] || EVENT_CATEGORY_STYLES.Operations;
                  const severityStyle = SEVERITY_STYLES[item.severity] || SEVERITY_STYLES.Medium;

                  return (
                    <tr
                      key={item.id}
                      className="border-b border-line hover:bg-surface-2 transition-colors group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.name}</div>
                        <div className="flex items-center gap-2 mt-0.5">
                          <span className="font-mono text-[10px] text-ink-500">{item.reference}</span>
                          <span className="text-[9px] text-ink-400">{item.event}</span>
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${categoryStyle.bg} ${categoryStyle.text}`}>
                          {item.eventCategory}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${severityStyle.bg} ${severityStyle.text}`}>
                          {severityStyle.icon}
                          {item.severity}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex gap-1">
                          {item.channels.map((channel) => (
                            <span
                              key={channel}
                              className="flex items-center gap-0.5 rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-600"
                              title={channel}
                            >
                              {CHANNEL_ICONS[channel] || <Bell className="h-3 w-3" />}
                              {channel}
                            </span>
                          ))}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <button
                          onClick={() => toggleRule(item.id)}
                          className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold transition ${
                            item.enabled
                              ? "bg-green-bg text-green hover:bg-green-bg/70"
                              : "bg-surface-2 text-ink-500 hover:bg-surface-3"
                          }`}
                        >
                          {item.enabled ? (
                            <ToggleRight className="h-4 w-4" />
                          ) : (
                            <ToggleLeft className="h-4 w-4" />
                          )}
                          {item.enabled ? "Enabled" : "Disabled"}
                        </button>
                      </td>
                      <td className="px-4 py-3 text-ink-500 text-xs">
                        {item.lastTriggered ? formatTime(item.lastTriggered) : "Never"}
                        <div className="text-[9px] text-ink-400">{item.triggerCount} triggers</div>
                      </td>
                      <td className="px-4 py-3 text-right">
                        <button
                          onClick={() => goToRuleDetail(item.id)}
                          className="text-ink-400 group-hover:text-primary transition-colors"
                        >
                          <Eye className="h-4 w-4" />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Bell className="h-4 w-4" />
          COM-03 · Notification Rules
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} rules shown
        </span>
      </div>
    </div>
  );
}