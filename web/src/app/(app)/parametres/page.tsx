"use client";

import { type FormEvent, type ReactNode, useState } from "react";
import { Badge, Button, ErrorBanner, fieldError, Input, Loading, Select, Soon } from "@/components/ui";
import type { Category, Items, Organization } from "@/lib/api";
import { api } from "@/lib/api";
import { ACTIVITY_LABEL, CITIES, dateTime } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { can, useSession } from "@/lib/session";

interface Settings {
  budget_alert_thresholds: number[];
}

interface SessionRow {
  id: string;
  user_agent: string | null;
  last_seen_at: string;
}

export default function SettingsPage() {
  const { user, organization } = useSession();
  const owner = can.manageOrg(user.role);
  const [active, setActive] = useState(owner ? "org" : "notif");
  const nav = [
    { id: "org", label: "Organisation", show: owner },
    { id: "cat", label: "Catégories de dépenses", show: owner },
    { id: "val", label: "Validation et alertes", show: owner },
    { id: "notif", label: "Notifications", show: true },
    { id: "secu", label: "Sécurité et données", show: true },
    { id: "abo", label: "Abonnement", show: owner },
  ].filter((n) => n.show);

  return (
    <>
      <div style={{ maxWidth: 1180 }}>
        <h1 className="page-h1">Paramètres</h1>
        <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
          Organisation, règles de dépenses, notifications et sécurité de {organization.name}
        </p>
      </div>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start", maxWidth: 1180, width: "100%" }}>
        <nav className="sub" aria-label="Sections des paramètres" style={{ flex: "1 1 200px", display: "flex", flexDirection: "column", gap: 2 }}>
          {nav.map((n) => (
            <a key={n.id} href={`#${n.id}`} className={active === n.id ? "on" : ""} onClick={() => setActive(n.id)}>
              {n.label}
            </a>
          ))}
        </nav>
        <div style={{ flex: "999 1 600px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          {owner && <Company />}
          {owner && <Categories />}
          {owner && <Validation />}
          <Notifications firstName={user.full_name.split(" ")[0] ?? ""} />
          <Security />
          {owner && <Subscription />}
        </div>
      </div>
    </>
  );
}

function Row({ title, text, children, danger }: { title: string; text: ReactNode; children: ReactNode; danger?: boolean }) {
  return (
    <div className="row">
      <div style={{ minWidth: 0 }}>
        <div className="rt" style={danger ? { color: "#B3261E" } : undefined}>
          {title}
        </div>
        <div className="rd">{text}</div>
      </div>
      <div style={{ flex: "none", display: "flex", alignItems: "center", gap: 8 }}>{children}</div>
    </div>
  );
}

function Company() {
  const { data, loading, reload } = useResource<Organization>("/organization");
  const [error, setError] = useState<unknown>(null);
  const [saved, setSaved] = useState(false);

  if (loading || !data) return <Loading />;

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setError(null);
    setSaved(false);
    try {
      const value = (k: string) => String(form.get(k) ?? "").trim() || undefined;
      await api("/organization", {
        method: "PATCH",
        body: { name: value("name"), legal_name: value("legal_name"), phone: value("phone"), email: value("email"), address: value("address"), city: value("city"), activity_type: value("activity_type") },
      });
      setSaved(true);
      reload();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <section id="org" className="card sec" aria-labelledby="h-org" style={{ scrollMarginTop: 16 }}>
      <div>
        <h2 id="h-org">Organisation</h2>
        <p className="d-txt">Ces informations apparaissent sur les exports PDF.</p>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 16, flexWrap: "wrap" }}>
        <div role="img" aria-label="Emplacement du logo" style={{ width: 64, height: 64, borderRadius: 12, background: "#F2F2F7", border: "1px dashed #C7C7CC", display: "flex", alignItems: "center", justifyContent: "center", color: "#48484A", fontSize: 11, fontWeight: 600 }}>
          Logo
        </div>
        <button type="button" className="btn btn-s" disabled>
          Importer un logo
        </button>
        <Soon />
      </div>
      <form key={data.name + data.city} onSubmit={submit} noValidate style={{ display: "flex", flexDirection: "column", gap: 18 }}>
        <div className="g2">
          <Input label="Nom de l'entreprise" name="name" required defaultValue={data.name} error={fieldError(error, "name")} />
          <Input label="Téléphone" name="phone" type="tel" defaultValue={data.phone ?? ""} error={fieldError(error, "phone")} />
          <Select label="Type d'activité" name="activity_type" defaultValue={data.activity_type ?? "general"} error={fieldError(error, "activity_type")}>
            {Object.entries(ACTIVITY_LABEL).map(([value, label]) => (
              <option key={value} value={value}>
                {label}
              </option>
            ))}
          </Select>
          <Select label="Ville" name="city" defaultValue={data.city ?? "Dakar"}>
            {[...new Set([data.city ?? "Dakar", ...CITIES])].map((c) => (
              <option key={c}>{c}</option>
            ))}
          </Select>
          <Input label="Raison sociale" name="legal_name" defaultValue={data.legal_name ?? ""} />
          <Input label="Email" name="email" type="email" defaultValue={data.email ?? ""} error={fieldError(error, "email")} />
        </div>
        <Input label="Adresse" name="address" defaultValue={data.address ?? ""} />
        <div className="g2">
          <Select label="Devise" disabled value="XOF" hint="Une seule devise par entreprise au MVP.">
            <option value="XOF">FCFA (XOF)</option>
          </Select>
          <Select label="Fuseau horaire" disabled value="dakar">
            <option value="dakar">{data.timezone} (UTC+0)</option>
          </Select>
        </div>
        <ErrorBanner error={error} />
        <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 12 }}>
          {saved && (
            <span role="status" style={{ fontSize: 14, color: "#1A7F45" }}>
              Enregistré.
            </span>
          )}
          <Button type="submit">Enregistrer</Button>
        </div>
      </form>
    </section>
  );
}

