// src/app/(dashboard)/wallet/providers/page.tsx

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
  Eye,
  ArrowRight,
  Sparkles,
  TrendingUp,
  TrendingDown,
  Wallet,
  User,
  MapPin,
  MoreHorizontal,
  CalendarDays,
  Download,
  FileText,
} from "lucide-react";

type ProviderSubWallet = {
  id: string;
  providerId: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  balance: number;
  currency: string;
  state: "Active" | "Pending" | "Suspended" | "Reconciling";
  lastUpdated: string;
  allocationCount: number;
  withdrawalCount: number;
  statementAvailable: boolean;
};

// Mock data
const PROVIDER_WALLETS: ProviderSubWallet[] = [
  {
    id: "PW-001",
    providerId: "P-044",
    name: "Peter Okafor",
    initials: "PO",
    role: "Dispatch Rider",
    branch: "Ikeja",
    balance: 284000,
    currency: "NGN",
    state: "Active",
    lastUpdated: "12 sec ago",
    allocationCount: 8,
    withdrawalCount: 3,
    statementAvailable: true,
  },
  {
    id: "PW-002",
    providerId: "P-038",
    name: "Musa Bello",
    initials: "MB",
    role: "Hauling Driver",
    branch: "Apapa",
    balance: 156000,
    currency: "NGN",
    state: "Active",
    lastUpdated: "45 sec ago",
    allocationCount: 5,
    withdrawalCount: 2,
    statementAvailable: true,
  },
  {
    id: "PW-003",
    providerId: "P-052",
    name: "Amina Yusuf",
    initials: "AY",
    role: "Dispatch Supervisor",
    branch: "Yaba",
    balance: 42000,
    currency: "NGN",
    state: "Pending",
    lastUpdated: "2 min ago",
    allocationCount: 3,
    withdrawalCount: 1,
    statementAvailable: true,
  },
  {
    id: "PW-004",
    providerId: "P-061",
    name: "Chinedu Uche",
    initials: "CU",
    role: "Dispatch Rider",
    branch: "Lekki",
    balance: 0,
    currency: "NGN",
    state: "Suspended",
    lastUpdated: "4 min ago",
    allocationCount: 2,
    withdrawalCount: 0,
    statementAvailable: false,
  },
  {
    id: "PW-005",
    providerId: "P-029",
    name: "Kabiru Ahmed",
    initials: "KA",
    role: "Hauling Driver",
    branch: "Lekki",
    balance: 89000,
    currency: "NGN",
    state: "Reconciling",
    lastUpdated: "1 hour ago",
    allocationCount: 4,
    withdrawalCount: 1,
    statementAvailable: true,
  },
  {
    id: "PW-006",
    providerId: "P-073",
    name: "Esther Obi",
    initials: "EO",
    role: "Fleet Manager",
    branch: "Ikeja",
    balance: 210000,
    currency: "NGN",
    state: "Active",
    lastUpdated: "3 min ago",
    allocationCount: 7,
    withdrawalCount: 2,
    statementAvailable: true,
  },
];

const STATE_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
  Suspended: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Reconciling: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <RefreshCw className="h-3.5 w-3.5" />,
  },
};

const BRANCHES = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
const STATE_FILTERS = ["All", "Active", "Pending", "Suspended", "Reconciling"];

