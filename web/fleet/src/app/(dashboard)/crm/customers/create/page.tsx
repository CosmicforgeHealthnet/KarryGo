// src/app/(dashboard)/crm/customers/create/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowLeft,
  User,
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
  UserPlus,
  DollarSign,
  Building2,
  Mail,
  Phone,
  Shield,
  Tag,
  Save,
  Send,
  X,
  Plus,
  Trash2,
} from "lucide-react";

type FormData = {
  name: string;
  type: "Corporate" | "SME" | "Enterprise" | "Individual";
  status: "Active" | "Follow-up" | "At Risk" | "Inactive";
  risk: "Low" | "Medium" | "High";
  email: string;
  phone: string;
  location: string;
  branch: string;
  accountOwner: string;
  notes: string;
  contacts: {
    name: string;
    role: string;
    email: string;
    phone: string;
    primary: boolean;
  }[];
};

const CUSTOMER_TYPES = ["Corporate", "SME", "Enterprise", "Individual"];
const STATUS_OPTIONS = ["Active", "Follow-up", "At Risk", "Inactive"];
const RISK_OPTIONS = ["Low", "Medium", "High"];
const BRANCHES = ["Ikeja", "Lekki", "Yaba", "Apapa"];
const ACCOUNT_OWNERS = ["F. Daniel", "A. Yusuf", "M. Bello", "O. Adebayo"];

