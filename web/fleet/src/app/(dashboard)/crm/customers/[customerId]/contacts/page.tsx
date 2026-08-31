// src/app/(dashboard)/crm/customers/[customerId]/contacts/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  Users,
  Search,
  RefreshCw,
  Filter,
  SlidersHorizontal,
  X,
  ChevronDown,
  Clock,
  CheckCircle2,
  AlertCircle,
  MoreHorizontal,
  Plus,
  Mail,
  Phone,
  Send,
} from "lucide-react";

type Contact = {
  id: string;
  name: string;
  role: string;
  email: string;
  phone: string;
  primary: boolean;
  preferredChannel: "Email" | "Phone" | "WhatsApp" | "SMS";
  consent: "Consented" | "Opted Out" | "Pending";
  department: string;
  notes: string | null;
  lastContact: string;
};

// Mock data
const MOCK_CONTACTS: Contact[] = [
  {
    id: "CON-001",
    name: "Dr. Adeola Ogunlesi",
    role: "Procurement Director",
    email: "adeola.ogunlesi@charitoshospital.com",
    phone: "+234 800 111 2222",
    primary: true,
    preferredChannel: "Email",
    consent: "Consented",
    department: "Procurement",
    notes: "Primary decision maker for logistics contracts.",
    lastContact: "2 days ago",
  },
  {
    id: "CON-002",
    name: "Mr. Femi Adeyemi",
    role: "Logistics Manager",
    email: "femi.adeyemi@charitoshospital.com",
    phone: "+234 800 333 4444",
    primary: false,
    preferredChannel: "Phone",
    consent: "Consented",
    department: "Logistics",
    notes: "Daily operations contact.",
    lastContact: "5 days ago",
  },
  {
    id: "CON-003",
    name: "Mrs. Toyin Akinola",
    role: "Finance Controller",
    email: "toyin.akinola@charitoshospital.com",
    phone: "+234 800 555 6666",
    primary: false,
    preferredChannel: "Email",
    consent: "Pending",
    department: "Finance",
    notes: "Handles invoice approvals and payments.",
    lastContact: "1 week ago",
  },
  {
    id: "CON-004",
    name: "Mr. Chidi Okonkwo",
    role: "Warehouse Supervisor",
    email: "chidi.okonkwo@charitoshospital.com",
    phone: "+234 800 777 8888",
    primary: false,
    preferredChannel: "WhatsApp",
    consent: "Opted Out",
    department: "Warehouse",
    notes: "Previously opted out of communication.",
    lastContact: "3 weeks ago",
  },
];

const CONSENT_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Consented: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-3.5 w-3.5" />,
  },
  "Opted Out": {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <AlertCircle className="h-3.5 w-3.5" />,
  },
  Pending: {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-3.5 w-3.5" />,
  },
};

const CHANNEL_STYLES: Record<string, { bg: string; text: string }> = {
  Email: { bg: "bg-blue-bg", text: "text-blue" },
  Phone: { bg: "bg-green-bg", text: "text-green" },
  WhatsApp: { bg: "bg-green-bg", text: "text-green" },
  SMS: { bg: "bg-amber-bg", text: "text-amber" },
};

const CONSENT_FILTERS = ["All", "Consented", "Opted Out", "Pending"];
const CHANNEL_FILTERS = ["All", "Email", "Phone", "WhatsApp", "SMS"];