export default function ProviderSubWalletsPage() {
  const router = useRouter();
  const [search, setSearch] = useState("");
  const [selectedBranch, setSelectedBranch] = useState("All branches");
  const [selectedState, setSelectedState] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ProviderSubWallet[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(PROVIDER_WALLETS);
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
      item.providerId.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase());

    const matchesBranch = selectedBranch === "All branches" || item.branch === selectedBranch;
    const matchesState = selectedState === "All" || item.state === selectedState;

    return matchesSearch && matchesBranch && matchesState;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading provider sub-wallets...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedBranch !== "All branches" || selectedState !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedBranch("All branches");
    setSelectedState("All");
  };

  const goToProviderWallet = (providerId: string) => {
    router.push(`/wallet/providers/${providerId}`);
  };

  // Stats
  const totalBalance = data.reduce((acc, item) => acc + item.balance, 0);
  const activeCount = data.filter((item) => item.state === "Active").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Provider Sub-wallets
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Organization-managed provider wallet states, balances, and permitted actions.
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
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            Export
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Providers</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Active</div>
          <div className="text-lg font-bold text-green">{activeCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Balance</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(totalBalance)}
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Avg Balance</div>
          <div className="text-lg font-bold text-ink-950">
            {new Intl.NumberFormat("en-NG", { style: "currency", currency: "NGN", minimumFractionDigits: 0 }).format(Math.round(totalBalance / data.length))}
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
            placeholder="Search by name, ID, or role..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const branches = ["All branches", "Ikeja", "Lekki", "Yaba", "Apapa"];
            const currentIndex = branches.indexOf(selectedBranch);
            const nextIndex = (currentIndex + 1) % branches.length;
            setSelectedBranch(branches[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <MapPin className="h-4 w-4 text-ink-400" />
          {selectedBranch}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const states = ["All", "Active", "Pending", "Suspended", "Reconciling"];
            const currentIndex = states.indexOf(selectedState);
            const nextIndex = (currentIndex + 1) % states.length;
            setSelectedState(states[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Wallet className="h-4 w-4 text-ink-400" />
          {selectedState}
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
              {(selectedBranch !== "All branches" ? 1 : 0) + (selectedState !== "All" ? 1 : 0)}
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
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">State</div>
              <div className="flex gap-1.5">
                {STATE_FILTERS.map((state) => (
                  <button
                    key={state}
                    onClick={() => setSelectedState(state)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedState === state
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {state}
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

      {/* ===== PROVIDER CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No providers found</h3>
          <p className="mt-1 text-sm text-ink-500">No provider sub-wallets match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filteredData.map((item) => {
            const stateStyle = STATE_STYLES[item.state] || STATE_STYLES.Active;
            const formattedBalance = new Intl.NumberFormat("en-NG", {
              style: "currency",
              currency: "NGN",
              minimumFractionDigits: 0,
              maximumFractionDigits: 0,
            }).format(item.balance);

            return (
              <div
                key={item.id}
                onClick={() => goToProviderWallet(item.providerId)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.state === "Active"
                    ? "border-green/20 bg-white hover:border-green/40"
                    : item.state === "Suspended"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : item.state === "Reconciling"
                    ? "border-blue/20 bg-blue-bg/10 hover:border-blue/40"
                    : "border-amber/20 bg-amber-bg/10 hover:border-amber/40"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.state === "Active" ? "bg-green-500/5" :
                  item.state === "Suspended" ? "bg-red-500/5" :
                  item.state === "Reconciling" ? "bg-blue-500/5" :
                  "bg-amber-500/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full text-[16px] font-bold text-white shadow-lg ${
                        item.state === "Active" ? "bg-primary" :
                        item.state === "Suspended" ? "bg-red" :
                        item.state === "Reconciling" ? "bg-blue" :
                        "bg-amber"
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
                          {item.providerId}
                        </span>
                      </div>
                      <div className="mt-0.5 flex flex-wrap items-center gap-1.5">
                        <span className="text-[12px] text-ink-500">{item.role}</span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">{item.branch}</span>
                      </div>
                    </div>
                  </div>
                  <span
                    className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${stateStyle.bg} ${stateStyle.text}`}
                  >
                    {stateStyle.icon}
                    {item.state}
                  </span>
                </div>

                <div className="relative z-10 mt-3 flex items-center justify-between">
                  <div>
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Balance</div>
                    <div className={`text-lg font-bold ${item.balance > 0 ? "text-ink-950" : "text-ink-400"}`}>
                      {formattedBalance}
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Allocations</div>
                    <div className="text-sm font-semibold text-ink-950">{item.allocationCount}</div>
                  </div>
                  <div className="text-right">
                    <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Withdrawals</div>
                    <div className="text-sm font-semibold text-ink-950">{item.withdrawalCount}</div>
                  </div>
                </div>

                <div className="relative z-10 mt-3 flex items-center justify-between border-t border-line pt-2.5">
                  <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
                    <Clock className="h-3.5 w-3.5" />
                    Updated: {item.lastUpdated}
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
          WAL-04 · Provider Sub-wallets
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} providers shown
        </span>
      </div>
    </div>
  );
}