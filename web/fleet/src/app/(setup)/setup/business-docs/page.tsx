// src/app/(setup)/setup/business-docs/page.tsx
// Screen: ORG05 - Business Documents
// Rebuilt from a dense data table into a required-documents checklist,
// matching the sectioned-card / icon-tile language used across Profile,
// Capabilities and Create Company. A raw table optimizes for scanning
// hundreds of rows; this is a 5-item checklist, so a card list that shows
// what's still missing does more real work than a table ever did here.

"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  X,
  AlertCircle,
  Upload,
  ChevronLeft,
  ChevronRight,
  FileText,
  Landmark,
  ShieldCheck,
  BadgeCheck,
  FileWarning,
  Trash2,
  Paperclip,
} from "lucide-react";

// ==================== TYPES ====================

type DocType = "Registration" | "Tax" | "Insurance" | "License" | "Compliance" | "Other";
type DocStatus = "Uploaded" | "Pending" | "Verified" | "Rejected" | "Expired";

type BusinessDocument = {
  id: string;
  name: string;
  type: DocType;
  status: DocStatus;
  fileName: string;
  fileSize: string;
  uploadedAt: string;
  uploadedBy: string;
  expiryDate: string | null;
  notes: string | null;
};

type RequiredSlot = {
  type: DocType;
  label: string;
  description: string;
  icon: React.ReactNode;
};

// ==================== CONSTANTS ====================

// Every document type the company must provide, in the order we want
// them reviewed. A slot with no matching uploaded document renders as
// "not yet uploaded" so the checklist always shows what's missing.
const REQUIRED_SLOTS: RequiredSlot[] = [
  {
    type: "Registration",
    label: "Business Registration",
    description: "Certificate of incorporation or registration",
    icon: <Landmark />,
  },
  {
    type: "Tax",
    label: "Tax ID Certificate",
    description: "Proof of tax registration",
    icon: <FileText />,
  },
  {
    type: "Insurance",
    label: "Insurance Policy",
    description: "Current liability or cargo insurance policy",
    icon: <ShieldCheck />,
  },
  {
    type: "License",
    label: "Operating License",
    description: "License to operate in your region",
    icon: <BadgeCheck />,
  },
];

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

const STATUS_STYLES: Record<DocStatus, { bg: string; text: string; icon: React.ReactNode }> = {
  Uploaded: { bg: "bg-blue-bg", text: "text-blue", icon: <Upload className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

// ==================== SHARED PRIMITIVES ====================
// (Carried over from Create Company / Profile / Capabilities so all
// onboarding screens read as one product.)

function IconTile({
  icon,
  theme = "brand",
  muted = false,
}: {
  icon: React.ReactNode;
  theme?: "brand" | "accent" | "amber" | "red" | "green";
  muted?: boolean;
}) {
  if (muted) {
    return (
      <div className="flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-xl bg-surface-2 text-ink-400">
        <span className="[&>svg]:h-6 [&>svg]:w-6">{icon}</span>
      </div>
    );
  }

  const gradient =
    theme === "brand"
      ? "from-primary to-accent"
      : theme === "accent"
      ? "from-accent to-accent-hover"
      : theme === "amber"
      ? "from-amber to-amber"
      : theme === "red"
      ? "from-red to-red"
      : "from-green to-green";

  return (
    <div
      className={`relative flex h-11 w-11 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-gradient-to-br text-white ring-1 ring-white/20 ${gradient}`}
      style={{
        boxShadow:
          "inset 0 1.5px 0 rgba(255,255,255,0.6), inset 0 -9px 14px rgba(0,0,0,0.3), 0 10px 18px -6px rgba(0,0,0,0.4)",
      }}
    >
      <span
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background: "radial-gradient(130% 110% at 25% 12%, rgba(255,255,255,0.55), transparent 55%)",
          mixBlendMode: "overlay",
        }}
      />
      <span
        className="relative [&>svg]:h-6 [&>svg]:w-6"
        style={{ filter: "drop-shadow(0 1.5px 1.5px rgba(0,0,0,0.35)) drop-shadow(0 -1px 0 rgba(255,255,255,0.25))" }}
      >
        {icon}
      </span>
    </div>
  );
}

