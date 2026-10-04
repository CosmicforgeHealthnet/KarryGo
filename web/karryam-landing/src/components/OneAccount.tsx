"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { Check } from "lucide-react";

const FEATURES = [
  "Unified activity timeline",
  "Real-time tracking",
  "Secure payments",
  "Personalised experience",
];

const vars = (o: Record<string, string | number>) => o as React.CSSProperties;

export default function OneAccount() {
  const sectionRef = useRef<HTMLElement>(null);
  const imgRef = useRef<HTMLImageElement>(null);
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
      { threshold: 0.15 }
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
    const img = imgRef.current;
    if (!img || !canTilt()) return;
    const r = e.currentTarget.getBoundingClientRect();
    const px = (e.clientX - r.left) / r.width - 0.5;
    const py = (e.clientY - r.top) / r.height - 0.5;
    img.style.setProperty("--ry", `${(px * 10).toFixed(2)}deg`);
    img.style.setProperty("--rx", `${(-py * 8).toFixed(2)}deg`);
  };

  const onLeave = () => {
    const img = imgRef.current;
    if (!img) return;
    img.style.setProperty("--ry", "0deg");
    img.style.setProperty("--rx", "0deg");
  };

  return (
    <section
      ref={sectionRef}
      data-inview={inView}
      id="account"
      className="relative bg-midnight text-white overflow-hidden py-14 sm:py-20"
    >
      <style>{`
        @keyframes acc-rise {
          from { opacity: 0; translate: 0 40px; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes acc-slide {
          from { opacity: 0; translate: -28px 0; }
          to { opacity: 1; translate: 0 0; }
        }
        @keyframes acc-pop {
          0% { opacity: 0; scale: 0.4; }
          70% { opacity: 1; scale: 1.18; }
          100% { opacity: 1; scale: 1; }
        }
        @keyframes acc-tick { to { stroke-dashoffset: 0; } }
        @keyframes acc-img-in {
          from { opacity: 0; scale: 0.93; }
          to { opacity: 1; scale: 1; }
        }
        @keyframes acc-float {
          0%, 100% { translate: 0 0; }
          50% { translate: 0 -12px; }
        }
        @keyframes acc-float-sm {
          0%, 100% { translate: 0 0; }
          50% { translate: 0 -6px; }
        }
        @keyframes acc-breathe {
          0%, 100% { transform: scale(1); opacity: 0.7; }
          50% { transform: scale(1.18); opacity: 1; }
        }
        @keyframes acc-drift {
          from { background-position: 0 0; }
          to { background-position: 28px 28px; }
        }
        @keyframes acc-shimmer {
          from { background-position: 0% 50%; }
          to { background-position: 200% 50%; }
        }

        .acc-rise, .acc-slide, .acc-pop, .acc-img { opacity: 0; }
        .acc-tick path { stroke-dasharray: 30; stroke-dashoffset: 30; }

        [data-inview="true"] .acc-rise {
          animation: acc-rise 0.7s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .acc-slide {
          animation: acc-slide 0.6s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .acc-pop {
          animation: acc-pop 0.5s cubic-bezier(0.34, 1.56, 0.64, 1) forwards;
          animation-delay: calc(var(--d, 0s) + 0.1s);
        }
        [data-inview="true"] .acc-tick path {
          animation: acc-tick 0.45s ease-out forwards;
          animation-delay: calc(var(--d, 0s) + 0.3s);
        }
        [data-inview="true"] .acc-img {
          animation:
            acc-img-in 1s cubic-bezier(0.22, 1, 0.36, 1) 0.2s forwards,
            acc-float-sm 7s ease-in-out 1.4s infinite;
        }
        @media (min-width: 1024px) {
          [data-inview="true"] .acc-img {
            animation:
              acc-img-in 1s cubic-bezier(0.22, 1, 0.36, 1) 0.2s forwards,
              acc-float 7s ease-in-out 1.4s infinite;
          }
        }

        /* tilt lives on transform, float/entrance live on translate/scale,
           so they never fight each other */
        .acc-img {
          transform: perspective(1200px) rotateX(var(--rx, 0deg)) rotateY(var(--ry, 0deg));
          transition: transform 0.25s ease-out;
          will-change: transform;
        }

        .acc-glow { animation: acc-breathe 6s ease-in-out infinite; }
        .acc-dots { animation: acc-drift 40s linear infinite; }

        .acc-shimmer {
          background-image: linear-gradient(90deg, #00C2A8, #7CF5E4, #00C2A8);
          background-size: 200% 100%;
          -webkit-background-clip: text;
          background-clip: text;
          color: transparent;
          animation: acc-shimmer 4s linear infinite;
        }

        @media (prefers-reduced-motion: reduce) {
          .acc-rise, .acc-slide, .acc-pop, .acc-img { opacity: 1; animation: none !important; }
          .acc-tick path { stroke-dashoffset: 0; animation: none !important; }
          .acc-glow, .acc-dots { animation: none; }
          .acc-shimmer { animation: none; }
          .acc-img { transition: none; }
        }
      `}</style>

      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_90%_50%_at_50%_75%,rgba(0,194,168,0.16),transparent_70%)] lg:bg-[radial-gradient(ellipse_55%_60%_at_75%_50%,rgba(0,194,168,0.16),transparent_70%)]"
      />
      <div
        aria-hidden
        className="absolute inset-0 bg-[radial-gradient(ellipse_70%_40%_at_0%_100%,rgba(30,64,175,0.28),transparent_70%)] lg:bg-[radial-gradient(ellipse_45%_45%_at_0%_100%,rgba(30,64,175,0.28),transparent_70%)]"
      />
      <div
        aria-hidden
        className="acc-dots absolute inset-0 opacity-[0.05] [background-image:radial-gradient(#fff_1px,transparent_1px)] [background-size:28px_28px] [mask-image:linear-gradient(to_bottom,transparent,black_30%,black_70%,transparent)]"
      />

      <div className="relative mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 grid lg:grid-cols-12 gap-8 sm:gap-12 lg:gap-8 items-center">
        {/* Left: copy */}
        <div className="lg:col-span-5">
          <p
            className="acc-rise text-aurora text-[10px] sm:text-[11px] tracking-[0.24em] font-semibold uppercase"
            style={vars({ "--d": "0s" })}
          >
            One Account. Total Control.
          </p>

          <h2
            className="acc-rise mt-4 sm:mt-5 text-[clamp(1.75rem,5vw,3rem)] font-bold leading-[1.1] tracking-tight"
            style={vars({ "--d": "0.1s" })}
          >
            Track everything.
            <br />
            <span className="acc-shimmer text-aurora">In one place.</span>
          </h2>

          <p
            className="acc-rise mt-4 sm:mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-white/70"
            style={vars({ "--d": "0.2s" })}
          >
            Your activities, orders and deliveries — all in one unified account.
            Stay in control with real-time updates and complete visibility.
          </p>

          <ul className="mt-6 sm:mt-8 flex flex-col gap-3.5 sm:gap-4">
            {FEATURES.map((item, i) => (
              <li
                key={item}
                className="acc-slide group flex items-center gap-3 transition-transform duration-300 hover:translate-x-1.5"
                style={vars({ "--d": `${0.4 + i * 0.12}s` })}
              >
                <span
                  className="acc-pop grid h-6 w-6 shrink-0 place-items-center rounded-full border border-aurora/50 bg-aurora/10 text-aurora transition-colors duration-300 group-hover:bg-aurora group-hover:text-midnight"
                  style={vars({ "--d": `${0.4 + i * 0.12}s` })}
                >
                  <span
                    className="acc-tick grid place-items-center"
                    style={vars({ "--d": `${0.4 + i * 0.12}s` })}
                  >
                    <Check size={13} strokeWidth={3} />
                  </span>
                </span>
                <span className="text-sm sm:text-[15px] text-white/85 transition-colors duration-300 group-hover:text-white">
                  {item}
                </span>
              </li>
            ))}
          </ul>
        </div>

        {/* Right: image. Bleeds slightly past the padding on mobile so it's bigger */}
        <div
          className="lg:col-span-7 relative -mx-3 sm:mx-0"
          onMouseMove={onMove}
          onMouseLeave={onLeave}
        >
          <div
            aria-hidden
            className="absolute left-1/2 top-1/2 h-[65%] w-[65%] -translate-x-1/2 -translate-y-1/2"
          >
            <div className="acc-glow h-full w-full rounded-full bg-aurora/10 blur-2xl sm:blur-3xl" />
          </div>
          <Image
            ref={imgRef}
            src="/images/one-account.png"
            alt="Karryam Activity screen showing ride, parcel, food and truck orders in one timeline"
            width={1672}
            height={941}
            sizes="(min-width: 1024px) 58vw, 100vw"
            className="acc-img relative mx-auto w-full max-w-[640px] lg:max-w-none h-auto select-none mix-blend-lighten [mask-image:radial-gradient(ellipse_72%_72%_at_50%_50%,#000_55%,transparent_100%)] [-webkit-mask-image:radial-gradient(ellipse_72%_72%_at_50%_50%,#000_55%,transparent_100%)]"
          />
        </div>
      </div>
    </section>
  );
}