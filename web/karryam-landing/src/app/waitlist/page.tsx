import Link from "next/link";

export default function WaitlistPage() {
  return (
    <main className="min-h-[100svh] bg-midnight text-white flex items-center justify-center px-5">
      <div className="max-w-md text-center">
        <p className="text-aurora text-[11px] tracking-[0.24em] font-semibold uppercase">
          Karryam
        </p>
        <h1 className="mt-4 text-3xl sm:text-4xl font-bold tracking-tight">
          The waitlist opens soon.
        </h1>
        <p className="mt-4 text-sm sm:text-[15px] leading-relaxed text-white/70">
          We are putting the finishing touches on sign-ups. Check back shortly.
        </p>
        <Link
          href="/"
          className="mt-8 inline-flex items-center rounded-full bg-aurora px-6 py-3 text-[13px] font-semibold text-midnight hover:brightness-110 transition"
        >
          Back to home
        </Link>
      </div>
    </main>
  );
}