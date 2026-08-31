// src/app/(dashboard)/workforce/training/[courseId]/completions/[personId]/page.tsx

"use client";

import { useState, useEffect } from "react";
import { useParams, useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  GraduationCap,
  CheckCircle2,
  XCircle,
  AlertCircle,
  Clock,
  Calendar,
  User,
  MapPin,
  FileText,
  Download,
  Printer,
  Eye,
  Award,
  TrendingUp,
  Sparkles,
  RefreshCw,
  Edit3,
  MoreVertical,
  ArrowRight,
  Users,
  Briefcase,
  Shield,
  Activity,
  CalendarDays,
  Phone,
  Mail,
  MessageSquare,
  Star,
} from "lucide-react";

type TrainingCompletion = {
  id: string;
  courseId: string;
  courseName: string;
  personId: string;
  personName: string;
  initials: string;
  status: "Completed" | "In Progress" | "Failed" | "Enrolled";
  score: number;
  maxScore: number;
  completionDate: string | null;
  expiryDate: string | null;
  certificateUrl: string | null;
  evidenceUrl: string | null;
  duration: string;
  instructor: string;
  branch: string;
  role: string;
  modules: {
    id: string;
    name: string;
    status: "Completed" | "In Progress" | "Pending" | "Failed";
    score: number;
  }[];
  attempts: {
    id: string;
    date: string;
    score: number;
    status: "Passed" | "Failed" | "In Progress";
  }[];
  notes: string | null;
  lastUpdated: string;
};

// Mock data
const COMPLETION_DETAIL: TrainingCompletion = {
  id: "TC-001",
  courseId: "TR-001",
  courseName: "Defensive Riding - Advanced",
  personId: "P-044",
  personName: "Peter Okafor",
  initials: "PO",
  status: "Completed",
  score: 92,
  maxScore: 100,
  completionDate: "2026-08-15",
  expiryDate: "2028-08-15",
  certificateUrl: "/certificates/po_defensive.pdf",
  evidenceUrl: "/evidence/po_defensive_proof.pdf",
  duration: "2 days (16 hours)",
  instructor: "T. Adeyemi",
  branch: "Ikeja",
  role: "Dispatch Rider",
  modules: [
    { id: "mod-001", name: "Introduction to Defensive Riding", status: "Completed", score: 95 },
    { id: "mod-002", name: "Hazard Perception", status: "Completed", score: 90 },
    { id: "mod-003", name: "Emergency Maneuvers", status: "Completed", score: 88 },
    { id: "mod-004", name: "Risk Assessment", status: "Completed", score: 94 },
    { id: "mod-005", name: "Practical Assessment", status: "Completed", score: 85 },
  ],
  attempts: [
    { id: "att-001", date: "2026-08-10", score: 78, status: "Failed" },
    { id: "att-002", date: "2026-08-15", score: 92, status: "Passed" },
  ],
  notes: "Excellent performance in practical assessment. Showed strong understanding of defensive riding techniques.",
  lastUpdated: "2 days ago",
};

const STATUS_STYLES: Record<string, { bg: string; text: string; dot: string; icon: React.ReactNode }> = {
  Completed: {
    bg: "bg-green-bg",
    text: "text-green",
    dot: "bg-green",
    icon: <CheckCircle2 className="h-4 w-4" />,
  },
  "In Progress": {
    bg: "bg-amber-bg",
    text: "text-amber",
    dot: "bg-amber",
    icon: <Clock className="h-4 w-4" />,
  },
  Failed: {
    bg: "bg-red-bg",
    text: "text-red",
    dot: "bg-red",
    icon: <XCircle className="h-4 w-4" />,
  },
  Enrolled: {
    bg: "bg-blue-bg",
    text: "text-blue",
    dot: "bg-blue",
    icon: <GraduationCap className="h-4 w-4" />,
  },
};