export default function CreateCustomerPage() {
  const router = useRouter();

  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [showSuccess, setShowSuccess] = useState(false);
  const [duplicateWarning, setDuplicateWarning] = useState(false);

  const [formData, setFormData] = useState<FormData>({
    name: "",
    type: "Corporate",
    status: "Active",
    risk: "Low",
    email: "",
    phone: "",
    location: "",
    branch: "Ikeja",
    accountOwner: "F. Daniel",
    notes: "",
    contacts: [
      { name: "", role: "", email: "", phone: "", primary: true },
    ],
  });

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const handleFieldChange = (field: keyof FormData, value: any) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleContactChange = (index: number, field: string, value: any) => {
    const newContacts = [...formData.contacts];
    newContacts[index] = { ...newContacts[index], [field]: value };
    setFormData((prev) => ({ ...prev, contacts: newContacts }));
  };

  const addContact = () => {
    setFormData((prev) => ({
      ...prev,
      contacts: [
        ...prev.contacts,
        { name: "", role: "", email: "", phone: "", primary: false },
      ],
    }));
  };

  const removeContact = (index: number) => {
    if (formData.contacts.length <= 1) return;
    const newContacts = formData.contacts.filter((_, i) => i !== index);
    setFormData((prev) => ({ ...prev, contacts: newContacts }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    // Validation
    if (!formData.name.trim()) {
      toast("Please enter a customer name");
      return;
    }
    if (!formData.email.trim()) {
      toast("Please enter an email address");
      return;
    }

    // Check for duplicate (mock)
    if (formData.name.toLowerCase() === "charitos hospital") {
      setDuplicateWarning(true);
      return;
    }

    setSaving(true);
    setTimeout(() => {
      setSaving(false);
      setShowSuccess(true);
      setTimeout(() => {
        router.push("/crm/customers");
      }, 1500);
    }, 1500);
  };

  const toast = (message: string) => {
    const t = document.createElement("div");
    t.className = "fixed bottom-6 right-6 z-50 rounded-xl bg-ink-950 px-5 py-3 text-sm text-white shadow-xl";
    t.textContent = message;
    document.body.appendChild(t);
    setTimeout(() => t.remove(), 2500);
  };

  const handleCancel = () => {
    router.push("/crm/customers");
  };

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
              Create Customer
            </h1>
            <p className="mt-1 text-sm text-ink-600">
              Add a new customer to the CRM with duplicate detection.
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
            disabled={saving}
            className={`flex h-9 items-center gap-1.5 rounded-xl px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition ${
              saving ? "bg-ink-300 cursor-not-allowed" : "bg-primary hover:bg-primary/90 cursor-pointer"
            }`}
          >
            {saving ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <UserPlus className="h-3.5 w-3.5" />
            )}
            {saving ? "Creating..." : "Create Customer"}
          </button>
        </div>
      </div>

      {/* ===== SUCCESS NOTICE ===== */}
      {showSuccess && (
        <div className="rounded-xl border border-green/20 bg-green-bg p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-green p-1.5">
              <CheckCircle2 className="h-4 w-4 text-white" />
            </div>
            <div>
              <div className="text-sm font-semibold text-green">Customer created successfully!</div>
              <div className="text-sm text-green/80">
                {formData.name} has been added to the CRM.
              </div>
            </div>
          </div>
        </div>
      )}

      {/* ===== DUPLICATE WARNING ===== */}
      {duplicateWarning && (
        <div className="rounded-xl border border-amber/20 bg-amber-bg p-4">
          <div className="flex items-start gap-3">
            <div className="rounded-full bg-amber p-1.5">
              <AlertCircle className="h-4 w-4 text-white" />
            </div>
            <div className="flex-1">
              <div className="text-sm font-semibold text-amber">Potential duplicate detected</div>
              <div className="text-sm text-amber/80">
                A customer with the name "{formData.name}" already exists. Do you want to continue?
              </div>
              <div className="mt-2 flex items-center gap-2">
                <button
                  onClick={() => {
                    setDuplicateWarning(false);
                    setSaving(true);
                    setTimeout(() => {
                      setSaving(false);
                      setShowSuccess(true);
                      setTimeout(() => {
                        router.push("/crm/customers");
                      }, 1500);
                    }, 1500);
                  }}
                  className="rounded-lg bg-amber px-3.5 py-1 text-[11px] font-medium text-white transition hover:bg-amber/90"
                >
                  Continue Anyway
                </button>
                <button
                  onClick={() => setDuplicateWarning(false)}
                  className="rounded-lg border border-amber/30 px-3.5 py-1 text-[11px] font-medium text-amber transition hover:bg-amber/10"
                >
                  Review
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
            {/* Basic Information */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-4">Basic Information</h3>
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Customer Name *
                  </label>
                  <input
                    value={formData.name}
                    onChange={(e) => handleFieldChange("name", e.target.value)}
                    placeholder="Enter customer name..."
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    required
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Customer Type
                  </label>
                  <select
                    value={formData.type}
                    onChange={(e) => handleFieldChange("type", e.target.value as any)}
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {CUSTOMER_TYPES.map((type) => (
                      <option key={type} value={type}>{type}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Status
                  </label>
                  <select
                    value={formData.status}
                    onChange={(e) => handleFieldChange("status", e.target.value as any)}
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {STATUS_OPTIONS.map((status) => (
                      <option key={status} value={status}>{status}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Risk Level
                  </label>
                  <select
                    value={formData.risk}
                    onChange={(e) => handleFieldChange("risk", e.target.value as any)}
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {RISK_OPTIONS.map((risk) => (
                      <option key={risk} value={risk}>{risk}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Branch
                  </label>
                  <select
                    value={formData.branch}
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
                    Account Owner
                  </label>
                  <select
                    value={formData.accountOwner}
                    onChange={(e) => handleFieldChange("accountOwner", e.target.value)}
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  >
                    {ACCOUNT_OWNERS.map((owner) => (
                      <option key={owner} value={owner}>{owner}</option>
                    ))}
                  </select>
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Email *
                  </label>
                  <input
                    value={formData.email}
                    onChange={(e) => handleFieldChange("email", e.target.value)}
                    placeholder="customer@example.com"
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                    required
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Phone
                  </label>
                  <input
                    value={formData.phone}
                    onChange={(e) => handleFieldChange("phone", e.target.value)}
                    placeholder="+234 800 123 4567"
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Location / Address
                  </label>
                  <input
                    value={formData.location}
                    onChange={(e) => handleFieldChange("location", e.target.value)}
                    placeholder="City, State, Country"
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20"
                  />
                </div>
                <div className="sm:col-span-2">
                  <label className="mb-1 block text-xs font-medium uppercase tracking-[0.06em] text-ink-400">
                    Notes
                  </label>
                  <textarea
                    value={formData.notes}
                    onChange={(e) => handleFieldChange("notes", e.target.value)}
                    placeholder="Additional notes about this customer..."
                    rows={3}
                    className="w-full rounded-xl border border-line bg-white/70 px-4 py-2.5 text-sm text-ink-950 outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20 resize-none"
                  />
                </div>
              </div>
            </div>
          </div>

          {/* ===== RIGHT COLUMN ===== */}
          <div className="space-y-5">
            {/* Contacts */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-[15px] font-semibold text-ink-950">Contacts</h3>
                <button
                  type="button"
                  onClick={addContact}
                  className="flex items-center gap-1 rounded-lg bg-primary px-3 py-1.5 text-[11px] font-medium text-white transition hover:bg-primary/90"
                >
                  <Plus className="h-3.5 w-3.5" />
                  Add Contact
                </button>
              </div>
              <div className="space-y-3">
                {formData.contacts.map((contact, index) => (
                  <div key={index} className="rounded-lg border border-line p-3">
                    <div className="flex items-center justify-between mb-2">
                      <span className="text-xs font-medium text-ink-500">
                        Contact {index + 1}
                        {contact.primary && (
                          <span className="ml-2 rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">
                            Primary
                          </span>
                        )}
                      </span>
                      {formData.contacts.length > 1 && (
                        <button
                          type="button"
                          onClick={() => removeContact(index)}
                          className="rounded-lg p-1 text-red hover:bg-red-bg transition-colors"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </div>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                      <input
                        value={contact.name}
                        onChange={(e) => handleContactChange(index, "name", e.target.value)}
                        placeholder="Full name"
                        className="rounded-lg border border-line bg-white/70 px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                      />
                      <input
                        value={contact.role}
                        onChange={(e) => handleContactChange(index, "role", e.target.value)}
                        placeholder="Role"
                        className="rounded-lg border border-line bg-white/70 px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                      />
                      <input
                        value={contact.email}
                        onChange={(e) => handleContactChange(index, "email", e.target.value)}
                        placeholder="Email"
                        className="rounded-lg border border-line bg-white/70 px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                      />
                      <input
                        value={contact.phone}
                        onChange={(e) => handleContactChange(index, "phone", e.target.value)}
                        placeholder="Phone"
                        className="rounded-lg border border-line bg-white/70 px-3 py-1.5 text-sm text-ink-950 outline-none transition focus:border-primary"
                      />
                    </div>
                    <div className="mt-2 flex items-center gap-2">
                      <input
                        type="checkbox"
                        checked={contact.primary}
                        onChange={(e) => {
                          const newContacts = [...formData.contacts];
                          newContacts.forEach((c, i) => c.primary = i === index && e.target.checked);
                          setFormData((prev) => ({ ...prev, contacts: newContacts }));
                        }}
                        className="h-4 w-4 rounded border-line text-primary focus:ring-primary"
                      />
                      <label className="text-sm text-ink-600">Set as primary contact</label>
                    </div>
                  </div>
                ))}
              </div>
            </div>

            {/* Summary */}
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Summary</h3>
              <div className="space-y-2 text-sm">
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Name</span>
                  <span className="font-medium text-ink-950">{formData.name || "—"}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Type</span>
                  <span className="font-medium text-ink-950">{formData.type}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Status</span>
                  <span className="font-medium text-ink-950">{formData.status}</span>
                </div>
                <div className="flex items-center justify-between border-b border-line pb-2">
                  <span className="text-ink-500">Branch</span>
                  <span className="font-medium text-ink-950">{formData.branch}</span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-ink-500">Account Owner</span>
                  <span className="font-medium text-ink-950">{formData.accountOwner}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </form>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <UserPlus className="h-4 w-4" />
          CRM-02 · Create Customer
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          {saving ? "Creating..." : "Ready to create"}
        </span>
      </div>
    </div>
  );
}