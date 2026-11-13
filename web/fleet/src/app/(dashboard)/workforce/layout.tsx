'use client';

import Link from 'next/link';
import { usePathname } from 'next/navigation';

// Workforce sections. Each page keeps its own header; this only makes the
// sections reachable from one another.
const TABS = [
  { label: 'Profiles', href: '/workforce', exact: true },
  { label: 'Shifts', href: '/workforce/shifts' },
  { label: 'Leave', href: '/workforce/leave' },
  { label: 'Qualifications', href: '/workforce/qualifications' },
  { label: 'Training', href: '/workforce/training' },
];

export default function WorkforceLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname() ?? '';
  const sectionOf = (tab: (typeof TABS)[number]) =>
    tab.exact
      ? pathname === tab.href || (/^\/workforce\/[^/]+(\/edit)?$/.test(pathname) && !TABS.some((t) => !t.exact && pathname.startsWith(t.href)))
      : pathname === tab.href || pathname.startsWith(tab.href + '/');

  return (
    <div className="space-y-5">
      <nav aria-label="Workforce sections" className="flex gap-1.5 overflow-x-auto border-b border-line pb-px">
        {TABS.map((tab) => {
          const active = sectionOf(tab);
          return (
            <Link
              key={tab.href}
              href={tab.href}
              aria-current={active ? 'page' : undefined}
              className={`whitespace-nowrap border-b-2 px-3 py-2 text-sm font-semibold transition-colors ${
                active ? 'border-primary text-primary' : 'border-transparent text-ink-500 hover:text-ink-800'
              }`}
            >
              {tab.label}
            </Link>
          );
        })}
      </nav>
      {children}
    </div>
  );
}
