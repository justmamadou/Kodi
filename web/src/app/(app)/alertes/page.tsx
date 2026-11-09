"use client";

import Link from "next/link";
import { IconAlert, IconCheck, IconChevron, IconDoc, IconBox, IconWarn } from "@/components/icons";
import { Card, Empty, ErrorBanner, Loading, PageTitle } from "@/components/ui";
import type { Dashboard, Expense, Paged } from "@/lib/api";
import { fcfa, pct } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

/** Les alertes sont calculées à la lecture depuis les données : il n'y a pas encore de notifications poussées. */
export default function AlertsPage() {
  const { user } = useSession();
  return (
    <>
      <PageTitle title="Alertes" subtitle="Ce qui demande votre attention, à jour à chaque ouverture." />
      <Card style={{ padding: "8px 20px" }} className="wrap">
        {can.viewFinance(user.role) ? <FinanceAlerts /> : <FieldAlerts />}
      </Card>
    </>
  );
}

function Row({ href, color, icon, title, text }: { href: string; color: string; icon: React.ReactNode; title: string; text: string }) {
  return (
    <Link href={href} className="dli">
      <span className="sq" style={{ background: color }}>
        {icon}
      </span>
      <span style={{ flex: 1, minWidth: 0 }}>
        <span style={{ display: "block", fontSize: 15, fontWeight: 600 }}>{title}</span>
        <span className="s2" style={{ display: "block", fontSize: 13 }}>
          {text}
        </span>
      </span>
      <span style={{ color: "#AEAEB2" }}>
        <IconChevron />
      </span>
    </Link>
  );
}

function FinanceAlerts() {
  const { data, error, loading } = useResource<Dashboard>("/dashboard");
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  const proc = data.to_process;
  const risky = data.projects.filter((p) => p.signal === "critical" || p.signal === "watch");
  if (proc.pending_count === 0 && risky.length === 0 && proc.missing_reports.length === 0) return <Empty>Aucune alerte pour le moment.</Empty>;
  return (
    <>
      {proc.pending_count > 0 && (
        <Row href="/depenses" color="#2B59E6" icon={<IconCheck size={18} />} title={`${proc.pending_count} dépense(s) à valider`} text={`${fcfa(proc.pending_amount)} FCFA${proc.duplicate_count > 0 ? ` · ${proc.duplicate_count} doublon(s) possible(s)` : ""}`} />
      )}
      {risky.map((p) => (
        <Row key={p.project_id} href={`/chantiers/${p.project_id}`} color={p.signal === "critical" ? "#D93025" : "#C76A00"} icon={p.signal === "critical" ? <IconAlert size={18} /> : <IconWarn size={18} />} title={`${p.name} à ${pct(p.approved, p.budget)} % du budget`} text={`Pour ${p.declared_progress_pct} % d'avancement. Ce signal invite à vérifier.`} />
      ))}
      {proc.missing_reports.map((m) => (
        <Row key={m.id} href={`/chantiers/${m.id}`} color="#C76A00" icon={<IconDoc size={18} />} title="Rapport manquant" text={`${m.name} · hier`} />
      ))}
    </>
  );
}

function FieldAlerts() {
  const { user } = useSession();
  const { data, error, loading } = useResource<Paged<Expense>>("/expenses?status=rejected&limit=50");
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  const mine = data.items.filter((e) => e.created_by === user.id);
  if (mine.length === 0) return <Empty>Aucune alerte pour le moment.</Empty>;
  return (
    <>
      {mine.map((e) => (
        <Row key={e.id} href={`/depenses/${e.id}`} color="#D93025" icon={<IconBox size={18} />} title={`Dépense rejetée : ${e.description}`} text={e.decision_comment ?? "Motif non précisé"} />
      ))}
    </>
  );
}
