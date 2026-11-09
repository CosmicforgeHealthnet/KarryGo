import type { Metadata } from "next";
import localFont from "next/font/local";

import { ThemeProvider } from "@/components/ThemeProvider";
import "./globals.css";

// The Karryam brand font: the exact Inter files bundled with the vendor app
// (apps/vendor/assets/fonts). One family for UI, headings and numbers.
const inter = localFont({
  variable: "--font-inter",
  display: "swap",
  src: [
    { path: "./fonts/inter-regular.ttf", weight: "400", style: "normal" },
    { path: "./fonts/inter-bold.ttf", weight: "700", style: "normal" },
  ],
});

export const metadata: Metadata = {
  title: "Karryam Fleet",
  description: "Karryam Fleet — corporate logistics",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html
      lang="en"
      className={`${inter.variable} h-full antialiased`}
      suppressHydrationWarning
    >
      <head>
        {/* Prevent flash of wrong theme on load */}
        <script
          dangerouslySetInnerHTML={{
            __html: `
              (function() {
                try {
                  // Light by default; dark only when chosen in the app.
                  const theme = localStorage.getItem('theme') === 'dark' ? 'dark' : 'light';
                  document.documentElement.setAttribute('data-theme', theme);
                } catch (_) {
                  // Fallback to light if localStorage is blocked
                  document.documentElement.setAttribute('data-theme', 'light');
                }
              })();
            `,
          }}
        />
      </head>
      <body className="min-h-full flex flex-col">
        <ThemeProvider>
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}