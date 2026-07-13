import { StatCardRow } from "@/components/dashboard/stat-card";
import { ActivityFeed } from "@/components/dashboard/activity-feed";
import { mockStats } from "@/lib/mock/mock-stats";
import { mockActivity } from "@/lib/mock/mock-activity";

export default function DashboardPage() {
  const partnerName = "Business Venture";

  return (
    <div>
      <h1 className="text-2xl font-bold text-ink-900">
        Welcome back, {partnerName}
      </h1>
      <p className="mt-1 text-sm text-ink-500">
        Here&apos;s what&apos;s on your counter today.
      </p>

      <div className="mt-6">
        <StatCardRow stats={mockStats} />
      </div>

      <div className="mt-8 flex items-center justify-between">
        <h2 className="text-lg font-bold text-ink-900">Recent Activity</h2>
        <a href="#" className="text-sm font-semibold text-brand-500">
          View All
        </a>
      </div>

      <div className="mt-4">
        <ActivityFeed items={mockActivity} />
      </div>
    </div>
  );
}