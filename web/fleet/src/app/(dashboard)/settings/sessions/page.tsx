"use client";

import { useState } from "react";

type Session = {
  id: string;
  device: string;
  location: string;
  createdAt: string;
  lastActive: string;
  isCurrent: boolean;
};

// MOCK: replace with the real session list once fleet-partner-service
// auth endpoints are wired in.
const MOCK_SESSIONS: Session[] = [
  {
    id: "sess_1",
    device: "Chrome on Windows",
    location: "Abuja, Nigeria",
    createdAt: "Aug 10, 2026",
    lastActive: "Active now",
    isCurrent: true,
  },
  {
    id: "sess_2",
    device: "KarryGo Mobile — Android",
    location: "Lagos, Nigeria",
    createdAt: "Aug 6, 2026",
    lastActive: "2 hours ago",
    isCurrent: false,
  },
  {
    id: "sess_3",
    device: "Safari on macOS",
    location: "Port Harcourt, Nigeria",
    createdAt: "Jul 29, 2026",
    lastActive: "5 days ago",
    isCurrent: false,
  },
];

export default function SessionsPage() {
  const [sessions, setSessions] = useState<Session[]>(MOCK_SESSIONS);
  const [revoking, setRevoking] = useState<string | null>(null);

  async function handleRevoke(session: Session) {
    if (revoking) return;

    if (session.isCurrent) {
      // Revoking the current session is high-risk: send through
      // reverification first rather than acting immediately.
      window.location.href = `/reverify?action=${encodeURIComponent(
        "Revoke your current session",
      )}&returnTo=${encodeURIComponent("/settings/sessions")}`;
      return;
    }

    setRevoking(session.id);
    // MOCK: simulates the revoke call until the real endpoint is wired in.
    await new Promise((resolve) => setTimeout(resolve, 500));
    setSessions((prev) => prev.filter((s) => s.id !== session.id));
    setRevoking(null);
  }

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <h1 className="font-display text-[28px] font-medium tracking-[-0.01em] text-ink-950">
        Sessions and devices
      </h1>
      <p className="mt-1.5 text-[13.5px] text-ink-600">
        Devices currently signed in to your account.
      </p>

      <div className="mt-8 divide-y divide-line rounded-xl border border-line bg-white">
        {sessions.map((session) => (
          <div
            key={session.id}
            className="flex items-center justify-between gap-4 px-5 py-4"
          >
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="truncate text-[14px] font-semibold text-ink-950">
                  {session.device}
                </span>
                {session.isCurrent && (
                  <span className="flex-shrink-0 rounded-full bg-green-bg px-2 py-0.5 text-[10.5px] font-semibold text-green">
                    This device
                  </span>
                )}
              </div>
              <div className="mt-1 text-[12.5px] text-ink-500">
                {session.location} · Signed in {session.createdAt} ·{" "}
                {session.lastActive}
              </div>
            </div>

            <button
              type="button"
              onClick={() => handleRevoke(session)}
              disabled={revoking === session.id}
              className="flex-shrink-0 text-[13px] font-medium text-red transition hover:text-red/80 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {revoking === session.id
                ? "Revoking…"
                : session.isCurrent
                  ? "Sign out"
                  : "Revoke"}
            </button>
          </div>
        ))}

        {sessions.length === 0 && (
          <div className="px-5 py-10 text-center text-[13px] text-ink-500">
            No active sessions.
          </div>
        )}
      </div>
    </div>
  );
}