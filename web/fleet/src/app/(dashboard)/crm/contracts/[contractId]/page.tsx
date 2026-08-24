// src/app/(dashboard)/crm/contracts/[contractId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  FileCheck,
  TrendingUp,
  TrendingDown,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Eye,
  ArrowRight,
  Sparkles,
  Users,
  MapPin,
  Briefcase,
  CalendarDays,
  MoreHorizontal,
  Download,
  FileText,
  Printer,
  User,
  DollarSign,
  Send,
  Edit3,
  XCircle,
  CreditCard,
  Building2,
  Hash,
  Tag,
  Mail,
  Phone,
  Calendar,
  Package,
  Truck,
  CreditCard as CreditCardIcon,
  Receipt,
  ThumbsUp,
  ThumbsDown,
  Info,
  FileCheck as FileCheckIcon,
  HandCoins,
  Link2,
  Copy,
  Check,
  FileText as FileTextIcon,
  Calendar as CalendarIcon,
  Shield,
  Award,
  FileSignature,
  PenTool,
} from "lucide-react";

type ContractDetail = {
  id: string;
  reference: string;
  customer: string;
  customerId: string;
  customerInitials: string;
  customerEmail: string;
  customerPhone: string;
  version: number;
  status: "Draft" | "Active" | "Expired" | "Terminated" | "Renewal Pending";
  amount: number;
  currency: string;
  startDate: string;
  endDate: string;
  renewalDate: string | null;
  serviceScope: string;
  createdAt: string;
  updatedAt: string;
  createdBy: string;
  approvedBy: string | null;
  approvedAt: string | null;
  terminatedBy: string | null;
  terminatedAt: string | null;
  terminationReason: string | null;
  branch: string;
  notes: string | null;
  terms: string | null;
  quotationRef: string | null;
  slaRef: string | null;
  documents: {
    id: string;
    name: string;
    type: string;
    uploadedAt: string;
    uploadedBy: string;
  }[];
  amendments: {
    id: string;
    version: number;
    description: string;
    effectiveDate: string;
    status: "Draft" | "Active" | "Superseded";
    createdBy: string;
    createdAt: string;
  }[];
  history: {
    id: string;
    action: string;
    status: string;
    timestamp: string;
    user: string;
    comment: string | null;
  }[];
};

// Mock data
const MOCK_CONTRACT: ContractDetail = {
  id: "CT-001",
  reference: "CT-2026-001",
  customer: "Charitos Hospital",
  customerId: "CUST-001",
  customerInitials: "CH",
  customerEmail: "accounts@charitoshospital.com",
  customerPhone: "+234 800 123 4567",
  version: 1,
  status: "Active",
  amount: 1280000,
  currency: "NGN",
  startDate: "2026-08-01",
  endDate: "2027-07-31",
  renewalDate: "2027-06-01",
  serviceScope: "Medical supplies delivery - 12 months",
  createdAt: "2026-07-25",
  updatedAt: "2026-08-01",
  createdBy: "F. Daniel",
  approvedBy: "O. Adebayo",
  approvedAt: "2026-07-30",
  terminatedBy: null,
  terminatedAt: null,
  terminationReason: null,
  branch: "Ikeja",
  notes: "Annual contract for medical logistics. Service includes cold chain delivery.",
  terms: "Payment due within 30 days of invoice. Late payment penalty applies.",
  quotationRef: "QT-2026-003",
  slaRef: "SLA-001",
  documents: [
    { id: "DOC-001", name: "Contract Agreement.pdf", type: "PDF", uploadedAt: "2026-07-30", uploadedBy: "F. Daniel" },
    { id: "DOC-002", name: "Service Level Agreement.pdf", type: "PDF", uploadedAt: "2026-07-30", uploadedBy: "F. Daniel" },
    { id: "DOC-003", name: "Appendix A - Pricing.xlsx", type: "Excel", uploadedAt: "2026-08-01", uploadedBy: "O. Adebayo" },
  ],
  amendments: [
    { id: "AM-001", version: 1, description: "Initial contract version", effectiveDate: "2026-08-01", status: "Active", createdBy: "F. Daniel", createdAt: "2026-07-25" },
  ],
  history: [
    { id: "HIS-001", action: "Contract created", status: "Draft", timestamp: "2026-07-25 09:00", user: "F. Daniel", comment: "Initial draft created." },
    { id: "HIS-002", action: "Contract approved", status: "Active", timestamp: "2026-07-30 10:00", user: "O. Adebayo", comment: "Approved and activated." },
    { id: "HIS-003", action: "Contract activated", status: "Active", timestamp: "2026-08-01 00:00", user: "System", comment: "Contract period started." },
  ],
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Draft: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <FileTextIcon className="h-4 w-4" />,
  },
  Active: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  Expired: {
    bg: "bg-surface-2",
    text: "text-ink-500",
    dot: "bg-ink-400",
    icon: <Clock className="h-4 w-4" />,
  },
  Terminated: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  "Renewal Pending": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <CalendarIcon className="h-4 w-4" />,
  },
};

