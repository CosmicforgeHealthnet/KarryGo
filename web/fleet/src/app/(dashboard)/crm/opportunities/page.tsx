// src/app/(dashboard)/crm/opportunities/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Target,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  ArrowRight,
  Building2,
  MoreHorizontal,
  CalendarDays,
  FileText,
  Plus,
  DollarSign,
  User,
  Mail,
  Phone,
  Zap,
  HandCoins,
} from "lucide-react";

type Opportunity = {
  id: string;
  name: string;
  customer: string;
  customerId: string;
  initials: string;
  stage: "Prospecting" | "Qualification" | "Proposal" | "Negotiation" | "Closed Won" | "Closed Lost";
  expectedValue: number;
  probability: number;
  owner: string;
  branch: string;
  createdAt: string;
  expectedClose: string;
  lastActivity: string;
  priority: "High" | "Medium" | "Low";
  description: string | null;
  competitors: string[] | null;
  nextAction: string | null;
};

// Mock data
const OPPORTUNITIES: Opportunity[] = [
  {
    id: "OPP-001",
    name: "Medical Logistics Contract - Lagos Medical Center",
    customer: "Lagos Medical Center",
    customerId: "CUST-009",
    initials: "LC",
    stage: "Proposal",
    expectedValue: 2800000,
    probability: 65,
    owner: "F. Daniel",
    branch: "Ikeja",
    createdAt: "2026-08-10",
    expectedClose: "2026-09-15",
    lastActivity: "2 days ago",
    priority: "High",
    description: "Monthly medical supply logistics. 3-year contract potential.",
    competitors: ["MediLog", "HealthExpress"],
    nextAction: "Submit proposal",
  },
  {
    id: "OPP-002",
    name: "Manufacturing Logistics - Adeyemi Industries",
    customer: "Adeyemi Industries",
    customerId: "CUST-010",
    initials: "AI",
    stage: "Qualification",
    expectedValue: 1500000,
    probability: 35,
    owner: "A. Yusuf",
    branch: "Lekki",
    createdAt: "2026-08-15",
    expectedClose: "2026-10-01",
    lastActivity: "1 day ago",
    priority: "Medium",
    description: "Raw material logistics for manufacturing plant.",
    competitors: null,
    nextAction: "Discovery call",
  },
  {
    id: "OPP-003",
    name: "Hauling Services - Okonkwo Enterprises",
    customer: "Okonkwo Enterprises",
    customerId: "CUST-011",
    initials: "OE",
    stage: "Negotiation",
    expectedValue: 3200000,
    probability: 80,
    owner: "M. Bello",
    branch: "Apapa",
    createdAt: "2026-08-01",
    expectedClose: "2026-09-30",
    lastActivity: "4 days ago",
    priority: "High",
    description: "Heavy hauling for construction materials.",
    competitors: ["SwiftHaul", "TruckPro"],
    nextAction: "Contract review",
  },
  {
    id: "OPP-004",
    name: "Warehouse & Distribution - Eze & Partners",
    customer: "Eze & Partners",
    customerId: "CUST-012",
    initials: "EP",
    stage: "Closed Won",
    expectedValue: 5000000,
    probability: 100,
    owner: "F. Daniel",
    branch: "Ikeja",
    createdAt: "2026-07-15",
    expectedClose: "2026-08-15",
    lastActivity: "5 days ago",
    priority: "High",
    description: "Warehouse storage and distribution services.",
    competitors: ["GlobalWare"],
    nextAction: null,
  },
  {
    id: "OPP-005",
    name: "Retail Logistics - Ogunyemi Global",
    customer: "Ogunyemi Global",
    customerId: "CUST-013",
    initials: "OG",
    stage: "Prospecting",
    expectedValue: 2100000,
    probability: 20,
    owner: "M. Bello",
    branch: "Lekki",
    createdAt: "2026-08-18",
    expectedClose: "2026-11-01",
    lastActivity: "Today",
    priority: "Low",
    description: "Last-mile delivery for retail chain.",
    competitors: null,
    nextAction: "Initial meeting",
  },
  {
    id: "OPP-006",
    name: "Corporate Logistics - Ajayi Group",
    customer: "Ajayi Group",
    customerId: "CUST-014",
    initials: "AG",
    stage: "Proposal",
    expectedValue: 1200000,
    probability: 45,
    owner: "F. Daniel",
    branch: "Ikeja",
    createdAt: "2026-08-20",
    expectedClose: "2026-09-30",
    lastActivity: "Today",
    priority: "Medium",
    description: "Corporate logistics for retail distribution.",
    competitors: ["QuickShip"],
    nextAction: "Proposal review",
  },
  {
    id: "OPP-007",
    name: "Cold Chain Logistics - Greenline Foods",
    customer: "Greenline Foods",
    customerId: "CUST-003",
    initials: "GF",
    stage: "Closed Lost",
    expectedValue: 4500000,
    probability: 0,
    owner: "F. Daniel",
    branch: "Ikeja",
    createdAt: "2026-06-01",
    expectedClose: "2026-07-31",
    lastActivity: "1 month ago",
    priority: "Medium",
    description: "Cold chain logistics for perishable goods.",
    competitors: ["ColdChainPro"],
    nextAction: null,
  },
];

