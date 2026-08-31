// src/app/(dashboard)/finance/invoices/[invoiceId]/edit/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft,
  Receipt,
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
  Save,
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
  Plus,
  Trash2,
  Edit3,
} from "lucide-react";

type InvoiceItem = {
  id: string;
  description: string;
  quantity: number;
  unitPrice: number;
  total: number;
};

type InvoiceData = {
  id: string;
  reference: string;
  customer: string;
  customerType: "Corporate" | "SME" | "Individual";
  customerEmail: string;
  customerPhone: string;
  issueDate: string;
  dueDate: string;
  branch: string;
  orderRef: string | null;
  notes: string | null;
  items: InvoiceItem[];
  subtotal: number;
  tax: number;
  discount: number;
  totalAmount: number;
  currency: string;
  status: "Draft" | "Pending" | "Paid" | "Overdue" | "Cancelled";
};

// Mock data
const MOCK_INVOICE: InvoiceData = {
  id: "INV-001",
  reference: "INV-2026-001",
  customer: "Charitos Hospital",
  customerType: "Corporate",
  customerEmail: "accounts@charitoshospital.com",
  customerPhone: "+234 800 123 4567",
  issueDate: "2026-08-01",
  dueDate: "2026-08-15",
  branch: "Ikeja",
  orderRef: "OR-129",
  notes: "Delivery completed and verified.",
  items: [
    { id: "ITEM-001", description: "Medical supply delivery - Order OR-129", quantity: 12, unitPrice: 106667, total: 1280000 },
  ],
  subtotal: 1280000,
  tax: 0,
  discount: 0,
  totalAmount: 1280000,
  currency: "NGN",
  status: "Draft",
};

const CUSTOMER_TYPES = ["Corporate", "SME", "Individual"];
const BRANCHES = ["Ikeja", "Lekki", "Yaba", "Apapa"];

