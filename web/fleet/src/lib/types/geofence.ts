// lib/types/geofence.ts

// Types mirroring dispatch.Geofence and the JSON the corporate geofence
// endpoints emit (internal/features/dispatch/domain.go).

export interface Point {
  latitude: number;
  longitude: number;
}

export interface Geometry {
  center?: Point;
  radiusMeters?: number;
  points?: Point[];
}

export interface Geofence {
  id: string;
  organizationId: string;
  branchId?: string;
  name: string;
  geometryType: string; // "CIRCLE" | "POLYGON"
  geometry: Geometry;
  geometryReference?: string;
  eventRules: string[];
  activeStatus: string; // "ACTIVE" | "INACTIVE"
  version: number;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
}

export interface GeofenceListFilter {
  branchId?: string;
  limit?: number;
  offset?: number;
}

export interface CreateGeofenceInput {
  branchId: string;
  name: string;
  geometryType: string;
  geometryReference?: string;
  geometry: Geometry;
  eventRules: string[];
}