function formatDate(date: string | null): string {
  if (!date) return "N/A";
  return new Date(date).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" });
}

// ==================== DOCUMENT CARD ====================

function DocumentCard({
  slot,
  doc,
  onUpload,
  onDelete,
  isUploading,
  confirmingDelete,
  onRequestDelete,
  onCancelDelete,
}: {
  slot: RequiredSlot;
  doc: BusinessDocument | undefined;
  onUpload: (type: DocType) => void;
  onDelete: (id: string) => void;
  isUploading: boolean;
  confirmingDelete: boolean;
  onRequestDelete: () => void;
  onCancelDelete: () => void;
}) {
  const style = doc ? STATUS_STYLES[doc.status] : null;
  const tileTheme = !doc ? undefined : doc.status === "Rejected" ? "red" : doc.status === "Verified" ? "green" : "brand";

  return (
    <div
      className={`rounded-2xl border p-5 transition-colors ${
        doc?.status === "Rejected"
          ? "border-red/30 bg-red-bg/20"
          : doc
          ? "border-line bg-white"
          : "border-dashed border-line bg-surface-2/40"
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile icon={slot.icon} theme={tileTheme as "brand" | "red" | "green" | undefined} muted={!doc} />
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-semibold text-ink-950 sm:text-base">{slot.label}</span>
              {doc && style && (
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${style.bg} ${style.text}`}>
                  {style.icon}
                  {doc.status}
                </span>
              )}
            </div>
            <p className="mt-0.5 text-xs text-ink-500 sm:text-sm">{slot.description}</p>

            {doc ? (
              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-ink-500">
                <span className="flex items-center gap-1">
                  <Paperclip className="h-3 w-3 text-ink-400" />
                  {doc.fileName} <span className="text-ink-400">({doc.fileSize})</span>
                </span>
                <span>Uploaded {formatDate(doc.uploadedAt)}</span>
                {doc.expiryDate && <span>Expires {formatDate(doc.expiryDate)}</span>}
              </div>
            ) : (
              <p className="mt-2 text-xs font-medium text-ink-400">Not yet uploaded</p>
            )}

            {doc?.notes && doc.status === "Rejected" && (
              <div className="mt-2 flex items-start gap-1.5 text-xs text-red">
                <FileWarning className="h-3.5 w-3.5 flex-shrink-0 mt-[1px]" />
                <span>{doc.notes}</span>
              </div>
            )}
          </div>
        </div>
      </div>

      <div className="mt-4 flex items-center justify-between gap-3 border-t border-line pt-3">
        <button
          onClick={() => onUpload(slot.type)}
          disabled={isUploading}
          className={`inline-flex items-center gap-1.5 rounded-lg px-3.5 py-1.5 text-xs font-medium transition ${
            doc
              ? "border border-line text-ink-700 hover:bg-surface-2"
              : "bg-primary text-white hover:bg-primary-hover"
          } ${isUploading ? "cursor-not-allowed opacity-60" : ""}`}
        >
          {isUploading ? (
            <RefreshCw className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <Upload className="h-3.5 w-3.5" />
          )}
          {doc ? (doc.status === "Rejected" ? "Re-upload" : "Replace") : "Upload"}
        </button>

        {doc && !confirmingDelete && (
          <button
            onClick={onRequestDelete}
            className="text-xs font-medium text-ink-400 hover:text-red transition-colors"
          >
            Remove
          </button>
        )}
        {doc && confirmingDelete && (
          <div className="flex items-center gap-2 text-xs">
            <span className="text-ink-500">Remove this document?</span>
            <button onClick={() => onDelete(doc.id)} className="font-semibold text-red hover:underline">
              Remove
            </button>
            <button onClick={onCancelDelete} className="font-medium text-ink-500 hover:underline">
              Cancel
            </button>
          </div>
        )}
      </div>
    </div>
  );
}

// ==================== MAIN PAGE ====================

