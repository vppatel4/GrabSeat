import type { Metadata } from "next";
import { Fraunces, Space_Grotesk } from "next/font/google";
import "./globals.css";

// Deliberately not Inter: a warm display serif for the marquee, a slightly
// technical grotesque for everything else.
const display = Fraunces({
  subsets: ["latin"],
  weight: ["400", "600", "700"],
  variable: "--font-display",
  display: "swap",
});

const sans = Space_Grotesk({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-sans",
  display: "swap",
});

export const metadata: Metadata = {
  title: "GrabSeat — live seat reservations",
  description:
    "Grab a seat in real time. One winner per seat, guaranteed, even under a stampede.",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en" className={`${display.variable} ${sans.variable}`}>
      <body className="font-sans text-steel-300 antialiased">{children}</body>
    </html>
  );
}
