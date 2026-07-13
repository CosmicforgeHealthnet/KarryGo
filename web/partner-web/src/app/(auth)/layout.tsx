import Image from "next/image";

export default function AuthLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <div className="flex min-h-screen">
      {/* Brand panel */}
      <div className="relative hidden w-[42%] shrink-0 overflow-hidden bg-brand-700 lg:block">
        <svg
          className="absolute inset-0 h-full w-full opacity-40"
          viewBox="0 0 500 800"
          fill="none"
          xmlns="http://www.w3.org/2000/svg"
        >
          <path
            d="M -20 650 C 100 620, 140 500, 90 420 C 40 340, 130 260, 250 280 C 370 300, 420 200, 380 100 C 350 20, 420 -30, 520 -10"
            stroke="white"
            strokeWidth="2"
            strokeDasharray="2 10"
            strokeLinecap="round"
          />
          <circle cx="-20" cy="650" r="6" fill="white" />
          <circle cx="90" cy="420" r="6" fill="white" />
          <circle cx="250" cy="280" r="6" fill="white" />
          <circle cx="380" cy="100" r="10" fill="white" />
        </svg>

        <div className="relative flex h-full flex-col justify-between p-12">
          <div className="flex items-center gap-2 text-white">
            <Image
              src="/images/logo.png"
              alt="KarryGo"
              width={36}
              height={36}
              className="h-9 w-9 rounded-lg object-contain"
            />
            {/* <span className="text-sm font-semibold tracking-wide">
              COSMICFORGE HEALTHNET
            </span> */}
          </div>

          <div className="max-w-sm text-white">
            <p className="text-2xl font-semibold leading-snug">
              Every delivery, tracked from pickup to doorstep.
            </p>
            <p className="mt-3 text-sm text-brand-100">
              Partner with KarryGo to move packages across the network
              reliably, with live status your customers can see too.
            </p>
          </div>
        </div>
      </div>

      {/* Form panel */}
      <div className="flex flex-1 items-center justify-center px-6 py-16">
        <div className="w-full max-w-sm">{children}</div>
      </div>
    </div>
  );
}