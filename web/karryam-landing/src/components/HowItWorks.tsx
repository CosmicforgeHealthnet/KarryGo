"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  ListChecks,
  MapPin,
  MousePointerClick,
  Workflow,
} from "lucide-react";

type Step = { title: string; text: string; icon: LucideIcon };

const STEPS: Step[] = [
  {
    title: "Choose",
    text: "Select your service and set your location.",
    icon: MousePointerClick,
  },
  {
    title: "Review",
    text: "See options, prices and details.",
    icon: ListChecks,
  },
  {
    title: "Karryam coordinates",
    text: "We match you with the best available provider.",
    icon: Workflow,
  },
  {
    title: "Track / complete",
    text: "Follow in real time and enjoy the service.",
    icon: MapPin,
  },
];

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function HowItWorks() {
  const sectionRef = useRef<HTMLElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);
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

  const canTilt = () =>
    typeof window !== "undefined" &&
    window.matchMedia(
      "(hover: hover) and (min-width: 1024px) and (prefers-reduced-motion: no-preference)"
    ).matches;

  const onMove = (e: React.MouseEvent<HTMLDivElement>) => {
    const card = cardRef.current;
    if (!card || !canTilt()) return;
    const r = e.currentTarget.getBoundingClientRect();
    const px = (e.clientX - r.left) / r.width - 0.5;
    const py = (e.clientY - r.top) / r.height - 0.5;
    card.style.setProperty("--ry", `${(px * 9).toFixed(2)}deg`);
    card.style.setProperty("--rx", `${(-py * 7).toFixed(2)}deg`);
  };

  const onLeave = () => {
    const card = cardRef.current;
    if (!card) return;
    card.style.setProperty("--ry", "0deg");
    card.style.setProperty("--rx", "0deg");
  };

  return (
    <section
      ref={sectionRef}
      data-inview={inView}
      id="how-it-works"
      className="relative bg-gradient-to-b from-white to-[#F3F8FB] text-midnight py-12 lg:py-20 overflow-hidden"
    >
      <style>{`
        @keyframes hiw-rise {
          from { opacity: 0; translate: 0 36px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes hiw-left {
          from { opacity: 0; translate: -56px 0; scale: 0.95; }
          to { opacity: 1; translate: 0 0; scale: 1; }
        }
        @keyframes hiw-sweep {
          from { translate: -120% 0; }
          to { translate: 420% 0; }
        }
        @keyframes hiw-pop {
          0% { opacity: 0; scale: 0.4; }
          70% { opacity: 1; scale: 1.15; }
          100% { opacity: 1; scale: 1; }
        }
        @keyframes hiw-line {
          from { transform: scaleX(0); }
          to { transform: scaleX(1); }
        }
        @keyframes hiw-active {
          0%, 20% {
            background-color: #00C2A8;
            color: #0B1F44;
            scale: 1.12;
            box-shadow: 0 0 0 6px rgba(0,194,168,0.18), 0 10px 28px -6px rgba(0,194,168,0.7);
          }
          28%, 100% {
            background-color: #ffffff;
            color: #00A28C;
            scale: 1;
            box-shadow: 0 8px 24px -10px rgba(0,194,168,0.5);
          }
        }

        .hiw-left, .hiw-rise, .hiw-pop { opacity: 0; }
        .hiw-line { transform: scaleX(0); transform-origin: left; }

        [data-inview="true"] .hiw-left {
          animation: hiw-left 0.9s cubic-bezier(0.22, 1, 0.36, 1) forwards;
        }
        [data-inview="true"] .hiw-sweep {
          animation: hiw-sweep 1.4s ease-in-out 0.9s 1 forwards;
        }
        [data-inview="true"] .hiw-rise {
          animation: hiw-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .hiw-pop {
          animation: hiw-pop 0.55s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .hiw-line {
          animation: hiw-line 1.2s cubic-bezier(0.22, 1, 0.36, 1) 0.5s forwards;
        }
        [data-inview="true"] .hiw-active {
          animation: hiw-active 8s ease-in-out infinite;
          animation-delay: calc(1.6s + var(--i, 0) * 2s);
        }

        /* tilt on transform; entrance uses translate/scale so they never clash */
        .hiw-card {
          transform: perspective(1200px) rotateX(var(--rx, 0deg)) rotateY(var(--ry, 0deg));
          transition: transform 0.25s ease-out;
          will-change: transform;
        }

        @media (prefers-reduced-motion: reduce) {
          .hiw-left, .hiw-rise, .hiw-pop { opacity: 1; animation: none !important; }
          .hiw-line { transform: none; animation: none !important; }
          .hiw-sweep { display: none; }
          .hiw-active { animation: none !important; }
          .hiw-card { transition: none; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-6 lg:px-12 grid lg:grid-cols-12 gap-12 lg:gap-12 items-center">
        <div
          className="lg:col-span-5"
          onMouseMove={onMove}
          onMouseLeave={onLeave}
        >
          <div ref={cardRef} className="hiw-card">
            <div className="hiw-left">
              <div className="relative overflow-hidden rounded-3xl shadow-[0_24px_60px_-20px_rgba(11,31,68,0.35)] ring-1 ring-midnight/5">
                <Image
                  src="/images/how-it-works.png"
                  alt="Customer using the Karryam app with a live order status and rewards offer"
                  width={1200}
                  height={1000}
                  sizes="(min-width: 1024px) 40vw, 100vw"
                  className="w-full h-auto select-none"
                />
                {/* one-time light sweep */}
                <span
                  aria-hidden
                  className="hiw-sweep pointer-events-none absolute inset-y-0 -left-1/3 w-1/4 -skew-x-12 bg-gradient-to-r from-transparent via-white/40 to-transparent"
                  style={{ translate: "-120% 0" }}
                />
              </div>
            </div>
          </div>
        </div>

        <div className="lg:col-span-7">
          <p
            className="hiw-rise text-[#00A28C] text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            Simple. Fast. Powerful.
          </p>

          <h2
            className="hiw-rise mt-5 text-[clamp(1.9rem,3vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            How Karryam Works
          </h2>

          <ol className="relative mt-10 grid gap-8 sm:grid-cols-2 lg:grid-cols-4 lg:gap-6">
            {/* connecting line */}
            <span
              aria-hidden
              className="pointer-events-none absolute left-6 right-6 top-6 hidden lg:block"
            >
              <span className="hiw-line block w-full border-t border-dashed border-aurora/40" />
            </span>

            {STEPS.map(({ title, text, icon: Icon }, i) => (
              <li key={title} className="group relative">
                <span
                  className="hiw-pop relative block h-12 w-12 transition-transform duration-300 group-hover:-translate-y-1.5"
                  style={vars({ "--d": `${0.7 + i * 0.18}s` })}
                >
                  <span
                    className="hiw-active grid h-full w-full place-items-center rounded-full border border-aurora/40 bg-white text-[#00A28C] shadow-[0_8px_24px_-10px_rgba(0,194,168,0.5)]"
                    style={vars({ "--i": i })}
                  >
                    <Icon size={20} strokeWidth={1.75} />
                  </span>
                </span>

                <div
                  className="hiw-rise"
                  style={vars({ "--d": `${0.85 + i * 0.18}s` })}
                >
                  <h3 className="mt-4 text-sm font-semibold leading-snug">
                    {i + 1}. {title}
                  </h3>
                  <p className="mt-2 text-[12px] leading-relaxed text-midnight/60">
                    {text}
                  </p>
                </div>
              </li>
            ))}
          </ol>

          <a
            href="/waitlist"
            className="hiw-rise group mt-10 inline-flex items-center gap-2 rounded-full border border-aurora px-6 py-3 text-[13px] font-semibold text-[#00A28C] hover:bg-aurora hover:text-midnight transition-colors"
            style={vars({ "--d": "1.7s" })}
          >
            Get started
            <ArrowRight
              size={14}
              strokeWidth={2.5}
              className="transition-transform duration-300 group-hover:translate-x-1"
            />
          </a>
        </div>
      </div>
    </section>
  );
}