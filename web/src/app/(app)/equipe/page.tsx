"use client";

import { Fragment, type FormEvent, useState } from "react";
import { IconPlus } from "@/components/icons";
import { Badge, Button, Empty, ErrorBanner, fieldError, Input, Loading, SearchBox, Select } from "@/components/ui";
import type { Items, Member, Paged, PendingInvitation, Project } from "@/lib/api";
import { api } from "@/lib/api";
import { ago, dayMonth, ROLE_LABEL } from "@/lib/format";
import { useResource } from "@/lib/hooks";
import { useSession } from "@/lib/session";

const AVATAR: Record<string, { background: string; color: string }> = {
  owner: { background: "#FFC21A", color: "#1D1D1F" },
  accountant: { background: "#D2D2D7", color: "#1D1D1F" },
  manager: { background: "#EBF0FD", color: "#1F48C8" },
  site_worker: { background: "#E8F5EC", color: "#1A7F45" },
  viewer: { background: "#F2F2F7", color: "#48484A" },
};

function initials(name: string) {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((w) => w[0]?.toUpperCase())
    .join("");
}

/**
 * Règles réellement appliquées par le serveur (internal/rbac) : ce tableau ne promet rien de plus.
 * Une cellule vaut « y » (Oui), « n » (Non) ou un libellé conditionnel.
 */
const MATRIX: { action: string; cells: [string, string, string, string, string] }[] = [
  { action: "Voir tous les chantiers", cells: ["Oui", "Assignés", "Assignés", "Oui", "Assignés"] },
  { action: "Créer une dépense", cells: ["Oui", "Oui", "Oui", "Oui", "Non"] },
  { action: "Publier un rapport journalier", cells: ["Oui", "Oui", "Oui", "Non", "Non"] },
  { action: "Approuver une dépense", cells: ["Oui", "Si délégué", "Non", "Oui", "Non"] },
  { action: "Réviser un budget", cells: ["Oui", "Oui", "Non", "Non", "Non"] },
  { action: "Exporter les données financières", cells: ["Oui", "Non", "Non", "Oui", "Non"] },
  { action: "Gérer les membres", cells: ["Oui", "Non", "Non", "Non", "Non"] },
];

function cellClass(value: string) {
  if (value === "Oui") return "y";
  if (value === "Si délégué") return "p";
  return "n";
}

