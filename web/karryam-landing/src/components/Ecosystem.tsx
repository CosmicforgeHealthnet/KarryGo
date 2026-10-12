"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import type { LucideIcon } from "lucide-react";
import {
  ArrowRight,
  Building2,
  Car,
  Code2,
  Package,
  ShoppingBag,
  Store,
  Truck,
  User,
  UtensilsCrossed,
} from "lucide-react";

type Node = { label: string; icon: LucideIcon; x: number };

// x = horizontal position in % of the diagram width
const SERVICES: Node[] = [
  { label: "Ride", icon: Car, x: 10 },
  { label: "Send", icon: Package, x: 30 },
  { label: "Truck", icon: Truck, x: 50 },
  { label: "Food", icon: UtensilsCrossed, x: 70 },
  { label: "Shop", icon: ShoppingBag, x: 90 },
];

const PARTNERS: Node[] = [
  { label: "Provider", icon: User, x: 12.5 },
  { label: "Vendor", icon: Store, x: 37.5 },
  { label: "Fleet", icon: Building2, x: 62.5 },
  { label: "API Partner", icon: Code2, x: 87.5 },
];

const W = 1000;

/**
 * Two geometries, same component:
 * - desktop: wide 1000 x 520 canvas
 * - mobile: taller 1000 x 1000 canvas so the center logo has real
 *   breathing room between the top and bottom node rows
 * Both SVGs are always rendered; CSS (sm:) decides which one shows.
 */
const GEO = {
  desktop: { H: 520, cy: 260, topEnd: 130, bottomEnd: 390 },
  mobile: { H: 1000, cy: 500, topEnd: 215, bottomEnd: 785 },
} as const;

type Geo = (typeof GEO)[keyof typeof GEO];

const delay = (s: number) => ({ "--d": `${s}s` } as React.CSSProperties);

function Lines({ geo, className }: { geo: Geo; className: string }) {
  const { H, cy, topEnd, bottomEnd } = geo;
  const curve = Math.round((cy - topEnd) * 0.45);

  const topPath = (x: number) =>
    `M500 ${cy} C 500 ${cy - curve}, ${x} ${topEnd + curve}, ${x} ${topEnd}`;
  const bottomPath = (x: number) =>
    `M500 ${cy} C 500 ${cy + curve}, ${x} ${bottomEnd - curve}, ${x} ${bottomEnd}`;

  const px = (n: Node) => (n.x / 100) * W;

  return (
    <svg
      viewBox={`0 0 ${W} ${H}`}
      className={`absolute inset-0 h-full w-full ${className}`}
      fill="none"
      aria-hidden
    >
      {/* base lines (draw in) */}
      <g
        stroke="var(--color-aurora)"
        strokeWidth="1.5"
        strokeOpacity="0.55"
        strokeLinecap="round"
        vectorEffect="non-scaling-stroke"
      >
        {SERVICES.map((n, i) => (
          <path
            key={n.label}
            className="eco-line"
            pathLength={1}
            style={delay(i * 0.08)}
            d={topPath(px(n))}
            vectorEffect="non-scaling-stroke"
          />
        ))}
        {PARTNERS.map((n, i) => (
          <path
            key={n.label}
            className="eco-line"
            pathLength={1}
            style={delay(0.2 + i * 0.08)}
            d={bottomPath(px(n))}
            vectorEffect="non-scaling-stroke"
          />
        ))}
      </g>

      {/* travelling pulses */}
      <g stroke="var(--color-aurora)" strokeWidth="3" strokeLinecap="round">
        {SERVICES.map((n, i) => (
          <path
            key={n.label}
            className="eco-flow"
            pathLength={1}
            style={delay(i * 0.35)}
            d={topPath(px(n))}
            vectorEffect="non-scaling-stroke"
          />
        ))}
        {PARTNERS.map((n, i) => (
          <path
            key={n.label}
            className="eco-flow"
            pathLength={1}
            style={delay(0.15 + i * 0.35)}
            d={bottomPath(px(n))}
            vectorEffect="non-scaling-stroke"
          />
        ))}
      </g>

      {/* end-point dots */}
      <g fill="var(--color-aurora)">
        {SERVICES.map((n, i) => (
          <circle
            key={n.label}
            className="eco-dot"
            style={delay(i * 0.08)}
            cx={px(n)}
            cy={topEnd}
            r={H > 600 ? 9 : 3.5}
          />
        ))}
        {PARTNERS.map((n, i) => (
          <circle
            key={n.label}
            className="eco-dot"
            style={delay(0.2 + i * 0.08)}
            cx={px(n)}
            cy={bottomEnd}
            r={H > 600 ? 9 : 3.5}
          />
        ))}
      </g>
    </svg>
  );
}

function NodeItem({
  node,
  position,
  index,
}: {
  node: Node;
  position: "top" | "bottom";
  index: number;
}) {
  const Icon = node.icon;
  return (
    <div
      className={`absolute -translate-x-1/2 ${
        position === "top" ? "top-0" : "bottom-0"
      }`}
      style={{ left: `${node.x}%` }}
    >
      <div
        className="eco-pop flex flex-col items-center gap-1.5 sm:gap-2"
        style={delay(0.9 + index * 0.1)}
      >
        <span
          className="eco-float grid h-10 w-10 sm:h-14 sm:w-14 place-items-center rounded-full border border-aurora/40 bg-midnight text-aurora shadow-[0_8px_24px_-6px_rgba(0,194,168,0.18)] transition-transform duration-300 hover:scale-110"
          style={delay(index * 0.4)}
        >
          <Icon className="h-4 w-4 sm:h-6 sm:w-6" strokeWidth={1.75} />
        </span>
        <span className="text-[10px] sm:text-xs font-semibold text-midnight whitespace-nowrap leading-none">
          {node.label}
        </span>
      </div>
    </div>
  );
}

