"use client";

import { useState } from "react";
import { Badge, Button, Card, Empty, ErrorBanner, Input, Loading, Select } from "@/components/ui";
import type { Directory, Items, Project, ProjectMember } from "@/lib/api";
import { api } from "@/lib/api";
import { fcfa, ROLE_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

export function TeamTab({ project }: { project: Project }) {
  const { user } = useSession();
  const { data, error, loading, reload } = useResource<Items<ProjectMember>>(`/projects/${project.id}/members`);
  const manage = can.manageProjects(user.role);
  // L'annuaire ne donne que le nom et le rôle des comptes actifs.
  const everyone = useResource<Items<Directory>>(manage ? "/directory" : null);
  const [userId, setUserId] = useState("");
  const [canApprove, setCanApprove] = useState(false);
  const [limit, setLimit] = useState("");
  const [actionError, setActionError] = useState<unknown>(null);
  const editable = can.manageProjects(user.role) && project.status !== "archived";

  async function run(fn: () => Promise<unknown>) {
    setActionError(null);
    try {
      await fn();
      reload();
    } catch (err) {
      setActionError(err);
    }
  }

  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  const assigned = new Set(data.items.map((m) => m.user_id));
  const candidates = everyone.data?.items.filter((u) => !assigned.has(u.id)) ?? [];

  return (
    <div className="flex flex-col gap-4">
      <Card className="p-6">
        <h2 className="mb-2 text-lg font-bold">Équipe du chantier</h2>
        {data.items.length === 0 ? (
          <Empty>Aucun membre affecté.</Empty>
        ) : (
          <ul className="divide-y divide-line">
            {data.items.map((m) => (
              <li key={m.user_id} className="flex flex-wrap items-center justify-between gap-2 py-2">
                <span className="min-w-0">
                  <span className="block truncate font-medium">{m.full_name}</span>
                  <span className="block text-xs text-muted">{ROLE_LABEL[m.role]}</span>
                </span>
                <span className="flex items-center gap-2">
                  {m.can_approve && <Badge tone="blue">Approuve{m.approval_limit !== null ? ` jusqu'à ${fcfa(m.approval_limit)}` : ""}</Badge>}
                  {editable && (
                    <Button variant="danger" onClick={() => run(() => api(`/projects/${project.id}/members/${m.user_id}`, { method: "DELETE" }))}>
                      Retirer
                    </Button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
      {editable && (
        <Card className="p-6">
          <h2 className="mb-3 text-lg font-bold">Affecter un membre</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <Select label="Membre" value={userId} onChange={(e) => setUserId(e.target.value)} hint="Seuls les comptes actifs sont proposés.">
              <option value="">Choisir…</option>
              {candidates.map((u) => (
                <option key={u.id} value={u.id}>
                  {u.full_name} · {ROLE_LABEL[u.role]}
                </option>
              ))}
            </Select>
            {can.manageMembers(user.role) && (
              <label className="flex min-h-11 items-center gap-3 self-end text-sm">
                <input type="checkbox" className="size-5" checked={canApprove} onChange={(e) => setCanApprove(e.target.checked)} />
                Peut approuver les dépenses de ce chantier
              </label>
            )}
            {canApprove && <Input label="Plafond d'approbation (FCFA)" inputMode="numeric" value={limit} onChange={(e) => setLimit(e.target.value)} hint="Laissez vide pour ne fixer aucun plafond." />}
            <div className="sm:col-span-2">
              <Button
                disabled={!userId}
                onClick={() =>
                  run(async () => {
                    await api(`/projects/${project.id}/members/${userId}`, {
                      method: "PUT",
                      body: { can_approve: canApprove, approval_limit: canApprove && limit ? Number(limit) : null },
                    });
                    setUserId("");
                    setCanApprove(false);
                    setLimit("");
                  })
                }
              >
                Affecter
              </Button>
            </div>
          </div>
        </Card>
      )}
      <ErrorBanner error={actionError} />
    </div>
  );
}
