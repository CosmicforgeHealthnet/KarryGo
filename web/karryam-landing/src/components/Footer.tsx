import Image from "next/image";

const LINKS = [
  { label: "Home", href: "#home" },
  { label: "Services", href: "#services" },
  { label: "For Providers", href: "#providers" },
  { label: "For Businesses", href: "#businesses" },
  { label: "About", href: "#about" },
];

const LEGAL = ["Privacy", "Terms", "Security"];

export default function Footer() {
  return (
    <footer className="bg-midnight text-white border-t border-white/10">
      <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 py-8 sm:py-10 flex flex-col gap-6 sm:gap-8 lg:flex-row lg:items-center lg:justify-between">
        <a href="#home" aria-label="Karryam home" className="flex items-center">
          <Image
            src="/images/logo.png"
            alt="Karryam"
            width={100}
            height={60}
            className="h-7 sm:h-8 lg:h-10 w-auto select-none"
          />
        </a>

        <nav aria-label="Footer">
          {/* mobile: tidy 2-column grid; sm+: wrapping row */}
          <ul className="grid grid-cols-2 gap-x-6 gap-y-1 sm:flex sm:flex-wrap sm:items-center sm:gap-x-8 sm:gap-y-3 text-[13px] text-white/70">
            {LINKS.map(({ label, href }) => (
              <li key={label}>
                <a
                  href={href}
                  className="inline-block py-2 sm:py-0 hover:text-white transition-colors"
                >
                  {label}
                </a>
              </li>
            ))}
          </ul>
        </nav>
      </div>

      <div className="border-t border-white/10">
        <div className="mx-auto max-w-[1440px] px-5 sm:px-8 lg:px-12 py-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between text-[12px] text-white/50">
          <p>© {new Date().getFullYear()} Karryam. All rights reserved.</p>
          <ul className="flex flex-wrap items-center gap-x-6 gap-y-2">
            {LEGAL.map((item) => (
              <li key={item}>
                <a href="#" className="hover:text-white transition-colors">
                  {item}
                </a>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </footer>
  );
}