export default function Ecosystem() {
  const ref = useRef<HTMLDivElement>(null);
  const [inView, setInView] = useState(false);

  useEffect(() => {
    const el = ref.current;
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
      id="ecosystem"
      className="relative bg-white text-midnight py-14 sm:py-20 overflow-hidden"
    >
      <style>{`
        @keyframes eco-halo { 0%, 100% { opacity: 0.3; transform: scale(1); } 50% { opacity: 0.6; transform: scale(1.06); } }
        @keyframes eco-draw { to { stroke-dashoffset: 0; } }
        @keyframes eco-flow { from { stroke-dashoffset: 0; } to { stroke-dashoffset: -1; } }
        @keyframes eco-show { to { opacity: 1; } }
        @keyframes eco-pop {
          from { opacity: 0; transform: translateY(10px) scale(0.85); }
          to { opacity: 1; transform: translateY(0) scale(1); }
        }
        @keyframes eco-float {
          0%, 100% { transform: translateY(0); }
          50% { transform: translateY(-4px); }
        }

        .eco-line { stroke-dasharray: 1; stroke-dashoffset: 1; }
        .eco-flow { stroke-dasharray: 0.07 0.93; stroke-dashoffset: 0; opacity: 0; }
        .eco-dot { opacity: 0; }
        .eco-pop { opacity: 0; }

        [data-inview="true"] .eco-line {
          animation: eco-draw 1.1s ease-out forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .eco-flow {
          animation:
            eco-show 0.4s ease forwards,
            eco-flow 3s linear infinite;
          animation-delay: calc(1.4s + var(--d, 0s)), calc(1.4s + var(--d, 0s));
        }
        [data-inview="true"] .eco-dot {
          animation: eco-show 0.4s ease forwards;
          animation-delay: calc(0.9s + var(--d, 0s));
        }
        [data-inview="true"] .eco-pop {
          animation: eco-pop 0.6s cubic-bezier(0.22, 1, 0.36, 1) forwards;
          animation-delay: var(--d, 0s);
        }
        [data-inview="true"] .eco-float {
          animation: eco-float 5s ease-in-out infinite;
          animation-delay: calc(1.6s + var(--d, 0s));
        }

        @media (prefers-reduced-motion: reduce) {
          .eco-line { stroke-dashoffset: 0; animation: none !important; }
          .eco-flow { display: none; }
          .eco-dot, .eco-pop { opacity: 1; animation: none !important; }
          .eco-float { animation: none !important; }
        }
      `}</style>

      <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 grid lg:grid-cols-12 gap-10 sm:gap-12 lg:gap-8 items-center">
        {/* Left: copy */}
        <div className="lg:col-span-5">
          <p className="text-aurora-ink text-[10px] sm:text-[11px] tracking-[0.24em] font-semibold uppercase">
            The Karryam Ecosystem
          </p>

          <h2 className="mt-4 sm:mt-5 text-[clamp(1.75rem,5vw,3rem)] font-bold leading-[1.1] tracking-tight">
            Everything that moves,{" "}
            <span className="text-aurora-ink">connected.</span>
          </h2>

          <p className="mt-4 sm:mt-5 max-w-md text-sm sm:text-[15px] leading-relaxed text-midnight/70">
            One platform. Multiple services. A unified ecosystem that connects
            customers, providers, vendors, logistics companies and business
            platforms.
          </p>

          <a
            href="#services"
            className="mt-6 sm:mt-8 inline-flex items-center gap-2 rounded-full border border-aurora px-5 sm:px-6 py-2.5 sm:py-3 text-[13px] font-semibold text-aurora-ink hover:bg-aurora hover:text-midnight transition-colors"
          >
            Explore the ecosystem
            <ArrowRight size={14} strokeWidth={2.5} />
          </a>
        </div>

        {/* Right: connected diagram */}
        <div className="lg:col-span-7">
          <div
            ref={ref}
            data-inview={inView}
            className="relative mx-auto w-full max-w-[380px] sm:max-w-[720px] aspect-square sm:aspect-[1000/520]"
          >
            {/* Connection lines: mobile (tall) + desktop (wide) */}
            <Lines geo={GEO.mobile} className="sm:hidden" />
            <Lines geo={GEO.desktop} className="hidden sm:block" />

            {/* Center pill with real logo */}
            <div className="absolute left-1/2 top-1/2 z-10 -translate-x-1/2 -translate-y-1/2">
              <span
                aria-hidden
                className="absolute inset-0 rounded-xl bg-aurora/10 motion-safe:animate-[eco-halo_5s_ease-in-out_infinite]"
              />
              <div className="relative flex items-center justify-center rounded-xl bg-midnight px-4 py-2.5 sm:px-7 sm:py-3.5 shadow-[0_12px_40px_-8px_rgba(0,194,168,0.20)] ring-1 ring-aurora/40">
                <Image
                  src="/images/logo.png"
                  alt="Karryam"
                  width={200}
                  height={50}
                  className="h-6 sm:h-10 w-auto select-none"
                />
              </div>
            </div>

            {/* Nodes */}
            <div className="absolute inset-0 z-10">
              {SERVICES.map((n, i) => (
                <NodeItem key={n.label} node={n} position="top" index={i} />
              ))}
              {PARTNERS.map((n, i) => (
                <NodeItem key={n.label} node={n} position="bottom" index={i} />
              ))}
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}