export default function BusinessDocumentsPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<BusinessDocument[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [uploadingType, setUploadingType] = useState<DocType | null>(null);
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);

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

  const handleUpload = async (type: DocType) => {
    setUploadingType(type);
    await new Promise((resolve) => setTimeout(resolve, 1200));
    setUploadingType(null);
    toast(`${type} document uploaded (mock)`);
  };

  const handleDelete = (id: string) => {
    setData((prev) => prev.filter((doc) => doc.id !== id));
    setConfirmingDeleteId(null);
    toast("Document removed (mock)");
  };

  const handleBack = () => router.push("/setup");
  const handleNext = () => router.push("/setup/verification");

  const docByType = (type: DocType) => data.find((d) => d.type === type);
  const completedCount = REQUIRED_SLOTS.filter((slot) => {
    const doc = docByType(slot.type);
    return doc && doc.status !== "Rejected" && doc.status !== "Expired";
  }).length;
  const allComplete = completedCount === REQUIRED_SLOTS.length;

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
        <div className="space-y-2">
          <div className="h-7 w-56 animate-pulse rounded bg-surface-2" />
          <div className="h-4 w-full max-w-md animate-pulse rounded bg-surface-2" />
        </div>
        <div className="h-10 w-full max-w-xs animate-pulse rounded bg-surface-2" />
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-6">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="h-40 animate-pulse rounded-2xl bg-surface-2" />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      {/* ===== HEADER ===== */}
      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Business Documents</span>
        </p>
        <h1 className="mt-1 text-ink-950">Business Documents</h1>
        <p className="mt-1.5 text-ink-600">Upload the documents required to verify your company.</p>
      </div>

      {/* ===== STATUS STRIP (matches Capabilities / Create Company) ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:gap-4">
        <p className="flex-shrink-0 text-sm font-medium text-ink-700">
          <span className="font-mono tabular-nums text-ink-950">{completedCount}</span>
          <span className="text-ink-400"> / {REQUIRED_SLOTS.length}</span> documents ready
        </p>
        <div className="h-1.5 w-full overflow-hidden rounded-full bg-surface-2 sm:max-w-xs">
          <div
            className="h-full rounded-full bg-primary transition-all"
            style={{ width: `${(completedCount / REQUIRED_SLOTS.length) * 100}%` }}
          />
        </div>
      </div>

      {/* ===== DOCUMENT CHECKLIST ===== */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-6">
        {REQUIRED_SLOTS.map((slot) => {
          const doc = docByType(slot.type);
          return (
            <DocumentCard
              key={slot.type}
              slot={slot}
              doc={doc}
              onUpload={handleUpload}
              onDelete={handleDelete}
              isUploading={uploadingType === slot.type}
              confirmingDelete={confirmingDeleteId === doc?.id}
              onRequestDelete={() => doc && setConfirmingDeleteId(doc.id)}
              onCancelDelete={() => setConfirmingDeleteId(null)}
            />
          );
        })}
      </div>

      {/* ===== BLOCKED NOTICE ===== */}
      {!allComplete && (
        <div className="rounded-2xl border border-amber/20 bg-amber-bg/40 p-5">
          <div className="flex items-start gap-3">
            <IconTile icon={<AlertCircle />} theme="amber" />
            <div className="min-w-0">
              <h4 className="text-sm font-semibold text-amber">Some documents still need attention</h4>
              <p className="mt-1 text-sm text-amber/80">
                Upload or re-upload the missing and rejected documents above before continuing to verification.
              </p>
            </div>
          </div>
        </div>
      )}

      {/* ===== NAVIGATION (matches Profile/Capabilities/Create Company) ===== */}
      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2"
        >
          <ChevronLeft className="h-4 w-4" />
          Back
        </button>
        <button
          onClick={handleNext}
          disabled={!allComplete}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            allComplete ? "bg-primary hover:bg-primary-hover" : "cursor-not-allowed bg-ink-400"
          }`}
        >
          Continue
          <ChevronRight className="h-4 w-4" />
        </button>
      </div>

    </div>
  );
}

function toast(message: string) {
  console.log(message);
}