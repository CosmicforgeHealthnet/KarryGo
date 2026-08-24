// src/app/(dashboard)/operations/marketplace/[offerId]/assign/page.tsx

"use client";

import { useState, useEffect } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  UserCheck,
  Truck,
  Clock,
  MapPin,
  Package,
  AlertCircle,
  CheckCircle2,
  ChevronDown,
  Search,
  X,
} from "lucide-react";

// Types
type Provider = {
  id: string;
  name: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Eligible" | "Warning" | "Blocked";
  asset: string;
  availability: "Available" | "Busy" | "Offline";
};

type Asset = {
  id: string;
  name: string;
  type: string;
  branch: string;
  status: "Available" | "Maintenance" | "Reserved" | "Blocked";
};

// Mock data
const PROVIDERS: Provider[] = [
  { id: "P-044", name: "Peter Okafor", service: "Dispatch", branch: "Ikeja", status: "Eligible", asset: "Bike BR-88", availability: "Available" },
  { id: "P-061", name: "Chinedu Uche", service: "Dispatch", branch: "Lekki", status: "Eligible", asset: "Bike BR-20", availability: "Available" },
  { id: "P-052", name: "Amina Yusuf", service: "Dispatch", branch: "Ikeja", status: "Warning", asset: "Bike BR-71", availability: "Available" },
];

const ASSETS: Asset[] = [
  { id: "BR-88", name: "Honda ACE 110", type: "Motorcycle", branch: "Ikeja", status: "Available" },
  { id: "BR-20", name: "Yamaha FZ", type: "Motorcycle", branch: "Lekki", status: "Available" },
  { id: "VN-04", name: "Mercedes Sprinter", type: "Van", branch: "Ikeja", status: "Reserved" },
  { id: "TR-12", name: "Scania R500", type: "Truck", branch: "Apapa", status: "Maintenance" },
];

const DEADLINE_OPTIONS = ["15 min", "30 min", "45 min", "1 hour", "2 hours"];

