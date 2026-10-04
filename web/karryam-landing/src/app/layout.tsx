import type { Metadata } from "next";
import { Inter } from "next/font/google";
import "./globals.css";

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

export const metadata: Metadata = {
  title: "Karryam — Ride, Delivery, Food, Shopping & Logistics in One Platform",
  description:
    "Karryam is a movement-intelligence platform for rides, parcel delivery, haulage, food, shopping, providers, vendors, fleets and business integrations. Join the early-access waitlist.",
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en" className={inter.variable}>
      <body>{children}</body>
    </html>
  );
}