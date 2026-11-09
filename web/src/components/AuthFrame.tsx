import type { ReactNode } from "react";
import { IconCheck, IconHelmet } from "@/components/icons";

const POINTS = ["Budget exposé et disponible, chantier par chantier", "Reçus photographiés sur le terrain, même sans réseau", "Alertes avant que le budget ne dérape"];

/** Mise en page de la maquette de connexion (01) : panneau sombre à gauche, formulaire à droite. */
export function AuthFrame({ children }: { children: ReactNode }) {
  return (
    <div style={{ display: "flex", flexWrap: "wrap", minHeight: "100dvh" }}>
      <section style={{ flex: "1 1 420px", background: "#1D1D1F", color: "#FFFFFF", padding: "48px 56px", display: "flex", flexDirection: "column", gap: 40, minWidth: 0, maxWidth: "100%" }} className="login-side">
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <div style={{ width: 44, height: 44, borderRadius: 11, background: "#FFC21A", color: "#1D1D1F", display: "flex", alignItems: "center", justifyContent: "center" }}>
            <IconHelmet size={24} width={2} />
          </div>
          <div style={{ fontSize: 22, fontWeight: 700 }}>
            Kodi
          </div>
        </div>
        <div style={{ marginTop: "auto", maxWidth: 520 }}>
          <h1 style={{ margin: 0, fontSize: 40, lineHeight: 1.15, fontWeight: 700, letterSpacing: "-0.02em" }}>Sachez combien chaque chantier vous coûte.</h1>
          <p style={{ margin: "18px 0 0", fontSize: 17, lineHeight: 1.55, color: "#D2D2D7" }}>Dépenses, justificatifs, budgets et rapports de vos équipes au même endroit, depuis le bureau ou le chantier.</p>
        </div>
        <ul style={{ listStyle: "none", margin: "0 0 auto", padding: 0, display: "flex", flexDirection: "column", gap: 14, fontSize: 15, color: "#E5E5EA" }}>
          {POINTS.map((p) => (
            <li key={p} style={{ display: "flex", gap: 12, alignItems: "center" }}>
              <span style={{ color: "#FFC21A" }}>
                <IconCheck size={20} width={2.4} />
              </span>
              {p}
            </li>
          ))}
        </ul>
      </section>
      <section style={{ flex: "1 1 420px", display: "flex", alignItems: "center", justifyContent: "center", padding: "48px 24px", background: "#FFFFFF", minWidth: 0 }}>
        <div style={{ width: "100%", maxWidth: 400 }}>{children}</div>
      </section>
      <style>{`@media (max-width: 880px) { .login-side { padding: 28px 24px !important; gap: 20px !important; } .login-side h1 { font-size: 28px !important; } }`}</style>
    </div>
  );
}

/** Titre et sous-titre du formulaire, comme « Connexion » dans la maquette. */
export function AuthHeading({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div>
      <h2 style={{ margin: 0, fontSize: 28, fontWeight: 700, letterSpacing: "-0.02em" }}>{title}</h2>
      {children && <p style={{ margin: "8px 0 0", fontSize: 15, lineHeight: 1.5, color: "#6E6E73" }}>{children}</p>}
    </div>
  );
}

export const authFormStyle = { width: "100%", display: "flex", flexDirection: "column", gap: 20 } as const;
export const authInputStyle = { height: 48, padding: "0 14px", fontSize: 15 } as const;
