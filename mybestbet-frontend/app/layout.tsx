import type React from "react";
import type { Metadata, Viewport } from "next";
import localFont from "next/font/local";
import { Analytics } from "@vercel/analytics/next";
import { DARK_ONLY } from "@/lib/theme";
import "./globals.css";

const mona = localFont({
  src: "../public/mybestbet/fonts/mona-sans-variable.woff2",
  variable: "--font-mona",
  weight: "200 900",
  display: "swap",
});

const outfit = localFont({
  src: "../public/mybestbet/fonts/outfit-variable.woff2",
  variable: "--font-outfit",
  weight: "100 900",
  display: "swap",
});

const SITE_URL = "https://mybestbet.app";

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  maximumScale: 5,
  viewportFit: "cover",
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f2f2f2" },
    { media: "(prefers-color-scheme: dark)", color: "#0d0d0d" },
  ],
};

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    template: "%s | mybestbet",
    default: "mybestbet: the best price on every bet",
  },
  description:
    "mybestbet compares odds from sportsbooks and prediction markets side by side, and flags arbitrage, value bets, line gaps and steam moves. An educational demo, not a bookmaker.",
  keywords: [
    "odds comparison",
    "best odds",
    "arbitrage betting",
    "value bets",
    "prediction markets",
    "player props",
    "line movement",
    "mybestbet",
  ],
  applicationName: "mybestbet",
  category: "sports",
  alternates: {
    canonical: "/",
  },
  robots: {
    index: true,
    follow: true,
  },
  openGraph: {
    type: "website",
    locale: "en_US",
    url: SITE_URL,
    siteName: "mybestbet",
    title: "mybestbet: the best price on every bet",
    description:
      "Every book's price on one screen, with arbs, value bets and price moves flagged as they happen.",
  },
  twitter: {
    card: "summary_large_image",
    title: "mybestbet: the best price on every bet",
    description:
      "Every book's price on one screen, with arbs, value bets and price moves flagged as they happen.",
  },
  formatDetection: {
    telephone: false,
    email: false,
    address: false,
  },
};

const jsonLd = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebSite",
      "@id": `${SITE_URL}#website`,
      url: SITE_URL,
      name: "mybestbet",
      description:
        "Odds from sportsbooks and prediction markets side by side. An educational demo, not affiliated with any book, league or team.",
      inLanguage: "en-US",
    },
    {
      "@type": "SoftwareApplication",
      "@id": `${SITE_URL}#app`,
      name: "mybestbet",
      applicationCategory: "SportsApplication",
      operatingSystem: "Web",
      url: SITE_URL,
      featureList: [
        "Best price per outcome across sportsbooks and prediction markets",
        "Arbitrage finder with a stake calculator",
        "Value bets against a margin-free fair price",
        "Line gaps and steam moves",
        "Player props with hit rates and game logs",
        "Price history for every book",
      ],
      offers: {
        "@type": "Offer",
        price: "0",
        priceCurrency: "USD",
      },
    },
  ],
};

// Runs before paint so the saved or system theme never flashes the wrong palette.
const themeScript = `(function(){var t=null;try{t=localStorage.getItem("mbb-theme")}catch(e){}if(t!=="light"&&t!=="dark"){try{t=matchMedia("(prefers-color-scheme: light)").matches?"light":"dark"}catch(e){t="dark"}}if(new RegExp(${JSON.stringify(DARK_ONLY)}).test(location.pathname))t="dark";document.documentElement.dataset.theme=t})()`;

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={`${mona.variable} ${outfit.variable}`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{ __html: JSON.stringify(jsonLd) }}
        />
      </head>
      <body>
        {children}
        {process.env.VERCEL ? <Analytics /> : null}
      </body>
    </html>
  );
}
