import type { Metadata, Viewport } from "next";
import { Inter } from "next/font/google";
import type { ReactNode } from "react";
import { RegisterServiceWorker } from "@/components/RegisterServiceWorker";
import "./globals.css";

const inter = Inter({ subsets: ["latin"], variable: "--font-inter", display: "swap" });

export const metadata: Metadata = {
  title: "Kodi",
  description: "Gestion de chantiers : budgets, dépenses, stock et suivi terrain.",
  manifest: "/manifest.webmanifest",
  icons: { icon: "/icons/icon-192.png", apple: "/icons/apple-touch-icon.png" },
  appleWebApp: { capable: true, title: "Kodi", statusBarStyle: "default" },
};

export const viewport: Viewport = { themeColor: "#FFC21A", width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="fr" className={inter.variable}>
      <body className="min-h-dvh">
        {children}
        <RegisterServiceWorker />
      </body>
    </html>
  );
}
