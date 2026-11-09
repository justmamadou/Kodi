"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useMemo, useState } from "react";
import { IconCheck, IconClose } from "@/components/icons";
import { Button, ErrorBanner, fieldError, Input, Select, Textarea } from "@/components/ui";
import type { Category, Directory, Items, Project } from "@/lib/api";
import { api } from "@/lib/api";
import { fcfa, ROLE_LABEL, today } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

/** « 24 000 000 » ou « 24000000 » → 24000000. */
function parseAmount(text: string): number {
  const digits = text.replace(/\D/g, "");
  return digits ? Number(digits) : 0;
}

function formatInput(text: string): string {
  const value = parseAmount(text);
  return value ? fcfa(value) : "";
}

export default function NewProjectPage() {
  const router = useRouter();
  const { user } = useSession();
  const directory = useResource<Items<Directory>>("/directory");
  const categories = useResource<Items<Category>>("/categories");
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<string | null>(null);
  const [total, setTotal] = useState("");
  const [amounts, setAmounts] = useState<Record<string, string>>({});
  const [newCategory, setNewCategory] = useState<string | null>(null);
  const [status, setStatus] = useState("in_progress");
  const [managerId, setManagerId] = useState("");
  const [members, setMembers] = useState<string[]>([]);
  const [adding, setAdding] = useState(false);

  const people = directory.data?.items ?? [];
  const managers = people.filter((p) => p.role === "manager" || p.role === "owner");
  const selectedManager = managerId || managers.find((m) => m.role === "manager")?.id || managers[0]?.id || "";
  const active = useMemo(() => categories.data?.items.filter((c) => c.is_active) ?? [], [categories.data]);
  const totalValue = parseAmount(total);
  const allocated = active.reduce((sum, c) => sum + parseAmount(amounts[c.id] ?? ""), 0);
  const complete = totalValue > 0 && allocated === totalValue;

  async function addCategory() {
    if (!newCategory?.trim()) return;
    setError(null);
    try {
      await api("/categories", { method: "POST", body: { name: newCategory.trim() } });
      setNewCategory(null);
      categories.reload();
    } catch (err) {
      setError(err);
    }
  }

  async function save(event: FormEvent<HTMLFormElement> | null, form: HTMLFormElement, draft: boolean) {
    event?.preventDefault();
    if (!form.reportValidity()) return;
    if (!draft && !complete) {
      setError(new Error(totalValue === 0 ? "Indiquez le budget total du chantier." : "La répartition par catégorie doit être égale au budget total."));
      return;
    }
    const data = new FormData(form);
    setBusy(true);
    setError(null);
    let projectId: string | null = null;
    try {
      const project = await api<Project>("/projects", {
        method: "POST",
        body: {
          reference: data.get("reference"),
          name: data.get("name"),
          client_name: data.get("client_name"),
          address: data.get("address"),
          description: data.get("description") || null,
          status: draft ? "draft" : status,
          start_date: data.get("start_date") || null,
          planned_end_date: data.get("planned_end_date") || null,
          manager_id: selectedManager || null,
        },
      });
      projectId = project.id;
      const lines = active.filter((c) => parseAmount(amounts[c.id] ?? "") > 0);
      if (lines.length > 0) {
        const version = await api<{ id: string }>(`/projects/${project.id}/budget/versions`, { method: "POST" });
        for (const c of lines) {
          await api(`/budget-versions/${version.id}/lines`, {
            method: "POST",
            body: { category_id: c.id, label: c.name, quantity: "1", unit: "forfait", unit_price: parseAmount(amounts[c.id] ?? "") },
          });
        }
        if (!draft) await api(`/budget-versions/${version.id}/publish`, { method: "POST" });
      }
      for (const member of members) {
        await api(`/projects/${project.id}/members/${member}`, { method: "PUT", body: { can_approve: false, approval_limit: null } });
      }
      router.push(`/chantiers/${project.id}`);
    } catch (err) {
      // Le chantier existe déjà : recréer ferait un doublon, on renvoie vers sa page pour terminer.
      setCreated(projectId);
      setError(err);
      setBusy(false);
    }
  }

  return (
    <>
      <div style={{ maxWidth: 1120, width: "100%" }}>
        <nav aria-label="Fil d'Ariane" style={{ fontSize: 14 }}>
          <Link href="/chantiers" style={{ fontWeight: 600 }}>
            Chantiers
          </Link>
          <span className="muted"> / Nouveau chantier</span>
        </nav>
        <h1 className="page-h1" style={{ marginTop: 10 }}>
          Nouveau chantier
        </h1>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
          Les champs marqués <span className="req">*</span> sont obligatoires. Vous pourrez réviser le budget plus tard : chaque révision est historisée.
        </p>
      </div>
      <form onSubmit={(e) => save(e, e.currentTarget, false)} style={{ maxWidth: 1120, width: "100%", display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <div style={{ flex: "999 1 520px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s1">
            <h2 id="s1" className="sec-t">
              Informations du chantier
            </h2>
            <Input label="Nom du chantier" name="name" required error={fieldError(error, "name")} />
            <div className="g2">
              <Input label="Référence" name="reference" required placeholder="CH-2026-014" error={fieldError(error, "reference")} />
              <Input label="Client / maître d'ouvrage" name="client_name" required error={fieldError(error, "client_name")} />
            </div>
            <Input label="Adresse du chantier" name="address" required error={fieldError(error, "address")} />
            <Textarea label="Description" name="description" />
          </section>
          <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s2">
            <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: 12, flexWrap: "wrap" }}>
              <h2 id="s2" className="sec-t">
                Budget prévisionnel
              </h2>
              <span className="hint">Devise de l&apos;entreprise : FCFA (XOF)</span>
            </div>
            <div className="f" style={{ maxWidth: 320 }}>
              <label htmlFor="bud">
                Budget total <span className="req">*</span>
              </label>
              <div style={{ position: "relative" }}>
                <input id="bud" className="inp num" inputMode="numeric" value={total} onChange={(e) => setTotal(formatInput(e.target.value))} style={{ paddingRight: 64, fontWeight: 600 }} />
                <span className="muted" style={{ position: "absolute", right: 12, top: 12, fontSize: 13, fontWeight: 600 }}>
                  FCFA
                </span>
              </div>
            </div>
            <div>
              <div className="lbl" style={{ marginBottom: 4 }}>
                Répartition par catégorie
              </div>
              <table className="tbl">
                <thead>
                  <tr>
                    <th style={{ paddingLeft: 0 }}>Catégorie</th>
                    <th className="text-right" style={{ width: 200, paddingRight: 0 }}>
                      Montant prévu (FCFA)
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {active.map((c) => (
                    <tr key={c.id}>
                      <td style={{ paddingLeft: 0 }}>{c.name}</td>
                      <td style={{ paddingRight: 0, paddingTop: 8, paddingBottom: 8 }}>
                        <input aria-label={c.name} className="inp num" inputMode="numeric" value={amounts[c.id] ?? ""} onChange={(e) => setAmounts((a) => ({ ...a, [c.id]: formatInput(e.target.value) }))} style={{ height: 36, textAlign: "right" }} />
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {totalValue > 0 && (
                <div className={complete ? "ok" : "warn"} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, marginTop: 12, padding: "12px 14px", borderRadius: 8, fontSize: 14, fontWeight: 600, flexWrap: "wrap" }}>
                  <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                    {complete && <IconCheck size={18} />}
                    {complete ? "Répartition complète" : allocated > totalValue ? "Répartition supérieure au budget" : "Reste à répartir"}
                  </span>
                  <span className="num">
                    {fcfa(allocated)} / {fcfa(totalValue)} FCFA
                  </span>
                </div>
              )}
              {can.manageOrg(user.role) &&
                (newCategory === null ? (
                  <button type="button" className="btn btn-g" style={{ padding: 0, marginTop: 8, color: "#2B59E6" }} onClick={() => setNewCategory("")}>
                    + Ajouter une catégorie
                  </button>
                ) : (
                  <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                    <input aria-label="Nom de la catégorie" className="inp" style={{ flex: "1 1 200px", width: "auto" }} value={newCategory} onChange={(e) => setNewCategory(e.target.value)} placeholder="Nom de la catégorie" />
                    <Button variant="secondary" onClick={addCategory}>
                      Ajouter
                    </Button>
                    <Button variant="ghost" onClick={() => setNewCategory(null)}>
                      Annuler
                    </Button>
                  </div>
                ))}
            </div>
          </section>
        </div>
        <div style={{ flex: "1 1 300px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s3">
            <h2 id="s3" className="sec-t">
              Planning
            </h2>
            <Input label="Date de démarrage" name="start_date" type="date" required defaultValue={today()} error={fieldError(error, "start_date")} />
            <Input label="Date de fin prévue" name="planned_end_date" type="date" error={fieldError(error, "planned_end_date")} />
            <fieldset style={{ border: 0, margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
              <legend className="lbl" style={{ marginBottom: 8 }}>
                Statut <span className="req">*</span>
              </legend>
              {[
                ["draft", "Brouillon"],
                ["planned", "Planifié"],
                ["in_progress", "En cours"],
              ].map(([value, label]) => (
                <label key={value} style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
                  <input type="radio" name="status" checked={status === value} onChange={() => setStatus(value ?? "draft")} /> {label}
                </label>
              ))}
            </fieldset>
          </section>
          <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s4">
            <h2 id="s4" className="sec-t">
              Équipe du chantier
            </h2>
            <Select label="Conducteur responsable" required value={selectedManager} onChange={(e) => setManagerId(e.target.value)} error={fieldError(error, "manager_id")}>
              {managers.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.full_name} — {ROLE_LABEL[m.role]}
                </option>
              ))}
            </Select>
            <div>
              <div className="lbl" style={{ marginBottom: 8 }}>
                Autres membres
              </div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                {members.map((id) => {
                  const person = people.find((p) => p.id === id);
                  return (
                    <span key={id} className="bdg mut" style={{ fontSize: 13, padding: "4px 4px 4px 12px" }}>
                      {person?.full_name} · {ROLE_LABEL[person?.role ?? ""]}
                      <button type="button" aria-label={`Retirer ${person?.full_name}`} onClick={() => setMembers((m) => m.filter((x) => x !== id))} style={{ width: 24, height: 24, border: 0, borderRadius: "50%", background: "transparent", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center" }}>
                        <IconClose size={13} />
                      </button>
                    </span>
                  );
                })}
              </div>
              {adding ? (
                <select
                  className="sel"
                  aria-label="Ajouter un membre"
                  style={{ marginTop: 12, width: "100%" }}
                  defaultValue=""
                  onChange={(e) => {
                    const id = e.target.value;
                    if (id) setMembers((m) => [...m, id]);
                    setAdding(false);
                  }}
                >
                  <option value="">Choisir…</option>
                  {people
                    .filter((p) => p.id !== selectedManager && !members.includes(p.id))
                    .map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.full_name} · {ROLE_LABEL[p.role]}
                      </option>
                    ))}
                </select>
              ) : (
                <button type="button" className="btn btn-s" style={{ marginTop: 12, height: 36 }} onClick={() => setAdding(true)}>
                  + Ajouter un membre
                </button>
              )}
              <p className="hint" style={{ margin: "10px 0 0" }}>
                Seuls les membres assignés voient ce chantier.
              </p>
            </div>
          </section>
        </div>
        <div style={{ flex: "1 1 100%", display: "flex", flexDirection: "column", gap: 12, paddingTop: 16, borderTop: "1px solid #E5E5EA" }}>
          <ErrorBanner error={error} />
          {created ? (
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, flexWrap: "wrap", alignItems: "center" }}>
              <span className="muted" style={{ fontSize: 14 }}>
                Le chantier a été créé, mais la suite n&apos;a pas abouti.
              </span>
              <Link href={`/chantiers/${created}`} className="btn btn-p">
                Terminer sur la page du chantier
              </Link>
            </div>
          ) : (
            <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, flexWrap: "wrap" }}>
              <Link href="/chantiers" className="btn btn-g">
                Annuler
              </Link>
              <button
                type="button"
                className="btn btn-s"
                disabled={busy}
                onClick={(e) => {
                  const form = e.currentTarget.form;
                  if (form) void save(null, form, true);
                }}
              >
                Enregistrer en brouillon
              </button>
              <button type="submit" className="btn btn-p" disabled={busy}>
                {busy ? "Création…" : "Créer le chantier"}
              </button>
            </div>
          )}
        </div>
      </form>
    </>
  );
}
