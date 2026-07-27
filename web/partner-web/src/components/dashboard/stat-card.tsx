import { Package, TrendingUp, CalendarCheck2 } from "lucide-react";
import type { DashboardSummary } from "@/types";
import { cn } from "@/lib/utils";

export function StatCardRow({ summary }: { summary: DashboardSummary }) {
  const items = [
    {
      label: "Total Deliveries",
      value: summary.total_deliveries.toLocaleString(),
      icon: Package,
      iconBg: "bg-brand-100",
      iconColor: "text-brand-600",
    },
    {
      label: "Success Rate",
      value: summary.success_rate_display,
      icon: TrendingUp,
      iconBg: "bg-blue-50",
      iconColor: "text-blue-600",
    },
    {
      label: "Deliveries Today",
      value: summary.deliveries_today.toLocaleString(),
      icon: CalendarCheck2,
      iconBg: "bg-amber-50",
      iconColor: "text-amber-600",
    },
  ];

  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
      {items.map((item) => (
        <div
          key={item.label}
          className="rounded-2xl border border-ink-100 bg-white p-5 shadow-sm transition-shadow hover:shadow-md"
        >
          <div className="flex items-center gap-3">
            <div
              className={cn(
                "flex h-10 w-10 items-center justify-center rounded-xl",
                item.iconBg
              )}
            >
              <item.icon className={cn("h-5 w-5", item.iconColor)} />
            </div>
            <p className="text-sm font-medium text-ink-500">{item.label}</p>
          </div>
          <p className="mt-4 text-3xl font-bold text-ink-900">{item.value}</p>
        </div>
      ))}
    </div>
  );
}