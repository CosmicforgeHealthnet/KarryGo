// src/app/(dashboard)/comms/sla/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Gauge,
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
  Target,
  Timer,
  Award,
  AlertOctagon,
  XCircle,
  FileCheck,
} from "lucide-react";

// ==================== TYPES ====================

type SLAMeasurement = {
  id: string;
  reference: string;
  agreementId: string;
  agreementName: string;
  customerId: string;
  customerName: string;
  metric: "OnTimeDelivery" | "PickupTime" | "ResponseTime" | "TransitTime" | "DamageRate" | "CancellationRate" | "CustomerSatisfaction";
  metricLabel: string;
  period: string;
  target: number;
  actual: number | null;
  unit: string;
  direction: "higher" | "lower";
  result: "MET" | "BREACHED" | "UNKNOWN" | "IN_PROGRESS";
  evidenceState: "Complete" | "Partial" | "Missing" | "Pending";
  sourceFreshness: string;
  branch: string;
  evaluatedAt: string;
  evaluatedBy: string | null;
  notes: string | null;
  tags: string[];
};

// ==================== MOCK DATA ====================

const MOCK_SLA_MEASUREMENTS: SLAMeasurement[] = [
  {
    id: "SLA-001",
    reference: "SLA-2026-001",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Charitos Hospital",
    customerId: "C-003",
    customerName: "Charitos Hospital",
    metric: "OnTimeDelivery",
    metricLabel: "On-Time Delivery Rate",
    period: "Q3 2026",
    target: 98,
    actual: 96.4,
    unit: "%",
    direction: "higher",
    result: "BREACHED",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T09:00:00Z",
    branch: "Ikeja",
    evaluatedAt: "2026-08-22T09:00:00Z",
    evaluatedBy: "System",
    notes: "SLA breached by 1.6%. Root cause: Provider capacity issues.",
    tags: ["SLA", "Delivery", "Breach"],
  },
  {
    id: "SLA-002",
    reference: "SLA-2026-002",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Charitos Hospital",
    customerId: "C-003",
    customerName: "Charitos Hospital",
    metric: "DamageRate",
    metricLabel: "Damage Rate",
    period: "Q3 2026",
    target: 1,
    actual: 0.8,
    unit: "%",
    direction: "lower",
    result: "MET",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T09:00:00Z",
    branch: "Ikeja",
    evaluatedAt: "2026-08-22T09:00:00Z",
    evaluatedBy: "System",
    notes: "SLA met. Damage rate below target.",
    tags: ["SLA", "Quality", "Met"],
  },
  {
    id: "SLA-003",
    reference: "SLA-2026-003",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Charitos Hospital",
    customerId: "C-003",
    customerName: "Charitos Hospital",
    metric: "TransitTime",
    metricLabel: "Average Transit Time",
    period: "Q3 2026",
    target: 45,
    actual: 42.3,
    unit: "min",
    direction: "lower",
    result: "MET",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T09:00:00Z",
    branch: "Ikeja",
    evaluatedAt: "2026-08-22T09:00:00Z",
    evaluatedBy: "System",
    notes: "SLA met. Transit time improved by 3.2 min.",
    tags: ["SLA", "Transit", "Met"],
  },
  {
    id: "SLA-004",
    reference: "SLA-2026-004",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Oakfield Stores",
    customerId: "C-002",
    customerName: "Oakfield Stores",
    metric: "OnTimeDelivery",
    metricLabel: "On-Time Delivery Rate",
    period: "Q3 2026",
    target: 97,
    actual: 91.2,
    unit: "%",
    direction: "higher",
    result: "BREACHED",
    evidenceState: "Partial",
    sourceFreshness: "2026-08-22T10:00:00Z",
    branch: "Lekki",
    evaluatedAt: "2026-08-22T10:00:00Z",
    evaluatedBy: "System",
    notes: "SLA breached. Incomplete evidence for 12 deliveries.",
    tags: ["SLA", "Delivery", "Breach", "Partial"],
  },
  {
    id: "SLA-005",
    reference: "SLA-2026-005",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Oakfield Stores",
    customerId: "C-002",
    customerName: "Oakfield Stores",
    metric: "PickupTime",
    metricLabel: "Pickup Response Time",
    period: "Q3 2026",
    target: 30,
    actual: null,
    unit: "min",
    direction: "lower",
    result: "UNKNOWN",
    evidenceState: "Missing",
    sourceFreshness: "2026-08-22T10:00:00Z",
    branch: "Lekki",
    evaluatedAt: "2026-08-22T10:00:00Z",
    evaluatedBy: "System",
    notes: "Insufficient data to evaluate. Pickup timestamps missing for 60% of trips.",
    tags: ["SLA", "Pickup", "Unknown"],
  },
  {
    id: "SLA-006",
    reference: "SLA-2026-006",
    agreementId: "CT-911",
    agreementName: "Service Agreement - Oakfield Stores",
    customerId: "C-002",
    customerName: "Oakfield Stores",
    metric: "CustomerSatisfaction",
    metricLabel: "Customer Satisfaction Score",
    period: "Q3 2026",
    target: 4.5,
    actual: 4.2,
    unit: "/5",
    direction: "higher",
    result: "BREACHED",
    evidenceState: "Partial",
    sourceFreshness: "2026-08-22T10:00:00Z",
    branch: "Lekki",
    evaluatedAt: "2026-08-22T10:00:00Z",
    evaluatedBy: "System",
    notes: "SLA breached. Survey response rate only 34%.",
    tags: ["SLA", "Satisfaction", "Breach"],
  },
  {
    id: "SLA-007",
    reference: "SLA-2026-007",
    agreementId: "CT-912",
    agreementName: "Service Agreement - Greenline Foods",
    customerId: "C-004",
    customerName: "Greenline Foods",
    metric: "OnTimeDelivery",
    metricLabel: "On-Time Delivery Rate",
    period: "Q3 2026",
    target: 99,
    actual: 99.1,
    unit: "%",
    direction: "higher",
    result: "MET",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T11:00:00Z",
    branch: "Lekki",
    evaluatedAt: "2026-08-22T11:00:00Z",
    evaluatedBy: "System",
    notes: "SLA met. Performance above target.",
    tags: ["SLA", "Delivery", "Met"],
  },
  {
    id: "SLA-008",
    reference: "SLA-2026-008",
    agreementId: "CT-912",
    agreementName: "Service Agreement - Greenline Foods",
    customerId: "C-004",
    customerName: "Greenline Foods",
    metric: "DamageRate",
    metricLabel: "Damage Rate",
    period: "Q3 2026",
    target: 0.5,
    actual: 0.3,
    unit: "%",
    direction: "lower",
    result: "MET",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T11:00:00Z",
    branch: "Lekki",
    evaluatedAt: "2026-08-22T11:00:00Z",
    evaluatedBy: "System",
    notes: "SLA met. Damage rate well below target.",
    tags: ["SLA", "Quality", "Met"],
  },
  {
    id: "SLA-009",
    reference: "SLA-2026-009",
    agreementId: "CT-913",
    agreementName: "Service Agreement - Solaris Construction",
    customerId: "C-005",
    customerName: "Solaris Construction",
    metric: "OnTimeDelivery",
    metricLabel: "On-Time Delivery Rate",
    period: "Q3 2026",
    target: 95,
    actual: null,
    unit: "%",
    direction: "higher",
    result: "IN_PROGRESS",
    evidenceState: "Pending",
    sourceFreshness: "2026-08-22T12:00:00Z",
    branch: "Apapa",
    evaluatedAt: "2026-08-22T12:00:00Z",
    evaluatedBy: "System",
    notes: "Measurement in progress. Period not yet complete.",
    tags: ["SLA", "Delivery", "InProgress"],
  },
  {
    id: "SLA-010",
    reference: "SLA-2026-010",
    agreementId: "CT-913",
    agreementName: "Service Agreement - Solaris Construction",
    customerId: "C-005",
    customerName: "Solaris Construction",
    metric: "TransitTime",
    metricLabel: "Average Transit Time",
    period: "Q3 2026",
    target: 60,
    actual: 58.7,
    unit: "min",
    direction: "lower",
    result: "MET",
    evidenceState: "Complete",
    sourceFreshness: "2026-08-22T12:00:00Z",
    branch: "Apapa",
    evaluatedAt: "2026-08-22T12:00:00Z",
    evaluatedBy: "System",
    notes: "SLA met. Transit time within target.",
    tags: ["SLA", "Transit", "Met"],
  },
];

