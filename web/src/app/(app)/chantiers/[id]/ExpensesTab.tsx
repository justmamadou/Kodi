"use client";

import Link from "next/link";
import { Card, Empty, ErrorBanner, Loading } from "@/components/ui";
import type { Expense, Paged } from "@/lib/api";
import { date, fcfa } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { expenseBadge } from "./OverviewTab";

export function ExpensesTab({ projectId }: { projectId: string }) {
  const { data, error, loading } = useResource<Paged<Expense>>(`/expenses?project_id=${projectId}&limit=100`);
  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  return (
    <Card>
      {data.items.length === 0 ? (
        <Empty>Aucune dépense sur ce chantier.</Empty>
      ) : (
        <div style={{ overflowX: "auto" }}>
          <table className="tbl">
            <thead>
              <tr>
                <th>Date</th>
                <th>Dépense</th>
                <th className="hide-sm">Saisie par</th>
                <th className="text-right">Montant (FCFA)</th>
                <th>Statut</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((e) => (
                <tr key={e.id}>
                  <td className="num muted" style={{ fontSize: 13 }}>
                    {date(e.expense_date)}
                  </td>
                  <td>
                    <Link href={`/depenses/${e.id}`} style={{ fontWeight: 600, color: "#1D1D1F" }}>
                      {e.description}
                    </Link>
                    <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                      {e.category_name}
                      {e.supplier_name ? ` · ${e.supplier_name}` : ""}
                    </div>
                  </td>
                  <td className="hide-sm">{e.created_by_name}</td>
                  <td className="text-right num" style={{ fontWeight: 600 }}>
                    {fcfa(e.amount)}
                  </td>
                  <td>{expenseBadge(e)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Card>
  );
}
