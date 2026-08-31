// src/app/(dashboard)/crm/leads/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Users,
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
  Plus,
  DollarSign,
  User,
  Mail,
  Phone,
  Tag,
  Target,
  UserPlus,
  Send,
  Star,
} from "lucide-react";

type Lead = {
  id: string;
  name: string;
  initials: string;
  company: string;
  source: "Website" | "Referral" | "LinkedIn" | "Cold Call" | "Email" | "Event" | "Other";
  status: "New" | "Contacted" | "Qualified" | "Converted" | "Lost" | "Disqualified";
  owner: string;
  branch: string;
  potentialValue: number;
  probability: number;
  email: string;
  phone: string;
  location: string;
  lastActivity: string;
  createdAt: string;
  notes: string | null;
  nextAction: string | null;
  nextActionDate: string | null;
};

// Mock data
const LEADS: Lead[] = [
  {
    id: "LEAD-001",
    name: "Dr. Adebayo Ogunleye",
    initials: "AO",
    company: "Lagos Medical Center",
    source: "Referral",
    status: "Qualified",
    owner: "F. Daniel",
    branch: "Ikeja",
    potentialValue: 2800000,
    probability: 75,
    email: "adebayo.ogunleye@lagosmedical.com",
    phone: "+234 800 111 2222",
    location: "Ikeja, Lagos",
    lastActivity: "2 days ago",
    createdAt: "2026-08-15",
    notes: "Referred by Charitos Hospital. Interested in medical logistics.",
    nextAction: "Send quotation",
    nextActionDate: "2026-08-25",
  },
  {
    id: "LEAD-002",
    name: "Mrs. Folake Adeyemi",
    initials: "FA",
    company: "Adeyemi Industries",
    source: "LinkedIn",
    status: "New",
    owner: "A. Yusuf",
    branch: "Lekki",
    potentialValue: 1500000,
    probability: 30,
    email: "folake.adeyemi@adeyemiindustries.com",
    phone: "+234 800 222 3333",
    location: "Lekki, Lagos",
    lastActivity: "1 day ago",
    createdAt: "2026-08-20",
    notes: "Manufacturing company looking for logistics solutions.",
    nextAction: "Initial call",
    nextActionDate: "2026-08-22",
  },
  {
    id: "LEAD-003",
    name: "Mr. Chidi Okonkwo",
    initials: "CO",
    company: "Okonkwo Enterprises",
    source: "Website",
    status: "Contacted",
    owner: "M. Bello",
    branch: "Apapa",
    potentialValue: 3200000,
    probability: 50,
    email: "chidi.okonkwo@okonkwoenterprises.com",
    phone: "+234 800 333 4444",
    location: "Apapa, Lagos",
    lastActivity: "3 days ago",
    createdAt: "2026-08-10",
    notes: "Submitted inquiry via website. Interested in hauling services.",
    nextAction: "Follow up call",
    nextActionDate: "2026-08-23",
  },
  {
    id: "LEAD-004",
    name: "Dr. Ngozi Eze",
    initials: "NE",
    company: "Eze & Partners",
    source: "Event",
    status: "Converted",
    owner: "F. Daniel",
    branch: "Ikeja",
    potentialValue: 5000000,
    probability: 100,
    email: "ngozi.eze@ezeandpartners.com",
    phone: "+234 800 444 5555",
    location: "Ikeja, Lagos",
    lastActivity: "5 days ago",
    createdAt: "2026-07-20",
    notes: "Converted to customer. Signed contract CT-2026-005.",
    nextAction: null,
    nextActionDate: null,
  },
  {
    id: "LEAD-005",
    name: "Mr. Tunde Fatoki",
    initials: "TF",
    company: "Fatoki Logistics",
    source: "Cold Call",
    status: "Lost",
    owner: "A. Yusuf",
    branch: "Yaba",
    potentialValue: 800000,
    probability: 10,
    email: "tunde.fatoki@fatokilogistics.com",
    phone: "+234 800 555 6666",
    location: "Yaba, Lagos",
    lastActivity: "1 week ago",
    createdAt: "2026-07-25",
    notes: "Not interested at this time.",
    nextAction: null,
    nextActionDate: null,
  },
  {
    id: "LEAD-006",
    name: "Mrs. Funke Ogunyemi",
    initials: "FO",
    company: "Ogunyemi Global",
    source: "Email",
    status: "Qualified",
    owner: "M. Bello",
    branch: "Lekki",
    potentialValue: 2100000,
    probability: 70,
    email: "funke.ogunyemi@ogunyemiglobal.com",
    phone: "+234 800 666 7777",
    location: "Lekki, Lagos",
    lastActivity: "4 days ago",
    createdAt: "2026-08-05",
    notes: "Global trading company. Needs warehousing and dispatch services.",
    nextAction: "Prepare proposal",
    nextActionDate: "2026-08-26",
  },
  {
    id: "LEAD-007",
    name: "Mr. Samuel Ajayi",
    initials: "SA",
    company: "Ajayi Group",
    source: "Referral",
    status: "New",
    owner: "F. Daniel",
    branch: "Ikeja",
    potentialValue: 1200000,
    probability: 20,
    email: "samuel.ajayi@ajayigroup.com",
    phone: "+234 800 777 8888",
    location: "Ikeja, Lagos",
    lastActivity: "Today",
    createdAt: "2026-08-21",
    notes: "Referred by Greenline Foods. Retail distribution needs.",
    nextAction: "Introductory call",
    nextActionDate: "2026-08-22",
  },
];

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  New: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <UserPlus className="h-3.5 w-3.5" />,
  },
  Contacted: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Send className="h-3.5 w-3.5" />,
  },
  Qualified: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <Star className="h-3.5 w-3.5" />,
  },
  Converted: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Lost: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Disqualified: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <X className="h-3.5 w-3.5" />,
  },
};

