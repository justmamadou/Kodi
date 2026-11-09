import { defineConfig } from "@playwright/test";

// Parcours multi-écrans (CF-029). Prérequis : API et PWA démarrées, base de test seedée (voir e2e/README).
export default defineConfig({
  testDir: "./e2e",
  timeout: 60_000,
  fullyParallel: false,
  use: { baseURL: process.env.BASE_URL ?? "http://localhost:3000", channel: "chrome", locale: "fr-FR" },
  projects: [
    { name: "petit-telephone", use: { viewport: { width: 320, height: 640 }, hasTouch: true } },
    { name: "telephone", use: { viewport: { width: 390, height: 844 }, hasTouch: true } },
    { name: "tablette", use: { viewport: { width: 768, height: 1024 } } },
    { name: "bureau", use: { viewport: { width: 1440, height: 900 } } },
  ],
});
