"use client";

import { useEffect } from "react";
import { AppErrorState } from "@/components/app-error-state";

type ErrorPageProps = {
  error: Error & { digest?: string };
  reset: () => void;
};

export default function ErrorPage({ error, reset }: ErrorPageProps) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <main className="min-h-screen bg-zinc-50 p-6">
      <AppErrorState
        message="The admin dashboard could not load this view. Please try again."
        actionLabel="Try again"
        onAction={reset}
      />
    </main>
  );
}