export default function AssignPage() {
  const params = useParams();
  const router = useRouter();
  const offerId = params.offerId as string;

  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  // Form state
  const [selectedProvider, setSelectedProvider] = useState<string>("");
  const [selectedAsset, setSelectedAsset] = useState<string>("");
  const [deadline, setDeadline] = useState("30 min");
  const [instructions, setInstructions] = useState("");
  const [searchProvider, setSearchProvider] = useState("");
  const [searchAsset, setSearchAsset] = useState("");

  // Check for candidate param from candidates page
  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const candidateId = urlParams.get("candidate");
    if (candidateId) {
      const provider = PROVIDERS.find(p => p.id === candidateId && p.availability === "Available" && p.status === "Eligible");
      if (provider) {
        setSelectedProvider(provider.id);
        const asset = ASSETS.find(a => a.id === provider.asset && a.status === "Available");
        if (asset) {
          setSelectedAsset(asset.id);
        }
      }
    }
  }, []);

  useEffect(() => {
    const timer = setTimeout(() => setLoading(false), 400);
    return () => clearTimeout(timer);
  }, []);

  const filteredProviders = PROVIDERS.filter(
    (p) =>
      p.name.toLowerCase().includes(searchProvider.toLowerCase()) ||
      p.id.toLowerCase().includes(searchProvider.toLowerCase())
  );

  const filteredAssets = ASSETS.filter(
    (a) =>
      a.name.toLowerCase().includes(searchAsset.toLowerCase()) ||
      a.id.toLowerCase().includes(searchAsset.toLowerCase())
  );

  const selectedProviderData = PROVIDERS.find((p) => p.id === selectedProvider);
  const selectedAssetData = ASSETS.find((a) => a.id === selectedAsset);

  const isFormValid = selectedProvider && selectedAsset && deadline;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!isFormValid) return;

    setSubmitting(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setSubmitting(false);

    router.push(`/operations/assignments/ASN-${Date.now().toString().slice(-6)}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-4xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent" />
          <p className="ml-3">Loading...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      {/* ── Header ── */}
      <div>
        <h1>Create assignment</h1>
        <p className="mt-1.5">
          Assign a provider and asset to offer <span className="font-mono text-ink-800">{offerId}</span>
        </p>
      </div>

      {/* ── Form ── */}
      <form onSubmit={handleSubmit} className="space-y-3.5">
        {/* Offer Summary */}
        <div className="rounded-lg border border-line bg-white p-4">
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex items-center gap-2">
              <Package className="h-4 w-4 text-ink-400" />
              <span className="font-semibold text-ink-950">{offerId}</span>
            </div>
            <div className="flex items-center gap-2">
              <MapPin className="h-4 w-4 text-ink-400" />
              <span>Ikeja → Lekki</span>
            </div>
            <div className="flex items-center gap-2">
              <Clock className="h-4 w-4 text-ink-400" />
              <span className="rounded-full bg-amber-bg px-2 py-0.5 text-xs font-semibold text-amber">
                27 min left
              </span>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-3.5 lg:grid-cols-2">
          {/* Select Provider */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-2">
              <label className="text-xs font-semibold text-ink-700">
                Select provider <span className="text-red">*</span>
              </label>

              <div className="flex h-9 items-center gap-2 rounded-lg border border-line bg-white px-3">
                <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-400" />
                <input
                  value={searchProvider}
                  onChange={(e) => setSearchProvider(e.target.value)}
                  placeholder="Search provider..."
                  className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
                />
              </div>

              <div className="max-h-52 overflow-y-auto space-y-1.5">
                {filteredProviders.length === 0 ? (
                  <div className="py-4 text-center text-sm text-ink-500">No providers found</div>
                ) : (
                  filteredProviders.map((p) => {
                    const isSelected = selectedProvider === p.id;
                    const isAvailable = p.availability === "Available" && p.status === "Eligible";

                    return (
                      <button
                        key={p.id}
                        type="button"
                        onClick={() => isAvailable && setSelectedProvider(p.id)}
                        className={`w-full rounded-lg border p-2.5 text-left transition ${
                          isSelected
                            ? "border-primary bg-primary-bg ring-2 ring-primary/30"
                            : isAvailable
                            ? "border-line hover:bg-surface-2"
                            : "border-line opacity-60 cursor-not-allowed"
                        }`}
                        disabled={!isAvailable}
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <div className="flex items-center gap-1.5">
                              <span className="font-semibold text-ink-950">{p.name}</span>
                              <span className="font-mono text-[10px] text-ink-400">{p.id}</span>
                            </div>
                            <div className="flex items-center gap-2 text-xs text-ink-500">
                              <span>{p.service}</span>
                              <span className="text-ink-300">·</span>
                              <span>{p.asset}</span>
                            </div>
                          </div>
                          <div className="flex items-center gap-1.5">
                            {p.status === "Warning" && (
                              <span className="text-[10px] font-semibold text-amber">⚠ Warning</span>
                            )}
                            <span
                              className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                                p.availability === "Available"
                                  ? "bg-green-bg text-green"
                                  : p.availability === "Busy"
                                  ? "bg-amber-bg text-amber"
                                  : "bg-surface-2 text-ink-500"
                              }`}
                            >
                              <span
                                className={`h-1.5 w-1.5 rounded-full ${
                                  p.availability === "Available"
                                    ? "bg-green"
                                    : p.availability === "Busy"
                                    ? "bg-amber"
                                    : "bg-ink-400"
                                }`}
                              />
                              {p.availability}
                            </span>
                          </div>
                        </div>
                      </button>
                    );
                  })
                )}
              </div>
            </div>
          </div>

          {/* Select Asset */}
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-2">
              <label className="text-xs font-semibold text-ink-700">
                Select asset <span className="text-red">*</span>
              </label>

              <div className="flex h-9 items-center gap-2 rounded-lg border border-line bg-white px-3">
                <Search className="h-3.5 w-3.5 flex-shrink-0 text-ink-400" />
                <input
                  value={searchAsset}
                  onChange={(e) => setSearchAsset(e.target.value)}
                  placeholder="Search asset..."
                  className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
                />
              </div>

              <div className="max-h-52 overflow-y-auto space-y-1.5">
                {filteredAssets.length === 0 ? (
                  <div className="py-4 text-center text-sm text-ink-500">No assets found</div>
                ) : (
                  filteredAssets.map((a) => {
                    const isSelected = selectedAsset === a.id;
                    const isAvailable = a.status === "Available";

                    return (
                      <button
                        key={a.id}
                        type="button"
                        onClick={() => isAvailable && setSelectedAsset(a.id)}
                        className={`w-full rounded-lg border p-2.5 text-left transition ${
                          isSelected
                            ? "border-primary bg-primary-bg ring-2 ring-primary/30"
                            : isAvailable
                            ? "border-line hover:bg-surface-2"
                            : "border-line opacity-60 cursor-not-allowed"
                        }`}
                        disabled={!isAvailable}
                      >
                        <div className="flex items-center justify-between">
                          <div>
                            <div className="flex items-center gap-1.5">
                              <span className="font-semibold text-ink-950">{a.name}</span>
                              <span className="font-mono text-[10px] text-ink-400">{a.id}</span>
                            </div>
                            <div className="flex items-center gap-2 text-xs text-ink-500">
                              <span>{a.type}</span>
                              <span className="text-ink-300">·</span>
                              <span>{a.branch}</span>
                            </div>
                          </div>
                          <span
                            className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                              a.status === "Available"
                                ? "bg-green-bg text-green"
                                : a.status === "Reserved"
                                ? "bg-amber-bg text-amber"
                                : "bg-surface-2 text-ink-500"
                            }`}
                          >
                            {a.status}
                          </span>
                        </div>
                      </button>
                    );
                  })
                )}
              </div>
            </div>
          </div>
        </div>

        {/* Deadline & Instructions */}
        <div className="grid grid-cols-1 gap-3.5 md:grid-cols-2">
          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-2">
              <label className="text-xs font-semibold text-ink-700">
                Response deadline <span className="text-red">*</span>
              </label>
              <div className="relative">
                <select
                  value={deadline}
                  onChange={(e) => setDeadline(e.target.value)}
                  className="w-full appearance-none rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {DEADLINE_OPTIONS.map((opt) => (
                    <option key={opt} value={opt}>
                      {opt}
                    </option>
                  ))}
                </select>
                <ChevronDown className="absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-ink-400 pointer-events-none" />
              </div>
            </div>
          </div>

          <div className="rounded-lg border border-line bg-white p-4">
            <div className="space-y-2">
              <label className="text-xs font-semibold text-ink-700">Instructions (optional)</label>
              <textarea
                value={instructions}
                onChange={(e) => setInstructions(e.target.value)}
                placeholder="Add any special instructions for this assignment..."
                className="w-full rounded-lg border border-line px-3 py-2 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none h-14 placeholder:text-ink-400"
              />
            </div>
          </div>
        </div>

        {/* Selected Summary */}
        {(selectedProviderData || selectedAssetData) && (
          <div className="rounded-lg border border-line bg-primary-bg/30 p-4">
            <div className="flex flex-wrap items-center gap-4">
              <div className="flex items-center gap-2 text-sm text-ink-600">
                <UserCheck className="h-4 w-4 text-primary" />
                <span className="font-semibold text-ink-950">Assignment ready</span>
              </div>
              {selectedProviderData && (
                <div className="flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-700">
                  <span className="h-1.5 w-1.5 rounded-full bg-green" />
                  {selectedProviderData.name}
                </div>
              )}
              {selectedAssetData && (
                <div className="flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-700">
                  <Truck className="h-3 w-3 text-ink-400" />
                  {selectedAssetData.id}
                </div>
              )}
              <div className="flex items-center gap-1.5 rounded-full border border-line bg-white px-2.5 py-1 text-xs font-medium text-ink-700">
                <Clock className="h-3 w-3 text-ink-400" />
                {deadline}
              </div>
            </div>
          </div>
        )}

        {/* Actions */}
        <div className="flex flex-col-reverse gap-2.5 sm:flex-row sm:justify-end">
          <Link
            href={`/operations/marketplace/${offerId}`}
            className="flex items-center justify-center gap-2 rounded-lg border border-line bg-white px-5 py-2.5 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
          >
            <X className="h-4 w-4" />
            Cancel
          </Link>
          <button
            type="submit"
            disabled={!isFormValid || submitting}
            className={`flex items-center justify-center gap-2 rounded-lg bg-primary px-5 py-2.5 text-sm font-semibold text-white transition-colors hover:bg-primary-hover disabled:opacity-60 disabled:cursor-not-allowed`}
          >
            {submitting ? (
              <>
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                Creating...
              </>
            ) : (
              <>
                <UserCheck className="h-4 w-4" />
                Create assignment
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}