// src/app/(setup)/setup/business-docs/page.tsx

"use client";

import { useEffect, useState, useRef } from "react";
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
  Paperclip,
  Loader2,
} from "lucide-react";
import { useAuthStore } from "@/lib/stores/authStore";
import { useOrganizationStore } from "@/lib/stores/organizationStore";
import { useOnboardingStore } from "@/lib/stores/onboardingStore";
import { organizationService } from "@/lib/services/organizationService";

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

const DOC_TYPE_MAP: Record<DocType, string> = {
  Registration: "CERTIFICATE_OF_INCORPORATION",
  Tax: "TAX_CLEARANCE",
  Insurance: "INSURANCE_POLICY",
  License: "OPERATING_LICENSE",
  Compliance: "COMPLIANCE_DOCUMENT",
  Other: "OTHER_DOCUMENT",
};

const STATUS_MAP: Record<string, DocStatus> = {
  ATTACHED: "Uploaded",
  READY: "Pending",
  VERIFIED: "Verified",
  REJECTED: "Rejected",
  EXPIRED: "Expired",
  REMOVED: "Expired",
};

const STATUS_STYLES: Record<DocStatus, { bg: string; text: string; icon: React.ReactNode }> = {
  Uploaded: { bg: "bg-blue-bg", text: "text-blue", icon: <Upload className="h-3.5 w-3.5" /> },
  Pending: { bg: "bg-amber-bg", text: "text-amber", icon: <Clock className="h-3.5 w-3.5" /> },
  Verified: { bg: "bg-green-bg", text: "text-green", icon: <CheckCircle2 className="h-3.5 w-3.5" /> },
  Rejected: { bg: "bg-red-bg", text: "text-red", icon: <X className="h-3.5 w-3.5" /> },
  Expired: { bg: "bg-surface-2", text: "text-ink-500", icon: <AlertCircle className="h-3.5 w-3.5" /> },
};

const MAX_FILE_SIZE = 10 * 1024 * 1024;
const ALLOWED_FILE_TYPES = [
  "application/pdf",
  "image/jpeg",
  "image/png",
  "application/msword",
  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
];

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

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return bytes + " B";
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
  return (bytes / (1024 * 1024)).toFixed(1) + " MB";
}

function mapDocType(docType: string): DocType {
  const mapping: Record<string, DocType> = {
    CERTIFICATE_OF_INCORPORATION: "Registration",
    TAX_CLEARANCE: "Tax",
    INSURANCE_POLICY: "Insurance",
    OPERATING_LICENSE: "License",
  };
  return mapping[docType] || "Other";
}

function mapStatus(status: string): DocStatus {
  return STATUS_MAP[status] || "Uploaded";
}

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

async function computeChecksum(file: File): Promise<string> {
  const buffer = await file.arrayBuffer();
  const hashBuffer = await crypto.subtle.digest('SHA-256', buffer);
  const hashArray = Array.from(new Uint8Array(hashBuffer));
  return hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
}

