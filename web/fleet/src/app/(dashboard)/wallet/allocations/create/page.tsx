// src/app/(dashboard)/wallet/allocations/create/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  CreditCard,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Wallet,
  Users,
  MoreHorizontal,
  CalendarDays,
  Plus,
  X,
  ChevronDown,
  Search,
  User,
  Briefcase,
  MapPin,
  DollarSign,
  FileText,
  Send,
  Save,
} from "lucide-react";

type Provider = {
  id: string;
  name: string;
  initials: string;
  role: string;
  branch: string;
  walletBalance: number;
};

type AllocationType = "Trip Settlement" | "Compensation" | "Adjustment" | "Reimbursement" | "Bonus";

// Mock data
const MOCK_PROVIDERS: Provider[] = [
  { id: "P-044", name: "Peter Okafor", initials: "PO", role: "Dispatch Rider", branch: "Ikeja", walletBalance: 284000 },
  { id: "P-038", name: "Musa Bello", initials: "MB", role: "Hauling Driver", branch: "Apapa", walletBalance: 156000 },
  { id: "P-052", name: "Amina Yusuf", initials: "AY", role: "Dispatch Supervisor", branch: "Yaba", walletBalance: 42000 },
  { id: "P-061", name: "Chinedu Uche", initials: "CU", role: "Dispatch Rider", branch: "Lekki", walletBalance: 0 },
  { id: "P-029", name: "Kabiru Ahmed", initials: "KA", role: "Hauling Driver", branch: "Lekki", walletBalance: 89000 },
  { id: "P-073", name: "Esther Obi", initials: "EO", role: "Fleet Manager", branch: "Ikeja", walletBalance: 210000 },
];

const ALLOCATION_TYPES: { value: AllocationType; label: string; description: string }[] = [
  { value: "Trip Settlement", label: "Trip Settlement", description: "Settlement for completed trips" },
  { value: "Compensation", label: "Compensation", description: "Regular compensation payments" },
  { value: "Adjustment", label: "Adjustment", description: "Adjustments to previous allocations" },
  { value: "Reimbursement", label: "Reimbursement", description: "Expense reimbursements" },
  { value: "Bonus", label: "Bonus", description: "Performance or incentive bonuses" },
];

