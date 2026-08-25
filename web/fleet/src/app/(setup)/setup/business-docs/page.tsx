// src/app/(setup)/setup/business-docs/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  X,
  AlertCircle,
  Upload,
  ArrowRight,
  FileText,
  Trash2,
} from "lucide-react";

// ==================== TYPES ====================

type BusinessDocument = {
  id: string;
  name: string;
  type: "Registration" | "Tax" | "Insurance" | "License" | "Compliance" | "Other";
  status: "Uploaded" | "Pending" | "Verified" | "Rejected" | "Expired";
  fileName: string;
  fileSize: string;
  uploadedAt: string;
  uploadedBy: string;
  expiryDate: string | null;
  notes: string | null;
};

// ==================== MOCK DATA ====================

const MOCK_DOCUMENTS: BusinessDocument[] = [
  {
    id: "DOC-001",
    name: "Business Registration",
    type: "Registration",
    status: "Verified",
    fileName: "business_registration.pdf",
    fileSize: "2.4 MB",
    uploadedAt: "2026-08-20T10:00:00Z",
    uploadedBy: "F. Daniel",
    expiryDate: null,
    notes: "Verified against government database.",
  },
  {
    id: "DOC-002",
    name: "Tax ID Certificate",
    type: "Tax",
    status: "Pending",
    fileName: "tax_id_cert.pdf",
    fileSize: "1.8 MB",
    uploadedAt: "2026-08-22T09:00:00Z",
    uploadedBy: "F. Daniel",
    expiryDate: null,
    notes: "Awaiting verification.",
  },
  {
    id: "DOC-003",
    name: "Insurance Policy",
    type: "Insurance",
    status: "Uploaded",
    fileName: "insurance_policy.pdf",
    fileSize: "3.2 MB",
    uploadedAt: "2026-08-21T14:30:00Z",
    uploadedBy: "F. Daniel",
    expiryDate: "2026-12-31",
    notes: null,
  },
  {
    id: "DOC-004",
    name: "Operating License",
    type: "License",
    status: "Rejected",
    fileName: "operating_license.pdf",
    fileSize: "1.2 MB",
    uploadedAt: "2026-08-19T11:00:00Z",
    uploadedBy: "F. Daniel",
    expiryDate: null,
    notes: "License number mismatch. Please re-upload with correct details.",
  },
];

type StatusStyle = {
  bg: string;
  text: string;
  icon: React.ReactNode;
};

const STATUS_STYLES: Record<string, StatusStyle> = {
  Uploaded: { bg: "bg-blue-bg", text: "text-blue", icon: <Upload className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

type TypeStyle = {
  bg: string;
  text: string;
};

const TYPE_STYLES: Record<string, TypeStyle> = {
  Registration: { bg: "bg-blue-bg", text: "text-blue" },
  Tax: { bg: "bg-purple-bg", text: "text-purple" },
  Insurance: { bg: "bg-green-bg", text: "text-green" },
  License: { bg: "bg-amber-bg", text: "text-amber" },
  Compliance: { bg: "bg-red-bg", text: "text-red" },
  Other: { bg: "bg-surface-2", text: "text-ink-500" },
};

export default function BusinessDocumentsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState<boolean>(true);
  const [data, setData] = useState<BusinessDocument[]>([]);
  const [secondsAgo, setSecondsAgo] = useState<number>(0);
  const [isUploading, setIsUploading] = useState<boolean>(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_DOCUMENTS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleUpload = async () => {
    setIsUploading(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setIsUploading(false);
    toast("Document uploaded successfully! (mock)");
  };

  const handleDelete = (id: string) => {
    setData((prev) => prev.filter((doc) => doc.id !== id));
    toast("Document removed (mock)");
  };

  const handleBack = () => {
    router.push("/setup");
  };

  const handleNext = () => {
    router.push("/setup/verification");
  };

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading documents...</p>
      </div>
    );
  }

  const formatDate = (date: string | null): string => {
    if (!date) return "N/A";
    const d = new Date(date);
    return d.toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto">
      {/* ===== HEADER ===== */}
      <div>
        <div className="flex items-center gap-2 text-sm text-ink-500">
          <button onClick={handleBack} className="hover:text-primary transition-colors">
            Onboarding
          </button>
          <span>/</span>
          <span className="text-ink-700 font-medium">Business Documents</span>
        </div>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink-950 sm:text-3xl">
          Business Documents
        </h1>
        <p className="mt-1.5 text-sm text-ink-600">
          Required documents for company verification.
        </p>
      </div>

      {/* ===== UPLOAD BUTTON ===== */}
      <button
        onClick={handleUpload}
        disabled={isUploading}
        className="flex items-center gap-2 rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90 disabled:opacity-50 disabled:cursor-not-allowed"
      >
        {isUploading ? (
          <>
            <RefreshCw className="h-4 w-4 animate-spin" />
            Uploading...
          </>
        ) : (
          <>
            <Upload className="h-4 w-4" />
            Upload Document
          </>
        )}
      </button>

      {/* ===== DOCUMENTS TABLE ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full border-collapse text-sm">
            <thead>
              <tr className="bg-surface-2/50">
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Document</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Type</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">File</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Status</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500">Expiry</th>
                <th className="border-b border-line px-4 py-2.5 text-left text-[10px] font-bold uppercase tracking-wide text-ink-500"></th>
              </tr>
            </thead>
            <tbody>
              {data.map((doc) => {
                const statusStyle = STATUS_STYLES[doc.status] || STATUS_STYLES.Uploaded;
                const typeStyle = TYPE_STYLES[doc.type] || TYPE_STYLES.Other;

                return (
                  <tr key={doc.id} className="border-b border-line hover:bg-surface-2 transition-colors">
                    <td className="px-4 py-3">
                      <div className="font-medium text-ink-950 text-xs">{doc.name}</div>
                      <div className="text-[10px] text-ink-500">Uploaded: {formatDate(doc.uploadedAt)}</div>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${typeStyle.bg} ${typeStyle.text}`}>
                        {doc.type}
                      </span>
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-1.5 text-xs text-ink-600">
                        <FileText className="h-3.5 w-3.5 text-ink-400" />
                        {doc.fileName}
                        <span className="text-ink-400">({doc.fileSize})</span>
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                        {statusStyle.icon}
                        {doc.status}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-xs text-ink-500">
                      {doc.expiryDate ? formatDate(doc.expiryDate) : "—"}
                    </td>
                    <td className="px-4 py-3 text-right">
                      <button
                        onClick={() => handleDelete(doc.id)}
                        className="text-ink-400 hover:text-red transition-colors"
                      >
                        <Trash2 className="h-4 w-4" />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      </div>

      {/* ===== NEXT ===== */}
      <div className="flex flex-wrap gap-3">
        <button
          onClick={handleNext}
          className="flex items-center gap-2 rounded-xl bg-primary px-6 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
        >
          Next Step <ArrowRight className="h-4 w-4" />
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
          <FileText className="h-4 w-4" />
          ORG-05 · Business Documents
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