"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { type ReactNode, useEffect, useState } from "react";
import { ApiError, api, cachedGet, type ExpenseCounts } from "@/lib/api";
import { ROLE_LABEL } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { clearLocalData, db } from "@/lib/offline/db";
import { useAutoSync } from "@/lib/offline/useSync";
import { can, SessionContext, type Session } from "@/lib/session";
import { IconBars, IconBell, IconBox, IconBuilding, IconCard, IconDoc, IconDocLines, IconGear, IconHelmet, IconHome, IconTruck, IconUser, IconUsers } from "./icons";
import { Avatar, Loading } from "./ui";

function isActive(pathname: string, href: string) {
  return href === "/" ? pathname === "/" : pathname === href || pathname.startsWith(`${href}/`);
}

/** Les formulaires de terrain ont leur propre en-tête et occupent tout l'écran du téléphone. */
function isFullScreenForm(pathname: string) {
  return pathname === "/depenses/nouvelle" || pathname === "/rapports/nouveau";
}

export function AppShell({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [fatal, setFatal] = useState<string | null>(null);
  const router = useRouter();
  const pathname = usePathname();
  const online = useOnline();
  const { pending, problems } = useAutoSync();

  useEffect(() => {
    cachedGet<Session>("/auth/me").then(
      ({ data }) => setSession(data),
      (err: unknown) => {
        if (err instanceof ApiError && err.status === 401) {
          router.replace("/connexion");
        } else {
          setFatal("Connectez-vous une première fois avec du réseau pour utiliser l'application hors connexion.");
        }
      },
    );
  }, [router]);

  if (fatal) {
    return (
      <p className="muted" style={{ maxWidth: 420, margin: "0 auto", padding: 32, textAlign: "center", fontSize: 14 }}>
        {fatal}
      </p>
    );
  }
  if (!session) return <Loading />;

  const fullScreen = isFullScreenForm(pathname);
  return (
    <SessionContext.Provider value={session}>
      <div style={{ display: "flex", flexWrap: "wrap", minHeight: "100dvh" }}>
        <SideNav session={session} pathname={pathname} pending={pending + problems} />
        {fullScreen ? (
          <div className="min-w-0" style={{ flex: "999 1 560px" }}>
            {children}
          </div>
        ) : (
          <main className="app-main">
            {!online && (
              <p role="status" className="info wrap" style={{ margin: 0, padding: "10px 14px", borderRadius: 10, fontSize: 14 }}>
                <strong>Hors connexion.</strong> Vos saisies sont gardées sur cet appareil et partiront au retour du réseau.
              </p>
            )}
            {online && problems > 0 && (
              <Link href="/synchro" className="err wrap" style={{ display: "block", padding: "10px 14px", borderRadius: 10, fontSize: 14, fontWeight: 600 }}>
                {problems} envoi(s) à vérifier
              </Link>
            )}
            {children}
          </main>
        )}
      </div>
      {!fullScreen && <MobileTabs pathname={pathname} waiting={pending + problems} />}
    </SessionContext.Provider>
  );
}

function SideNav({ session, pathname, pending }: { session: Session; pathname: string; pending: number }) {
  const role = session.user.role;
  const counts = useResource<ExpenseCounts>(can.approve(role) ? "/expenses/counts" : null);
  const toValidate = counts.data?.pending ?? 0;
  const router = useRouter();

  async function logout() {
    const waiting = await db.outbox.where("state").anyOf("pending", "failed", "conflict").count();
    if (waiting > 0 && !window.confirm(`${waiting} élément(s) n'ont pas encore été envoyés et seront perdus. Se déconnecter quand même ?`)) return;
    await api("/auth/logout", { method: "POST" }).catch(() => undefined);
    await clearLocalData();
    router.replace("/connexion");
  }

  const item = (href: string, label: string, icon: ReactNode, badge?: ReactNode) => (
    <Link key={href} href={href} className={`nv ${isActive(pathname, href) ? "on" : ""}`} aria-current={isActive(pathname, href) ? "page" : undefined}>
      {icon}
      {label}
      {badge}
    </Link>
  );

  return (
    <aside className="app-side">
      <Link href="/" style={{ display: "flex", alignItems: "center", gap: 10, padding: "0 8px", color: "#1D1D1F" }}>
        <span style={{ width: 30, height: 30, borderRadius: 8, background: "#1D1D1F", color: "#FFC21A", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <IconHelmet size={17} width={2.2} />
        </span>
        <span style={{ fontSize: 16, fontWeight: 700, letterSpacing: "-0.02em" }}>Kodi</span>
      </Link>
      <nav aria-label="Menu principal" style={{ display: "flex", flexDirection: "column", gap: 2 }}>
        {item("/", "Accueil", <IconHome />)}
        {item("/chantiers", "Chantiers", <IconBuilding />)}
        {item("/depenses", "Dépenses", <IconCard />, toValidate > 0 ? <span className="nv-bdg">{toValidate}</span> : undefined)}
        {item("/materiaux", "Matériaux", <IconBox />)}
        {can.viewRFQ(role) && item("/achats", "Demandes de prix", <IconDocLines />)}
        {item("/commandes", "Commandes", <IconDoc />)}
        {can.viewFinance(role) && item("/rapports", "Rapports", <IconBars />)}
        <div className="s2" style={{ fontSize: 12, fontWeight: 600, padding: "16px 10px 6px" }}>
          Organisation
        </div>
        {can.manageMembers(role) && item("/equipe", "Équipe", <IconUsers />)}
        {item("/fournisseurs", "Fournisseurs", <IconTruck />)}
        {item("/parametres", "Paramètres", <IconGear />)}
        {pending > 0 && item("/synchro", "Envois", <IconBell />, <span className="nv-bdg" style={{ background: "#C76A00" }}>{pending}</span>)}
      </nav>
      <div style={{ marginTop: "auto", padding: 8 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
          <Avatar name={session.user.full_name} background={role === "owner" ? "#FFC21A" : "#D2D2D7"} />
          <div style={{ minWidth: 0 }}>
            <div style={{ fontSize: 13, fontWeight: 600, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{session.user.full_name}</div>
            <div className="s2" style={{ fontSize: 12, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {role === "owner" ? session.organization.name : ROLE_LABEL[role]}
            </div>
          </div>
        </div>
        <button type="button" onClick={logout} className="s2" style={{ border: 0, background: "transparent", cursor: "pointer", fontSize: 12, padding: "8px 0 0", textDecoration: "underline" }}>
          Se déconnecter
        </button>
      </div>
    </aside>
  );
}

/** Barre d'onglets de la maquette 08. */
function MobileTabs({ pathname, waiting }: { pathname: string; waiting: number }) {
  const tab = (href: string, label: string, icon: ReactNode, badge?: number) => (
    <Link href={href} className={`mtab ${isActive(pathname, href) ? "on" : ""}`} aria-current={isActive(pathname, href) ? "page" : undefined}>
      {icon}
      {label}
      {badge ? (
        <span style={{ position: "absolute", top: 4, left: "55%", minWidth: 16, height: 16, borderRadius: 8, background: "#C76A00", color: "#fff", fontSize: 10, display: "flex", alignItems: "center", justifyContent: "center", padding: "0 4px" }}>{badge}</span>
      ) : null}
    </Link>
  );
  return (
    <nav
      aria-label="Navigation principale"
      className="m-tabbar"
      style={{ position: "fixed", left: 0, right: 0, bottom: 0, zIndex: 30, display: "flex", padding: "0 6px calc(env(safe-area-inset-bottom) + 8px)", background: "rgba(249,249,251,.82)", backdropFilter: "blur(20px) saturate(180%)", WebkitBackdropFilter: "blur(20px) saturate(180%)", borderTop: "0.5px solid rgba(0,0,0,.12)" }}
    >
      {tab("/", "Accueil", <svg width="25" height="25" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 3l9 8h-2.5v9.5h-5v-6h-3v6h-5V11H3z" /></svg>)}
      {tab("/chantiers", "Chantiers", <IconBuilding size={25} />)}
      {tab(
        "/ajouter",
        "Ajouter",
        <svg width="25" height="25" viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="12" r="10.5" fill="#2B59E6" /><path d="M12 7.5v9M7.5 12h9" stroke="#FFFFFF" strokeWidth="2.2" strokeLinecap="round" /></svg>,
        waiting,
      )}
      {tab("/alertes", "Alertes", <IconBell size={25} />)}
      {tab("/profil", "Profil", <IconUser size={25} />)}
    </nav>
  );
}
