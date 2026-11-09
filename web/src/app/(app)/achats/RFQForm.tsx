"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useEffect, useState } from "react";
import { IconClose, IconPlus } from "@/components/icons";
import { ErrorBanner, fieldError, Input, Select, Textarea } from "@/components/ui";
import type { Items, Material, Project, RFQDetail, Supplier } from "@/lib/api";
import { api, ApiError, fetchAll } from "@/lib/api";
import { SERVICE_UNIT_LABEL, UNIT_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";

interface Line {
  key: number;
  kind: "material" | "service";
  material_id: string;
  label: string;
  quantity: string;
  unit: string;
  specifications: string;
  needed_by: string;
}

let nextKey = 1;
const blankLine = (): Line => ({ key: nextKey++, kind: "material", material_id: "", label: "", quantity: "", unit: "sac", specifications: "", needed_by: "" });

function linesFrom(rfq?: RFQDetail): Line[] {
  if (!rfq || rfq.items.length === 0) return [blankLine()];
  return rfq.items.map((i) => ({
    key: nextKey++,
    kind: i.kind,
    material_id: i.material_id ?? "",
    label: i.label,
    quantity: i.quantity.replace(".", ","),
    unit: i.unit,
    specifications: i.specifications ?? "",
    needed_by: i.needed_by ?? "",
  }));
}

/** Création et modification d'un brouillon : le contenu est remplacé en entier à chaque enregistrement. */
export function RFQForm({ rfq, projectId }: { rfq?: RFQDetail; projectId?: string }) {
  const router = useRouter();
  const [projects, setProjects] = useState<Project[]>([]);
  const materials = useResource<Items<Material>>("/materials");
  const suppliers = useResource<Items<Supplier>>("/suppliers");
  const [project, setProject] = useState(rfq?.project_id ?? projectId ?? "");
  const [lines, setLines] = useState<Line[]>(() => linesFrom(rfq));
  const [chosen, setChosen] = useState<string[]>(() => rfq?.suppliers.map((s) => s.supplier_id) ?? []);
  const [newSupplier, setNewSupplier] = useState(false);
  const [supplierError, setSupplierError] = useState<unknown>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (rfq) return;
    let cancelled = false;
    fetchAll<Project>("/projects").then(
      ({ items }) => {
        if (cancelled) return;
        const open = items.filter((p) => p.status !== "archived");
        setProjects(open);
        setProject((current) => current || open[0]?.id || "");
      },
      (err: unknown) => !cancelled && setError(err),
    );
    return () => {
      cancelled = true;
    };
  }, [rfq]);

  const catalog = (materials.data?.items ?? []).filter((m) => !m.archived_at);
  const directory = suppliers.data?.items ?? [];

  function update(key: number, patch: Partial<Line>) {
    setLines((ls) => ls.map((l) => (l.key === key ? { ...l, ...patch } : l)));
  }

  function pickMaterial(key: number, materialId: string) {
    const m = catalog.find((x) => x.id === materialId);
    update(key, m ? { material_id: m.id, label: m.name, unit: m.unit } : { material_id: "" });
  }

  async function addSupplier(form: HTMLFormElement | null) {
    if (!form) return;
    const data = new FormData(form);
    setSupplierError(null);
    try {
      const created = await api<Supplier>("/suppliers", {
        method: "POST",
        body: { name: data.get("sup_name"), phone: data.get("sup_phone") || null, email: data.get("sup_email") || null, confirm_not_duplicate: data.get("sup_confirm") === "on" },
      });
      suppliers.reload();
      setChosen((c) => [...c, created.id]);
      setNewSupplier(false);
    } catch (err) {
      setSupplierError(err);
    }
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const body = {
      title: form.get("title"),
      notes: form.get("notes") || null,
      response_deadline: form.get("response_deadline") || null,
      delivery_date: form.get("delivery_date") || null,
      items: lines
        .filter((l) => l.label.trim() || l.quantity.trim())
        .map((l) => ({
          kind: l.kind,
          material_id: l.kind === "material" && l.material_id ? l.material_id : null,
          label: l.label,
          quantity: l.quantity.replace(/\s/g, "").replace(",", ".") || "0",
          unit: l.unit,
          specifications: l.specifications || null,
          needed_by: l.needed_by || null,
        })),
      supplier_ids: chosen,
    };
    setBusy(true);
    setError(null);
    try {
      const saved = rfq
        ? await api<RFQDetail>(`/rfqs/${rfq.id}`, { method: "PUT", body })
        : await api<RFQDetail>(`/projects/${project}/rfqs`, { method: "POST", body });
      router.replace(`/achats/${saved.id}`);
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  const units = [...Object.entries(UNIT_LABEL), ...Object.entries(SERVICE_UNIT_LABEL)];

  return (
    <form onSubmit={save} noValidate className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
      <div style={{ flex: "999 1 520px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s-dem">
          <h2 id="s-dem" className="sec-t">
            Demande
          </h2>
          {rfq ? (
            <p className="muted" style={{ margin: 0, fontSize: 14 }}>
              Chantier : <strong style={{ color: "#1D1D1F" }}>{rfq.project_name}</strong>
            </p>
          ) : (
            <Select label="Chantier" required value={project} onChange={(e) => setProject(e.target.value)}>
              {projects.length === 0 && <option value="">Aucun chantier disponible</option>}
              {projects.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name} — {p.reference}
                </option>
              ))}
            </Select>
          )}
          <Input label="Objet" name="title" required defaultValue={rfq?.title} placeholder="Ex. : gros œuvre R+1, lot ferraillage" error={fieldError(error, "title")} />
          <Textarea label="Précisions pour les fournisseurs" name="notes" defaultValue={rfq?.notes ?? ""} placeholder="Accès au chantier, conditionnement, documents attendus…" error={fieldError(error, "notes")} />
        </section>

        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 16 }} aria-labelledby="s-lignes">
          <div>
            <h2 id="s-lignes" className="sec-t">
              Matériaux et prestations
            </h2>
            <p className="hint" style={{ margin: "4px 0 0" }}>
              Chaque fournisseur chiffre ces lignes. Elles ne changent plus après la publication.
            </p>
          </div>
          {fieldError(error, "items") && (
            <span role="alert" className="field-err">
              {fieldError(error, "items")}
            </span>
          )}
          {lines.map((l, i) => (
            <fieldset key={l.key} style={{ border: "1px solid #E5E5EA", borderRadius: 12, padding: 16, margin: 0, display: "flex", flexWrap: "wrap", gap: 12, minWidth: 0 }}>
              <legend className="lbl" style={{ padding: "0 6px" }}>
                Ligne {i + 1}
              </legend>
              <div className="seg" role="group" aria-label={`Type de la ligne ${i + 1}`} style={{ flex: "1 1 100%", display: "flex", gap: 6, alignItems: "center" }}>
                {(["material", "service"] as const).map((kind) => (
                  <button
                    key={kind}
                    type="button"
                    className={`tab ${l.kind === kind ? "on" : ""}`}
                    aria-pressed={l.kind === kind}
                    onClick={() => update(l.key, kind === "service" ? { kind, material_id: "", unit: l.unit in SERVICE_UNIT_LABEL ? l.unit : "forfait" } : { kind })}
                  >
                    {kind === "material" ? "Matériau" : "Prestation"}
                  </button>
                ))}
                {lines.length > 1 && (
                  <button type="button" className="btn btn-g" aria-label={`Retirer la ligne ${i + 1}`} style={{ marginLeft: "auto", padding: "0 8px" }} onClick={() => setLines((ls) => ls.filter((x) => x.key !== l.key))}>
                    <IconClose size={16} />
                  </button>
                )}
              </div>
              {l.kind === "material" && catalog.length > 0 && (
                <div className="f" style={{ flex: "1 1 100%" }}>
                  <label htmlFor={`mat-${l.key}`}>Article du catalogue</label>
                  <select id={`mat-${l.key}`} className="sel" value={l.material_id} onChange={(e) => pickMaterial(l.key, e.target.value)} style={{ width: "100%" }}>
                    <option value="">Saisie libre</option>
                    {catalog.map((m) => (
                      <option key={m.id} value={m.id}>
                        {m.name} ({UNIT_LABEL[m.unit] ?? m.unit})
                      </option>
                    ))}
                  </select>
                </div>
              )}
              <div className="f" style={{ flex: "3 1 240px", minWidth: 0 }}>
                <label htmlFor={`lab-${l.key}`}>
                  Désignation <span className="req">*</span>
                </label>
                <input id={`lab-${l.key}`} className="inp" value={l.label} onChange={(e) => update(l.key, { label: e.target.value })} aria-invalid={fieldError(error, `items.${i}.label`) ? true : undefined} />
                {fieldError(error, `items.${i}.label`) && <span className="field-err">{fieldError(error, `items.${i}.label`)}</span>}
              </div>
              <div className="f" style={{ flex: "1 1 110px", minWidth: 0 }}>
                <label htmlFor={`qty-${l.key}`}>
                  Quantité <span className="req">*</span>
                </label>
                <input id={`qty-${l.key}`} className="inp num" inputMode="decimal" value={l.quantity} onChange={(e) => update(l.key, { quantity: e.target.value })} aria-invalid={fieldError(error, `items.${i}.quantity`) ? true : undefined} />
                {fieldError(error, `items.${i}.quantity`) && <span className="field-err">{fieldError(error, `items.${i}.quantity`)}</span>}
              </div>
              <div className="f" style={{ flex: "1 1 110px", minWidth: 0 }}>
                <label htmlFor={`unit-${l.key}`}>Unité</label>
                <select id={`unit-${l.key}`} className="sel" value={l.unit} onChange={(e) => update(l.key, { unit: e.target.value })} style={{ width: "100%" }}>
                  {units.map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
              <div className="f" style={{ flex: "3 1 240px", minWidth: 0 }}>
                <label htmlFor={`spec-${l.key}`}>Spécifications</label>
                <input id={`spec-${l.key}`} className="inp" value={l.specifications} placeholder="Norme, marque, dimensions…" onChange={(e) => update(l.key, { specifications: e.target.value })} />
              </div>
              <div className="f" style={{ flex: "1 1 150px", minWidth: 0 }}>
                <label htmlFor={`need-${l.key}`}>Pour le</label>
                <input id={`need-${l.key}`} className="inp" type="date" value={l.needed_by} onChange={(e) => update(l.key, { needed_by: e.target.value })} />
              </div>
            </fieldset>
          ))}
          <button type="button" className="btn btn-s" style={{ alignSelf: "flex-start" }} onClick={() => setLines((ls) => [...ls, blankLine()])}>
            <IconPlus size={14} /> Ajouter une ligne
          </button>
        </section>
      </div>

      <div style={{ flex: "1 1 300px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s-dates">
          <h2 id="s-dates" className="sec-t">
            Échéances
          </h2>
          <Input label="Date limite de réponse" name="response_deadline" type="date" defaultValue={rfq?.response_deadline ?? ""} hint="Incluse. Obligatoire pour publier." error={fieldError(error, "response_deadline")} />
          <Input label="Livraison souhaitée" name="delivery_date" type="date" defaultValue={rfq?.delivery_date ?? ""} error={fieldError(error, "delivery_date")} />
        </section>

        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 14 }} aria-labelledby="s-four">
          <h2 id="s-four" className="sec-t">
            Fournisseurs consultés
          </h2>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
            {chosen.length === 0 && <span className="muted" style={{ fontSize: 14 }}>Aucun pour l&apos;instant.</span>}
            {chosen.map((id) => {
              const s = directory.find((x) => x.id === id) ?? rfq?.suppliers.find((x) => x.supplier_id === id);
              const name = s ? ("name" in s ? s.name : s.supplier_name) : "…";
              return (
                <span key={id} className="bdg mut" style={{ fontSize: 13, padding: "4px 4px 4px 12px", whiteSpace: "normal", overflowWrap: "anywhere", maxWidth: "100%" }}>
                  {name}
                  <button type="button" aria-label={`Retirer ${name}`} onClick={() => setChosen((c) => c.filter((x) => x !== id))} style={{ flex: "none", width: 24, height: 24, border: 0, borderRadius: "50%", background: "transparent", cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "center" }}>
                    <IconClose size={13} />
                  </button>
                </span>
              );
            })}
          </div>
          <select
            className="sel"
            aria-label="Ajouter un fournisseur"
            value=""
            onChange={(e) => {
              const id = e.target.value;
              if (id) setChosen((c) => [...c, id]);
            }}
            style={{ width: "100%" }}
          >
            <option value="">+ Ajouter un fournisseur du répertoire…</option>
            {directory
              .filter((s) => !chosen.includes(s.id))
              .map((s) => (
                <option key={s.id} value={s.id}>
                  {s.name}
                  {s.email ? " · email" : s.phone ? " · téléphone" : ""}
                </option>
              ))}
          </select>
          {fieldError(error, "supplier_ids") && (
            <span role="alert" className="field-err">
              {fieldError(error, "supplier_ids")}
            </span>
          )}
          {newSupplier ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 12, borderTop: "1px solid #E5E5EA", paddingTop: 14 }}>
              <Input label="Nom du fournisseur" name="sup_name" error={fieldError(supplierError, "name")} />
              <Input label="Téléphone (WhatsApp)" name="sup_phone" type="tel" placeholder="+221 77 123 45 67" />
              <Input label="Email" name="sup_email" type="email" hint="Le lien de la demande y sera envoyé." error={fieldError(supplierError, "email")} />
              {!fieldError(supplierError, "name") && !fieldError(supplierError, "email") && <ErrorBanner error={supplierError} />}
              {supplierError instanceof ApiError && supplierError.code === "supplier.possible_duplicate" && (
                <label style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 14, fontWeight: 600 }}>
                  <input type="checkbox" name="sup_confirm" style={{ width: 18, height: 18 }} /> Il s&apos;agit bien d&apos;un autre fournisseur
                </label>
              )}
              <div style={{ display: "flex", gap: 8 }}>
                <button type="button" className="btn btn-s" onClick={(e) => void addSupplier(e.currentTarget.form)}>
                  Ajouter au répertoire
                </button>
                <button type="button" className="btn btn-g" onClick={() => setNewSupplier(false)}>
                  Annuler
                </button>
              </div>
            </div>
          ) : (
            <button type="button" className="btn btn-g" style={{ padding: 0, alignSelf: "flex-start", color: "#2B59E6" }} onClick={() => setNewSupplier(true)}>
              + Nouveau fournisseur
            </button>
          )}
          <p className="hint" style={{ margin: 0 }}>
            À la publication, chacun reçoit un lien personnel pour déposer son offre : par email s&apos;il en a un, sinon à lui transmettre par WhatsApp.
          </p>
        </section>
      </div>

      <div style={{ flex: "1 1 100%", display: "flex", flexDirection: "column", gap: 12, paddingTop: 16, borderTop: "1px solid #E5E5EA" }}>
        <ErrorBanner error={error} />
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, flexWrap: "wrap" }}>
          <Link href={rfq ? `/achats/${rfq.id}` : "/achats"} className="btn btn-g">
            Annuler
          </Link>
          <button type="submit" className="btn btn-p" disabled={busy || (!rfq && !project)}>
            {busy ? "Enregistrement…" : "Enregistrer le brouillon"}
          </button>
        </div>
      </div>
    </form>
  );
}