const STAGE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Prospecting: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <Target className="h-3.5 w-3.5" />,
  },
  Qualification: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Filter className="h-3.5 w-3.5" />,
  },
  Proposal: {
    bg: "bg-purple-bg",
    text: "text-purple",
    dot: "bg-purple",
    icon: <FileText className="h-3.5 w-3.5" />,
  },
  Negotiation: {
    bg: "bg-orange-bg",
    text: "text-orange",
    dot: "bg-orange",
    icon: <HandCoins className="h-3.5 w-3.5" />,
  },
  "Closed Won": {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  "Closed Lost": {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <X className="h-3.5 w-3.5" />,
  },
};

const PRIORITY_STYLES: Record<string, { bg: string; text: string }> = {
  High: { bg: "bg-red-bg", text: "text-red" },
  Medium: { bg: "bg-amber-bg", text: "text-amber" },
  Low: { bg: "bg-green-bg", text: "text-green" },
};

const STAGE_FILTERS = ["All", "Prospecting", "Qualification", "Proposal", "Negotiation", "Closed Won", "Closed Lost"];
const PRIORITY_FILTERS = ["All", "High", "Medium", "Low"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function OpportunitiesPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStage, setSelectedStage] = useState("All");
  const [selectedPriority, setSelectedPriority] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Opportunity[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(OPPORTUNITIES);
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
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.customer.toLowerCase().includes(search.toLowerCase()) ||
      item.owner.toLowerCase().includes(search.toLowerCase());

    const matchesStage = selectedStage === "All" || item.stage === selectedStage;
    const matchesPriority = selectedPriority === "All" || item.priority === selectedPriority;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStage && matchesPriority && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading opportunities...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStage !== "All" || selectedPriority !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStage("All");
    setSelectedPriority("All");
    setSelectedBranch("All branches");
  };

  const goToOpportunityDetail = (opportunityId: string) => {
    router.push(`/crm/opportunities/${opportunityId}`);
  };

  const goToCreateOpportunity = () => {
    router.push("/crm/opportunities/create");
  };

  // Stats
  const totalOpportunities = data.length;
  const wonCount = data.filter((d) => d.stage === "Closed Won").length;
  const lostCount = data.filter((d) => d.stage === "Closed Lost").length;
  const openCount = totalOpportunities - wonCount - lostCount;
  const weightedValue = data.reduce((acc, d) => acc + (d.expectedValue * d.probability / 100), 0);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Sales Opportunities
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Pipeline stage, expected value snapshot, probability, owner, next action and linked customer.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button
            onClick={goToCreateOpportunity}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Opportunity
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total</div>
          <div className="text-lg font-bold text-ink-950">{totalOpportunities}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Open</div>
          <div className="text-lg font-bold text-blue">{openCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Won</div>
          <div className="text-lg font-bold text-green">{wonCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Pipeline Value</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(weightedValue)}
          </div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by opportunity name, customer, or owner..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const stages = ["All", "Prospecting", "Qualification", "Proposal", "Negotiation", "Closed Won", "Closed Lost"];
            const currentIndex = stages.indexOf(selectedStage);
            const nextIndex = (currentIndex + 1) % stages.length;
            setSelectedStage(stages[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStage}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const priorities = ["All", "High", "Medium", "Low"];
            const currentIndex = priorities.indexOf(selectedPriority);
            const nextIndex = (currentIndex + 1) % priorities.length;
            setSelectedPriority(priorities[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Zap className="h-4 w-4 text-ink-400" />
          {selectedPriority}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Building2 className="h-4 w-4 text-ink-400" />
          {selectedBranch}
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
              {(selectedStage !== "All" ? 1 : 0) + (selectedPriority !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Stage</div>
              <div className="flex gap-1.5">
                {STAGE_FILTERS.map((stage) => (
                  <button
                    key={stage}
                    onClick={() => setSelectedStage(stage)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedStage === stage
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {stage}
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

      {/* ===== OPPORTUNITY CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Target className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No opportunities found</h3>
          <p className="mt-1 text-sm text-ink-500">No opportunities match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const stageStyle = STAGE_STYLES[item.stage] || STAGE_STYLES.Prospecting;
            const priorityStyle = PRIORITY_STYLES[item.priority] || PRIORITY_STYLES.Medium;
            const formattedValue = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(item.expectedValue);

            return (
              <div
                key={item.id}
                onClick={() => goToOpportunityDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.stage === "Closed Won"
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.stage === "Closed Lost"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : item.priority === "High"
                    ? "border-red/20 bg-red-bg/5 hover:border-red/40"
                    : "border-line bg-white hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.stage === "Closed Won" ? "bg-green-500/5" :
                  item.stage === "Closed Lost" ? "bg-red-500/5" :
                  item.priority === "High" ? "bg-red-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-[16px] font-bold text-white shadow-lg ${
                        item.stage === "Closed Won" ? "bg-green" :
                        item.stage === "Closed Lost" ? "bg-red" :
                        item.priority === "High" ? "bg-red" :
                        "bg-primary"
                      }`}
                    >
                      {item.initials}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.name}
                        </span>
                        <span className="font-mono text-[10px] text-ink-400">
                          {item.id}
                        </span>
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${stageStyle.bg} ${stageStyle.text}`}
                        >
                          {stageStyle.icon}
                          {item.stage}
                        </span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${priorityStyle.bg} ${priorityStyle.text}`}
                        >
                          {item.priority}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-2">
                  <div className="text-sm font-medium text-ink-950">{item.customer}</div>
                  <div className="mt-1 flex flex-wrap items-center gap-3 text-[12px] text-ink-500">
                    <span className="flex items-center gap-1.5">
                      <User className="h-3.5 w-3.5" />
                      {item.owner}
                    </span>
                    <span className="text-ink-300">·</span>
                    <span className="flex items-center gap-1.5">
                      <Building2 className="h-3.5 w-3.5" />
                      {item.branch}
                    </span>
                  </div>
                </div>

                <div className="relative z-10 mt-3 grid grid-cols-2 gap-1.5">
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <DollarSign className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-semibold text-ink-950">{formattedValue}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Target className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="font-semibold text-ink-950">{item.probability}%</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <CalendarDays className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>Close: {item.expectedClose}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Clock className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.lastActivity}</span>
                  </div>
                </div>

                {/* Next Action */}
                {item.nextAction && (
                  <div className="relative z-10 mt-3 border-t border-line pt-2.5">
                    <div className="flex items-center gap-2 text-[12px]">
                      <Zap className="h-3.5 w-3.5 text-amber" />
                      <span className="text-ink-500">Next Action:</span>
                      <span className="font-medium text-ink-950">{item.nextAction}</span>
                    </div>
                  </div>
                )}

                {item.competitors && item.competitors.length > 0 && (
                  <div className="relative z-10 mt-2 flex flex-wrap items-center gap-1">
                    <span className="text-[11px] text-ink-400">Competitors:</span>
                    {item.competitors.map((comp, idx) => (
                      <span
                        key={idx}
                        className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500"
                      >
                        {comp}
                      </span>
                    ))}
                  </div>
                )}

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Created: {item.createdAt}
                  </div>
                  <div className="flex items-center gap-1 text-[11px] font-medium text-ink-400 group-hover:text-primary transition-colors">
                    View <ArrowRight className="h-3.5 w-3.5 group-hover:translate-x-0.5 transition-transform" />
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Target className="h-4 w-4" />
          CRM-08 · Sales Opportunities
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} opportunities shown
        </span>
      </div>
    </div>
  );
}