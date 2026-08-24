// src/lib/mock-routes-data.ts

export type RouteStop = {
  id: string;
  type: "Pickup" | "Dropoff" | "Waypoint";
  location: string;
  address: string;
  status: "Pending" | "Completed" | "Skipped" | "Exception";
  timeWindow?: string;
  assignedTo?: string;
};

export type Route = {
  id: string;
  name: string;
  service: "Dispatch" | "Hauling";
  branch: string;
  status: "Draft" | "Locked" | "Published" | "Archived";
  stopCount: number;
  provider: {
    id: string;
    name: string;
    rating: number;
  };
  asset: {
    id: string;
    name: string;
    type: string;
  };
  distance: string;
  duration: string;
  eta: string;
  stops: RouteStop[];
  instructions?: string;
  lastUpdated: string;
  progress: number;
};

export const ROUTES: Route[] = [
  {
    id: "RTE-0142",
    name: "Ikeja morning wave",
    service: "Dispatch",
    branch: "Ikeja",
    status: "Published",
    stopCount: 8,
    provider: {
      id: "P-044",
      name: "Peter Okafor",
      rating: 4.8,
    },
    asset: {
      id: "BR-88",
      name: "Honda ACE 110",
      type: "Motorcycle",
    },
    distance: "18.4 km",
    duration: "45 min",
    eta: "10:45",
    stops: [
      { id: "STOP-001", type: "Pickup", location: "Ikeja Mall", address: "42 Awolowo Way", status: "Completed" },
      { id: "STOP-002", type: "Dropoff", location: "Mushin", address: "15 Ogunlana Drive", status: "Completed" },
      { id: "STOP-003", type: "Pickup", location: "Oshodi", address: "8 Oshodi-Apapa Expressway", status: "Pending" },
      { id: "STOP-004", type: "Dropoff", location: "Lekki Phase 1", address: "3 Admiralty Way", status: "Pending" },
    ],
    instructions: "Priority route - all parcels are time-sensitive.",
    lastUpdated: "12 sec ago",
    progress: 25,
  },
  {
    id: "RTE-0141",
    name: "Lekki express window",
    service: "Dispatch",
    branch: "Lekki",
    status: "Locked",
    stopCount: 6,
    provider: {
      id: "P-061",
      name: "Chinedu Uche",
      rating: 4.9,
    },
    asset: {
      id: "BR-20",
      name: "Yamaha FZ",
      type: "Motorcycle",
    },
    distance: "12.6 km",
    duration: "32 min",
    eta: "11:20",
    stops: [
      { id: "STOP-005", type: "Pickup", location: "Lekki Phase 1", address: "3 Admiralty Way", status: "Pending" },
      { id: "STOP-006", type: "Dropoff", location: "Victoria Island", address: "5 Ahmadu Bello Way", status: "Pending" },
    ],
    instructions: "Express delivery - require recipient signature.",
    lastUpdated: "45 sec ago",
    progress: 0,
  },
  {
    id: "RTE-0140",
    name: "Apapa hauling block",
    service: "Hauling",
    branch: "Apapa",
    status: "Draft",
    stopCount: 4,
    provider: {
      id: "P-038",
      name: "Musa Bello",
      rating: 4.2,
    },
    asset: {
      id: "VN-04",
      name: "Mercedes Sprinter",
      type: "Van",
    },
    distance: "22.8 km",
    duration: "55 min",
    eta: "14:30",
    stops: [
      { id: "STOP-007", type: "Pickup", location: "Apapa Port", address: "7 Wharf Road", status: "Pending" },
      { id: "STOP-008", type: "Dropoff", location: "Tin Can", address: "12 Tin Can Road", status: "Pending" },
    ],
    lastUpdated: "3 min ago",
    progress: 0,
  },
  {
    id: "RTE-0139",
    name: "Yaba evening wave",
    service: "Dispatch",
    branch: "Yaba",
    status: "Published",
    stopCount: 10,
    provider: {
      id: "P-052",
      name: "Amina Yusuf",
      rating: 4.6,
    },
    asset: {
      id: "BR-71",
      name: "Honda ACE 110",
      type: "Motorcycle",
    },
    distance: "15.2 km",
    duration: "38 min",
    eta: "17:15",
    stops: [
      { id: "STOP-009", type: "Pickup", location: "Yaba Tech", address: "28 Herbert Macaulay Way", status: "Completed" },
      { id: "STOP-010", type: "Dropoff", location: "UNILAG", address: "13 Akoka Road", status: "Pending" },
    ],
    lastUpdated: "2 min ago",
    progress: 10,
  },
];

export const ROUTE_DETAIL = ROUTES[0];