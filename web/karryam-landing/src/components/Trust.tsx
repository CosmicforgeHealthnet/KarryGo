"use client";

import { useEffect, useRef, useState } from "react";
import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  BadgeCheck,
  Headset,
  MapPinned,
  ShieldCheck,
  Wallet,
} from "lucide-react";

type Item = { title: string; sub: string; icon: LucideIcon };

const ITEMS: Item[] = [
  { title: "Verified", sub: "Providers & Merchants", icon: BadgeCheck },
  { title: "Real-Time Tracking", sub: "& Updates", icon: MapPinned },
  { title: "Secure Handover", sub: "& Delivery", icon: ShieldCheck },
  { title: "Trusted Payments", sub: "& Refunds", icon: Wallet },
  { title: "24/7 Support", sub: "& Notifications", icon: Headset },
];

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function Trust() {
  const sectionRef = useRef<HTMLElement>(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) return;
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setInView(true);
          io.disconnect();
        }
      },
      { threshold: 0.2 }
    );
    io.observe(el);
    return () => io.disconnect();
  }, []);

  return (
    <section
      ref={sectionRef}
      data-inview={inView}
      id="about"
      className="relative scroll-mt-20 overflow-hidden bg-gradient-to-b from-[#EAF4F8] to-[#F3F8FB] text-midnight py-16 lg:py-20"
    >
      <style>{`
        @keyframes trs-rise {
          from { opacity: 0; translate: 0 32px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes trs-pop {
          0% { opacity: 0; scale: 0.4; }
          70% { opacity: 1; scale: 1.15; }
          100% { opacity: 1; scale: 1; }
        }
        @keyframes trs-shimmer {
          from { background-position: 0% 50%; }
          to { background-position: 200% 50%; }
        }

        .trs-shimmer {
          background-image: linear-gradient(90deg, #0B1F44 0%, #0B1F44 35%, #00A28C 50%, #0B1F44 65%, #0B1F44 100%);
          background-size: 200% 100%;
          -webkit-background-clip: text;
          background-clip: text;
          color: transparent;
          animation: trs-shimmer 7s linear infinite;
        }

        .trs-rise, .trs-pop { opacity: 0; }

        [data-inview="true"] .trs-rise {
          animation: trs-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .trs-pop {
          animation: trs-pop 0.55s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
          animation-delay: var(--d, 0s);
        }

        @media (prefers-reduced-motion: reduce) {
          .trs-rise, .trs-pop { opacity: 1; animation: none !important; }
          .trs-shimmer { animation: none; background-image: none; color: inherit; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-10 lg:gap-8 items-center">
        <div className="lg:col-span-4">
          <p
            className="trs-rise text-[#00A28C] text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            Built on Trust
          </p>

          <h2
            className="trs-rise mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            Your safety.
            <br />
            <span className="trs-shimmer">Our priority.</span>
          </h2>

          <p
            className="trs-rise mt-5 max-w-sm text-sm sm:text-[15px] leading-relaxed text-midnight/70"
            style={vars({ "--d": "0.2s" })}
          >
            We use advanced technology and verified networks to keep you, your
            goods and your business safe.
          </p>

          <a
            href="#faq"
            className="trs-rise group mt-8 inline-flex items-center gap-2 rounded-full border border-aurora px-6 py-3 text-[13px] font-semibold text-[#00A28C] hover:bg-aurora hover:text-midnight transition-colors"
            style={vars({ "--d": "0.3s" })}
          >
            Learn more
            <ArrowRight
              size={14}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>

        <ul className="lg:col-span-8 grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-6 lg:gap-4">
          {ITEMS.map(({ title, sub, icon: Icon }, i) => (
            <li
              key={title}
              className="group flex flex-col items-center text-center transition-transform duration-300 hover:-translate-y-1.5"
            >
              <span
                className="trs-pop grid h-14 w-14 place-items-center rounded-full border border-aurora/40 bg-white text-[#00A28C] shadow-[0_10px_28px_-12px_rgba(0,194,168,0.55)] transition-all duration-300 group-hover:bg-aurora group-hover:text-midnight group-hover:shadow-[0_14px_34px_-10px_rgba(0,194,168,0.85)]"
                style={vars({ "--d": `${0.4 + i * 0.12}s` })}
              >
                <Icon size={22} strokeWidth={1.75} />
              </span>

              <div
                className="trs-rise"
                style={vars({ "--d": `${0.55 + i * 0.12}s` })}
              >
                <p className="mt-4 text-[13px] font-semibold leading-snug">
                  {title}
                </p>
                <p className="mt-0.5 text-[12px] leading-snug text-midnight/60">
                  {sub}
                </p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}