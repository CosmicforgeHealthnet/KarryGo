export const SEGMENTS = [
  { value: "customer", label: "Customer", hint: "Ride, Send, Truck, Food, Shop" },
  { value: "provider", label: "Provider", hint: "Driver or rider" },
  { value: "vendor", label: "Vendor", hint: "Food and shop business" },
  { value: "fleet", label: "Fleet", hint: "Logistics company" },
  { value: "api", label: "API Partner", hint: "Integrate delivery" },
] as const;

export type Segment = (typeof SEGMENTS)[number]["value"];

export function isSegment(value: unknown): value is Segment {
  return SEGMENTS.some((s) => s.value === value);
}