const MODULE_STATUS_STYLES: Record<string, { bg: string; text: string; dot: string }> = {
  Completed: { bg: "bg-green-bg", text: "text-green", dot: "bg-green" },
  "In Progress": { bg: "bg-amber-bg", text: "text-amber", dot: "bg-amber" },
  Pending: { bg: "bg-surface-2", text: "text-ink-500", dot: "bg-ink-400" },
  Failed: { bg: "bg-red-bg", text: "text-red", dot: "bg-red" },
};

export default function TrainingCompletionPage() {
  const params = useParams();
  const router = useRouter();
  const courseId = params.courseId as string;
  const personId = params.personId as string;

  const [loading, setLoading] = useState(true);
  const [completion, setCompletion] = useState<TrainingCompletion | null>(null);
  const [secondsAgo, setSecondsAgo] = useState(0);

  useEffect(() => {
    const loadData = async () => {
      await new Promise((resolve) => setTimeout(resolve, 500));
      setCompletion(COMPLETION_DETAIL);
      setLoading(false);
    };
    loadData();
  }, [courseId, personId]);

  useEffect(() => {
    const interval = setInterval(() => setSecondsAgo((s) => s + 1), 1000);
    return () => clearInterval(interval);
  }, []);

  // Navigation
  const goBack = () => {
    router.push("/workforce/training");
  };

  const goToPerson = (personId: string) => {
    router.push(`/workforce/${personId}`);
  };

  const goToCourse = () => {
    router.push(`/workforce/training/${courseId}`);
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex items-center justify-center">
          <div className="h-8 w-8 animate-spin rounded-full border-4 border-primary border-t-transparent"></div>
          <span className="ml-3 text-[13px] text-ink-600">Loading completion details...</span>
        </div>
      </div>
    );
  }

  if (!completion) {
    return (
      <div className="mx-auto max-w-5xl py-12">
        <div className="flex flex-col items-center justify-center rounded-2xl border border-dashed border-line bg-white/50 py-14">
          <div className="rounded-full bg-surface-2 p-4">
            <GraduationCap className="h-8 w-8 text-ink-400" />
          </div>
          <h3 className="mt-3 text-[15px] font-semibold text-ink-950">Completion record not found</h3>
          <p className="mt-1 max-w-sm text-center text-[13px] text-ink-500">
            The training completion record you're looking for doesn't exist.
          </p>
          <button
            onClick={goBack}
            className="mt-3.5 rounded-lg bg-primary px-4 py-1.5 text-[11.5px] font-semibold text-white transition hover:bg-primary/90"
          >
            Back to training
          </button>
        </div>
      </div>
    );
  }

  const statusStyle = STATUS_STYLES[completion.status] || STATUS_STYLES.Enrolled;
  const isCompleted = completion.status === "Completed";
  const scorePercentage = Math.round((completion.score / completion.maxScore) * 100);

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* ===== HEADER ===== */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex items-center gap-3">
            <button
              onClick={goBack}
              className="flex h-9 w-9 items-center justify-center rounded-xl border border-line bg-white/70 text-ink-600 transition hover:bg-white hover:text-ink-900"
            >
              <ArrowLeft className="h-4 w-4" />
            </button>
            <div>
              <div className="flex items-center gap-2.5">
                <h1 className="font-display text-[22px] font-semibold tracking-[-0.015em] text-ink-950">
                  Training Completion
                </h1>
                <span className={`flex items-center gap-1 rounded-full px-2.5 py-0.5 text-[11px] font-semibold ${statusStyle.bg} ${statusStyle.text}`}>
                  {statusStyle.icon}
                  {completion.status}
                </span>
              </div>
              <p className="mt-0.5 text-[13.5px] text-ink-600">
                {completion.courseName} · {completion.personName}
              </p>
            </div>
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
          {completion.certificateUrl && (
            <button className="flex h-9 items-center gap-1.5 rounded-xl border border-line bg-white/70 px-3 text-[11.5px] font-medium text-ink-600 shadow-sm transition hover:bg-white">
              <Download className="h-3.5 w-3.5" />
              Certificate
            </button>
          )}
          <button className="flex h-9 items-center gap-1.5 rounded-xl bg-primary px-3.5 text-[11.5px] font-semibold text-white shadow-sm shadow-primary/20 transition hover:bg-primary/90">
            <Edit3 className="h-3.5 w-3.5" />
            Edit
          </button>
        </div>
      </div>

      {/* ===== STATS ROW ===== */}
      <div className="grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-primary-bg p-1.5">
              <Award className="h-4 w-4 text-primary" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Score</div>
              <div className="text-[16px] font-semibold text-ink-950">{completion.score}%</div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-green-bg p-1.5">
              <CalendarDays className="h-4 w-4 text-green" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Completed</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {completion.completionDate || "—"}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-amber-bg p-1.5">
              <Clock className="h-4 w-4 text-amber" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Expires</div>
              <div className="text-[16px] font-semibold text-ink-950">
                {completion.expiryDate || "—"}
              </div>
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-line bg-white p-3.5 shadow-sm">
          <div className="flex items-center gap-2">
            <div className="rounded-full bg-blue-bg p-1.5">
              <Clock className="h-4 w-4 text-blue" />
            </div>
            <div>
              <div className="text-[10px] font-medium uppercase tracking-[0.06em] text-ink-500">Duration</div>
              <div className="text-[16px] font-semibold text-ink-950">{completion.duration}</div>
            </div>
          </div>
        </div>
      </div>

      {/* ===== SCORE BAR ===== */}
      <div className="rounded-xl border border-line bg-white p-4 shadow-sm">
        <div className="flex items-center justify-between mb-2">
          <span className="text-[12px] font-medium text-ink-600">Overall Score</span>
          <span className={`text-[12px] font-semibold ${
            scorePercentage >= 80 ? "text-green" :
            scorePercentage >= 60 ? "text-amber" :
            "text-red"
          }`}>
            {scorePercentage}%
          </span>
        </div>
        <div className="h-2 w-full rounded-full bg-surface-3 overflow-hidden">
          <div
            className={`h-full rounded-full transition-all ${
              scorePercentage >= 80 ? "bg-green" :
              scorePercentage >= 60 ? "bg-amber" :
              "bg-red"
            }`}
            style={{ width: `${scorePercentage}%` }}
          />
        </div>
        <div className="mt-1 flex justify-between text-[10px] text-ink-400">
          <span>Passing score: 80%</span>
          <span>{completion.score}/{completion.maxScore} points</span>
        </div>
      </div>

      {/* ===== MAIN GRID ===== */}
      <div className="grid grid-cols-1 gap-5 lg:grid-cols-[1.4fr_1fr]">
        {/* LEFT COLUMN */}
        <div className="space-y-5">
          {/* Person Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <User className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Participant</h3>
            </div>
            <div
              onClick={() => goToPerson(completion.personId)}
              className="flex cursor-pointer items-center gap-3 rounded-xl border border-line p-3 transition hover:bg-surface-2"
            >
              <div className="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full bg-primary text-[16px] font-bold text-white shadow-lg">
                {completion.initials}
              </div>
              <div>
                <div className="text-[14px] font-semibold text-ink-950">{completion.personName}</div>
                <div className="text-[12px] text-ink-500">{completion.role} · {completion.branch}</div>
              </div>
              <ArrowRight className="ml-auto h-4 w-4 text-ink-400" />
            </div>
          </div>

          {/* Course Info */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <GraduationCap className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Course</h3>
            </div>
            <div
              onClick={goToCourse}
              className="flex cursor-pointer items-center justify-between rounded-xl border border-line p-3 transition hover:bg-surface-2"
            >
              <div>
                <div className="text-[14px] font-semibold text-ink-950">{completion.courseName}</div>
                <div className="text-[12px] text-ink-500">{completion.courseId} · Instructor: {completion.instructor}</div>
              </div>
              <ArrowRight className="h-4 w-4 text-ink-400" />
            </div>
          </div>

          {/* Notes */}
          {completion.notes && (
            <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
              <div className="flex items-center gap-2.5 mb-2">
                <FileText className="h-5 w-5 text-ink-400" />
                <h3 className="text-[15px] font-semibold text-ink-950">Notes</h3>
              </div>
              <p className="text-[13px] text-ink-700">{completion.notes}</p>
            </div>
          )}
        </div>

        {/* RIGHT COLUMN */}
        <div className="space-y-5">
          {/* Modules */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <FileText className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Modules</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {completion.modules.length}
              </span>
            </div>
            <div className="space-y-2">
              {completion.modules.map((module) => {
                const modStyle = MODULE_STATUS_STYLES[module.status] || MODULE_STATUS_STYLES.Pending;
                return (
                  <div key={module.id} className="flex items-center justify-between rounded-lg border border-line p-2.5">
                    <div className="flex-1 min-w-0">
                      <div className="text-[13px] font-medium text-ink-950 truncate">{module.name}</div>
                      <div className="text-[11px] text-ink-500">Score: {module.score}%</div>
                    </div>
                    <span className={`flex items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-semibold ${modStyle.bg} ${modStyle.text}`}>
                      <span className={`h-1.5 w-1.5 rounded-full ${modStyle.dot}`} />
                      {module.status}
                    </span>
                  </div>
                );
              })}
            </div>
          </div>

          {/* Attempts */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <div className="flex items-center gap-2.5 mb-3">
              <Clock className="h-5 w-5 text-ink-400" />
              <h3 className="text-[15px] font-semibold text-ink-950">Attempts</h3>
              <span className="rounded-full bg-surface-2 px-2 py-0.5 text-[10px] font-medium text-ink-500">
                {completion.attempts.length}
              </span>
            </div>
            <div className="space-y-2">
              {completion.attempts.map((attempt) => (
                <div key={attempt.id} className="flex items-center justify-between rounded-lg border border-line p-2.5">
                  <div>
                    <div className="text-[13px] font-medium text-ink-950">{attempt.date}</div>
                    <div className="text-[11px] text-ink-500">Score: {attempt.score}%</div>
                  </div>
                  <span className={`rounded-full px-2.5 py-0.5 text-[10px] font-semibold ${
                    attempt.status === "Passed" ? "bg-green-bg text-green" :
                    attempt.status === "Failed" ? "bg-red-bg text-red" :
                    "bg-amber-bg text-amber"
                  }`}>
                    {attempt.status}
                  </span>
                </div>
              ))}
            </div>
          </div>

          {/* Actions */}
          <div className="rounded-2xl border border-line bg-white p-5 shadow-sm">
            <h3 className="text-[15px] font-semibold text-ink-950 mb-3">Actions</h3>
            <div className="space-y-2">
              {completion.certificateUrl && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <Download className="h-4 w-4" />
                    Download Certificate
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              {completion.evidenceUrl && (
                <button className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white">
                  <span className="flex items-center gap-2">
                    <Eye className="h-4 w-4" />
                    View Evidence
                  </span>
                  <ArrowRight className="h-4 w-4 text-ink-400" />
                </button>
              )}
              <button
                onClick={() => goToPerson(completion.personId)}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <User className="h-4 w-4" />
                  View Profile
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
              <button
                onClick={goToCourse}
                className="flex w-full items-center justify-between rounded-xl border border-line bg-white/70 px-4 py-2.5 text-[12.5px] font-medium text-ink-700 transition hover:bg-white"
              >
                <span className="flex items-center gap-2">
                  <GraduationCap className="h-4 w-4" />
                  View Course
                </span>
                <ArrowRight className="h-4 w-4 text-ink-400" />
              </button>
            </div>
          </div>
        </div>
      </div>

      {/* ===== NAVIGATION FOOTER ===== */}
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:justify-between">
        <button
          onClick={goBack}
          className="flex items-center gap-1.5 text-[12px] font-medium text-ink-500 hover:text-ink-900 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to training
        </button>
        <button
          onClick={() => goToPerson(completion.personId)}
          className="flex items-center gap-1.5 text-[12px] font-medium text-primary hover:text-primary-hover transition-colors"
        >
          View Profile
          <ArrowRight className="h-4 w-4" />
        </button>
      </div>
    </div>
  );
}