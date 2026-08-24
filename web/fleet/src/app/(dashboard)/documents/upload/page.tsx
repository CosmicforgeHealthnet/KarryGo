// src/app/(dashboard)/documents/upload/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  FileText,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Plus,
  Users,
  Building2,
  Tag,
  Calendar,
  Info,
  ListChecks,
  Download,
  Printer,
  Edit3,
  Trash2,
  Check,
  MoreHorizontal,
  User,
  Mail,
  Phone,
  MapPin,
  Briefcase,
  Send,
  Paperclip,
  Link2,
  ExternalLink,
  File,
  FileArchive,
  FileImage,
  FileJson,
  FileSpreadsheet,
  FileText as FileTextIcon,
  FolderOpen,
  Shield,
  Lock,
  EyeOff,
  Clock as ClockIcon,
  CheckCircle,
  XCircle,
  AlertOctagon,
  FileCheck,
  FileSignature,
  Folder,
  HardDrive,
  Database,
  Cloud,
  Upload,
  UploadCloud,
  FileUp,
  Image,
  FileType,
  FileVideo,
  FileAudio,
  Archive,
  FileCode,
} from "lucide-react";

// ==================== TYPES ====================

type UploadState = {
  file: File | null;
  name: string;
  type: string;
  category: string;
  ownerType: string;
  ownerId: string;
  visibility: string;
  description: string;
  tags: string[];
  retentionPeriod: string;
};

const DOCUMENT_TYPES = [
  "PDF", "Image", "Spreadsheet", "Document", "Archive", "Video", "Audio", "Code", "Other"
];

const CATEGORIES = [
  "Business", "Compliance", "Provider", "Fleet", "Financial", "Operational", "Legal", "Other"
];

const OWNER_TYPES = [
  "Provider", "Asset", "Customer", "Staff", "Company", "Trip", "Other"
];

const VISIBILITY_OPTIONS = [
  "Internal", "Restricted", "Confidential"
];

const RETENTION_PERIODS = [
  "1 year", "2 years", "3 years", "5 years", "7 years", "10 years", "Indefinite"
];

