// lib/types/route.ts

// Types mirroring dispatch.RoutePlan and dispatch.RouteStop from
// internal/features/dispatch/domain.go. Field names match the Go JSON tags.

export interface RoutePlan {
  id: string;
  organizationId: string;
  branchId: string;
  dispatchPlanId?: string;
  replacementForRouteId?: string;
  routeReference: string;
  routeType: string;
  providerMembershipId?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  executionService: string;
  status: string;
  plannedDistance: number;
  distanceUnit: string;
  plannedDurationSeconds: number;
  optimizationReference?: string;
  optimizationVersion: number;
  constraintSnapshot: unknown;
  approvedBy?: string;
  approvedAt?: string;
  publishedAt?: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface RouteStop {
  id: string;
  organizationId: string;
  routePlanId: string;
  sequence: number;
  stopType: string;
  orderId?: string;
  shipmentId?: string;
  packageId?: string;
  manifestId?: string;
  sourceOrderStopId?: string;
  locationReference?: string;
  latitude?: number;
  longitude?: number;
  timeWindowStart?: string;
  timeWindowEnd?: string;
  serviceDurationSeconds: number;
  plannedArrivalAt?: string;
  plannedDepartureAt?: string;
  actualArrivalAt?: string;
  actualDepartureAt?: string;
  status: string;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface RouteListFilter {
  branchId?: string;
  status?: string;
  service?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  limit?: number;
  offset?: number;
}

export interface CreateRouteInput {
  branchId: string;
  dispatchPlanId?: string;
  routeReference: string;
  routeType: string;
  providerMembershipId?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  executionService: string;
  constraintSnapshot?: unknown;
}

export interface EditRouteInput {
  providerMembershipId?: string;
  providerId?: string;
  vehicleOrTruckReference?: string;
  constraintSnapshot?: unknown;
  expectedVersion: number;
}

export interface RouteActionInput {
  expectedVersion: number;
  reason?: string;
  overrideReason?: string;
}

export interface CreateStopInput {
  stopType: string;
  orderId?: string;
  shipmentId?: string;
  packageId?: string;
  manifestId?: string;
  sourceOrderStopId?: string;
  locationReference?: string;
  latitude?: number;
  longitude?: number;
  timeWindowStart?: string;
  timeWindowEnd?: string;
  serviceDurationSeconds?: number;
  plannedArrivalAt?: string;
  plannedDepartureAt?: string;
}

export interface EditStopInput extends CreateStopInput {
  expectedVersion: number;
}