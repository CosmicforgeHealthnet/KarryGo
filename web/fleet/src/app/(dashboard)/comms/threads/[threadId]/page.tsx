// src/app/(dashboard)/comms/threads/[threadId]/page.tsx

"use client";

import { useState, useEffect, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  MessageSquare,
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Eye,
  ArrowRight,
  Search,
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
  MessageCircle,
  UserCheck,
  UserX,
  Clock as ClockIcon,
  Lock,
  Shield,
  ChevronDown,
  ChevronUp,
  Reply,
  Forward,
  Copy,
  Pin,
  Star,
  Flag,
  Smile,
  Mic,
  Image,
  File,
  AtSign,
  Hash,
  X,
  CheckCheck,
  Clock as ClockIcon2,
  AlertOctagon,
} from "lucide-react";

// ==================== TYPES ====================

type Message = {
  id: string;
  content: string;
  sender: {
    id: string;
    name: string;
    role: string;
    avatar?: string;
  };
  timestamp: string;
  status: "Sending" | "Sent" | "Delivered" | "Read" | "Failed";
  attachments: {
    id: string;
    name: string;
    size: string;
    type: string;
  }[];
  isInternal: boolean;
  replyTo?: string;
};

type ThreadDetail = {
  id: string;
  reference: string;
  subject: string;
  participants: {
    id: string;
    name: string;
    role: string;
    avatar?: string;
    online?: boolean;
  }[];
  visibility: "Internal" | "External" | "Restricted";
  status: "Active" | "Archived" | "Closed";
  branch: string;
  relatedTo: {
    type: "Customer" | "Provider" | "Order" | "Trip" | "Incident" | "Support";
    reference: string;
    id: string;
  } | null;
  messages: Message[];
  createdAt: string;
  updatedAt: string;
  tags: string[];
  sourceFreshness: string;
};

// ==================== MOCK DATA ====================

