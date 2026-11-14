"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { DataTable, Panel, StatusPill, formatWhen, humanize } from "@/components/features/OpsUi";
import { describeApiError, type ApiErrorInfo } from "@/lib/api/errors";
import { rescueService } from "@/lib/services/operationsService";
import type { GeofenceEvent } from "@/lib/types/operations";

// Entries and exits the tracking pipeline recorded for this geofence.
export function GeofenceEvents({ geofenceId }: { geofenceId: string }) {
  const [events, setEvents] = useState<GeofenceEvent[]>([]);
  const [error, setError] = useState<ApiErrorInfo | null>(null);

  useEffect(() => {
    rescueService
      .geofenceEvents({ geofenceId, limit: 100 })
      .then(setEvents)
      .catch((e) => setError(describeApiError(e, "Geofence events could not be loaded.")));
  }, [geofenceId]);

  return (
    <Panel title="Geofence events" description="Trips that entered, left or dwelled in this area.">
      <ApiErrorNotice error={error} />
      {events.length === 0 ? (
        <p className="text-sm text-ink-500">No events recorded for this geofence yet.</p>
      ) : (
        <DataTable
          minWidth={720}
          rows={events}
          rowKey={(e) => e.id}
          columns={[
            { header: "When", cell: (e) => formatWhen(e.occurredAt) },
            { header: "Event", cell: (e) => humanize(e.eventRule) },
            {
              header: "Trip",
              cell: (e) => (
                <Link className="font-mono text-primary" href={`/operations/trips/${encodeURIComponent(e.canonicalTripReference)}`}>
                  {e.canonicalTripReference}
                </Link>
              ),
            },
            { header: "Confidence", cell: (e) => humanize(e.confidence) },
            { header: "Data", cell: (e) => <StatusPill status={e.dataFreshness} /> },
          ]}
        />
      )}
    </Panel>
  );
}
