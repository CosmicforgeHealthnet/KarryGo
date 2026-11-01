import { Bike, CarTaxiFront, Truck, type LucideIcon } from "lucide-react";

export type DriverAppId = "taxi" | "truck" | "dispatch";

type DriverApp = {
  id: DriverAppId;
  name: string;
  audience: string;
  icon: LucideIcon;
  file: string;
  size: string;
  version: string;
};

export const DRIVER_APPS: DriverApp[] = [
  {
    id: "taxi",
    name: "Taxi Driver",
    audience: "For car owners and taxi drivers taking ride requests.",
    icon: CarTaxiFront,
    file: "karryam-taxi-driver.apk",
    size: "73 MB",
    version: "1.0.0",
  },
  {
    id: "truck",
    name: "Truck Driver",
    audience: "For truck owners and drivers taking haulage jobs.",
    icon: Truck,
    file: "karryam-truck-driver.apk",
    size: "77 MB",
    version: "1.0.0",
  },
  {
    id: "dispatch",
    name: "Dispatch Rider",
    audience: "For bike riders delivering packages and orders.",
    icon: Bike,
    file: "karryam-dispatch-driver.apk",
    size: "88 MB",
    version: "1.0.0",
  },
];

/**
 * Returns the download address for an app, or null while it should show
 * "Coming soon". An app is live only once its file is hosted and it is named in
 * the ready list, so a release can never point visitors at a missing file.
 */
export function driverAppUrl(
  app: Pick<DriverApp, "id" | "file">,
  baseUrl = process.env.NEXT_PUBLIC_DRIVER_DOWNLOADS_BASE_URL,
  ready = process.env.NEXT_PUBLIC_DRIVER_APPS_READY,
): string | null {
  const base = baseUrl?.trim().replace(/\/+$/, "");
  if (!base || !/^https:\/\//.test(base)) return null;
  const readyIds = (ready ?? "").split(",").map((id) => id.trim());
  return readyIds.includes(app.id) ? `${base}/${app.file}` : null;
}