export default function TeamPage() {
  const { user } = useSession();
  const members = useResource<Items<Member>>("/members");
  const invitations = useResource<Items<PendingInvitation>>("/invitations");
  const projects = useResource<Paged<Project>>("/projects?limit=100");
  const [error, setError] = useState<unknown>(null);
  const [q, setQ] = useState("");
  const [role, setRole] = useState("");
  const [open, setOpen] = useState<string | null>(null);
  const [link, setLink] = useState<{ name: string; url: string } | null>(null);

  function refresh() {
    members.reload();
    invitations.reload();
  }

  async function run(fn: () => Promise<unknown>) {
    setError(null);
    try {
      await fn();
      refresh();
    } catch (err) {
      setError(err);
    }
  }

  /** Le lien n'est lisible qu'à sa création : renvoyer en crée un nouveau, le serveur révoque l'ancien. */
  async function resend(i: PendingInvitation) {
    setError(null);
    setLink(null);
    try {
      const result = await api<{ invite_url: string }>("/invitations", { method: "POST", body: { full_name: i.full_name, email: i.email, phone: i.phone, role: i.role, project_ids: i.project_ids ?? [] } });
      setLink({ name: i.full_name, url: result.invite_url });
      refresh();
    } catch (err) {
      setError(err);
    }
  }

  if (members.loading) return <Loading />;
  if (!members.data) return <ErrorBanner error={members.error} />;

  const all = members.data.items;
  const pendingById = new Map(invitations.data?.items.map((i) => [i.user_id, i]));
  const needle = q.trim().toLowerCase();
  const visible = all.filter((m) => (!role || m.role === role) && (!needle || `${m.full_name} ${m.email ?? ""} ${m.phone ?? ""}`.toLowerCase().includes(needle)));
  const active = all.filter((m) => m.status === "active").length;
  const invited = all.filter((m) => m.status === "invited").length;
  const suspended = all.filter((m) => m.status === "suspended").length;

  return (
    <>
      <div className="wrap" style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h1 className="page-h1">Équipe et permissions</h1>
          <p className="muted" style={{ margin: "6px 0 0", fontSize: 15 }}>
            {all.length} membre{all.length > 1 ? "s" : ""} · {active} actif{active > 1 ? "s" : ""}, {invited} invitation{invited > 1 ? "s" : ""} en attente, {suspended} désactivé{suspended > 1 ? "s" : ""}
          </p>
        </div>
        <a href="#h-inv" className="btn btn-p">
          <IconPlus size={16} />
          Inviter un membre
        </a>
      </div>
      <ErrorBanner error={error} />
      <div className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
        <div style={{ flex: "999 1 560px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <section className="card" aria-labelledby="h-mem">
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12, padding: 16, flexWrap: "wrap" }}>
              <h2 id="h-mem" className="sec-t">
                Membres
              </h2>
              <div style={{ display: "flex", gap: 10, flexWrap: "wrap", maxWidth: "100%" }}>
                <SearchBox label="Rechercher un membre" placeholder="Nom, email, téléphone…" value={q} onChange={setQ} width={220} />
                <select className="sel" aria-label="Rôle" value={role} onChange={(e) => setRole(e.target.value)}>
                  <option value="">Tous les rôles</option>
                  {Object.entries(ROLE_LABEL).map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </select>
              </div>
            </div>
            {visible.length === 0 ? (
              <Empty>Aucun membre ne correspond.</Empty>
            ) : (
              <div style={{ overflowX: "auto" }}>
                <table className="tbl">
                  <thead>
                    <tr>
                      <th>Membre</th>
                      <th>Rôle</th>
                      <th className="hide-sm">Chantiers</th>
                      <th>Statut</th>
                      <th>
                        <span className="sr-only">Actions</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {visible.map((m) => {
                      const pending = pendingById.get(m.id);
                      const off = m.status === "suspended";
                      const avatar = off ? { background: "#F2F2F7", color: "#6E6E73" } : AVATAR[m.role];
                      const all = m.role === "owner" || m.role === "accountant";
                      return (
                        <Fragment key={m.id}>
                          <tr>
                            <td>
                              <div className="who">
                                <span className="av" style={avatar} aria-hidden="true">
                                  {initials(m.full_name)}
                                </span>
                                <span style={{ minWidth: 0 }}>
                                  <span style={{ fontWeight: 600, display: "block", color: off ? "#6E6E73" : undefined }}>{m.full_name}</span>
                                  <span className="muted" style={{ fontSize: 12 }}>
                                    {m.email ?? m.phone}
                                  </span>
                                </span>
                              </div>
                            </td>
                            <td>
                              <Badge tone={m.role === "owner" ? "blue" : "neutral"}>{ROLE_LABEL[m.role]}</Badge>
                            </td>
                            <td className="hide-sm">
                              {all ? (
                                "Tous"
                              ) : m.project_names.length === 0 ? (
                                <span className="muted">—</span>
                              ) : (
                                <>
                                  {m.project_names[0]}
                                  {m.project_names.length > 1 && <span className="muted"> +{m.project_names.length - 1}</span>}
                                </>
                              )}
                            </td>
                            <td>
                              {m.status === "active" && <Badge tone="green">Actif</Badge>}
                              {m.status === "invited" && <Badge tone="orange">Invitation envoyée</Badge>}
                              {off && <Badge>Désactivé</Badge>}
                              <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
                                {m.status === "active" ? ago(m.last_login_at) : off ? "Historique conservé" : pending ? `Expire le ${dayMonth(pending.expires_at)}` : ""}
                              </div>
                            </td>
                            <td style={{ whiteSpace: "nowrap" }}>
                              {m.status === "invited" && pending && (
                                <button type="button" className="link-btn" onClick={() => resend(pending)}>
                                  Renvoyer
                                </button>
                              )}
                              {off && (
                                <button type="button" className="link-btn" onClick={() => run(() => api(`/members/${m.id}`, { method: "PATCH", body: { status: "active" } }))}>
                                  Réactiver
                                </button>
                              )}
                              {m.status === "active" && m.id !== user.id && (
                                <button type="button" aria-label={`Gérer ${m.full_name}`} aria-expanded={open === m.id} onClick={() => setOpen(open === m.id ? null : m.id)} style={{ border: 0, background: "transparent", cursor: "pointer", color: "#6E6E73", fontSize: 18, fontWeight: 700, minWidth: 40, minHeight: 40 }}>
                                  ···
                                </button>
                              )}
                            </td>
                          </tr>
                          {open === m.id && (
                            <tr>
                              <td colSpan={5} style={{ background: "#FAFAFC" }}>
                                <div style={{ display: "flex", gap: 12, alignItems: "flex-end", flexWrap: "wrap" }}>
                                  <div style={{ minWidth: 200 }}>
                                    <Select label="Rôle" value={m.role} onChange={(e) => run(() => api(`/members/${m.id}`, { method: "PATCH", body: { role: e.target.value } }))}>
                                      {Object.entries(ROLE_LABEL).map(([value, label]) => (
                                        <option key={value} value={value}>
                                          {label}
                                        </option>
                                      ))}
                                    </Select>
                                  </div>
                                  <Button variant="danger" onClick={() => window.confirm(`Désactiver ${m.full_name} ? Ses sessions seront immédiatement fermées, son historique est conservé.`) && void run(() => api(`/members/${m.id}`, { method: "PATCH", body: { status: "suspended" } }))}>
                                    Désactiver
                                  </Button>
                                </div>
                              </td>
                            </tr>
                          )}
                        </Fragment>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </section>
          <section className="card" aria-labelledby="h-mx">
            <div style={{ padding: "18px 16px 6px" }}>
              <h2 id="h-mx" className="sec-t">
                Permissions par rôle
              </h2>
              <p className="muted" style={{ margin: "4px 0 0", fontSize: 13 }}>
                Un rôle ne donne accès qu&apos;aux chantiers où le membre est assigné, sauf Owner et Finance.
              </p>
            </div>
            <div style={{ overflowX: "auto" }}>
              <table className="tbl mx">
                <thead>
                  <tr>
                    <th>Action</th>
                    <th>Owner / Admin</th>
                    <th>Conducteur</th>
                    <th>Chef de chantier</th>
                    <th>Finance</th>
                    <th>Lecture seule</th>
                  </tr>
                </thead>
                <tbody>
                  {MATRIX.map((row) => (
                    <tr key={row.action}>
                      <td>{row.action}</td>
                      {row.cells.map((cell, i) => (
                        <td key={i} className={cellClass(cell)}>
                          {cell}
                        </td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        </div>
        <InviteForm projects={projects.data?.items.filter((p) => p.status !== "archived") ?? []} onDone={(l) => { setLink(l); refresh(); }} link={link} />
      </div>
    </>
  );
}

function InviteForm({ projects, onDone, link }: { projects: Project[]; onDone: (link: { name: string; url: string }) => void; link: { name: string; url: string } | null }) {
  const [error, setError] = useState<unknown>(null);
  const [formKey, setFormKey] = useState(0);
  const [role, setRole] = useState("site_worker");

  async function invite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const contact = String(form.get("contact") ?? "").trim();
    const isEmail = contact.includes("@");
    setError(null);
    try {
      const fullName = String(form.get("full_name"));
      const result = await api<{ invite_url: string }>("/invitations", {
        method: "POST",
        body: {
          full_name: fullName,
          email: isEmail ? contact : null,
          phone: !isEmail && contact ? contact.replace(/[\s.-]/g, "") : null,
          role: form.get("role"),
          project_ids: form.getAll("project_ids"),
        },
      });
      setFormKey((k) => k + 1);
      onDone({ name: fullName, url: result.invite_url });
    } catch (err) {
      setError(err);
    }
  }

  const everywhere = role === "owner" || role === "accountant";
  return (
    <section className="card" style={{ flex: "1 1 320px", minWidth: 0, padding: 20, display: "flex", flexDirection: "column", gap: 16, scrollMarginTop: 16 }} aria-labelledby="h-inv" id="inviter">
      <h2 id="h-inv" className="sec-t">
        Inviter un membre
      </h2>
      <form key={formKey} onSubmit={invite} noValidate style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <Input label="Email ou téléphone" name="contact" required placeholder="+221 77 123 45 67" error={fieldError(error, "email") ?? fieldError(error, "phone") ?? fieldError(error, "contact")} />
        <Input label="Nom" name="full_name" required error={fieldError(error, "full_name")} />
        <Select label="Rôle" name="role" required value={role} onChange={(e) => setRole(e.target.value)} error={fieldError(error, "role")}>
          {["site_worker", "manager", "accountant", "viewer", "owner"].map((value) => (
            <option key={value} value={value}>
              {ROLE_LABEL[value]}
            </option>
          ))}
        </Select>
        {!everywhere && (
          <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
            <legend style={{ fontSize: 13, fontWeight: 600, marginBottom: 4 }}>Chantiers accessibles</legend>
            {projects.length === 0 && <p className="hint">Aucun chantier pour le moment.</p>}
            {projects.map((p) => (
              <label key={p.id} className="ck-line">
                <input type="checkbox" name="project_ids" value={p.id} /> {p.name}
              </label>
            ))}
          </fieldset>
        )}
        <div className="muted" style={{ padding: "10px 12px", borderRadius: 8, background: "#F5F5F7", fontSize: 13 }}>
          Lien d&apos;invitation à usage unique, valable 7 jours, révocable à tout moment.
        </div>
        <ErrorBanner error={error} />
        <button type="submit" className="btn btn-p" style={{ height: 42 }}>
          Envoyer l&apos;invitation
        </button>
      </form>
      {link && (
        <div role="status" className="ok" style={{ padding: "12px 14px", borderRadius: 10, fontSize: 13 }}>
          <strong>Invitation créée pour {link.name}.</strong> À transmettre maintenant, ce lien ne sera plus affiché :
          <p style={{ margin: "6px 0", wordBreak: "break-all", color: "#1D1D1F" }}>{link.url}</p>
          <button type="button" className="btn btn-s" style={{ height: 34 }} onClick={() => navigator.clipboard.writeText(link.url)}>
            Copier le lien
          </button>
        </div>
      )}
    </section>
  );
}
