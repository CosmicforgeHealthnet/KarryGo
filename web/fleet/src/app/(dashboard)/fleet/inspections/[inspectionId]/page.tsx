'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { useParams, useRouter } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';
import { inspectionService } from '@/lib/services/inspectionService';
import type { Inspection } from '@/lib/types/inspection';

export default function InspectionDetailRouter() {
  const params = useParams();
  const router = useRouter();
  const inspectionId = params.inspectionId as string;

  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!inspectionId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);

    inspectionService
      .getInspection(inspectionId)
      .then((data: Inspection) => {
        if (cancelled) return;
        if (data.status === 'DRAFT' || data.status === 'IN_PROGRESS') {
          router.replace(`/fleet/inspections/${data.id}/perform`);
        } else {
          router.replace(`/fleet/inspections/${data.id}/result`);
        }
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err?.response?.data?.error?.message ??
            err?.message ??
            'Inspection not found.',
        );
        setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [inspectionId, router]);

  if (error) {
    return (
      <div className="space-y-4">
        <div className="rounded-lg border border-red/30 bg-red-bg px-4 py-2 text-sm text-red">
          {error}
        </div>
        <Link
          href="/fleet/inspections"
          className="inline-flex items-center gap-1.5 rounded-lg border border-line bg-surface px-4 py-2 text-sm font-medium text-ink-700 transition-colors hover:bg-surface-2"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to inspections
        </Link>
      </div>
    );
  }

  if (loading) {
    return (
      <div className="rounded-lg border border-line bg-surface px-4 py-16 text-center text-sm text-ink-500">
        Loading inspection…
      </div>
    );
  }

  return null;
}