export default function CustomerContactsPage() {
  const router = useRouter();
  const params = useParams();
  const customerId = params.customerId as string;

  const [search, setSearch] = useState("");
  const [selectedConsent, setSelectedConsent] = useState("All");
  const [selectedChannel, setSelectedChannel] = useState("All");
  const [showFilters, setShowFilters] = useState(false);
  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<Contact[]>([]);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      setData(MOCK_CONTACTS);
      setLoading(false);
    };
    loadData();
  }, []);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  const filteredData = data.filter((item) => {
    const matchesSearch =
      search.trim() === "" ||
      item.name.toLowerCase().includes(search.toLowerCase()) ||
      item.role.toLowerCase().includes(search.toLowerCase()) ||
      item.email.toLowerCase().includes(search.toLowerCase()) ||
      item.department.toLowerCase().includes(search.toLowerCase());

    const matchesConsent = selectedConsent === "All" || item.consent === selectedConsent;
    const matchesChannel = selectedChannel === "All" || item.preferredChannel === selectedChannel;

    return matchesSearch && matchesConsent && matchesChannel;
  });

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading contacts...</p>
      </div>
    );
  }

  const hasActiveFilters = selectedConsent !== "All" || selectedChannel !== "All" || search !== "";
  const clearFilters = () => {
    setSearch("");
    setSelectedConsent("All");
    setSelectedChannel("All");
  };

  const goToContactDetail = (contactId: string) => {
    router.push(`/crm/customers/${customerId}/contacts/${contactId}`);
  };

  const goToAddContact = () => {
    router.push(`/crm/customers/${customerId}/contacts/create`);
  };

  // Stats
  const totalContacts = data.length;
  const primaryCount = data.filter((d) => d.primary).length;
  const consentedCount = data.filter((d) => d.consent === "Consented").length;

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl">
            Customer Contacts
          </h1>
          <p className="mt-1 text-sm text-ink-600">
            People, role, email/phone, preferred channel, consent and primary-contact state.
          </p>
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
            onClick={goToAddContact}
            className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90"
          >
            <Plus className="h-3.5 w-3.5" />
            Add Contact
          </button>
        </div>
      </div>

      {/* ===== STATS ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Total Contacts</div>
          <div className="text-lg font-bold text-ink-950">{totalContacts}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Primary</div>
          <div className="text-lg font-bold text-primary">{primaryCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Consented</div>
          <div className="text-lg font-bold text-green">{consentedCount}</div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-400">Opted Out</div>
          <div className="text-lg font-bold text-red">{totalContacts - consentedCount}</div>
        </div>
      </div>

      {/* ===== FILTERS ===== */}
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex h-11 flex-1 min-w-[200px] items-center gap-2.5 rounded-xl border border-line bg-white/70 px-4 shadow-sm">
          <Search className="h-4 w-4 text-ink-400" />
          <input
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by name, role, email, or department..."
            className="w-full bg-transparent text-sm text-ink-950 outline-none placeholder:text-ink-400"
          />
        </div>

        <button
          onClick={() => {
            const consents = ["All", "Consented", "Opted Out", "Pending"];
            const currentIndex = consents.indexOf(selectedConsent);
            const nextIndex = (currentIndex + 1) % consents.length;
            setSelectedConsent(consents[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Filter className="h-4 w-4 text-ink-400" />
          {selectedConsent}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => {
            const channels = ["All", "Email", "Phone", "WhatsApp", "SMS"];
            const currentIndex = channels.indexOf(selectedChannel);
            const nextIndex = (currentIndex + 1) % channels.length;
            setSelectedChannel(channels[nextIndex]);
          }}
          className="flex h-11 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-sm font-medium text-ink-600 shadow-sm transition hover:bg-white"
        >
          <Send className="h-4 w-4 text-ink-400" />
          {selectedChannel}
          <ChevronDown className="h-4 w-4 text-ink-400" />
        </button>

        <button
          onClick={() => setShowFilters(!showFilters)}
          className={`flex h-11 items-center gap-1.5 rounded-xl border px-3.5 text-sm font-medium transition ${
            showFilters || hasActiveFilters
              ? "border-primary bg-primary-bg text-primary"
              : "border-line bg-white/70 text-ink-600 shadow-sm hover:bg-white"
          }`}
        >
          <SlidersHorizontal className="h-4 w-4" />
          Filters
          {hasActiveFilters && (
            <span className="rounded-full bg-primary px-1.5 py-0.5 text-[9px] text-white">
              {(selectedConsent !== "All" ? 1 : 0) + (selectedChannel !== "All" ? 1 : 0)}
            </span>
          )}
        </button>

        {hasActiveFilters && (
          <button
            onClick={clearFilters}
            className="flex h-11 items-center gap-1 rounded-xl border border-line bg-white/70 px-3 text-sm font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <X className="h-4 w-4" />
            Clear
          </button>
        )}
      </div>

      {/* ===== FILTER DRAWER ===== */}
      {showFilters && (
        <div className="rounded-xl border border-line bg-white/70 p-3.5 shadow-sm backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-6">
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Consent</div>
              <div className="flex gap-1.5">
                {CONSENT_FILTERS.map((consent) => (
                  <button
                    key={consent}
                    onClick={() => setSelectedConsent(consent)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedConsent === consent
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {consent}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <div className="mb-1 text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Preferred Channel</div>
              <div className="flex gap-1.5">
                {CHANNEL_FILTERS.map((channel) => (
                  <button
                    key={channel}
                    onClick={() => setSelectedChannel(channel)}
                    className={`rounded-full px-3 py-1 text-[11px] font-semibold transition ${
                      selectedChannel === channel
                        ? "bg-primary-bg text-primary ring-2 ring-primary/30"
                        : "bg-surface-2 text-ink-600 hover:bg-surface-3"
                    }`}
                  >
                    {channel}
                  </button>
                ))}
              </div>
            </div>
            <button
              onClick={clearFilters}
              className="rounded-lg border border-line px-3.5 py-1.5 text-[11px] font-medium text-ink-600 hover:bg-white transition-colors"
            >
              Clear all
            </button>
          </div>
        </div>
      )}

      {/* ===== CONTACT CARDS ===== */}
      {filteredData.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <Users className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">No contacts found</h3>
          <p className="mt-1 text-sm text-ink-500">No contacts match your current filters.</p>
          <button onClick={clearFilters} className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90">
            Clear filters
          </button>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredData.map((item) => {
            const consentStyle = CONSENT_STYLES[item.consent] || CONSENT_STYLES.Pending;
            const channelStyle = CHANNEL_STYLES[item.preferredChannel] || CHANNEL_STYLES.Email;

            return (
              <div
                key={item.id}
                onClick={() => goToContactDetail(item.id)}
                className={`group relative cursor-pointer rounded-xl border p-4 transition-all duration-300 hover:shadow-xl hover:-translate-y-1 ${
                  item.primary
                    ? "border-primary/30 bg-primary-bg/10 hover:border-primary/50"
                    : item.consent === "Opted Out"
                    ? "border-red/20 bg-red-bg/10 hover:border-red/40"
                    : "border-line bg-white hover:border-ink-300"
                }`}
              >
                {/* Glow effect */}
                <div className={`absolute inset-0 rounded-xl opacity-0 group-hover:opacity-100 transition-opacity duration-500 ${
                  item.primary ? "bg-primary/5" :
                  item.consent === "Opted Out" ? "bg-red-500/5" :
                  "bg-primary/5"
                }`} />

                <div className="relative z-10 flex items-start justify-between">
                  <div className="flex items-center gap-3">
                    <div
                      className={`flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full text-[14px] font-bold text-white shadow-lg ${
                        item.primary ? "bg-primary" :
                        item.consent === "Opted Out" ? "bg-red" :
                        "bg-ink-400"
                      }`}
                    >
                      {item.name.split(" ").map(n => n[0]).join("")}
                    </div>
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-[15px] font-semibold text-ink-950">
                          {item.name}
                        </span>
                        {item.primary && (
                          <span className="rounded-full bg-primary-bg px-2 py-0.5 text-[9px] font-semibold text-primary">
                            Primary
                          </span>
                        )}
                      </div>
                      <div className="flex flex-wrap items-center gap-1.5 mt-0.5">
                        <span className="text-[12px] text-ink-500">{item.role}</span>
                        <span className="text-ink-300">·</span>
                        <span className="text-[12px] text-ink-500">{item.department}</span>
                      </div>
                    </div>
                  </div>
                  <button className="flex-shrink-0 text-ink-300 hover:text-ink-600 transition-colors">
                    <MoreHorizontal className="h-4 w-4" />
                  </button>
                </div>

                <div className="relative z-10 mt-3 space-y-1.5 text-sm">
                  <div className="flex items-center gap-2 text-ink-600">
                    <Mail className="h-4 w-4 text-ink-400" />
                    <span>{item.email}</span>
                  </div>
                  <div className="flex items-center gap-2 text-ink-600">
                    <Phone className="h-4 w-4 text-ink-400" />
                    <span>{item.phone}</span>
                  </div>
                </div>

                <div className="relative z-10 mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-2.5">
                  <span
                    className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${consentStyle.bg} ${consentStyle.text}`}
                  >
                    {consentStyle.icon}
                    {item.consent}
                  </span>
                  <span
                    className={`rounded-full px-2.5 py-0.5 text-[10px] font-medium ${channelStyle.bg} ${channelStyle.text}`}
                  >
                    {item.preferredChannel}
                  </span>
                  <span className="ml-auto text-[11px] text-ink-400">
                    Last contact: {item.lastContact}
                  </span>
                </div>

                {item.notes && (
                  <div className="relative z-10 mt-2 text-sm text-ink-500 bg-surface-2 rounded-lg px-3 py-1.5">
                    {item.notes}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <Users className="h-4 w-4" />
          CRM-04 · Customer Contacts
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {filteredData.length} contacts shown
        </span>
      </div>
    </div>
  );
}