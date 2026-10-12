"use client";

import Image from "next/image";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { ArrowRight, Check } from "lucide-react";

type Props = {
  id?: string;
  tone: "light" | "dark";
  eyebrow: string;
  title: ReactNode;
  text: string;
  points?: string[];
  cta: string;
  href: string;
  image?: string;
  alt?: string;
  imageWidth?: number;
  imageHeight?: number;
  backgroundImage?: string;
};

export default function PartnerCard({
  id,
  tone,
  eyebrow,
  title,
  text,
  points,
  cta,
  href,
  image,
  alt = "",
  imageWidth = 1000,
  imageHeight = 1200,
  backgroundImage,
}: Props) {
  const dark = tone === "dark";
  const ref = useRef<HTMLElement>(null);
  const [reveal, setReveal] = useState("visible");

  useEffect(() => {
    const el = ref.current;
    if (!el || !window.IntersectionObserver || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const rect = el.getBoundingClientRect();
    if (rect.top < window.innerHeight && rect.bottom > 0) return;
    setReveal("pending");
    const observer = new IntersectionObserver(([entry]) => {
      if (entry.isIntersecting) {
        setReveal("visible");
        observer.disconnect();
      }
    }, { threshold: 0.1 });
    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <article
      id={id}
      ref={ref}
      data-reveal={reveal}
      className={`partner-card relative scroll-mt-24 overflow-hidden rounded-2xl sm:rounded-3xl p-5 sm:p-7 lg:p-9 ring-1 ${
        dark
          ? "bg-midnight text-white ring-white/10"
          : "bg-gradient-to-br from-white to-[#EAF4F8] text-midnight ring-midnight/5 shadow-[0_24px_60px_-28px_rgba(11,31,68,0.25)]"
      }`}
    >
      {backgroundImage && (
        <>
          <Image
            src={backgroundImage}
            alt=""
            fill
            sizes="(min-width: 1024px) 50vw, 100vw"
            className="object-cover object-right select-none"
          />
          {/* Mobile: text spans the full width, so darken top-to-bottom for contrast.
              sm+: original left-to-right fade. */}
          <div
            aria-hidden
            className="absolute inset-0 bg-gradient-to-b from-midnight via-midnight/85 to-midnight/55 sm:bg-gradient-to-r sm:from-midnight sm:via-midnight/80 sm:to-midnight/10"
          />
          <div
            aria-hidden
            className="absolute inset-x-0 bottom-0 h-1/3 bg-gradient-to-t from-midnight/70 to-transparent"
          />
        </>
      )}

      {dark && !backgroundImage && (
        <div
          aria-hidden
          className="absolute inset-0 bg-[radial-gradient(ellipse_60%_60%_at_85%_60%,rgba(0,194,168,0.08),transparent_70%)]"
        />
      )}

      {/* Layout:
          - base: stacked
          - md (card is full width): text + image side by side
          - lg (card is only half width): stacked again, too narrow for two columns
          - xl: side by side */}
      <div
        className={`relative grid gap-6 sm:gap-8 ${
          image
            ? "md:grid-cols-[1.05fr_1fr] md:items-center lg:grid-cols-1 lg:items-start xl:grid-cols-[1.05fr_1fr] xl:items-center"
            : "min-h-[300px] sm:min-h-[320px] content-center"
        }`}
      >
        <div className={image ? "" : "max-w-[24rem]"}>
          <p
            className={`text-[10px] sm:text-[11px] tracking-[0.24em] font-semibold uppercase ${
              dark ? "text-aurora" : "text-aurora-ink"
            }`}
          >
            {eyebrow}
          </p>

          <h3 className="mt-3 sm:mt-4 text-[clamp(1.45rem,5.5vw,2.1rem)] lg:text-[clamp(1.45rem,2.1vw,2.1rem)] font-bold leading-[1.12] tracking-tight [text-wrap:balance]">
            {title}
          </h3>

          <p
            className={`mt-3 sm:mt-4 text-[13px] sm:text-sm leading-relaxed ${
              dark ? "text-white/70" : "text-midnight/70"
            }`}
          >
            {text}
          </p>

          {points && (
            <ul className="mt-5 grid gap-2.5">
              {points.map((p) => (
                <li key={p} className="flex items-center gap-2.5">
                  <span
                    className={`grid h-4 w-4 shrink-0 place-items-center rounded-full bg-aurora/15 ${
                      dark ? "text-aurora" : "text-aurora-ink"
                    }`}
                  >
                    <Check size={10} strokeWidth={3} />
                  </span>
                  <span
                    className={`text-[12px] ${
                      dark ? "text-white/85" : "text-midnight/80"
                    }`}
                  >
                    {p}
                  </span>
                </li>
              ))}
            </ul>
          )}

          <a
            href={href}
            className={`partner-cta mt-6 sm:mt-7 inline-flex items-center gap-2 rounded-full px-5 py-3 sm:py-2.5 text-[12px] font-semibold transition-colors duration-200 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-aurora ${
              dark
                ? "bg-aurora text-midnight shadow-sm hover:brightness-110"
                : "border border-aurora text-aurora-ink hover:bg-aurora hover:text-midnight"
            }`}
          >
            {cta}
            <ArrowRight aria-hidden size={13} strokeWidth={2.5} className="partner-arrow" />
          </a>
        </div>

        {image && (
          <div className="relative">
            <Image
              src={image}
              alt={alt}
              width={imageWidth}
              height={imageHeight}
              sizes="(min-width: 1280px) 22vw, (min-width: 768px) 40vw, 100vw"
              className="relative -mx-3 h-auto w-[calc(100%+1.5rem)] max-w-none select-none sm:mx-auto sm:w-full sm:max-w-[480px] md:max-w-[420px] lg:max-w-[460px] xl:max-w-[420px]"
            />
          </div>
        )}
      </div>
    </article>
  );
}