const MOCK_THREAD_DATA: Record<string, ThreadDetail> = {
  "THR-001": {
    id: "THR-001",
    reference: "THR-2026-001",
    subject: "Delivery issue - KG-8491",
    participants: [
      { id: "P-044", name: "Peter Okafor", role: "Provider", online: true },
      { id: "C-001", name: "Adeyemi Stores", role: "Customer", online: false },
      { id: "S-001", name: "F. Daniel", role: "Dispatcher", online: true },
    ],
    visibility: "External",
    status: "Active",
    branch: "Ikeja",
    relatedTo: {
      type: "Trip",
      reference: "KG-8491",
      id: "KG-8491",
    },
    messages: [
      {
        id: "MSG-001",
        content: "I'm having trouble with the delivery. The address seems incorrect. Can you confirm?",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-22T10:15:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-002",
        content: "Let me check the system. I'll get back to you shortly.",
        sender: { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
        timestamp: "2026-08-22T10:20:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-003",
        content: "I've confirmed the address. It's 42 Adebayo Street, not 24. I've updated the system.",
        sender: { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
        timestamp: "2026-08-22T10:30:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-004",
        content: "Thanks for confirming. I'm on my way now. ETA 25 minutes.",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-22T10:35:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-005",
        content: "Update: The customer is requesting to reschedule. They're not available until 3pm.",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-22T14:15:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-006",
        content: "I'll reschedule for tomorrow morning. Let me check availability.",
        sender: { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
        timestamp: "2026-08-22T14:25:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-007",
        content: "Rescheduled for 8am tomorrow. I'll send the confirmation to the customer.",
        sender: { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
        timestamp: "2026-08-22T14:30:00Z",
        status: "Sent",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-008",
        content: "This is an internal note: Customer has been difficult. Consider adding a note to their profile.",
        sender: { id: "S-001", name: "F. Daniel", role: "Dispatcher" },
        timestamp: "2026-08-22T14:32:00Z",
        status: "Read",
        attachments: [],
        isInternal: true,
      },
    ],
    createdAt: "2026-08-22T10:15:00Z",
    updatedAt: "2026-08-22T14:30:00Z",
    tags: ["Delivery", "Customer", "Urgent"],
    sourceFreshness: "2026-08-22T14:30:00Z",
  },
  "THR-002": {
    id: "THR-002",
    reference: "THR-2026-002",
    subject: "Vehicle maintenance request - BR-204",
    participants: [
      { id: "P-044", name: "Peter Okafor", role: "Provider", online: false },
      { id: "S-002", name: "M. Bello", role: "Fleet Manager", online: true },
    ],
    visibility: "Internal",
    status: "Active",
    branch: "Ikeja",
    relatedTo: {
      type: "Incident",
      reference: "INC-771",
      id: "INC-771",
    },
    messages: [
      {
        id: "MSG-009",
        content: "I need to report an issue with BR-204. The front brake is making a grinding noise.",
        sender: { id: "P-044", name: "Peter Okafor", role: "Provider" },
        timestamp: "2026-08-22T08:30:00Z",
        status: "Read",
        attachments: [
          { id: "ATT-001", name: "brake_photo.jpg", size: "2.4 MB", type: "image" },
        ],
        isInternal: false,
      },
      {
        id: "MSG-010",
        content: "Thanks for reporting. I've created work order WO-1917. Bring the vehicle to the workshop.",
        sender: { id: "S-002", name: "M. Bello", role: "Fleet Manager" },
        timestamp: "2026-08-22T09:00:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
      {
        id: "MSG-011",
        content: "Work order created. Scheduled for tomorrow at 9am.",
        sender: { id: "S-002", name: "M. Bello", role: "Fleet Manager" },
        timestamp: "2026-08-22T11:45:00Z",
        status: "Read",
        attachments: [],
        isInternal: false,
      },
    ],
    createdAt: "2026-08-22T08:30:00Z",
    updatedAt: "2026-08-22T11:45:00Z",
    tags: ["Maintenance", "Vehicle"],
    sourceFreshness: "2026-08-22T11:45:00Z",
  },
};

const VISIBILITY_STYLES: Record<string, { bg: string; text: string; icon: React.ReactNode }> = {
  Internal: { bg: "bg-purple-bg", text: "text-purple", icon: <Lock className="h-4 w-4" /> },
  External: { bg: "bg-blue-bg", text: "text-blue", icon: <Users className="h-4 w-4" /> },
  Restricted: { bg: "bg-red-bg", text: "text-red", icon: <Shield className="h-4 w-4" /> },
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Active: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  Archived: { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Closed: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
};

const MESSAGE_STATUS_ICONS: Record<string, React.ReactNode> = {
  Sending: <ClockIcon2 className="h-3.5 w-3.5 text-ink-400" />,
  Sent: <Check className="h-3.5 w-3.5 text-ink-400" />,
  Delivered: <CheckCheck className="h-3.5 w-3.5 text-ink-400" />,
  Read: <CheckCheck className="h-3.5 w-3.5 text-blue" />,
  Failed: <AlertOctagon className="h-3.5 w-3.5 text-red" />,
};

export default function ThreadDetailPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;

  const [loading, setLoading] = useState(true);
  const [data, setData] = useState<ThreadDetail | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);
  const [newMessage, setNewMessage] = useState("");
  const [showParticipants, setShowParticipants] = useState(true);
  const messagesEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 600));
      const mockData = MOCK_THREAD_DATA[threadId] || MOCK_THREAD_DATA["THR-001"];
      setData(mockData);
      setLoading(false);
    };
    loadData();
  }, [threadId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (messagesEndRef.current) {
      messagesEndRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [data]);

  if (loading) {
    return (
      <div className="py-12 text-center">
        <div className="inline-block h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
        <p className="mt-3 text-sm text-ink-600">Loading thread...</p>
      </div>
    );
  }

  if (!data) {
    return (
      <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
        <div className="rounded-full bg-surface-2 p-4">
          <MessageSquare className="h-8 w-8 text-ink-400" />
        </div>
        <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Thread not found</h3>
        <p className="mt-1 text-sm text-ink-500">No thread found with this ID.</p>
        <button
          onClick={() => router.push("/comms/threads")}
          className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
        >
          Back to Threads
        </button>
      </div>
    );
  }

  const visibilityStyle = VISIBILITY_STYLES[data.visibility] || VISIBILITY_STYLES.External;
  const statusStyle = STATUS_STYLES[data.status] || STATUS_STYLES.Active;

  const formatTime = (date: string) => {
    const d = new Date(date);
    const now = new Date();
    const diffMs = now.getTime() - d.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    const diffHr = Math.floor(diffMs / 3600000);
    const diffDay = Math.floor(diffMs / 86400000);

    if (diffMin < 1) return "Just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    if (diffHr < 24) return `${diffHr}h ago`;
    return `${diffDay}d ago`;
  };

  const formatTimestamp = (date: string) => {
    const d = new Date(date);
    return d.toLocaleString("en-GB", {
      day: "2-digit",
      month: "short",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const handleSendMessage = () => {
    if (!newMessage.trim()) return;
    // Mock send
    toast("Message sent (mock)");
    setNewMessage("");
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSendMessage();
    }
  };

  return (
    <div className="space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="flex items-center gap-2 text-sm text-ink-500">
            <button
              onClick={() => router.push("/comms/threads")}
              className="hover:text-primary transition-colors"
            >
              Threads
            </button>
            <span>/</span>
            <span className="text-ink-700 font-medium">{data.reference}</span>
          </div>
          <h1 className="text-xl font-semibold tracking-tight text-ink-950 sm:text-2xl mt-1">
            {data.subject}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${visibilityStyle.bg} ${visibilityStyle.text}`}>
              {visibilityStyle.icon}
              {data.visibility}
            </span>
            <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
              <span className={`h-1.5 w-1.5 rounded-full ${statusStyle.dot}`} />
              {data.status}
            </span>
            <span className="text-[10px] text-ink-500">{data.branch}</span>
            {data.relatedTo && (
              <span className="flex items-center gap-1 text-[10px] text-blue font-medium">
                <Link2 className="h-3 w-3" />
                {data.relatedTo.reference}
              </span>
            )}
            {data.tags.map((tag) => (
              <span key={tag} className="rounded-full bg-surface-2 px-2 py-0.5 text-[9px] font-medium text-ink-500">
                {tag}
              </span>
            ))}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 flex-shrink-0">
          <button
            onClick={() => setSecondsAgo(0)}
            className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-500 shadow-sm transition hover:bg-white"
          >
            <RefreshCw className="h-3 w-3" />
            {secondsAgo}s
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <Reply className="h-3.5 w-3.5" />
            Reply All
          </button>
          <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3.5 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
            <MoreHorizontal className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>

      {/* ===== PARTICIPANTS ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <button
          onClick={() => setShowParticipants(!showParticipants)}
          className="flex w-full items-center justify-between p-3 hover:bg-surface-2 transition-colors"
        >
          <div className="flex items-center gap-3">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Users className="h-4 w-4 text-blue" />
            </div>
            <div>
              <span className="font-medium text-ink-950 text-sm">Participants</span>
              <span className="ml-2 text-sm text-ink-500">{data.participants.length} people</span>
            </div>
          </div>
          {showParticipants ? (
            <ChevronUp className="h-4 w-4 text-ink-400" />
          ) : (
            <ChevronDown className="h-4 w-4 text-ink-400" />
          )}
        </button>

        {showParticipants && (
          <div className="border-t border-line p-3">
            <div className="flex flex-wrap gap-2">
              {data.participants.map((p) => (
                <div
                  key={p.id}
                  className="flex items-center gap-2 rounded-xl border border-line px-3 py-1.5"
                >
                  <div className="relative">
                    <div className="h-6 w-6 rounded-full bg-primary-bg flex items-center justify-center text-[10px] font-bold text-primary">
                      {p.name.charAt(0)}
                    </div>
                    {p.online && (
                      <span className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full bg-green border-2 border-white" />
                    )}
                  </div>
                  <div>
                    <div className="text-xs font-medium text-ink-950">{p.name}</div>
                    <div className="text-[9px] text-ink-500">{p.role}</div>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* ===== MESSAGES ===== */}
      <div className="rounded-2xl border border-line bg-white shadow-sm overflow-hidden">
        <div className="max-h-[500px] overflow-y-auto p-4 space-y-4">
          {data.messages.map((message) => {
            const isOwn = message.sender.id === "S-001";
            const statusIcon = MESSAGE_STATUS_ICONS[message.status] || MESSAGE_STATUS_ICONS.Sent;

            return (
              <div
                key={message.id}
                className={`flex ${isOwn ? "justify-end" : "justify-start"}`}
              >
                <div
                  className={`max-w-[80%] rounded-2xl px-4 py-3 ${
                    isOwn
                      ? "bg-primary text-white"
                      : message.isInternal
                      ? "bg-purple-bg border border-purple/20"
                      : "bg-surface-2"
                  }`}
                >
                  <div className="flex items-center gap-2 mb-1">
                    <span className={`text-xs font-medium ${isOwn ? "text-white/90" : "text-ink-950"}`}>
                      {message.sender.name}
                    </span>
                    {message.isInternal && (
                      <span className="rounded-full bg-purple-bg px-1.5 py-0.5 text-[8px] font-medium text-purple">
                        Internal
                      </span>
                    )}
                  </div>
                  <div className={`text-sm ${isOwn ? "text-white" : "text-ink-950"}`}>
                    {message.content}
                  </div>
                  {message.attachments.length > 0 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {message.attachments.map((att) => (
                        <div
                          key={att.id}
                          className={`flex items-center gap-1 rounded-lg px-2 py-1 text-[10px] ${
                            isOwn ? "bg-white/10 text-white/80" : "bg-white text-ink-700"
                          }`}
                        >
                          <Paperclip className="h-3 w-3" />
                          {att.name}
                          <span className="opacity-50">({att.size})</span>
                        </div>
                      ))}
                    </div>
                  )}
                  <div className={`flex items-center gap-1.5 mt-1.5 text-[10px] ${
                    isOwn ? "text-white/60" : "text-ink-400"
                  }`}>
                    <span>{formatTimestamp(message.timestamp)}</span>
                    {isOwn && statusIcon}
                  </div>
                </div>
              </div>
            );
          })}
          <div ref={messagesEndRef} />
        </div>

        {/* ===== MESSAGE INPUT ===== */}
        <div className="border-t border-line p-3">
          <div className="flex items-end gap-2">
            <button className="flex-shrink-0 rounded-lg p-2 hover:bg-surface-2 transition-colors text-ink-400 hover:text-ink-600">
              <Paperclip className="h-5 w-5" />
            </button>
            <button className="flex-shrink-0 rounded-lg p-2 hover:bg-surface-2 transition-colors text-ink-400 hover:text-ink-600">
              <Image className="h-5 w-5" />
            </button>
            <div className="flex-1 min-w-0">
              <textarea
                value={newMessage}
                onChange={(e) => setNewMessage(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Type a message..."
                className="w-full resize-none rounded-xl border border-line px-3 py-2 text-sm text-ink-950 outline-none placeholder:text-ink-400 focus:border-primary min-h-[44px] max-h-[120px]"
                rows={1}
              />
            </div>
            <button
              onClick={handleSendMessage}
              disabled={!newMessage.trim()}
              className={`flex-shrink-0 rounded-xl px-4 py-2.5 text-sm font-semibold transition ${
                newMessage.trim()
                  ? "bg-primary text-white shadow-sm shadow-primary/20 hover:bg-primary/90"
                  : "bg-surface-2 text-ink-400 cursor-not-allowed"
              }`}
            >
              <Send className="h-4 w-4" />
            </button>
          </div>
          <div className="mt-1.5 text-[10px] text-ink-400 flex items-center gap-2">
            <span>Press Enter to send</span>
            <span>·</span>
            <span className="flex items-center gap-1">
              <Lock className="h-3 w-3" />
              Messages are secure
            </span>
          </div>
        </div>
      </div>

      {/* ===== FOOTER ===== */}
      <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-line bg-white/50 px-4 py-3 text-sm text-ink-400">
        <span className="flex items-center gap-2">
          <MessageSquare className="h-4 w-4" />
          COM-02 · Thread Detail · {data.reference}
        </span>
        <span className="flex items-center gap-1.5">
          <Clock className="h-4 w-4" />
          Updated {secondsAgo}s ago · {data.messages.length} messages
        </span>
      </div>
    </div>
  );
}