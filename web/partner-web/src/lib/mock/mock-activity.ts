import type { ActivityItem } from "@/types";

export const mockActivity: ActivityItem[] = [
  { id: "1", message: "Your package has arrived!", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "2", message: "Your Package has been delivered", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "3", message: "Pick up is here!", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "4", message: "Package has been delivered.", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "5", message: "Leave a review on your last delivery.", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "6", message: "You have uploaded a profile picture.", timestamp: "12:05 PM", group: "Today", read: false },
  { id: "7", message: "Your Ride has arrived!", timestamp: "12:05 PM", group: "Yesterday", read: true },
  { id: "8", message: "A driver confirmed your booking, he's on his way.", timestamp: "12:05 PM", group: "Yesterday", read: false },
  { id: "9", message: "Your package has been picked.", timestamp: "12:05 PM", group: "3 days ago", read: true },
  { id: "10", message: "Driver Canceled the order!", timestamp: "12:05 PM", group: "3 days ago", read: true },
  { id: "11", message: "Truck booking confirmed! Driver on his way.", timestamp: "12:05 PM", group: "4 days Ago", read: false },
];