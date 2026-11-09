"use client";

import Link from "next/link";
import { IconPlus } from "@/components/icons";
import { Badge, Card, Empty, ErrorBanner, Loading } from "@/components/ui";
import type { DailyReport, Items, Project } from "@/lib/api";
import { date, WEATHER_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

export function ReportsTab({ project }: { project: Project }) {
  const { user } = useSession();
  const { data, error, loading } = useResource<Items<DailyReport>>(`/projects/${project.id}/daily-reports`);
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <Card style={{ padding: "18px 20px" }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, flexWrap: "wrap" }}>
        <h2 className="sec-t">Rapports journaliers</h2>
        {project.status !== "archived" && can.writeReport(user.role) && (
          <Link href={`/rapports/nouveau?project=${project.id}`} className="btn btn-s">
            <IconPlus size={15} />
            Nouveau rapport
          </Link>
        )}
      </div>
      {data.items.length === 0 ? (
        <Empty>Aucun rapport journalier.</Empty>
      ) : (
        data.items.map((r) => (
          <Link key={r.id} href={`/rapports/${r.id}`} className="li" style={{ color: "#1D1D1F" }}>
            <span style={{ minWidth: 0 }}>
              {date(r.report_date)}
              {r.team_label ? ` · ${r.team_label}` : ""}
              <span className="muted" style={{ display: "block", fontSize: 12 }}>
                {[r.work_done.slice(0, 90), r.workforce_count !== null ? `${r.workforce_count} ouvriers` : null, r.weather ? WEATHER_LABEL[r.weather] : null].filter(Boolean).join(" · ")}
              </span>
            </span>
            <Badge tone={r.status === "submitted" ? "green" : "orange"}>{r.status === "submitted" ? "Publié" : "Brouillon"}</Badge>
          </Link>
        ))
      )}
    </Card>
  );
}
