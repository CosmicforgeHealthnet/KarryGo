"use client";

import { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { ArrowRight, Menu, X } from "lucide-react";

const NAV_ITEMS = [
  { label: "Home", id: "home" },
  { label: "About", id: "about" },
  { label: "Services", id: "services" },
  { label: "FAQs", id: "faq" },
];

export default function Navbar() {
  const [scrolled, setScrolled] = useState(false);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState("home");
  const navListRef = useRef<HTMLUListElement>(null);
  const [underline, setUnderline] = useState({ x: 0, width: 0 });

  useEffect(() => {
    const onScroll = () => {
      setScrolled(window.scrollY > 24);

      // scroll-spy: last section whose top has passed the header
      let current = "home";
      let nearest = -Infinity;
      for (const { id } of NAV_ITEMS) {
        const top = document.getElementById(id)?.getBoundingClientRect().top;
        if (top !== undefined && top <= 120 && top > nearest) {
          nearest = top;
          current = id;
        }
      }
      setActive(current);
    };
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  // Slide one shared underline to the active link; fonts and resizes move links.
  useEffect(() => {
    const list = navListRef.current;
    if (!list) return;
    const measure = () => {
      const link = list.querySelector<HTMLElement>('[aria-current="page"]');
      if (link) setUnderline({ x: link.offsetLeft, width: link.offsetWidth });
    };
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(list);
    return () => observer.disconnect();
  }, [active]);

  // Mobile menu: lock page scroll, close on Escape, close if viewport grows to desktop
  useEffect(() => {
    if (!open) return;

    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    const mq = window.matchMedia("(min-width: 1024px)");
    const onMq = () => {
      if (mq.matches) setOpen(false);
    };

    window.addEventListener("keydown", onKey);
    mq.addEventListener("change", onMq);

    return () => {
      document.body.style.overflow = prevOverflow;
      window.removeEventListener("keydown", onKey);
      mq.removeEventListener("change", onMq);
    };
  }, [open]);

  return (
    <header
      className={`fixed inset-x-0 top-0 z-50 transition-all duration-300 ${
        scrolled || open
          ? "bg-white/90 backdrop-blur-xl border-b border-midnight/5 shadow-[0_1px_12px_rgba(11,31,68,0.06)]"
          : "bg-white border-b border-transparent"
      }`}
    >
      <span
        aria-hidden
        className="scroll-progress absolute inset-x-0 bottom-0 h-0.5 bg-aurora"
      />
      <nav className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 h-16 lg:h-20 flex items-center justify-between lg:grid lg:grid-cols-[1fr_auto_1fr]">
        {/* Logo */}
        <a
          href="#home"
          className="flex items-center justify-self-start"
          aria-label="Karryam home"
        >
          <Image
            src="/images/logo-navy.png"
            alt="Karryam"
            width={100}
            height={60}
            priority
            className="brand-logo h-7 sm:h-8 lg:h-10 w-auto select-none"
          />
        </a>

        {/* Center nav */}
        <ul
          ref={navListRef}
          className="relative hidden lg:flex items-center gap-10 xl:gap-14 text-[15px] font-medium"
        >
          {NAV_ITEMS.map(({ label, id }) => {
            const isActive = active === id;
            return (
              <li key={id}>
                <a
                  href={`#${id}`}
                  aria-current={isActive ? "page" : undefined}
                  className={`py-2 whitespace-nowrap transition-colors ${
                    isActive
                      ? "text-[#00A38C]"
                      : "text-midnight/75 hover:text-midnight"
                  }`}
                >
                  {label}
                </a>
              </li>
            );
          })}
          <li
            aria-hidden
            className="nav-underline absolute -bottom-2 left-0 h-[2px] rounded-full bg-aurora"
            style={{ width: underline.width, transform: `translateX(${underline.x}px)` }}
          />
        </ul>

        {/* Right */}
        <div className="flex items-center gap-3 lg:gap-6 justify-self-end">
          <a
            href="/waitlist"
            className="hidden sm:inline-flex items-center gap-2 whitespace-nowrap bg-aurora text-midnight font-semibold text-[14px] px-6 py-3 rounded-full hover:brightness-105 transition"
          >
            Join the Waitlist
            <ArrowRight size={14} strokeWidth={2.5} />
          </a>
          <button
            onClick={() => setOpen(!open)}
            className="lg:hidden text-midnight p-2 -mr-2"
            aria-label="Toggle menu"
            aria-expanded={open}
          >
            {open ? <X size={22} /> : <Menu size={22} />}
          </button>
        </div>
      </nav>

      {/* Mobile sheet */}
      {open && (
        <div className="nav-sheet lg:hidden bg-white border-t border-midnight/5 max-h-[calc(100dvh-4rem)] overflow-y-auto overscroll-contain">
          <ul className="px-5 sm:px-8 py-5 flex flex-col gap-1 text-midnight">
            {NAV_ITEMS.map(({ label, id }) => (
              <li key={id}>
                <a
                  href={`#${id}`}
                  onClick={() => setOpen(false)}
                  className={`block py-3 text-base ${
                    active === id ? "text-[#00A38C] font-semibold" : ""
                  }`}
                >
                  {label}
                </a>
              </li>
            ))}
            <li className="pt-3 pb-2 flex flex-col gap-3">
            
              <a
                href="/waitlist"
                className="flex items-center justify-center gap-2 bg-aurora text-midnight font-semibold px-5 py-3 rounded-full"
              >
                Join the Waitlist
                <ArrowRight size={16} strokeWidth={2.5} />
              </a>
            </li>
          </ul>
        </div>
      )}
    </header>
  );
}