"use client";

import { useEffect } from "react";
import { restoreSession } from "@/lib/browser-session";
import { useAuthStore } from "@/store/auth-store";

export function SessionBootstrap({ children }: { children: React.ReactNode }) {
  const ready = useAuthStore((state) => state._hasHydrated);
  useEffect(() => { void restoreSession(); }, []);
  if (!ready) return <div role="status" className="p-6">Restoring session…</div>;
  return children;
}