export default function InvoiceEditorPage() {
  const router = useRouter();
  const params = useParams();
  const invoiceId = params.invoiceId as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [data, setData] = useState<InvoiceData | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData({ ...MOCK_INVOICE, id: invoiceId });
      setLoading(false);
    };
    loadData();
  }, [invoiceId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading invoice...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <Receipt className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Invoice not found</h3>
        <p className="mt-1 text-sm text-ink-500">The invoice you're trying to edit doesn't exist.</p>
        <button
          onClick={() => router.push("/finance/invoices")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          ← Back to invoices
        </button>
      </div>
    );
  }

  const handleItemChange = (index: number, field: keyof InvoiceItem, value: string | number) => {
    if (!data) return;
    const newItems = [...data.items];
    const item = { ...newItems[index] };
    
    if (field === "description") {
      item.description = value as string;
    } else if (field === "quantity") {
      const qty = Number(value) || 0;
      item.quantity = qty;
      item.total = qty * item.unitPrice;
    } else if (field === "unitPrice") {
      const price = Number(value) || 0;
      item.unitPrice = price;
      item.total = item.quantity * price;
    }
    
    newItems[index] = item;
    setData({ ...data, items: newItems });
    recalculateTotals(newItems);
  };

  const addItem = () => {
    if (!data) return;
    const newItem: InvoiceItem = {
      id: `ITEM-${String(data.items.length + 1).padStart(3, "0")}`,
      description: "",
      quantity: 1,
      unitPrice: 0,
      total: 0,
    };
    setData({
      ...data,
      items: [...data.items, newItem],
    });
  };

  const removeItem = (index: number) => {
    if (!data) return;
    const newItems = data.items.filter((_, i) => i !== index);
    setData({ ...data, items: newItems });
    recalculateTotals(newItems);
  };

  const recalculateTotals = (items: InvoiceItem[]) => {
    const subtotal = items.reduce((acc, item) => acc + item.total, 0);
    const totalAmount = subtotal + (data?.tax || 0) - (data?.discount || 0);
    setData((prev) => prev ? { ...prev, subtotal, totalAmount } : null);
  };

  const handleFieldChange = (field: keyof InvoiceData, value: any) => {
    if (!data) return;
    setData({ ...data, [field]: value });
  };

  const handleSave = async () => {
    setSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setSaving(false);
    toast("Invoice saved successfully");
  };

  const handleSubmit = async () => {
    setSaving(true);
    await new Promise((resolve) => setTimeout(resolve, 1500));
    setSaving(false);
    toast("Invoice submitted for approval");
    router.push(`/finance/invoices/${data.id}`);
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  const formatCurrency = (amount: number) => {
    return new Intl.NumberFormat("en-NG", {
      style: "currency",
      currency: "NGN",
      minimumFractionDigits: 0,
      maximumFractionDigits: 0,
    }).format(amount);
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <button
            onClick={() => router.push(`/finance/invoices/${data.id}`)}
            className="mt-0.5 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-xl border border-line bg-white text-ink-500 transition hover:bg-surface-2 hover:text-ink-700"
          >
            <ArrowLeft className="h-4 w-4" />
          </button>
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
                Edit Invoice
              </h1>
              <span className="text-sm text-ink-400">{data.reference}</span>
              <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-medium ${
                data.status === "Draft" ? "bg-blue-bg text-blue" :
                data.status === "Pending" ? "bg-amber-bg text-amber" :
                "bg-surface-2 text-ink-500"
              }`}>
                {data.status}
              </span>
            </div>
            <p className="mt-0.5 text-sm text-ink-500">Update invoice details and line items</p>
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
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white"
          >
            <Save className="h-3.5 w-3.5" />
            {saving ? "Saving..." : "Save Draft"}
          </button>
          <button
            onClick={handleSubmit}
            disabled={saving}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            {saving ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Send className="h-3.5 w-3.5" />
            )}
            Submit
          </button>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[2fr_1fr]">
        {/* ===== LEFT COLUMN ===== */}
        <div className="space-y-5">
          {/* Invoice Details */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-4">Invoice Details</h3>
            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Customer
                </label>
                <input
                  value={data.customer}
                  onChange={(e) => handleFieldChange("customer", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  placeholder="Customer name"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Customer Type
                </label>
                <select
                  value={data.customerType}
                  onChange={(e) => handleFieldChange("customerType", e.target.value as any)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {CUSTOMER_TYPES.map((type) => (
                    <option key={type} value={type}>{type}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Customer Email
                </label>
                <input
                  value={data.customerEmail}
                  onChange={(e) => handleFieldChange("customerEmail", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  placeholder="customer@example.com"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Customer Phone
                </label>
                <input
                  value={data.customerPhone}
                  onChange={(e) => handleFieldChange("customerPhone", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  placeholder="+234 800 123 4567"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Issue Date
                </label>
                <input
                  type="date"
                  value={data.issueDate}
                  onChange={(e) => handleFieldChange("issueDate", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Due Date
                </label>
                <input
                  type="date"
                  value={data.dueDate}
                  onChange={(e) => handleFieldChange("dueDate", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Branch
                </label>
                <select
                  value={data.branch}
                  onChange={(e) => handleFieldChange("branch", e.target.value)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                >
                  {BRANCHES.map((branch) => (
                    <option key={branch} value={branch}>{branch}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                  Order Reference
                </label>
                <input
                  value={data.orderRef || ""}
                  onChange={(e) => handleFieldChange("orderRef", e.target.value || null)}
                  className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  placeholder="OR-123"
                />
              </div>
            </div>
          </div>

          {/* Line Items */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center justify-between mb-4">
              <h3 className="text-[15px] font-semibold text-ink-950">Line Items</h3>
              <button
                onClick={addItem}
                className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-[11px] font-medium text-white transition hover:bg-primary/90"
              >
                <Plus className="h-3.5 w-3.5" />
                Add Item
              </button>
            </div>
            <div className="space-y-2">
              {data.items.map((item, index) => (
                <div key={item.id} className="flex flex-wrap items-center gap-2 rounded-lg border border-line p-3">
                  <div className="flex-1 min-w-[150px]">
                    <input
                      value={item.description}
                      onChange={(e) => handleItemChange(index, "description", e.target.value)}
                      placeholder="Item description"
                      className="w-full rounded-lg border border-line bg-white/70 px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                    />
                  </div>
                  <div className="w-20">
                    <input
                      type="number"
                      value={item.quantity}
                      onChange={(e) => handleItemChange(index, "quantity", e.target.value)}
                      placeholder="Qty"
                      className="w-full rounded-lg border border-line bg-white/70 px-2 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                    />
                  </div>
                  <div className="w-28">
                    <input
                      type="number"
                      value={item.unitPrice}
                      onChange={(e) => handleItemChange(index, "unitPrice", e.target.value)}
                      placeholder="Price"
                      className="w-full rounded-lg border border-line bg-white/70 px-2 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                    />
                  </div>
                  <div className="w-24 text-sm font-medium text-ink-950 text-right">
                    {formatCurrency(item.total)}
                  </div>
                  <button
                    onClick={() => removeItem(index)}
                    className="rounded-lg p-1.5 text-red hover:bg-red-bg transition-colors"
                  >
                    <Trash2 className="h-4 w-4" />
                  </button>
                </div>
              ))}
            </div>
          </div>

          {/* Notes */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-2">Notes</h3>
            <textarea
              value={data.notes || ""}
              onChange={(e) => handleFieldChange("notes", e.target.value || null)}
              rows={3}
              placeholder="Add notes to this invoice..."
              className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
            />
          </div>
        </div>

        {/* ===== RIGHT COLUMN ===== */}
        <div className="space-y-5">
          {/* Summary */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Summary</h3>
            <div className="space-y-2">
              <div className="flex items-center justify-between text-sm">
                <span className="text-ink-500">Subtotal</span>
                <span className="font-medium text-ink-950">{formatCurrency(data.subtotal)}</span>
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-ink-500">Tax</span>
                <input
                  type="number"
                  value={data.tax}
                  onChange={(e) => {
                    const tax = Number(e.target.value) || 0;
                    setData((prev) => prev ? { ...prev, tax, totalAmount: prev.subtotal + tax - prev.discount } : null);
                  }}
                  className="w-32 rounded-lg border border-line bg-white/70 px-2 py-1 text-right text-sm text-ink-950 outline-none transition focus:border-primary"
                />
              </div>
              <div className="flex items-center justify-between text-sm">
                <span className="text-ink-500">Discount</span>
                <input
                  type="number"
                  value={data.discount}
                  onChange={(e) => {
                    const discount = Number(e.target.value) || 0;
                    setData((prev) => prev ? { ...prev, discount, totalAmount: prev.subtotal + prev.tax - discount } : null);
                  }}
                  className="w-32 rounded-lg border border-line bg-white/70 px-2 py-1 text-right text-sm text-ink-950 outline-none transition focus:border-primary"
                />
              </div>
              <div className="flex items-center justify-between border-t border-line pt-2 text-sm font-semibold">
                <span className="text-ink-950">Total</span>
                <span className="text-lg font-bold text-ink-950">{formatCurrency(data.totalAmount)}</span>
              </div>
            </div>
          </div>

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              <button
                onClick={handleSave}
                disabled={saving}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Save className="h-4 w-4" />
                  Save Draft
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={handleSubmit}
                disabled={saving}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <Send className="h-4 w-4" />
                  Submit for Approval
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={() => setShowDeleteConfirm(true)}
                className="flex w-full items-center justify-between rounded-xl border border-red/30 bg-red-bg/50 px-4 py-2.5 text-[12.5px] font-medium text-red transition hover:bg-red-bg"
              >
                <span className="flex items-center gap-2">
                  <Trash2 className="h-4 w-4" />
                  Delete Invoice
                </span>
                <ArrowRight className="h-4 w-4 text-red/50" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== DELETE CONFIRM MODAL ===== */}
      {showDeleteConfirm && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-2xl">
            <div className="flex items-center gap-2">
              <AlertCircle className="h-5 w-5 text-red" />
              <h3 className="text-lg font-semibold text-ink-950">Delete Invoice?</h3>
            </div>
            <p className="mt-2 text-sm text-ink-600">
              Are you sure you want to delete this invoice? This action cannot be undone.
            </p>
            <div className="mt-5 flex gap-2.5">
              <button
                onClick={() => setShowDeleteConfirm(false)}
                className="flex-1 rounded-lg border border-line bg-white/70 px-4 py-2 text-sm font-medium text-ink-600 transition hover:bg-white"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  setShowDeleteConfirm(false);
                  toast("Invoice deleted");
                  router.push("/finance/invoices");
                }}
                className="flex-1 rounded-lg bg-red px-4 py-2 text-sm font-medium text-white transition hover:bg-red/90"
              >
                Delete
              </button>
            </div>
          </div>
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Receipt className="h-4 w-4" />
          FIN-03 · Invoice Editor
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Last saved {secondsAgo}s ago
        </span>
      </div>
    </div>
  );
}