export default function CreateAllocationPage() {
  const router = useRouter();

  const [loading, setLoading] = useState(false);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [searchProviders, setSearchProviders] = useState("");
  const [showProviderDropdown, setShowProviderDropdown] = useState(false);

  // Form state
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [allocationType, setAllocationType] = useState<AllocationType>("Trip Settlement");
  const [amount, setAmount] = useState<string>("");
  const [description, setDescription] = useState("");
  const [businessReference, setBusinessReference] = useState("");
  const [reason, setReason] = useState("");
  const [showApprovalNotice, setShowApprovalNotice] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 400));
      setProviders(MOCK_PROVIDERS);
    };
    loadData();
  }, []);

  const filteredProviders = providers.filter(
    (p) =>
      p.name.toLowerCase().includes(searchProviders.toLowerCase()) ||
      p.id.toLowerCase().includes(searchProviders.toLowerCase()) ||
      p.role.toLowerCase().includes(searchProviders.toLowerCase())
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    // Simulate submission
    setTimeout(() => {
      setLoading(false);
      setShowApprovalNotice(true);
      // In real app, would redirect to allocation detail or approvals inbox
    }, 1500);
  };

  const handleProviderSelect = (provider: Provider) => {
    setSelectedProvider(provider);
    setSearchProviders(provider.name);
    setShowProviderDropdown(false);
  };

  const handleCancel = () => {
    router.push("/wallet/allocations");
  };

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  const parsedAmount = parseFloat(amount) || 0;
  const isValid = selectedProvider && parsedAmount > 0 && description.trim() !== "";

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={handleCancel}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
              Create Allocation
            </h1>
            <p className="mt-1 text-sm text-ink-600">
              Create a new allocation instruction for a provider. Routes to approval when required.
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={handleCancel}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={!isValid || loading}
            className={`flex h-9 items-center gap-1.5 rounded-xl px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition ${
              isValid && !loading
                ? "bg-primary hover:bg-primary/90 cursor-pointer"
                : "bg-ink-300 cursor-not-allowed"
            }`}
          >
            {loading ? (
              <>
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                Submitting...
              </>
            ) : (
              <>
                <Send className="h-3.5 w-3.5" />
                Submit
              </>
            )}
          </button>
        </div>
      </div>

      {/* ===== APPROVAL NOTICE ===== */}
      {showApprovalNotice && (
        <div className="rounded-xl border border-primary/20 bg-primary-bg p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-primary p-1.5">
              <CheckCircle2 className="h-4 w-4 text-white" />
            </div>
            <div>
              <div className="text-sm font-semibold text-primary">Allocation submitted for approval</div>
              <div className="text-sm text-primary/80">
                Your allocation request has been submitted and is awaiting approval.
              </div>
              <div className="mt-2 flex items-center gap-2">
                <button
                  onClick={() => router.push("/wallet/allocations")}
                  className="rounded-lg bg-primary px-3.5 py-1 text-[11px] font-medium text-white transition hover:bg-primary/90"
                >
                  View Allocations
                </button>
                <button
                  onClick={() => setShowApprovalNotice(false)}
                  className="rounded-lg border border-primary/30 px-3.5 py-1 text-[11px] font-medium text-primary transition hover:bg-primary/10"
                >
                  Continue Editing
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ===== FORM ===== */}
      <form onSubmit={handleSubmit}>
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
          {/* ===== LEFT COLUMN ===== */}
          <div className="space-y-5">
            {/* Provider Selection */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Provider</h3>
              <div className="relative">
                <div className="flex items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 py-2.5 shadow-sm focus-within:border-primary focus-within:ring-2 focus-within:ring-primary/20 transition">
                  <Search className="h-4 w-4 text-ink-400" />
                  <input
                    value={searchProviders}
                    onChange={(e) => {
                      setSearchProviders(e.target.value);
                      setShowProviderDropdown(true);
                      if (e.target.value === "") {
                        setSelectedProvider(null);
                      }
                    }}
                    onFocus={() => setShowProviderDropdown(true)}
                    placeholder="Search by name, ID, or role..."
                    className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
                  />
                  {selectedProvider && (
                    <button
                      type="button"
                      onClick={() => {
                        setSelectedProvider(null);
                        setSearchProviders("");
                      }}
                      className="text-ink-400 hover:text-ink-600"
                    >
                      <X className="h-4 w-4" />
                    </button>
                  )}
                </div>

                {/* Dropdown */}
                {showProviderDropdown && searchProviders.trim() !== "" && (
                  <div className="absolute left-0 right-0 top-full mt-1.5 z-20 max-h-[240px] overflow-y-auto rounded-xl border border-line bg-white p-1 shadow-xl">
                    {filteredProviders.length === 0 ? (
                      <div className="px-3 py-3 text-sm text-ink-500">No providers found</div>
                    ) : (
                      filteredProviders.map((provider) => (
                        <button
                          key={provider.id}
                          type="button"
                          onClick={() => handleProviderSelect(provider)}
                          className="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition hover:bg-surface-2"
                        >
                          <div className="flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full bg-primary text-xs font-bold text-white">
                            {provider.initials}
                          </div>
                          <div className="flex-1 min-w-0">
                            <div className="text-sm font-medium text-ink-950">{provider.name}</div>
                            <div className="flex items-center gap-2 text-xs text-ink-500">
                              <span className="font-mono">{provider.id}</span>
                              <span className="text-ink-300">·</span>
                              <span>{provider.role}</span>
                              <span className="text-ink-300">·</span>
                              <span>{provider.branch}</span>
                            </div>
                          </div>
                          <div className="text-right">
                            <div className="text-xs text-ink-400">Balance</div>
                            <div className="text-sm font-semibold text-ink-950">
                              {formatCurrency(provider.walletBalance)}
                            </div>
                          </div>
                        </button>
                      ))
                    )}
                  </div>
                )}
              </div>

              {selectedProvider && (
                <div className="mt-3 rounded-lg bg-surface-2 p-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary text-xs font-bold text-white">
                        {selectedProvider.initials}
                      </div>
                      <div>
                        <div className="text-sm font-medium text-ink-950">{selectedProvider.name}</div>
                        <div className="flex items-center gap-2 text-xs text-ink-500">
                          <span className="font-mono">{selectedProvider.id}</span>
                          <span className="text-ink-300">·</span>
                          <span>{selectedProvider.role}</span>
                          <span className="text-ink-300">·</span>
                          <span>{selectedProvider.branch}</span>
                        </div>
                      </div>
                    </div>
                    <div className="text-right">
                      <div className="text-xs text-ink-400">Current Balance</div>
                      <div className="text-sm font-semibold text-ink-950">
                        {formatCurrency(selectedProvider.walletBalance)}
                      </div>
                    </div>
                  </div>
                </div>
              )}
            </div>

            {/* Allocation Details */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Allocation Details</h3>
              <div className="space-y-4">
                {/* Type */}
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Allocation Type
                  </label>
                  <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
                    {ALLOCATION_TYPES.map((type) => (
                      <button
                        key={type.value}
                        type="button"
                        onClick={() => setAllocationType(type.value)}
                        className={`rounded-lg border p-2.5 text-left transition ${
                          allocationType === type.value
                            ? "border-primary bg-primary-bg"
                            : "border-line hover:bg-surface-2"
                        }`}
                      >
                        <div className={`text-xs font-medium ${allocationType === type.value ? "text-primary" : "text-ink-600"}`}>
                          {type.label}
                        </div>
                        <div className="text-[10px] text-ink-400">{type.description}</div>
                      </button>
                    ))}
                  </div>
                </div>

                {/* Amount */}
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Amount
                  </label>
                  <div className="relative">
                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-sm text-ink-500">₦</span>
                    <input
                      type="number"
                      value={amount}
                      onChange={(e) => setAmount(e.target.value)}
                      placeholder="0.00"
                      className="w-full rounded-xl border border-line bg-white/70 px-8 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    />
                  </div>
                </div>

                {/* Description */}
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Description
                  </label>
                  <input
                    value={description}
                    onChange={(e) => setDescription(e.target.value)}
                    placeholder="Brief description of this allocation..."
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>

                {/* Business Reference */}
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Business Reference
                  </label>
                  <input
                    value={businessReference}
                    onChange={(e) => setBusinessReference(e.target.value)}
                    placeholder="Order, trip, or reference number..."
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
              </div>
            </div>

            {/* Reason */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Supporting Basis</h3>
              <textarea
                value={reason}
                onChange={(e) => setReason(e.target.value)}
                placeholder="Provide the basis for this allocation (e.g., completed trips, contract terms, etc.)..."
                rows={3}
                className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
              />
              <p className="mt-1.5 text-xs text-ink-400">
                Required supporting basis helps approvers understand the allocation reason.
              </p>
            </div>
          </div>

          {/* ===== RIGHT COLUMN ===== */}
          <div className="space-y-5">
            {/* Summary */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Summary</h3>
              <div className="space-y-3">
                <div className="flex items-center justify-between border-b border-line pb-2 text-sm">
                  <span className="text-ink-500">Provider</span>
                  <span className="font-medium text-ink-950">{selectedProvider?.name || "—"}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2 text-sm">
                  <span className="text-ink-500">Type</span>
                  <span className="font-medium text-ink-950">{allocationType}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2 text-sm">
                  <span className="text-ink-500">Amount</span>
                  <span className={`font-semibold ${parsedAmount > 0 ? "text-ink-950" : "text-ink-400"}`}>
                    {parsedAmount > 0 ? formatCurrency(parsedAmount) : "—"}
                  </span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2 text-sm">
                  <span className="text-ink-500">Reference</span>
                  <span className="font-medium text-ink-950 font-mono text-[12px]">
                    {businessReference || "—"}
                  </span>
                </div>
                <div className="pt-2">
                  <div className="text-xs text-ink-400">Description</div>
                  <div className="text-sm text-ink-700 mt-0.5">{description || "—"}</div>
                </div>
              </div>
            </div>

            {/* Approval Info */}
            <div className="rounded-2xl border border-line bg-amber-bg/20 p-4">
              <div className="flex items-start gap-2.5">
                <AlertCircle className="h-4 w-4 flex-shrink-0 text-amber" />
                <div className="text-sm text-amber/80">
                  <span className="font-medium">Approval required:</span> This allocation will route to the configured approval workflow based on company policy.
                </div>
              </div>
            </div>

            {/* Quick Actions */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Quick Actions</h3>
              <div className="space-y-2">
                <button
                  type="button"
                  onClick={() => {
                    setSelectedProvider(null);
                    setSearchProviders("");
                    setAmount("");
                    setDescription("");
                    setBusinessReference("");
                    setReason("");
                    setAllocationType("Trip Settlement");
                  }}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <RefreshCw className="h-4 w-4" />
                    Reset Form
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
                <button
                  type="button"
                  onClick={() => router.push("/wallet/allocations")}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <ArrowLeft className="h-4 w-4" />
                    Back to Allocations
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              </div>
            </div>
          </div>
        </div>
      </form>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <CreditCard className="h-4 w-4" />
          WAL-07 · Create Allocation
        </span>
        <span className="flex items-center gap-1.5">
          <AlertCircle className="h-4 w-4" />
          Routes to approval when required
        </span>
      </div>
    </div>
  );
}