import type { Metadata } from "next";
import { Geist } from "next/font/google";
import "./globals.css";
import { SessionBootstrap } from "@/components/session-bootstrap";

const geist = Geist({
  subsets: ["latin"],
  variable: "--font-geist",
});

export const metadata: Metadata = {
  title: "CosmicForge Healthnet | Partner",
  description: "Logistics partner portal for CosmicForge Healthnet",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en">
      <body className={`${geist.variable} antialiased`}><SessionBootstrap>{children}</SessionBootstrap></body>
    </html>
  );
}