const SOURCE_STYLES: Record<string, { bg: string; text: string }> = {
  Website: { bg: "bg-blue-bg", text: "text-blue" },
  Referral: { bg: "bg-green-bg", text: "text-green" },
  LinkedIn: { bg: "bg-purple-bg", text: "text-purple" },
  "Cold Call": { bg: "bg-amber-bg", text: "text-amber" },
  Email: { bg: "bg-blue-bg", text: "text-blue" },
  Event: { bg: "bg-pink-bg", text: "text-pink" },
  Other: { bg: "bg-surface-2", text: "text-ink-500" },
};

const STATUS_FILTERS = ["All", "New", "Contacted", "Qualified", "Converted", "Lost", "Disqualified"];
const SOURCE_FILTERS = ["All", "Website", "Referral", "LinkedIn", "Cold Call", "Email", "Event", "Other"];
const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];

export default function LeadsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedStatus, setSelectedStatus] = useState("All");
  const [selectedSource, setSelectedSource] = useState("All");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Lead[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(LEADS);
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
      item.company.toLowerCase().includes(search.toLowerCase()) ||
      item.email.toLowerCase().includes(search.toLowerCase()) ||
      item.owner.toLowerCase().includes(search.toLowerCase());

    const matchesStatus = selectedStatus === "All" || item.status === selectedStatus;
    const matchesSource = selectedSource === "All" || item.source === selectedSource;
    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;

    return matchesSearch && matchesStatus && matchesSource && matchesBranch;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading leads...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedStatus !== "All" || selectedSource !== "All" || selectedBranch !== "All branches" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedStatus("All");
    setSelectedSource("All");
    setSelectedBranch("All branches");
  };

  const goToLeadDetail = (leadId: string) => {
    router.push(`/crm/leads/${leadId}`);
  };

  const goToCreateLead = () => {
    router.push("/crm/leads/create");
  };

  // Stats
  const totalLeads = data.length;
  const newCount = data.filter((d) => d.status === "New").length;
  const qualifiedCount = data.filter((d) => d.status === "Qualified").length;
  const convertedCount = data.filter((d) => d.status === "Converted").length;
  const totalPotentialValue = data.reduce((acc, d) => acc + d.potentialValue, 0);

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Leads
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Lead source, owner, status, next action and contact summary.
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
            onClick={goToCreateLead}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Lead
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Leads</div>
          <div className="text-lg font-bold text-ink-950">{totalLeads}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">New</div>
          <div className="text-lg font-bold text-blue">{newCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Qualified</div>
          <div className="text-lg font-bold text-green">{qualifiedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Converted</div>
          <div className="text-lg font-bold text-green">{convertedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, company, or email..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const statuses = ["All", "New", "Contacted", "Qualified", "Converted", "Lost", "Disqualified"];
            const currentIndex = statuses.indexOf(selectedStatus);
            const nextIndex = (currentIndex + 1) % statuses.length;
            setSelectedStatus(statuses[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedStatus}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const sources = ["All", "Website", "Referral", "LinkedIn", "Cold Call", "Email", "Event", "Other"];
            const currentIndex = sources.indexOf(selectedSource);
            const nextIndex = (currentIndex + 1) % sources.length;
            setSelectedSource(sources[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Tag className="h-4 w-4 text-ink-400" />
          {selectedSource}
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
              {(selectedStatus !== "All" ? 1 : 0) + (selectedSource !== "All" ? 1 : 0) + (selectedBranch !== "All branches" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Status</div>
              <div className="flex gap-1.5">
                {STATUS_FILTERS.map((status) => (
                  <button
                    key={status}
                    onClick={() => setSelectedStatus(status)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedStatus === status
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {status}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Source</div>
              <div className="flex gap-1.5">
                {SOURCE_FILTERS.map((source) => (
                  <button
                    key={source}
                    onClick={() => setSelectedSource(source)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedSource === source
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {source}
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

      {/* ===== LEAD CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No leads found</h3>
          <p className="mt-1 text-sm text-ink-500">No leads match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const statusStyle = STATUS_STYLES[item.status] || STATUS_STYLES.New;
            const sourceStyle = SOURCE_STYLES[item.source] || SOURCE_STYLES.Other;
            const formattedPotential = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(item.potentialValue);

            return (
              <div
                key={item.id}
                onClick={() => goToLeadDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.status === "Converted"
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.status === "Lost"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : item.status === "Qualified"
                    ? "border-primary/20 bg-primary-bg/10 hover:border-primary/40"
                    : "border-line bg-white hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.status === "Converted" ? "bg-green-500/5" :
                  item.status === "Lost" ? "bg-red-500/5" :
                  item.status === "Qualified" ? "bg-primary/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-[16px] font-bold text-white shadow-lg ${
                        item.status === "Converted" ? "bg-green" :
                        item.status === "Lost" ? "bg-red" :
                        item.status === "Qualified" ? "bg-primary" :
                        "bg-ink-400"
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
                          className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}
                        >
                          {statusStyle.icon}
                          {item.status}
                        </span>
                        <span
                          className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${sourceStyle.bg} ${sourceStyle.text}`}
                        >
                          {item.source}
                        </span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-2">
                  <div className="text-sm font-medium text-ink-950">{item.company}</div>
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
                    <span>{formattedPotential}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500">
                    <Target className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.probability}% probability</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500 col-span-2">
                    <Mail className="h-3.5 w-3.5 flex-shrink-0" />
                    <span className="truncate">{item.email}</span>
                  </div>
                  <div className="flex items-center gap-1.5 text-[12px] text-ink-500 col-span-2">
                    <Phone className="h-3.5 w-3.5 flex-shrink-0" />
                    <span>{item.phone}</span>
                  </div>
                </div>

                {/* Next Action */}
                {item.nextAction && (
                  <div className="relative z-10 mt-3 border-t border-line pt-2.5">
                    <div className="flex items-center justify-between text-[12px]">
                      <span className="text-ink-500">Next Action</span>
                      <span className="font-medium text-ink-950">{item.nextAction}</span>
                    </div>
                    {item.nextActionDate && (
                      <div className="text-[11px] text-ink-400 mt-0.5">
                        Due: {item.nextActionDate}
                      </div>
                    )}
                  </div>
                )}

                <div className="relative z-10 mt-2.5 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Last activity: {item.lastActivity}
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
          <Users className="h-4 w-4" />
          CRM-07 · Leads
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} leads shown
        </span>
      </div>
    </div>
  );
}