// ==================== STYLES ====================

const METRIC_ICONS: Record<string, React.ReactNode> = {
  OnTimeDelivery: <Timer className="h-3.5 w-3.5" />,
  PickupTime: <Clock className="h-3.5 w-3.5" />,
  ResponseTime: <Gauge className="h-3.5 w-3.5" />,
  TransitTime: <Timer className="h-3.5 w-3.5" />,
  DamageRate: <AlertTriangle className="h-3.5 w-3.5" />,
  CancellationRate: <XCircle className="h-3.5 w-3.5" />,
  CustomerSatisfaction: <Award className="h-3.5 w-3.5" />,
};

const RESULT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  MET: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  BREACHED: { bg: "bg-red-bg", text: "text-red", icon: <AlertOctagon className="h-3.5 w-3.5" /> },
  UNKNOWN: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  IN_PROGRESS: { bg: "bg-blue-bg", text: "text-blue", icon: <RefreshCw className="h-3.5 w-3.5" /> },
};

const EVIDENCE_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Complete: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Partial: { bg: "bg-amber-bg", text: "text-amber", icon: <AlertTriangle className="h-3.5 w-3.5" /> },
  Missing: { bg: "bg-red-bg", text: "text-red", icon: <AlertCircle className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-blue-bg", text: "text-blue", icon: <Clock className="h-3.5 w-3.5" /> },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

// ==================== COMPONENT ====================

export default function SLAMeasurementsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<SLAMeasurement[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [search, setSearch] = useState("");
  const [selectedMetric, setSelectedMetric] = useState("All");
  const [selectedResult, setSelectedResult] = useState("All");
  const [selectedEvidence, setSelectedEvidence] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_SLA_MEASUREMENTS);
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
      item.agreementName.toLowerCase().includes(search.toLowerCase()) ||
      item.customerName.toLowerCase().includes(search.toLowerCase()) ||
      item.metricLabel.toLowerCase().includes(search.toLowerCase());

    const matchesMetric = selectedMetric === "All" || item.metric === selectedMetric;
    const matchesResult = selectedResult === "All" || item.result === selectedResult;
    const matchesEvidence = selectedEvidence === "All" || item.evidenceState === selectedEvidence;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesMetric && matchesResult && matchesEvidence && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading SLA measurements...</p>
      </div>
    );
  }

  const hasActiveFilters =
    selectedMetric !== "All" ||
    selectedResult !== "All" ||
    selectedEvidence !== "All" ||
    selectedBranch !== "All branches" ||
    search !== "";

  const clearFilters = () => {
    setSearch("");
    setSelectedMetric("All");
    setSelectedResult("All");
    setSelectedEvidence("All");
    setSelectedBranch("All branches");
  };

  const filterCount =
    (selectedMetric !== "All" ? 1 : 0) +
    (selectedResult !== "All" ? 1 : 0) +
    (selectedEvidence !== "All" ? 1 : 0) +
    (selectedBranch !== "All branches" ? 1 : 0);

  const formatTime = (date: string) => {
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
  const totalMet = data.filter((d) => d.result === "MET").length;
  const totalBreached = data.filter((d) => d.result === "BREACHED").length;
  const totalUnknown = data.filter((d) => d.result === "UNKNOWN").length;
  const totalComplete = data.filter((d) => d.evidenceState === "Complete").length;

  const goToMeasurementDetail = (id: string) => {
    router.push(`/comms/sla/${id}`);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            SLA Measurements
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Agreement metrics, periods, source evidence state, value/target and MET/BREACHED/UNKNOWN results.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Plus className="h-3.5 w-3.5" />
            Run Measurement
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
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">MET</div>
          <div className="text-lg font-bold text-green">{totalMet}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">BREACHED</div>
          <div className="text-lg font-bold text-red">{totalBreached}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Complete Evidence</div>
          <div className="text-lg font-bold text-blue">{totalComplete}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by customer, agreement, metric..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const metrics = ["All", "OnTimeDelivery", "PickupTime", "ResponseTime", "TransitTime", "DamageRate", "CancellationRate", "CustomerSatisfaction"];
            const currentIndex = metrics.indexOf(selectedMetric);
            const nextIndex = (currentIndex + 1) % metrics.length;
            setSelectedMetric(metrics[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Target className="h-4 w-4 text-ink-400" />
          {selectedMetric}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const results = ["All", "MET", "BREACHED", "UNKNOWN", "IN_PROGRESS"];
            const currentIndex = results.indexOf(selectedResult);
            const nextIndex = (currentIndex + 1) % results.length;
            setSelectedResult(results[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Gauge className="h-4 w-4 text-ink-400" />
          {selectedResult}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const evidences = ["All", "Complete", "Partial", "Missing", "Pending"];
            const currentIndex = evidences.indexOf(selectedEvidence);
            const nextIndex = (currentIndex + 1) % evidences.length;
            setSelectedEvidence(evidences[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <FileCheck className="h-4 w-4 text-ink-400" />
          {selectedEvidence}
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

      {/* ===== MEASUREMENTS TABLE ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Gauge className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No SLA measurements found</h3>
          <p className="mt-1 text-sm text-ink-500">No measurements match your current filters.</p>
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
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Measurement</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Customer</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Metric</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Result</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Evidence</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Value</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Branch</th>
                  <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
                </tr>
              </thead>
              <tbody>
                {filteredData.map((item) => {
                  const resultStyle = RESULT_STYLES[item.result] || RESULT_STYLES.UNKNOWN;
                  const evidenceStyle = EVIDENCE_STYLES[item.evidenceState] || EVIDENCE_STYLES.Pending;
                  const metricIcon = METRIC_ICONS[item.metric] || <Target className="h-3.5 w-3.5" />;

                  return (
                    <tr
                      key={item.id}
                      onClick={() => goToMeasurementDetail(item.id)}
                      className="border-b border-line hover:bg-surface-2 transition-colors cursor-pointer group"
                    >
                      <td className="px-4 py-3">
                        <div className="font-medium text-ink-950 text-xs">{item.agreementName}</div>
                        <div className="font-mono text-[10px] text-ink-500 mt-0.5">{item.reference}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.customerName}</td>
                      <td className="px-4 py-3">
                        <span className="flex items-center gap-1 text-xs font-medium text-ink-950">
                          {metricIcon}
                          {item.metricLabel}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}>
                          {resultStyle.icon}
                          {item.result}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${evidenceStyle.bg} ${evidenceStyle.text}`}>
                          {evidenceStyle.icon}
                          {item.evidenceState}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <div className="text-xs text-ink-950">
                          {item.actual !== null ? `${item.actual}${item.unit}` : "—"}
                        </div>
                        <div className="text-[9px] text-ink-400">Target: {item.target}{item.unit}</div>
                      </td>
                      <td className="px-4 py-3 text-ink-600 text-xs">{item.branch}</td>
                      <td className="px-4 py-3 text-right">
                        <button className="text-ink-400 group-hover:text-primary transition-colors">
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
          <Gauge className="h-4 w-4" />
          COM-08 · SLA Measurements
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} measurements shown
        </span>
      </div>
    </div>
  );
}