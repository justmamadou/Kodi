import type { NextConfig } from "next";

const apiUrl = process.env.API_URL ?? "http://localhost:8080";
// Origine du stockage S3 : la PWA y dépose les photos directement, via une URL pré-signée.
const s3Origin = process.env.NEXT_PUBLIC_S3_ORIGIN ?? "http://localhost:9100";

const csp = [
  "default-src 'self'",
  // Next.js injecte des scripts de démarrage en ligne.
  "script-src 'self' 'unsafe-inline'" + (process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : ""),
  "style-src 'self' 'unsafe-inline'",
  `img-src 'self' blob: data: ${s3Origin}`,
  "font-src 'self'",
  `connect-src 'self' ${s3Origin}` + (process.env.NODE_ENV === "development" ? " ws://localhost:3000" : ""),
  "worker-src 'self'",
  "manifest-src 'self'",
  "object-src 'none'",
  "base-uri 'self'",
  "form-action 'self'",
  "frame-ancestors 'none'",
].join("; ");

const nextConfig: NextConfig = {
  reactStrictMode: true,
  poweredByHeader: false,
  // En production, Caddy route /api vers l'API ; cette réécriture sert au développement local.
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiUrl}/api/:path*` }];
  },
  async headers() {
    return [
      {
        source: "/:path*",
        headers: [
          { key: "Content-Security-Policy", value: csp },
          { key: "X-Content-Type-Options", value: "nosniff" },
          { key: "Referrer-Policy", value: "strict-origin-when-cross-origin" },
        ],
      },
      {
        // Le service worker doit toujours être revalidé pour que les mises à jour arrivent.
        source: "/sw.js",
        headers: [{ key: "Cache-Control", value: "no-cache, no-store, must-revalidate" }],
      },
    ];
  },
};

export default nextConfig;
