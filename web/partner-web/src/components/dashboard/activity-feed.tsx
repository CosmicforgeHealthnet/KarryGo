import type { ActivityItem } from "@/types";
import { cn } from "@/lib/utils";

export function ActivityFeed({ items }: { items: ActivityItem[] }) {
  if (items.length === 0) {
    return (
      <div className="flex h-64 items-center justify-center">
        <p className="text-sm text-ink-500">There is no Activity here.</p>
      </div>
    );
  }

  const groups: { group: string; items: ActivityItem[] }[] = [];
  for (const item of items) {
    const existing = groups.find((g) => g.group === item.group);
    if (existing) {
      existing.items.push(item);
    } else {
      groups.push({ group: item.group, items: [item] });
    }
  }

  return (
    <div className="space-y-6">
      {groups.map((g) => (
        <div key={g.group}>
          <p className="mb-2 text-sm font-medium text-ink-500">{g.group}</p>
          <div className="space-y-2">
            {g.items.map((item) => (
              <div
                key={item.id}
                className="flex items-center justify-between rounded-xl bg-ink-100/60 px-4 py-3"
              >
                <div className="flex items-center gap-3">
                  <span
                    className={cn(
                      "h-2.5 w-2.5 shrink-0 rounded-full",
                      item.read ? "bg-brand-200" : "bg-brand-500"
                    )}
                  />
                  <p className="text-sm text-ink-900">{item.message}</p>
                </div>
                <span className="shrink-0 text-xs text-ink-500">
                  {item.timestamp}
                </span>
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}