import { Clock3, Download, Smartphone } from "lucide-react";
import { DRIVER_APPS, driverAppUrl } from "@/lib/driverApps";

const STEPS = [
  { title: "Download", text: "Tap your app's download button on your Android phone." },
  { title: "Allow the install", text: "If asked, allow installs from your browser. Android may warn you because the app isn't on Google Play yet." },
  { title: "Open and sign up", text: "Open the app, enter your phone number and complete your driver profile." },
];

export default function DriverApp() {
  return (
    <section
      id="driver-app"
      className="relative scroll-mt-20 overflow-hidden bg-midnight py-14 text-white lg:py-20"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_55%_70%_at_90%_10%,rgba(0,194,168,0.14),transparent_70%)]"
      />
      <svg
        aria-hidden
        viewBox="0 0 1440 300"
        preserveAspectRatio="none"
        fill="none"
        className="absolute right-0 top-10 hidden h-56 w-1/2 opacity-40 lg:block"
      >
        <path
          d="M-10 250 C 260 240 300 70 620 110 S 1080 280 1450 40"
          pathLength={1}
          stroke="#00C2A8"
          strokeWidth={2}
          vectorEffect="non-scaling-stroke"
          className="driver-route"
        />
      </svg>

      <div className="relative mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12">
        <div className="scroll-reveal max-w-2xl">
          <p className="text-[11px] font-semibold uppercase tracking-[0.24em] text-aurora">
            Driver Apps
          </p>
          <h2 className="mt-4 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight">
            Drive with Karryam.
            <br />
            <span className="text-aurora">Start from your phone.</span>
          </h2>
          <p className="mt-4 text-sm leading-relaxed text-white/70 sm:text-[15px]">
            Choose the app for the work you do and install it directly on your
            Android phone.
          </p>
        </div>

        <ul className="mt-9 grid gap-4 md:grid-cols-3 lg:gap-6">
          {DRIVER_APPS.map((app) => {
            const Icon = app.icon;
            const url = driverAppUrl(app);
            return (
              <li key={app.id} className="scroll-reveal">
                <article className="driver-card flex h-full flex-col rounded-2xl border border-white/15 bg-white/[0.06] p-5 backdrop-blur sm:p-6">
                  <span className="grid h-11 w-11 place-items-center rounded-xl border border-aurora/40 bg-aurora/10 text-aurora">
                    <Icon aria-hidden size={22} strokeWidth={1.75} />
                  </span>
                  <h3 className="mt-4 text-lg font-semibold">{app.name}</h3>
                  <p className="mt-1.5 flex-1 text-[13px] leading-relaxed text-white/70">
                    {app.audience}
                  </p>
                  {url ? (
                    <a
                      href={url}
                      download={app.file}
                      className="driver-download mt-5 inline-flex items-center justify-center gap-2 rounded-full bg-aurora px-5 py-3 text-[13px] font-semibold text-midnight hover:brightness-110 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora"
                    >
                      <Download aria-hidden size={15} strokeWidth={2.5} className="driver-arrow" />
                      Download APK
                      <span className="font-medium text-midnight/70">{app.size}</span>
                    </a>
                  ) : (
                    <span
                      aria-disabled="true"
                      className="mt-5 inline-flex items-center justify-center gap-2 rounded-full border border-white/15 px-5 py-3 text-[13px] font-semibold text-white/60"
                    >
                      <Clock3 aria-hidden size={15} strokeWidth={2} />
                      Coming soon
                    </span>
                  )}
                </article>
              </li>
            );
          })}
        </ul>

        <ol className="scroll-reveal mt-9 grid gap-5 border-t border-white/10 pt-7 md:grid-cols-3 lg:gap-6">
          {STEPS.map(({ title, text }, index) => (
            <li key={title} className="flex gap-3">
              <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-aurora/15 text-[12px] font-semibold text-aurora">
                {index + 1}
              </span>
              <span>
                <span className="block text-sm font-semibold">{title}</span>
                <span className="mt-1 block text-[13px] leading-relaxed text-white/65">{text}</span>
              </span>
            </li>
          ))}
        </ol>

        <p className="mt-7 flex items-start gap-2 text-[12px] leading-relaxed text-white/60">
          <Smartphone aria-hidden size={15} className="mt-px shrink-0 text-aurora" />
          Android only for now, with iPhone to follow. On a computer? Open this
          page on your Android phone to download.
        </p>
      </div>
    </section>
  );
}
