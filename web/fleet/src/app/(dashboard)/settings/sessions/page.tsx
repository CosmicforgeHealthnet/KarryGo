"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { sessionService, type AuthSession } from "@/lib/services/authService";
import { useAuthStore } from "@/lib/stores/authStore";

export default function SessionsPage() {
  const router = useRouter();
  const logout = useAuthStore((s) => s.logout);
  const [sessions, setSessions] = useState<AuthSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [revoking, setRevoking] = useState<string | null>(null);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  const load = useCallback(async () => {
    try {
      setSessions(await sessionService.list());
      setError(null);
    } catch (e) {
      setError(describeApiError(e, "Your sessions could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    Promise.resolve().then(load);
  }, [load]);

  async function handleRevoke(session: AuthSession) {
    if (revoking) return;
    setRevoking(session.sessionId);
    setError(null);

    if (session.current) {
      // Signing this device out ends the session on the server and here.
      try {
        await logout();
      } finally {
        router.replace("/sign-in");
      }
      return;
    }

    try {
      await sessionService.revoke(session.sessionId);
      setSessions((prev) => prev.filter((s) => s.sessionId !== session.sessionId));
    } catch (e) {
      setError(describeApiError(e, "The session could not be revoked."));
    } finally {
      setRevoking(null);
    }
  }

  return (
    <div className="mx-auto max-w-3xl px-8 py-10">
      <h1 className="font-display text-[28px] font-medium tracking-[-0.01em] text-ink-950">
        Sessions and devices
      </h1>
      <p className="mt-1.5 text-[13.5px] text-ink-600">
        Devices currently signed in to your account.
      </p>

      <ApiErrorNotice error={error} className="mt-6" />

      <div className="mt-8 divide-y divide-line rounded-xl border border-line bg-white">
        {sessions.map((session) => (
          <div
            key={session.sessionId}
            className="flex items-center justify-between gap-4 px-5 py-4"
          >
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <span className="truncate text-[14px] font-semibold text-ink-950">
                  {session.deviceLabel || "Unnamed device"}
                </span>
                {session.current && (
                  <span className="flex-shrink-0 rounded-full bg-green-bg px-2 py-0.5 text-[10.5px] font-semibold text-green">
                    This device
                  </span>
                )}
              </div>
              <div className="mt-1 text-[12.5px] text-ink-500">
                {humanize(session.status)} · Signed in {formatWhen(session.createdAt)} ·{" "}
                {session.lastSeenAt ? `Last active ${formatWhen(session.lastSeenAt)}` : "No activity recorded"}
              </div>
            </div>

            <button
              type="button"
              onClick={() => handleRevoke(session)}
              disabled={revoking === session.sessionId}
              className="flex-shrink-0 text-[13px] font-medium text-red transition hover:text-red/80 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {revoking === session.sessionId
                ? "Revoking…"
                : session.current
                  ? "Sign out"
                  : "Revoke"}
            </button>
          </div>
        ))}

        {sessions.length === 0 && (
          <div className="px-5 py-10 text-center text-[13px] text-ink-500">
            {loading ? "Loading sessions…" : "No active sessions."}
          </div>
        )}
      </div>
    </div>
  );
}
