"use client";

import { type FormEvent, useState } from "react";
import { Badge, Button, Card, Empty, ErrorBanner, fieldError, Input, Loading, Select } from "@/components/ui";
import type { BudgetDetail, BudgetVersion, Category, Comparison, Items, Project } from "@/lib/api";
import { api } from "@/lib/api";
import { dateTime, fcfa } from "@/lib/format";
import { useOnline, useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

const VERSION_LABEL: Record<string, string> = { draft: "Brouillon", published: "Publiée", superseded: "Remplacée", discarded: "Abandonnée" };
const CHANGE_LABEL: Record<string, string> = { added: "Ajoutée", removed: "Retirée", changed: "Modifiée", unchanged: "Inchangée" };

export function BudgetTab({ project, onChange }: { project: Project; onChange: () => void }) {
  const { user } = useSession();
  const online = useOnline();
  const { data, error, loading, reload } = useResource<Items<BudgetVersion>>(`/projects/${project.id}/budget/versions`);
  const [selected, setSelected] = useState<string | null>(null);
  const [compare, setCompare] = useState<Comparison | null>(null);
  const [actionError, setActionError] = useState<unknown>(null);
  const [reason, setReason] = useState("");

  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;

  const versions = data.items;
  const draft = versions.find((v) => v.status === "draft");
  const published = versions.find((v) => v.status === "published");
  const currentId = selected ?? draft?.id ?? published?.id ?? null;
  const editable = can.editBudget(user.role) && project.status !== "archived" && online;

  async function run(fn: () => Promise<unknown>) {
    setActionError(null);
    try {
      await fn();
      reload();
      onChange();
    } catch (err) {
      setActionError(err);
    }
  }

  async function showComparison(version: BudgetVersion) {
    const previous = versions.find((v) => v.version_no === version.version_no - 1);
    if (!previous) return;
    setActionError(null);
    try {
      setCompare(await api<Comparison>(`/budget-versions/compare?from=${previous.id}&to=${version.id}`));
    } catch (err) {
      setActionError(err);
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {!online && <p className="text-sm text-warn-ink">Connexion requise pour modifier ou publier un budget.</p>}
      <ErrorBanner error={actionError} />
      {versions.length === 0 ? (
        <Card className="p-6">
          <Empty>Ce chantier n&apos;a pas encore de budget.</Empty>
          {editable && (
            <div className="text-center">
              <Button onClick={() => run(() => api(`/projects/${project.id}/budget/versions`, { method: "POST" }))}>Créer le budget</Button>
            </div>
          )}
        </Card>
      ) : (
        currentId && <VersionEditor key={currentId} versionId={currentId} editable={editable} onChange={() => { reload(); onChange(); }} />
      )}

      {editable && published && !draft && (
        <Card className="p-6">
          <h2 className="mb-2 text-lg font-bold">Réviser le budget</h2>
          <p className="mb-3 text-sm text-muted">La version publiée reste conservée. Les dépenses déjà approuvées ne sont pas réaffectées.</p>
          <form
            className="flex flex-col gap-3 sm:flex-row sm:items-end"
            onSubmit={(e: FormEvent) => {
              e.preventDefault();
              void run(async () => {
                await api(`/budget-versions/${published.id}/revise`, { method: "POST", body: { reason } });
                setReason("");
                setSelected(null);
              });
            }}
          >
            <div className="min-w-0 flex-1">
              <Input label="Motif de la révision" required value={reason} onChange={(e) => setReason(e.target.value)} error={fieldError(actionError, "reason")} />
            </div>
            <Button type="submit" variant="secondary" disabled={reason.trim().length < 3}>
              Créer la révision
            </Button>
          </form>
        </Card>
      )}

      {versions.length > 0 && (
        <Card className="p-6">
          <h2 className="mb-2 text-lg font-bold">Historique des versions</h2>
          <ul className="divide-y divide-line">
            {versions.map((v) => (
              <li key={v.id} className="flex flex-wrap items-center justify-between gap-2 py-2">
                <button type="button" onClick={() => setSelected(v.id)} className={`min-h-10 min-w-0 text-left ${v.id === currentId ? "font-bold" : "font-medium"}`}>
                  v{v.version_no} · <span className="tabular">{fcfa(v.total_amount)} FCFA</span>
                  {v.revision_reason && <span className="block text-xs font-normal text-muted">{v.revision_reason}</span>}
                  {v.published_at && (
                    <span className="block text-xs font-normal text-muted">
                      Publiée le {dateTime(v.published_at)}
                      {v.published_by_name ? ` par ${v.published_by_name}` : ""}
                    </span>
                  )}
                </button>
                <span className="flex items-center gap-2">
                  <Badge tone={v.status === "published" ? "green" : v.status === "draft" ? "orange" : "neutral"}>{VERSION_LABEL[v.status]}</Badge>
                  {v.version_no > 1 && (
                    <Button variant="secondary" onClick={() => showComparison(v)}>
                      Comparer
                    </Button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        </Card>
      )}

      {compare && (
        <Card className="p-6">
          <h2 className="text-lg font-bold">
            v{compare.from_version_no} → v{compare.to_version_no}
          </h2>
          <p className="tabular mb-3 text-sm text-muted">
            {fcfa(compare.from_total)} → {fcfa(compare.to_total)} FCFA ({compare.delta >= 0 ? "+" : ""}
            {fcfa(compare.delta)})
          </p>
          <div className="overflow-x-auto">
            <table className="tbl">
              <thead>
                <tr>
                  <th >Ligne</th>
                  <th >Évolution</th>
                  <th className="text-right">Avant</th>
                  <th className="text-right">Après</th>
                  <th className="text-right">Écart</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {compare.lines.map((l) => (
                  <tr key={l.line_key}>
                    <td >{l.label}</td>
                    <td >{CHANGE_LABEL[l.change]}</td>
                    <td className="text-right num">{fcfa(l.from_amount)}</td>
                    <td className="text-right num">{fcfa(l.to_amount)}</td>
                    <td className="text-right num">{fcfa(l.delta)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </Card>
      )}
    </div>
  );
}

function VersionEditor({ versionId, editable, onChange }: { versionId: string; editable: boolean; onChange: () => void }) {
  const { data, error, loading, reload } = useResource<BudgetDetail>(`/budget-versions/${versionId}`);
  const categories = useResource<Items<Category>>("/categories");
  const [actionError, setActionError] = useState<unknown>(null);
  const [formKey, setFormKey] = useState(0);

  if (loading) return <Loading />;
  if (!data) return <ErrorBanner error={error} />;
  const isDraft = data.status === "draft";

  async function run(fn: () => Promise<unknown>) {
    setActionError(null);
    try {
      await fn();
      reload();
      onChange();
    } catch (err) {
      setActionError(err);
    }
  }

  function publish() {
    if (window.confirm(`Publier la version ${data?.version_no} pour ${fcfa(data?.total_amount ?? 0)} FCFA ? Elle deviendra la référence du chantier et ne pourra plus être modifiée.`)) {
      void run(() => api(`/budget-versions/${versionId}/publish`, { method: "POST" }));
    }
  }

  return (
    <Card className="p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-lg font-bold">Version {data.version_no}</h2>
          <p className="tabular text-2xl font-bold">
            {fcfa(data.total_amount)} <span className="text-sm font-semibold text-muted">FCFA</span>
          </p>
        </div>
        {isDraft && editable && (
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => window.confirm("Abandonner ce brouillon ?") && void run(() => api(`/budget-versions/${versionId}/discard`, { method: "POST" }))}>
              Abandonner
            </Button>
            <Button onClick={publish} disabled={data.lines.length === 0}>
              Publier
            </Button>
          </div>
        )}
      </div>
      {data.lines.length === 0 ? (
        <Empty>Aucune ligne. Un budget doit en contenir au moins une pour être publié.</Empty>
      ) : (
        <div className="overflow-x-auto">
          <table className="tbl">
            <thead>
              <tr>
                <th >Libellé</th>
                <th >Catégorie</th>
                <th className="text-right">Quantité</th>
                <th className="text-right">Prix unitaire</th>
                <th className="text-right">Montant</th>
                {isDraft && editable && <th />}
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {data.lines.map((l) => (
                <tr key={l.id}>
                  <td >{l.label}</td>
                  <td className="muted">{l.category_name}</td>
                  <td className="text-right num">
                    {l.quantity} {l.unit}
                  </td>
                  <td className="text-right num">{fcfa(l.unit_price)}</td>
                  <td className="text-right num font-semibold">{fcfa(l.amount)}</td>
                  {isDraft && editable && (
                    <td className="text-right num">
                      <button type="button" aria-label={`Retirer ${l.label}`} className="min-h-10 px-2 text-bad-ink" onClick={() => run(() => api(`/budget-lines/${l.id}`, { method: "DELETE" }))}>
                        ✕
                      </button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {isDraft && editable && (
        <form
          key={formKey}
          noValidate
          className="mt-4 grid gap-3 sm:grid-cols-5"
          onSubmit={(e: FormEvent<HTMLFormElement>) => {
            e.preventDefault();
            const form = new FormData(e.currentTarget);
            void run(async () => {
              await api(`/budget-versions/${versionId}/lines`, {
                method: "POST",
                body: {
                  category_id: form.get("category_id") || null,
                  label: form.get("label"),
                  quantity: String(form.get("quantity")).replace(",", "."),
                  unit: form.get("unit"),
                  unit_price: Number(form.get("unit_price")),
                },
              });
              setFormKey((k) => k + 1);
            });
          }}
        >
          <div className="sm:col-span-2">
            <Input label="Libellé" name="label" required error={fieldError(actionError, "label")} />
          </div>
          <div className="sm:col-span-3">
            <Select label="Catégorie" name="category_id" required defaultValue="" error={fieldError(actionError, "category_id")}>
              <option value="">Choisir…</option>
              {categories.data?.items
                .filter((c) => c.is_active)
                .map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.name}
                  </option>
                ))}
            </Select>
          </div>
          <Input label="Quantité" name="quantity" inputMode="decimal" required error={fieldError(actionError, "quantity")} />
          <Input label="Unité" name="unit" required placeholder="sac" error={fieldError(actionError, "unit")} />
          <Input label="Prix unitaire" name="unit_price" inputMode="numeric" required error={fieldError(actionError, "unit_price")} />
          <div className="flex items-end sm:col-span-2">
            <Button type="submit" variant="secondary" className="w-full">
              Ajouter la ligne
            </Button>
          </div>
        </form>
      )}
      {actionError ? (
        <div className="mt-3">
          <ErrorBanner error={actionError} />
        </div>
      ) : null}
    </Card>
  );
}
