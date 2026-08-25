// src/app/(dashboard)/settings/audit/verification/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  Shield,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertTriangle,
} from "lucide-react";

// ==================== TYPES ====================

type VerificationMonth = {
  month: string;
  year: number;
  recordCount: number;
  result: "Intact" | "Broken";
  breakReason: string | null;
  breakReference: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_VERIFICATION_MONTHS: VerificationMonth[] = [
  { month: "August", year: 2026, recordCount: 342, result: "Intact", breakReason: null, breakReference: null },
  { month: "July", year: 2026, recordCount: 318, result: "Intact", breakReason: null, breakReference: null },
  { month: "June", year: 2026, recordCount: 289, result: "Intact", breakReason: null, breakReference: null },
  { month: "May", year: 2026, recordCount: 256, result: "Broken", breakReason: "Data migration event", breakReference: "MIG-2026-001" },
  { month: "April", year: 2026, recordCount: 231, result: "Intact", breakReason: null, breakReference: null },
  { month: "March", year: 2026, recordCount: 198, result: "Intact", breakReason: null, breakReference: null },
  { month: "February", year: 2026, recordCount: 167, result: "Intact", breakReason: null, breakReference: null },
  { month: "January", year: 2026, recordCount: 145, result: "Intact", breakReason: null, breakReference: null },
];

const RESULT_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Intact: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-4 w-4" /> },
  Broken: { bg: "bg-red-bg", text: "text-red", icon: <AlertTriangle className="h-4 w-4" /> },
};

export default function AuditChainVerificationPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<VerificationMonth[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isVerifying, setIsVerifying] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_VERIFICATION_MONTHS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleVerify = async () => {
    setIsVerifying(true);
    await new Promise((resolve) => setTimeout(resolve, 2000));
    setIsVerifying(false);
    toast("Audit chain verification completed successfully (mock)");
  };

  const handleBack = () => {
    router.push("/settings/audit");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading verification data...</p>
      </div>
    );
  }

  // Stats
  const totalRecords = data.reduce((acc, m) => acc + m.recordCount, 0);
  const brokenMonths = data.filter((m) => m.result === "Broken").length;
  const intactMonths = data.filter((m) => m.result === "Intact").length;

  return (
    <div className="space-y-5 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button
            onClick={() => router.push("/settings/audit")}
            className="hover:text-primary transition-colors"
          >
            Audit
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Chain Verification</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950">
          Audit Chain Verification
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Monthly verification of audit chain integrity.
        </p>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Records</div>
          <div className="text-lg font-bold text-ink-950">{totalRecords.toLocaleString()}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Months Verified</div>
          <div className="text-lg font-bold text-ink-950">{data.length}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Intact</div>
          <div className="text-lg font-bold text-green">{intactMonths}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Broken</div>
          <div className="text-lg font-bold text-red">{brokenMonths}</div>
        </div>
      </div>

      {/* ===== VERIFICATION TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-4 border-b border-line bg-surface-2/50">
          <div className="flex items-center gap-2">
            <Shield className="h-4 w-4 text-ink-400" />
            <span className="text-sm font-semibold text-ink-950">Monthly Verification</span>
          </div>
        </div>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Month</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Records</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Result</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Break Reason</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Reference</th>
              </tr>
            </thead>
            <tbody>
              {data.map((item) => {
                const resultStyle = RESULT_STYLES[item.result] || RESULT_STYLES.Intact;

                return (
                  <tr key={`${item.month}-${item.year}`} className="border-b border-line hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950">{item.month}</div>
                      <div className="text-[10px] text-ink-500">{item.year}</div>
                    </td>
                    <td className="px-4 py-3 text-ink-600">{item.recordCount.toLocaleString()}</td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${resultStyle.bg} ${resultStyle.text}`}>
                        {resultStyle.icon}
                        {item.result}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-ink-600">
                      {item.breakReason || <span className="text-ink-400">—</span>}
                    </td>
                    <td className="px-4 py-3 text-ink-600">
                      {item.breakReference || <span className="text-ink-400">—</span>}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== ACTIONS ===== */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={handleVerify}
          disabled={isVerifying}
          className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
            isVerifying
              ? "bg-ink-400 cursor-not-allowed"
              : "bg-primary shadow-primary/20 hover:bg-primary/90"
          }`}
        >
          {isVerifying ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Verifying...
            </>
          ) : (
            <>
              <Shield className="h-4 w-4" />
              Run Verification
            </>
          )}
        </button>
        <button
          onClick={handleBack}
          className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
        >
          Back
        </button>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Shield className="h-4 w-4" />
          SEC-04 · Audit Chain Verification
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