export default function BusinessDocumentsPage() {
  const router = useRouter();
  const { _hasHydrated } = useAuthStore();
  const { organization } = useOrganizationStore();
  const { setStageComplete } = useOnboardingStore();

  const fileInputRef = useRef<HTMLInputElement>(null);

  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [data, setData] = useState<BusinessDocument[]>([]);
  const [uploadingType, setUploadingType] = useState<DocType | null>(null);
  const [uploadError, setUploadError] = useState<string | null>(null);
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<string | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [selectedDocumentType, setSelectedDocumentType] = useState<DocType | null>(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [isContinuing, setIsContinuing] = useState(false);

  useEffect(() => {
    if (!_hasHydrated) return;

    const fetchDocuments = async () => {
      if (!organization?.organizationId) {
        setLoading(false);
        return;
      }

      try {
        const response = await organizationService.listDocuments(organization.organizationId);
        const docs = Array.isArray(response) ? response : [];

        const mappedDocs: BusinessDocument[] = docs.map((doc: any) => ({
          id: doc.id || `doc_${Date.now()}`,
          name: doc.documentType || "Document",
          type: mapDocType(doc.documentType || ""),
          status: mapStatus(doc.status || "ATTACHED"),
          fileName: doc.fileId || "document.pdf",
          fileSize: formatFileSize(0),
          uploadedAt: doc.createdAt || new Date().toISOString(),
          uploadedBy: "You",
          expiryDate: doc.expiresAt || null,
          notes: doc.verificationReference || null,
        }));

        setData(mappedDocs);
        setError(null);
      } catch (err: any) {
        const message = err?.response?.data?.error?.message || err?.message || "Failed to load documents.";
        setError(message);
      } finally {
        setLoading(false);
      }
    };

    fetchDocuments();
  }, [_hasHydrated, organization?.organizationId]);

  const handleUploadClick = (type: DocType) => {
    setSelectedDocumentType(type);
    setUploadError(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
      fileInputRef.current.click();
    }
  };

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    const docType = selectedDocumentType;

    e.target.value = "";

    if (!file || !docType || !organization?.organizationId) {
      return;
    }

    if (file.size > MAX_FILE_SIZE) {
      setUploadError(`File size exceeds ${MAX_FILE_SIZE / 1024 / 1024}MB limit.`);
      return;
    }

    if (!ALLOWED_FILE_TYPES.includes(file.type)) {
      setUploadError("Invalid file type. Please upload PDF, JPG, PNG, or DOC files.");
      return;
    }

    setUploadingType(docType);
    setUploadError(null);
    setUploadProgress(0);

    const idempotencyKey = crypto.randomUUID();

    try {
      setUploadProgress(10);
      const grant = await organizationService.requestUploadAuthorization(
        organization.organizationId,
        {
          documentType: DOC_TYPE_MAP[docType],
          uploadPurpose: "COMPANY_DOCUMENT",
          allowedMimeTypes: ["application/pdf", "image/jpeg", "image/png", "image/webp"],
          maxSizeBytes: MAX_FILE_SIZE,
          idempotencyKey,
        }
      );
      setUploadProgress(30);

      const checksum = await computeChecksum(file);
      setUploadProgress(50);

      const result = await organizationService.completeDocumentUpload(
        organization.organizationId,
        {
          mediaObjectId: grant.mediaObjectId,
          authorizationId: grant.authorizationId,
          uploadToken: grant.uploadToken,
          declaredMime: file.type || "application/octet-stream",
          declaredSize: file.size,
          declaredChecksum: checksum,
          file: file,
        }
      );
      setUploadProgress(80);

      const attachResponse = await organizationService.attachDocument(
        organization.organizationId,
        DOC_TYPE_MAP[docType],
        result.mediaObjectId
      );
      setUploadProgress(100);

      const newDoc: BusinessDocument = {
        id: attachResponse.documentId || `doc_${Date.now()}`,
        name: docType,
        type: docType,
        status: "Uploaded",
        fileName: file.name,
        fileSize: formatFileSize(file.size),
        uploadedAt: new Date().toISOString(),
        uploadedBy: "You",
        expiryDate: null,
        notes: null,
      };

      setData((prev) => {
        const filtered = prev.filter((d) => d.type !== docType);
        return [...filtered, newDoc];
      });

      setSelectedDocumentType(null);
    } catch (err: any) {
      const message = err?.response?.data?.error?.message || err?.message || "Failed to upload document.";
      setUploadError(message);
    } finally {
      setUploadingType(null);
      setUploadProgress(0);
    }
  };

  const handleDelete = async (id: string) => {
    setData((prev) => prev.filter((doc) => doc.id !== id));
    setConfirmingDeleteId(null);
  };

  const handleContinue = async () => {
    if (!organization?.organizationId) {
      setSaveError("Organization not found.");
      return;
    }

    const allComplete = REQUIRED_SLOTS.every((slot) => {
      const doc = data.find((d) => d.type === slot.type);
      return doc && doc.status !== "Rejected" && doc.status !== "Expired";
    });

    if (!allComplete) {
      setSaveError("Please upload all required documents before continuing.");
      return;
    }

    setIsContinuing(true);
    setSaveError(null);
    setSaveSuccess(false);

    try {
      await organizationService.markStageComplete(
        organization.organizationId,
        'BUSINESS_DOCUMENTS'
      );
      setStageComplete("documents");
      setSaveSuccess(true);

      setTimeout(() => {
        router.push("/setup/submit");
      }, 1000);
    } catch (err: any) {
      const message = err?.message || "Failed to save.";
      setSaveError(message);
    } finally {
      setIsContinuing(false);
    }
  };

  const handleBack = () => router.push("/setup/headquarters");

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
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
        </div>
      </div>
    );
  }

  if (error && data.length === 0) {
    return (
      <div className="mx-auto w-full max-w-5xl">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-red/20 bg-red-bg/30 px-4 py-16 text-center">
          <div className="rounded-full bg-red-bg p-4">
            <AlertCircle className="h-8 w-8 text-red" />
          </div>
          <h3 className="mt-4 text-[15px] font-semibold text-ink-950">Unable to load documents</h3>
          <p className="mt-1.5 text-sm text-ink-500">{error}</p>
          <button
            onClick={() => window.location.reload()}
            className="mt-5 rounded-lg bg-primary px-5 py-2 text-[13px] font-semibold text-white transition hover:bg-primary/90"
          >
            Try again
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6 sm:space-y-8">
      <input
        ref={fileInputRef}
        type="file"
        className="hidden"
        onChange={handleFileChange}
        accept=".pdf,.jpg,.jpeg,.png,.doc,.docx"
      />

      <div>
        <p className="text-xs font-medium text-ink-500">
          Onboarding <span className="mx-1 text-ink-400">/</span>{" "}
          <span className="text-ink-700">Business Documents</span>
        </p>
        <h1 className="mt-1 text-ink-950">Business Documents</h1>
        <p className="mt-1.5 text-ink-600">Upload the documents required to verify your company.</p>
      </div>

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

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 sm:gap-6">
        {REQUIRED_SLOTS.map((slot) => {
          const doc = docByType(slot.type);
          const isUploading = uploadingType === slot.type;

          return (
            <DocumentCard
              key={slot.type}
              slot={slot}
              doc={doc}
              onUpload={handleUploadClick}
              onDelete={handleDelete}
              isUploading={isUploading}
              confirmingDelete={confirmingDeleteId === doc?.id}
              onRequestDelete={() => doc && setConfirmingDeleteId(doc.id)}
              onCancelDelete={() => setConfirmingDeleteId(null)}
            />
          );
        })}
      </div>

      {uploadingType && uploadProgress > 0 && uploadProgress < 100 && (
        <div className="rounded-xl border border-blue/20 bg-blue-bg/40 p-4 sticky top-0 z-10">
          <div className="flex items-center gap-3">
            <Loader2 className="h-4 w-4 animate-spin text-blue" />
            <span className="text-sm text-ink-600">Uploading... {uploadProgress}%</span>
          </div>
          <div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-surface-2">
            <div
              className="h-full rounded-full bg-blue transition-all"
              style={{ width: `${uploadProgress}%` }}
            />
          </div>
        </div>
      )}

      {uploadError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {uploadError}
        </div>
      )}

      {!allComplete && (
        <div className="rounded-2xl border border-amber/20 bg-amber-bg/40 p-5">
          <div className="flex items-start gap-3">
            <IconTile icon={<AlertCircle />} theme="amber" />
            <div className="min-w-0">
              <h4 className="text-sm font-semibold text-amber">Some documents still need attention</h4>
              <p className="mt-1 text-sm text-amber/80">
                Upload or re-upload the missing and rejected documents above before continuing to submit.
              </p>
            </div>
          </div>
        </div>
      )}

      {saveError && (
        <div className="flex items-center gap-2 rounded-xl border border-red/20 bg-red-bg/40 px-4 py-3 text-sm text-red">
          <AlertCircle className="h-4 w-4 flex-shrink-0" />
          {saveError}
        </div>
      )}

      {saveSuccess && (
        <div className="flex items-center gap-2 rounded-xl border border-green/20 bg-green-bg/40 px-4 py-3 text-sm text-green">
          <CheckCircle2 className="h-4 w-4 flex-shrink-0" />
          Documents saved successfully! Redirecting...
        </div>
      )}

      <div className="flex flex-col-reverse items-center gap-3 border-t border-line pt-6 sm:flex-row sm:items-center sm:justify-between">
        <button
          onClick={handleBack}
          disabled={isContinuing || isSaving}
          className="inline-flex items-center justify-center gap-2 rounded-lg border border-line px-4 py-2 text-sm font-medium text-ink-700 hover:bg-surface-2 disabled:opacity-50"
        >
          <ChevronLeft className="h-4 w-4" />
          Back
        </button>
        <button
          onClick={handleContinue}
          disabled={!allComplete || isContinuing || isSaving}
          className={`inline-flex items-center justify-center gap-2 rounded-lg px-6 py-2 text-sm font-medium text-white transition ${
            !allComplete || isContinuing || isSaving
              ? "cursor-not-allowed bg-ink-400"
              : "bg-primary hover:bg-primary-hover"
          }`}
        >
          {isContinuing ? (
            <>
              <RefreshCw className="h-4 w-4 animate-spin" />
              Saving...
            </>
          ) : saveSuccess ? (
            <>
              <CheckCircle2 className="h-4 w-4" />
              Saved!
            </>
          ) : (
            <>
              Continue
              <ChevronRight className="h-4 w-4" />
            </>
          )}
        </button>
      </div>
    </div>
  );
}