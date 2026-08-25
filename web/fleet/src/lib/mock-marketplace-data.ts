// src/lib/mock-marketplace-data.ts

export const OFFER_DETAIL = {
  id: "MKT-24081",
  service: "Dispatch" as const,
  pickup: "Ikeja",
  dropoff: "Lekki",
  deadline: "27 min",
  vehicle: "Bike",
  distance: "6.8 km",
  load: "18 kg",
  instructions: "Fragile electronics — handle upright, no stacking.",
  window: "Today, 2:00 PM – 4:30 PM",
  branch: "Ikeja",
};

export type Candidate = {
  id: string;
  name: string;
  asset: string;
  distanceToPickup: string;
  eligibility: "Eligible" | "Warning" | "Blocked";
  reason: string;
};

export const CANDIDATES: Candidate[] = [
  { id: "prov_po", name: "Peter Okafor", asset: "Bike BR-88", distanceToPickup: "1.2 km", eligibility: "Eligible", reason: "Online, fully eligible" },
  { id: "prov_ay", name: "Amina Yusuf", asset: "Bike BR-71", distanceToPickup: "2.6 km", eligibility: "Eligible", reason: "Online, fully eligible" },
  { id: "prov_cu", name: "Chinedu Uche", asset: "Bike BR-20", distanceToPickup: "0.9 km", eligibility: "Warning", reason: "Training renewal due in 6 days" },
  { id: "prov_ka", name: "Kabiru Ahmed", asset: "Truck TR-12", distanceToPickup: "4.1 km", eligibility: "Blocked", reason: "Safety block — vehicle inspection failed" },
];

export type AssignmentRow = {
  id: string;
  offerId: string;
  provider: string;
  branch: string;
  status: "Pending response" | "Accepted" | "Declined" | "Expired";
  deadline: string;
};

export const ASSIGNMENTS: AssignmentRow[] = [
  { id: "AS-1187", offerId: "MKT-24081", provider: "Peter Okafor", branch: "Ikeja", status: "Pending response", deadline: "18 min left" },
  { id: "AS-1186", offerId: "MKT-24079", provider: "Musa Bello", branch: "Lekki", status: "Accepted", deadline: "—" },
  { id: "AS-1185", offerId: "MKT-24073", provider: "Amina Yusuf", branch: "Yaba", status: "Accepted", deadline: "—" },
  { id: "AS-1184", offerId: "MKT-24069", provider: "Kabiru Ahmed", branch: "Apapa", status: "Declined", deadline: "—" },
  { id: "AS-1183", offerId: "MKT-24058", provider: "Chinedu Uche", branch: "Ikeja", status: "Expired", deadline: "—" },
];

// ✅ FIX: Removed 'as const' and added union type for status
export const ASSIGNMENT_DETAIL = {
  id: "AS-1187",
  offerId: "MKT-24081",
  provider: "Peter Okafor",
  asset: "Bike BR-88",
  branch: "Ikeja",
  status: "Pending response" as "Pending response" | "Accepted" | "Declined" | "Expired",
  deadline: "18 min left",
  created: "Today, 1:52 PM",
  instructions: "Fragile electronics — handle upright, no stacking.",
};

export type ResponseHistoryEvent = {
  id: string;
  label: string;
  actor: string;
  detail: string;
  time: string;
};

export const RESPONSE_HISTORY: ResponseHistoryEvent[] = [
  { id: "rh_1", label: "Assignment created", actor: "Dispatcher — Ops Manager", detail: "Sent to Peter Okafor", time: "1:52 PM" },
  { id: "rh_2", label: "Awaiting provider response", actor: "System", detail: "Response window: 30 min", time: "1:52 PM" },
];

export type RescueCase = {
  id: string;
  reference: string;
  severity: "Critical" | "High" | "Medium";
  reason: string;
  branch: string;
  owner: string;
  time: string;
};

export const RESCUE_CASES: RescueCase[] = [
  { id: "res_1", reference: "KG-8470", severity: "Critical", reason: "Vehicle safety block mid-route", branch: "Ikeja", owner: "Unassigned", time: "13:38" },
  { id: "res_2", reference: "KG-8466", severity: "Medium", reason: "Receiver unavailable, reschedule needed", branch: "Lekki", owner: "T. Adeyemi", time: "13:29" },
  { id: "res_3", reference: "AS-1184", severity: "High", reason: "Provider declined — needs replacement", branch: "Apapa", owner: "Unassigned", time: "12:58" },
];