const AMENDMENT_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Draft: { bg: "bg-blue-bg", text: "text-blue", dot: "bg-blue" },
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Superseded: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

export default function ContractDetailPage() {
  const router = useRouter();
  const params = useParams();
  const contractId = params.contractId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ContractDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_CONTRACT, id: contractId });
      setLoading(false);
    };
    loadData();
  }, [contractId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading contract details...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <FileCheck className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Contract not found</h3>
        <p className="mt-1 text-sm text-ink-500">The contract you're looking for doesn't exist.</p>
        <button
          onClick={() => router.push("/crm/contracts")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to contracts
        </button>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Draft;
  const formattedAmount = new Intl.NumberFormat("en-NG", {
    style: "currency",
    currency: "NGN",
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  }).format(data.amount);
  const isActive = data.status === "Active";
  const isEditable = data.status === "Draft";
  const isTerminated = data.status === "Terminated";

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push("/crm/contracts")}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Contract {data.reference}
              </h1>
              <span
                className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${statusStyle.bg} ${statusStyle.text}`}
              >
                {statusStyle.icon}
                {data.status}
              </span>
              <span className="text-sm text-ink-400">v{data.version}</span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">{data.serviceScope}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Download className="h-3.5 w-3.5" />
            PDF
          </button>
          {isEditable && (
            <button
              onClick={() => router.push(`/crm/contracts/${data.id}/edit`)}
              className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
            >
              <Edit3 className="h-3.5 w-3.5" />
              Edit
            </button>
          )}
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Amount Card */}
          <div className="rounded-2xl border border-line bg-white p-6 shadow-sm">
            <div className="flex items-center gap-2 text-sm text-ink-500">
              <span className="font-medium">{data.branch}</span>
              <span className="text-ink-300">·</span>
              <span>{data.startDate} → {data.endDate}</span>
              {data.renewalDate && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="text-amber">Renewal: {data.renewalDate}</span>
                </>
              )}
            </div>
            <div className="mt-2">
              <span className="text-3xl font-bold text-ink-950">
                {formattedAmount}
              </span>
              <span className="ml-2 text-sm text-ink-400">{data.currency}</span>
            </div>
            <div className="mt-3 flex flex-wrap items-center gap-4 text-sm text-ink-500">
              <span className="flex items-center gap-1.5">
                <User className="h-4 w-4" />
                Created by {data.createdBy}
              </span>
              <span className="text-ink-300">·</span>
              <span className="flex items-center gap-1.5">
                <CalendarDays className="h-4 w-4" />
                {data.createdAt}
              </span>
              {data.approvedBy && (
                <>
                  <span className="text-ink-300">·</span>
                  <span className="flex items-center gap-1.5 text-green">
                    <CheckCircle2 className="h-4 w-4" />
                    Approved by {data.approvedBy}
                  </span>
                </>
              )}
            </div>
          </div>

          {/* Amendments */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Amendments</h3>
              <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1">
                <Plus className="h-3.5 w-3.5" />
                New Amendment
              </button>
            </div>
            <div className="space-y-2">
              {data.amendments.map((amendment) => {
                const amStatusStyle = AMENDMENT_STATUS_STYLES[amendment.status] || AMENDMENT_STATUS_STYLES.Draft;
                return (
                  <div key={amendment.id} className="flex items-center justify-between rounded-lg border border-line p-3 transition hover:bg-surface-2">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-ink-950">v{amendment.version}</span>
                        <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[9px] font-semibold ${amStatusStyle.bg} ${amStatusStyle.text}`}>
                          <span className={`h-1 w-1 rounded-full ${amStatusStyle.dot}`} />
                          {amendment.status}
                        </span>
                      </div>
                      <div className="text-xs text-ink-500">{amendment.description}</div>
                    </div>
                    <div className="text-right">
                      <div className="text-xs text-ink-400">{amendment.effectiveDate}</div>
                      <div className="text-xs text-ink-400">{amendment.createdBy}</div>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>

          {/* History */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">History</h3>
            <div className="space-y-3">
              {data.history.map((event, index) => (
                <div key={event.id} className="flex items-start gap-3">
                  <div className="flex flex-col items-center">
                    <div className={`h-2.5 w-2.5 rounded-full ${
                      event.status === "Active" ? "bg-green" :
                      event.status === "Draft" ? "bg-blue" :
                      event.status === "Terminated" ? "bg-red" :
                      "bg-ink-400"
                    }`} />
                    {index < data.history.length - 1 && (
                      <div className="h-full w-0.5 bg-line" />
                    )}
                  </div>
                  <div className="flex-1 pb-3">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium text-ink-950">{event.action}</span>
                      <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${
                        event.status === "Active" ? "bg-green-bg text-green" :
                        event.status === "Draft" ? "bg-blue-bg text-blue" :
                        event.status === "Terminated" ? "bg-red-bg text-red" :
                        "bg-surface-2 text-ink-500"
                      }`}>
                        {event.status}
                      </span>
                    </div>
                    <div className="text-xs text-ink-500">
                      {event.user} · {event.timestamp}
                    </div>
                    {event.comment && (
                      <div className="mt-1 text-xs text-ink-600 bg-surface-2 rounded-lg px-2.5 py-1.5">
                        "{event.comment}"
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Customer Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Customer</h3>
            <div className="space-y-3">
              <div className="flex items-center gap-3">
                <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[16px] font-bold text-white shadow-lg shadow-primary/20">
                  {data.customerInitials}
                </div>
                <div>
                  <div className="text-sm font-semibold text-ink-950">{data.customer}</div>
                  <div className="text-xs text-ink-500 font-mono">{data.customerId}</div>
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Email</div>
                <div className="text-sm font-medium text-ink-950 flex items-center gap-1.5">
                  <Mail className="h-3.5 w-3.5 text-ink-400" />
                  {data.customerEmail}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Phone</div>
                <div className="text-sm font-medium text-ink-950 flex items-center gap-1.5">
                  <Phone className="h-3.5 w-3.5 text-ink-400" />
                  {data.customerPhone}
                </div>
              </div>
              <button
                onClick={() => router.push(`/crm/customers/${data.customerId}`)}
                className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1"
              >
                View Customer <ArrowRight className="h-3.5 w-3.5" />
              </button>
            </div>
          </div>

          {/* References */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">References</h3>
            <div className="space-y-2">
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Quotation</div>
                <div className="text-sm font-medium text-ink-950 font-mono">
                  {data.quotationRef || "—"}
                </div>
              </div>
              <div>
                <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">SLA</div>
                <div className="text-sm font-medium text-ink-950 font-mono">
                  {data.slaRef || "—"}
                </div>
              </div>
            </div>
          </div>

          {/* Documents */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-3">
              <h3 className="text-[15px] font-semibold text-ink-950">Documents</h3>
              <button className="text-xs font-medium text-primary hover:text-primary-hover transition-colors flex items-center gap-1">
                <Plus className="h-3.5 w-3.5" />
                Upload
              </button>
            </div>
            <div className="space-y-2">
              {data.documents.map((doc) => (
                <div key={doc.id} className="flex items-center justify-between rounded-lg border border-line p-2.5 transition hover:bg-surface-2">
                  <div className="flex items-center gap-2">
                    <FileTextIcon className="h-4 w-4 text-ink-400" />
                    <div>
                      <div className="text-sm font-medium text-ink-950">{doc.name}</div>
                      <div className="text-xs text-ink-500">{doc.type} · {doc.uploadedBy}</div>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="text-xs text-ink-400">{doc.uploadedAt}</div>
                    <button className="text-xs text-primary hover:text-primary-hover transition-colors">
                      View
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Terms */}
          {data.terms && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Terms</h3>
              <p className="text-sm text-ink-600">{data.terms}</p>
            </div>
          )}

          {/* Notes */}
          {data.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
              <p className="text-sm text-ink-600">{data.notes}</p>
            </div>
          )}

          {/* Termination Info */}
          {isTerminated && data.terminationReason && (
            <div className="rounded-2xl border border-red/20 bg-red-bg/30 p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-red mb-2">Termination</h3>
              <p className="text-sm text-red/80">{data.terminationReason}</p>
              <div className="mt-2 text-xs text-red/60">
                {data.terminatedBy} · {data.terminatedAt}
              </div>
            </div>
          )}

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              <button
                onClick={() => router.push(`/crm/contracts/${data.id}/edit`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="h-4 w-4" />
                  Edit Contract
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => router.push(`/crm/customers/${data.customerId}`)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Users className="h-4 w-4" />
                  View Customer
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              {data.quotationRef && (
                <button
                  onClick={() => router.push(`/crm/quotations/${data.quotationRef}`)}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <FileTextIcon className="h-4 w-4" />
                    View Quotation
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              {data.slaRef && (
                <button
                  onClick={() => router.push(`/crm/slas/${data.slaRef}`)}
                  className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
                >
                  <span className="flex items-center gap-2">
                    <FileCheckIcon className="h-4 w-4" />
                    View SLA
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
            </div>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <FileCheck className="h-4 w-4" />
          CRM-14 · Contract Detail
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.status}
        </span>
      </div>
    </div>
  );
}