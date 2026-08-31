export type NavItem = {
  label: string;
  href: string;
};

export const NAV_ITEMS: NavItem[] = [
  { label: "Home", href: "/" },
  { label: "Operations", href: "/operations/dispatch" },
  { label: "Wallet", href: "/wallet" },
  { label: "Providers", href: "/providers" },
  { label: "Fleet", href: "/fleet/assets" },
  { label: "Workforce", href: "/workforce" },
  { label: "Warehouse", href: "/warehouse" },
  { label: "CRM & Commercial", href: "/crm/customers" },
  { label: "Private Operations", href: "/private-ops/orders" },
  { label: "Finance", href: "/finance" },
  { label: "Compliance & Safety", href: "/compliance" },
  { label: "Communications & Support", href: "/comms/threads" },
  { label: "Documents & Data", href: "/documents" },
  { label: "Reports", href: "/reports/library" },
  { label: "Integrations", href: "/settings/integrations" },
  { label: "Settings & Governance", href: "/settings" },
];