export default function DocumentUploadPage() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [uploadStatus, setUploadStatus] = useState<"idle" | "uploading" | "scanning" | "complete" | "error">("idle");
  const [secondsAgo, setSecondsAgo] = useState(0);

  const [formData, setFormData] = useState<UploadState>({
    file: null,
    name: "",
    type: "PDF",
    category: "Business",
    ownerType: "Provider",
    ownerId: "",
    visibility: "Internal",
    description: "",
    tags: [],
    retentionPeriod: "3 years",
  });

  const [tagInput, setTagInput] = useState("");
  const [dragActive, setDragActive] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleFileChange = (file: File) => {
    setFormData((prev) => ({
      ...prev,
      file,
      name: file.name.replace(/\.[^/.]+$/, ""),
    }));
    setErrors((prev) => ({ ...prev, file: "" }));
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === "dragenter" || e.type === "dragover") {
      setDragActive(true);
    } else if (e.type === "dragleave") {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      handleFileChange(e.dataTransfer.files[0]);
    }
  };

  const handleAddTag = () => {
    if (tagInput.trim() && !formData.tags.includes(tagInput.trim())) {
      setFormData((prev) => ({
        ...prev,
        tags: [...prev.tags, tagInput.trim()],
      }));
      setTagInput("");
    }
  };

  const handleRemoveTag = (tag: string) => {
    setFormData((prev) => ({
      ...prev,
      tags: prev.tags.filter((t) => t !== tag),
    }));
  };

  const validateForm = () => {
    const newErrors: Record<string, string> = {};
    if (!formData.file) newErrors.file = "Please select a file to upload";
    if (!formData.name.trim()) newErrors.name = "Document name is required";
    if (!formData.ownerId.trim()) newErrors.ownerId = "Owner ID is required";
    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleUpload = async () => {
    if (!validateForm()) return;

    setLoading(true);
    setUploadStatus("uploading");
    setUploadProgress(0);

    // Simulate upload progress
    for (let i = 0; i <= 100; i += 5) {
      await new Promise((resolve) => setTimeout(resolve, 100));
      setUploadProgress(i);
    }

    // Simulate scanning
    setUploadStatus("scanning");
    await new Promise((resolve) => setTimeout(resolve, 1500));

    setUploadStatus("complete");
    setLoading(false);

    // Navigate back to document library after success
    setTimeout(() => {
      router.push("/documents");
    }, 2000);
  };

  const handleCancel = () => {
    router.push("/documents");
  };

  const getFileIcon = (type: string) => {
    switch (type) {
      case "PDF": return <File className="h-12 w-12 text-red" />;
      case "Image": return <Image className="h-12 w-12 text-blue" />;
      case "Spreadsheet": return <FileSpreadsheet className="h-12 w-12 text-green" />;
      case "Document": return <FileTextIcon className="h-12 w-12 text-blue" />;
      case "Archive": return <Archive className="h-12 w-12 text-amber" />;
      case "Video": return <FileVideo className="h-12 w-12 text-purple" />;
      case "Audio": return <FileAudio className="h-12 w-12 text-blue" />;
      case "Code": return <FileCode className="h-12 w-12 text-ink-950" />;
      default: return <File className="h-12 w-12 text-ink-400" />;
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/documents")}
              className="hover:text-primary transition-colors"
            >
              Documents
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">Upload Document</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            Upload Document
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            Secure document upload with scanning and verification.
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
          <button
            onClick={handleCancel}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <X className="h-3.5 w-3.5" />
            Cancel
          </button>
        </div>
      </div>

      {/* ===== UPLOAD FORM ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="p-6">
          {/* Drag and Drop Area */}
          <div
            className={`relative rounded-2xl border-2 border-dashed p-8 text-center transition-colors ${
              dragActive
                ? "border-primary bg-primary-bg/20"
                : formData.file
                ? "border-green bg-green-bg/20"
                : "border-line hover:border-primary/50 hover:bg-surface-2"
            }`}
            onDragEnter={handleDrag}
            onDragLeave={handleDrag}
            onDragOver={handleDrag}
            onDrop={handleDrop}
          >
            {formData.file ? (
              <div className="flex flex-col items-center gap-3">
                {getFileIcon(formData.type)}
                <div>
                  <div className="font-medium text-ink-950">{formData.file.name}</div>
                  <div className="text-sm text-ink-500">
                    {(formData.file.size / 1024 / 1024).toFixed(2)} MB · {formData.type}
                  </div>
                </div>
                <button
                  onClick={() => setFormData((prev) => ({ ...prev, file: null }))}
                  className="text-sm text-red hover:underline"
                >
                  Remove file
                </button>
              </div>
            ) : (
              <>
                <div className="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-full bg-surface-2">
                  <UploadCloud className="h-8 w-8 text-ink-400" />
                </div>
                <h3 className="text-sm font-medium text-ink-950">
                  Drop your file here or click to browse
                </h3>
                <p className="mt-1 text-xs text-ink-500">
                  Supports PDF, Images, Spreadsheets, Documents, and more (Max 50MB)
                </p>
                <label className="mt-3 inline-flex cursor-pointer rounded-xl bg-primary px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-primary/20 hover:bg-primary/90 transition-colors">
                  Browse Files
                  <input
                    type="file"
                    className="hidden"
                    onChange={(e) => {
                      if (e.target.files && e.target.files[0]) {
                        handleFileChange(e.target.files[0]);
                      }
                    }}
                  />
                </label>
              </>
            )}
            {errors.file && (
              <p className="mt-2 text-sm text-red">{errors.file}</p>
            )}
          </div>

          {/* Form Fields */}
          <div className="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Document Name <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.name}
                onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="Enter document name"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.name ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.name && <p className="mt-1 text-sm text-red">{errors.name}</p>}
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Document Type
              </label>
              <select
                value={formData.type}
                onChange={(e) => setFormData((prev) => ({ ...prev, type: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {DOCUMENT_TYPES.map((type) => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Category
              </label>
              <select
                value={formData.category}
                onChange={(e) => setFormData((prev) => ({ ...prev, category: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {CATEGORIES.map((cat) => (
                  <option key={cat} value={cat}>{cat}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Owner Type
              </label>
              <select
                value={formData.ownerType}
                onChange={(e) => setFormData((prev) => ({ ...prev, ownerType: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {OWNER_TYPES.map((type) => (
                  <option key={type} value={type}>{type}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Owner ID <span className="text-red">*</span>
              </label>
              <input
                type="text"
                value={formData.ownerId}
                onChange={(e) => setFormData((prev) => ({ ...prev, ownerId: e.target.value }))}
                placeholder="e.g. P-044, BR-204, ORG-001"
                className={`mt-1 w-full rounded-xl border px-4 py-2.5 text-sm text-ink-950 outline-none transition-colors ${
                  errors.ownerId ? "border-red focus:border-red" : "border-line focus:border-primary"
                }`}
              />
              {errors.ownerId && <p className="mt-1 text-sm text-red">{errors.ownerId}</p>}
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Visibility
              </label>
              <select
                value={formData.visibility}
                onChange={(e) => setFormData((prev) => ({ ...prev, visibility: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {VISIBILITY_OPTIONS.map((opt) => (
                  <option key={opt} value={opt}>{opt}</option>
                ))}
              </select>
            </div>

            <div>
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Retention Period
              </label>
              <select
                value={formData.retentionPeriod}
                onChange={(e) => setFormData((prev) => ({ ...prev, retentionPeriod: e.target.value }))}
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
              >
                {RETENTION_PERIODS.map((period) => (
                  <option key={period} value={period}>{period}</option>
                ))}
              </select>
            </div>

            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Description
              </label>
              <textarea
                value={formData.description}
                onChange={(e) => setFormData((prev) => ({ ...prev, description: e.target.value }))}
                placeholder="Add a description for this document..."
                className="mt-1 w-full rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary resize-none min-h-[80px]"
              />
            </div>

            <div className="sm:col-span-2">
              <label className="text-xs font-medium uppercase tracking-[0.06em] text-ink-500">
                Tags
              </label>
              <div className="mt-1 flex gap-2">
                <input
                  type="text"
                  value={tagInput}
                  onChange={(e) => setTagInput(e.target.value)}
                  onKeyDown={(e) => e.key === "Enter" && handleAddTag()}
                  placeholder="Add tags..."
                  className="flex-1 rounded-xl border border-line px-4 py-2.5 text-sm text-ink-950 outline-none focus:border-primary"
                />
                <button
                  onClick={handleAddTag}
                  className="rounded-xl bg-primary px-4 py-2.5 text-sm font-semibold text-white shadow-sm shadow-primary/20 hover:bg-primary/90 transition-colors"
                >
                  Add
                </button>
              </div>
              {formData.tags.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {formData.tags.map((tag) => (
                    <span
                      key={tag}
                      className="flex items-center gap-1 rounded-full bg-surface-2 px-3 py-1 text-xs font-medium text-ink-700"
                    >
                      {tag}
                      <button
                        onClick={() => handleRemoveTag(tag)}
                        className="text-ink-400 hover:text-red transition-colors"
                      >
                        <X className="h-3 w-3" />
                      </button>
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>

          {/* Upload Progress */}
          {uploadStatus !== "idle" && (
            <div className="mt-6 rounded-xl border border-line p-4">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                  {uploadStatus === "uploading" && (
                    <RefreshCw className="h-5 w-5 animate-spin text-blue" />
                  )}
                  {uploadStatus === "scanning" && (
                    <Shield className="h-5 w-5 animate-pulse text-amber" />
                  )}
                  {uploadStatus === "complete" && (
                    <CheckCircle2 className="h-5 w-5 text-green" />
                  )}
                  {uploadStatus === "error" && (
                    <AlertCircle className="h-5 w-5 text-red" />
                  )}
                  <div>
                    <div className="text-sm font-medium text-ink-950">
                      {uploadStatus === "uploading" && "Uploading..."}
                      {uploadStatus === "scanning" && "Scanning for viruses..."}
                      {uploadStatus === "complete" && "Upload complete!"}
                      {uploadStatus === "error" && "Upload failed"}
                    </div>
                    <div className="text-xs text-ink-500">
                      {uploadStatus === "uploading" && `${uploadProgress}% uploaded`}
                      {uploadStatus === "scanning" && "This may take a moment..."}
                      {uploadStatus === "complete" && "Redirecting to document library..."}
                    </div>
                  </div>
                </div>
                {uploadStatus === "uploading" && (
                  <span className="text-sm font-medium text-ink-950">{uploadProgress}%</span>
                )}
              </div>
              {uploadStatus === "uploading" && (
                <div className="mt-2 h-1.5 w-full rounded-full bg-surface-2 overflow-hidden">
                  <div
                    className="h-full rounded-full bg-primary transition-all duration-300"
                    style={{ width: `${uploadProgress}%` }}
                  />
                </div>
              )}
            </div>
          )}

          {/* Submit Buttons */}
          <div className="mt-6 flex flex-wrap gap-3">
            <button
              onClick={handleUpload}
              disabled={loading}
              className={`flex items-center gap-2 rounded-xl px-6 py-2.5 text-sm font-semibold text-white shadow-sm transition ${
                loading
                  ? "bg-ink-400 cursor-not-allowed"
                  : "bg-primary shadow-primary/20 hover:bg-primary/90"
              }`}
            >
              {loading ? (
                <>
                  <RefreshCw className="h-4 w-4 animate-spin" />
                  {uploadStatus === "uploading" ? "Uploading..." : "Processing..."}
                </>
              ) : (
                <>
                  <Upload className="h-4 w-4" />
                  Upload Document
                </>
              )}
            </button>
            <button
              onClick={handleCancel}
              className="rounded-xl border border-line px-6 py-2.5 text-sm font-medium text-ink-700 hover:bg-white transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileText className="h-4 w-4" />
          DAT-02 · Document Upload
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}