function Categories() {
  const { data, reload } = useResource<Items<Category>>("/categories?include_inactive=true");
  const [error, setError] = useState<unknown>(null);
  const [adding, setAdding] = useState<string | null>(null);
  const [editing, setEditing] = useState<{ id: string; name: string } | null>(null);

  async function run(fn: () => Promise<unknown>) {
    setError(null);
    try {
      await fn();
      reload();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <section id="cat" className="card sec" aria-labelledby="h-cat" style={{ scrollMarginTop: 16 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
        <div>
          <h2 id="h-cat">Catégories de dépenses</h2>
          <p className="d-txt">Les catégories système restent communes à tous les chantiers pour des rapports comparables.</p>
        </div>
        <button type="button" className="btn btn-s" onClick={() => setAdding("")}>
          + Ajouter une catégorie
        </button>
      </div>
      {adding !== null && (
        <form
          style={{ display: "flex", gap: 8, flexWrap: "wrap" }}
          onSubmit={(e) => {
            e.preventDefault();
            void run(async () => {
              await api("/categories", { method: "POST", body: { name: adding } });
              setAdding(null);
            });
          }}
        >
          <input aria-label="Nom de la catégorie" className="inp" style={{ flex: "1 1 200px", width: "auto" }} value={adding} onChange={(e) => setAdding(e.target.value)} />
          <Button type="submit" variant="secondary">
            Ajouter
          </Button>
          <Button variant="ghost" onClick={() => setAdding(null)}>
            Annuler
          </Button>
        </form>
      )}
      <ErrorBanner error={error} />
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(260px, 1fr))", gap: 8 }}>
        {data?.items.map((c) =>
          editing?.id === c.id ? (
            <form
              key={c.id}
              className="cat-item"
              onSubmit={(e) => {
                e.preventDefault();
                void run(async () => {
                  await api(`/categories/${c.id}`, { method: "PATCH", body: { name: editing.name } });
                  setEditing(null);
                });
              }}
            >
              <input aria-label={`Nouveau nom de ${c.name}`} className="inp" style={{ flex: 1, height: 32, width: "auto", minWidth: 0 }} value={editing.name} onChange={(e) => setEditing({ id: c.id, name: e.target.value })} />
              <button type="submit" className="link-btn">
                OK
              </button>
            </form>
          ) : (
            <div key={c.id} className="cat-item" style={c.is_active ? undefined : { opacity: 0.6 }}>
              <span style={{ flex: 1, fontWeight: 600, minWidth: 0 }}>{c.name}</span>
              {c.system_code ? <Badge>Système</Badge> : <Badge tone="blue">Personnalisée</Badge>}
              {!c.system_code && (
                <button type="button" className="link-btn" aria-label={`Modifier ${c.name}`} onClick={() => setEditing({ id: c.id, name: c.name })}>
                  Modifier
                </button>
              )}
            </div>
          ),
        )}
      </div>
    </section>
  );
}

function Validation() {
  const { data, reload } = useResource<Settings>("/organization/settings");
  const [error, setError] = useState<unknown>(null);
  const thresholds = data?.budget_alert_thresholds ?? [];

  async function toggle(value: number) {
    const next = thresholds.includes(value) ? thresholds.filter((t) => t !== value) : [...thresholds, value].sort((a, b) => a - b);
    setError(null);
    try {
      await api("/organization/settings", { method: "PATCH", body: { budget_alert_thresholds: next } });
      reload();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <section id="val" className="card sec" aria-labelledby="h-val" style={{ scrollMarginTop: 16 }}>
      <div>
        <h2 id="h-val">Validation et alertes</h2>
        <p className="d-txt">Règles appliquées à tous les chantiers.</p>
      </div>
      <ErrorBanner error={error} />
      <div>
        <Row title="Validation obligatoire des dépenses" text="Toute dépense soumise attend l'approbation d'un Owner, de Finance ou d'un conducteur délégué.">
          <Soon />
          <input type="checkbox" className="sw" checked disabled readOnly aria-label="Validation obligatoire des dépenses" />
        </Row>
        <Row title="Seuil de validation" text="En dessous, la dépense serait acceptée directement.">
          <Soon />
        </Row>
        <Row title="Alertes de budget exposé" text="Signaler quand un chantier atteint ces seuils.">
          <div style={{ display: "flex", gap: 6 }}>
            {[80, 90, 100].map((value) => (
              <button key={value} type="button" className={`chip-s ${thresholds.includes(value) ? "on" : ""}`} aria-pressed={thresholds.includes(value)} onClick={() => toggle(value)}>
                {value} %
              </button>
            ))}
          </div>
        </Row>
        <Row title="Écart budget / avancement" text="Le budget exposé dépasse l'avancement déclaré de plus de 10 points.">
          <Soon />
        </Row>
        <Row title="Avertissement doublon" text="Même montant, date, fournisseur et chantier. La décision reste humaine.">
          <Soon />
          <input type="checkbox" className="sw" checked disabled readOnly aria-label="Avertissement doublon" />
        </Row>
        <Row title="Rappel de rapport journalier" text="Un seul rappel par jour au conducteur si aucun rapport n'est publié.">
          <Soon />
        </Row>
      </div>
    </section>
  );
}

function Notifications({ firstName }: { firstName: string }) {
  const events = ["Dépense à valider", "Seuil de budget atteint", "Rapport journalier manquant", "Synthèse hebdomadaire (lundi 8 h)"];
  return (
    <section id="notif" className="card sec" aria-labelledby="h-not" style={{ scrollMarginTop: 16 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12 }}>
        <div>
          <h2 id="h-not">Notifications</h2>
          <p className="d-txt">Vos préférences personnelles, {firstName}. En attendant, l&apos;écran Alertes rassemble ce qui demande votre attention.</p>
        </div>
        <Soon />
      </div>
      <div style={{ overflowX: "auto" }}>
        <table style={{ borderCollapse: "collapse", width: "100%", fontSize: 14 }}>
          <thead>
            <tr>
              <th style={{ textAlign: "left", fontSize: 12, color: "#6E6E73", padding: "8px 0", borderBottom: "1px solid #E5E5EA", fontWeight: 500 }}>Événement</th>
              {["Dans l'app", "Email", "WhatsApp"].map((c) => (
                <th key={c} style={{ fontSize: 12, color: "#6E6E73", padding: 8, borderBottom: "1px solid #E5E5EA", width: 90, fontWeight: 500 }}>
                  {c}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {events.map((event) => (
              <tr key={event}>
                <td style={{ padding: "12px 0", borderBottom: "1px solid #F2F2F7" }}>{event}</td>
                {["app", "email", "whatsapp"].map((channel) => (
                  <td key={channel} style={{ textAlign: "center", borderBottom: "1px solid #F2F2F7" }}>
                    <input type="checkbox" className="sw" disabled aria-label={`${event}, ${channel}`} />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function Security() {
  const { data, reload } = useResource<Items<SessionRow>>("/auth/sessions");
  const [error, setError] = useState<unknown>(null);

  async function revoke(id: string) {
    setError(null);
    try {
      await api(`/auth/sessions/${id}`, { method: "DELETE" });
      reload();
    } catch (err) {
      setError(err);
    }
  }

  return (
    <section id="secu" className="card sec" aria-labelledby="h-sec" style={{ scrollMarginTop: 16 }}>
      <div>
        <h2 id="h-sec">Sécurité et données</h2>
      </div>
      <ErrorBanner error={error} />
      <div>
        <Row title="Mot de passe" text="Le changement depuis l'application arrive prochainement.">
          <Soon />
        </Row>
        <Row title="Vérification en deux étapes" text="Code par SMS à chaque connexion sur un nouvel appareil.">
          <Soon />
        </Row>
        <div className="row" style={{ display: "block" }}>
          <div className="rt">Sessions actives</div>
          {data?.items.map((s) => (
            <div key={s.id} style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: "8px 0" }}>
              <div style={{ minWidth: 0 }}>
                <div className="rd" style={{ overflow: "hidden", textOverflow: "ellipsis", color: "#1D1D1F" }}>
                  {s.user_agent ?? "Appareil inconnu"}
                </div>
                <div className="rd">Vu le {dateTime(s.last_seen_at)}</div>
              </div>
              <button type="button" className="btn btn-s" style={{ flex: "none" }} onClick={() => revoke(s.id)}>
                Déconnecter
              </button>
            </div>
          ))}
        </div>
        <Row title="Exporter les données de l'entreprise" text="Chantiers, dépenses, justificatifs et rapports, au format ZIP.">
          <Soon />
        </Row>
        <Row title="Conservation des justificatifs" text="Durée avant suppression des reçus des chantiers archivés.">
          <Soon />
        </Row>
        <Row title="Clôturer le compte" text="Un export complet est proposé avant toute suppression." danger>
          <Soon />
        </Row>
      </div>
    </section>
  );
}

function Subscription() {
  return (
    <section id="abo" className="card sec" aria-labelledby="h-abo" style={{ scrollMarginTop: 16 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
        <div>
          <h2 id="h-abo">Abonnement</h2>
          <p className="d-txt">Facturation mensuelle par entreprise, sans coût par utilisateur.</p>
        </div>
        <Soon />
      </div>
      <p className="muted" style={{ margin: 0, fontSize: 14, padding: 16, borderRadius: 10, background: "#F5F5F7" }}>
        La gestion de l&apos;offre, des factures et du moyen de paiement n&apos;est pas encore disponible.
      </p>
    </section>
  );
}
