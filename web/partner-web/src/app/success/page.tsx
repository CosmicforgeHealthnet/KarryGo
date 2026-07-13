"use client";

import { useRouter } from "next/navigation";
import { Button } from "@/components/ui/button";

export default function SuccessPage() {
  const router = useRouter();

  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-surface px-6 py-16">
      <div className="w-full max-w-sm text-center">
        <SuccessIllustration />

        <h1 className="mt-8 text-2xl font-bold text-ink-900">
          You have created a Partner Account
        </h1>
        <p className="mt-3 text-sm text-ink-500">
          Your partner account has been created. Go to dashboard to start
          sending packages.
        </p>

        <p className="mt-6 text-sm font-medium text-brand-500">
          A confirmation link has been sent to your email.
          <br />
          Please verify to secure your account.
        </p>

        <div className="mt-10">
          <Button onClick={() => router.push("/dashboard")}>
            Go to Dashboard
          </Button>
        </div>
      </div>
    </div>
  );
}

function SuccessIllustration() {
  return (
    <svg
      width="220"
      height="160"
      viewBox="0 0 220 160"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className="mx-auto"
      aria-hidden
    >
      <circle cx="110" cy="80" r="72" fill="#eef8f1" />
      <circle cx="110" cy="34" r="26" fill="#2e9e4f" />
      <path
        d="M98 34l8 8 16-16"
        stroke="white"
        strokeWidth="4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M40 120c8-20 30-32 70-32s62 12 70 32"
        stroke="#4cb96e"
        strokeWidth="3"
        strokeLinecap="round"
        strokeDasharray="1 9"
      />
      <circle cx="46" cy="60" r="4" fill="#a9dcb8" />
      <circle cx="176" cy="52" r="5" fill="#4cb96e" />
      <circle cx="184" cy="96" r="3" fill="#a9dcb8" />
    </svg>
  );
}