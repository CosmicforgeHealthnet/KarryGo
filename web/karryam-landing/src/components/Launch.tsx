import Image from "next/image";
import { ArrowRight, MapPin } from "lucide-react";

const CITIES = ["Lagos", "Abuja", "Port Harcourt", "and more..."];

export default function Launch() {
  return (
    <section
      id="launch"
      className="relative scroll-mt-20 overflow-hidden bg-midnight text-white py-16 lg:py-16"
    >
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_55%_70%_at_70%_50%,rgba(0,194,168,0.18),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_45%_45%_at_0%_100%,rgba(30,64,175,0.28),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 opacity-[0.05] [background-image:radial-gradient(#fff_1px,transparent_1px)] [background-size:28px_28px] [mask-image:linear-gradient(to_bottom,transparent,black_30%,black_70%,transparent)]"
      />

      <div className="relative mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-8 lg:gap-4 items-center">
        <div className="lg:col-span-4">
          <p className="text-aurora text-[11px] tracking-[0.24em] font-semibold uppercase">
            Launching Soon
          </p>

          <h2 className="mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight">
            Nigeria,
            <br />
            <span className="text-aurora">get ready.</span>
          </h2>

          <p className="mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-white/70">
            Karryam is launching soon in selected Nigerian markets. Be among the
            first to move, send, sell and grow with us.
          </p>

          <a
            href="/waitlist"
            className="mt-8 inline-flex items-center gap-2 rounded-full bg-aurora px-6 py-3 text-[13px] font-semibold text-midnight shadow-[0_0_24px_-6px_rgba(0,194,168,0.7)] hover:brightness-110 transition"
          >
            Join the waitlist
            <ArrowRight size={14} strokeWidth={2.5} />
          </a>
        </div>

        <div className="lg:col-span-8 relative">
          <div
            aria-hidden
            className="absolute left-1/2 top-1/2 h-[70%] w-[70%] -translate-x-1/2 -translate-y-1/2 rounded-full bg-aurora/10 blur-3xl"
          />

          <Image
            src="/images/nigeria-map.png"
            alt="Glowing map of Nigeria with Karryam launch cities"
            width={1200}
            height={1000}
            sizes="(min-width: 1024px) 60vw, 100vw"
            className="relative h-auto w-full select-none lg:-ml-6"
          />

          <ul className="relative mt-6 flex flex-col gap-2.5 sm:absolute sm:right-0 sm:top-1/2 sm:mt-0 sm:w-[200px] sm:-translate-y-1/2">
            {CITIES.map((city, i) => {
              const last = i === CITIES.length - 1;
              return (
                <li
                  key={city}
                  className="flex items-center gap-3 rounded-xl border border-white/10 bg-midnight/70 px-4 py-2.5 backdrop-blur"
                >
                  {last ? (
                    <span className="h-[14px] w-[14px]" aria-hidden />
                  ) : (
                    <MapPin size={14} className="text-aurora" strokeWidth={2} />
                  )}
                  <span
                    className={`text-[13px] ${
                      last ? "text-white/60" : "font-semibold text-white"
                    }`}
                  >
                    {city}
                  </span>
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </section>
  );
}