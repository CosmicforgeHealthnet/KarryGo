export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="relative min-h-screen bg-white">
      <div className="absolute left-8 top-8">
        <span className="font-display text-[16px] font-semibold tracking-tight text-ink-950">
          KarryGo
        </span>
        <span className="ml-2 text-[11px] font-semibold tracking-[0.2em] text-ink-500">
          FLEET OS
        </span>
      </div>

      <div className="flex min-h-screen items-center justify-center px-6">
        <div className="fade-in-up w-full max-w-[380px]">{children}</div>
      </div>
    </div>
  );
}