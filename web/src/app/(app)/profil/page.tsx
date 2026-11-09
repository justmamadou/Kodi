"use client";

import { useLiveQuery } from "dexie-react-hooks";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { IconBars, IconBell, IconBox, IconCard, IconChevron, IconDoc, IconDocLines, IconGear, IconTruck, IconUsers } from "@/components/icons";
import { Avatar, PageTitle } from "@/components/ui";
import { api } from "@/lib/api";
import { ROLE_LABEL } from "@/lib/format";
import { clearLocalData, db } from "@/lib/offline/db";
import { can, useSession } from "@/lib/session";

/** Sur téléphone, la barre d'onglets est courte : le reste du menu principal vit ici. */
export default function ProfilePage() {
  const { user, organization } = useSession();
  const router = useRouter();
  const waiting = useLiveQuery(() => db.outbox.where("state").anyOf("pending", "failed", "conflict").count(), [], 0);

  async function logout() {
    if (waiting > 0 && !window.confirm(`${waiting} élément(s) n'ont pas encore été envoyés et seront perdus. Se déconnecter quand même ?`)) return;
    await api("/auth/logout", { method: "POST" }).catch(() => undefined);
    await clearLocalData();
    router.replace("/connexion");
  }

  const entries = [
    { href: "/depenses", label: "Dépenses", icon: <IconCard size={18} />, show: true },
    { href: "/materiaux", label: "Matériaux", icon: <IconBox size={18} />, show: true },
    { href: "/achats", label: "Demandes de prix", icon: <IconDocLines size={18} />, show: can.viewRFQ(user.role) },
    { href: "/commandes", label: "Commandes", icon: <IconDoc size={18} />, show: true },
    { href: "/rapports", label: "Rapports", icon: <IconBars size={18} />, show: can.viewFinance(user.role) },
    { href: "/equipe", label: "Équipe", icon: <IconUsers size={18} />, show: can.manageMembers(user.role) },
    { href: "/fournisseurs", label: "Fournisseurs", icon: <IconTruck size={18} />, show: true },
    { href: "/parametres", label: "Paramètres", icon: <IconGear size={18} />, show: true },
    { href: "/synchro", label: waiting > 0 ? `Envois (${waiting} en attente)` : "Envois", icon: <IconBell size={18} />, show: true },
  ].filter((e) => e.show);

  return (
    <div style={{ maxWidth: 560, width: "100%", margin: "0 auto", display: "flex", flexDirection: "column", gap: 20 }}>
      <PageTitle title="Profil" />
      <section className="grp" style={{ padding: 16, display: "flex", alignItems: "center", gap: 14 }}>
        <Avatar name={user.full_name} size={48} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontSize: 17, fontWeight: 600 }}>{user.full_name}</div>
          <div className="s2" style={{ fontSize: 14 }}>
            {ROLE_LABEL[user.role]} · {organization.name}
          </div>
          <div className="s2" style={{ fontSize: 13, overflow: "hidden", textOverflow: "ellipsis" }}>
            {user.email ?? user.phone}
          </div>
        </div>
      </section>
      <div className="grp">
        {entries.map((e) => (
          <Link key={e.href} href={e.href} className="cell">
            <span style={{ width: 30, height: 30, borderRadius: 8, background: "#F2F2F7", color: "#48484A", display: "flex", alignItems: "center", justifyContent: "center" }}>{e.icon}</span>
            <span style={{ flex: 1 }}>{e.label}</span>
            <span style={{ color: "#C7C7CC" }}>
              <IconChevron size={14} width={3} />
            </span>
          </Link>
        ))}
      </div>
      <button type="button" className="btn btn-d" onClick={logout} style={{ height: 48 }}>
        Se déconnecter
      </button>
    </div>
  );
}
