export const SEGMENTS = [
  { key: "CUSTOMER",          card: "I want to use Karryam",          focus: "Ride / Send / Truck / Food / Shop" },
  { key: "TAXI_PROVIDER",     card: "I want to drive",                focus: "Taxi provider" },
  { key: "DISPATCH_PROVIDER", card: "I want to deliver",              focus: "Dispatch provider" },
  { key: "TRUCK_PROVIDER",    card: "I move freight",                 focus: "Haulage provider" },
  { key: "VENDOR",            card: "I sell food or products",        focus: "Food / Shop merchant" },
  { key: "FLEET",             card: "I run a fleet/logistics company", focus: "Corporate logistics" },
  { key: "API_PARTNER",       card: "I want to integrate Karryam",    focus: "External platform/business" },
] as const;

export type Segment = typeof SEGMENTS[number]["key"];