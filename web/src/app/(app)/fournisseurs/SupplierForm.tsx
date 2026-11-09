"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { IconAlertCircle, IconWarn } from "@/components/icons";
import { Banner, ErrorBanner, fieldError, Input, Select, Textarea } from "@/components/ui";
import type { Items, Supplier, SupplierDuplicate } from "@/lib/api";
import { api, ApiError } from "@/lib/api";
import { CITIES, DUPLICATE_REASON_LABEL, LEGAL_FORM_LABEL, SUPPLIER_ACTIVITY_LABEL } from "@/lib/format";

type Values = Record<"name" | "legal_name" | "legal_form" | "ninea" | "rccm" | "phone" | "email" | "address" | "city" | "notes", string>;

const KEYS: (keyof Values)[] = ["name", "legal_name", "legal_form", "ninea", "rccm", "phone", "email", "address", "city", "notes"];

function valuesOf(s?: Supplier): Values {
  const v = {} as Values;
  for (const k of KEYS) v[k] = (s?.[k] as string | null | undefined) ?? "";
  return v;
}

function reasons(d: SupplierDuplicate) {
  return d.reasons.map((r) => DUPLICATE_REASON_LABEL[r] ?? r).join(", ");
}

/** Création et modification d'une fiche. Les doublons sont signalés pendant la saisie, puis contrôlés par le serveur. */
export function SupplierForm({ supplier }: { supplier?: Supplier }) {
  const router = useRouter();
  const [values, setValues] = useState<Values>(() => valuesOf(supplier));
  const [activities, setActivities] = useState<string[]>(supplier?.activities ?? []);
  const [duplicates, setDuplicates] = useState<SupplierDuplicate[]>([]);
  const [confirmed, setConfirmed] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  const set = (key: keyof Values) => (e: { target: { value: string } }) => setValues((v) => ({ ...v, [key]: e.target.value }));

  async function checkDuplicates(current = values) {
    if (current.name.trim().length < 2) return;
    try {
      const { items } = await api<Items<SupplierDuplicate>>("/suppliers/duplicates", {
        method: "POST",
        body: { name: current.name, ninea: current.ninea, rccm: current.rccm, phone: current.phone, email: current.email, exclude_id: supplier?.id ?? null },
      });
      setDuplicates(items);
    } catch {
      // Un format encore invalide (NINEA, téléphone...) : le contrôle reprendra à l'enregistrement.
    }
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const body: Record<string, unknown> = { ...values, activities, confirm_not_duplicate: confirmed };
    setBusy(true);
    setError(null);
    try {
      const saved = supplier
        ? await api<Supplier>(`/suppliers/${supplier.id}`, { method: "PATCH", body })
        : await api<Supplier>("/suppliers", { method: "POST", body });
      router.replace(`/fournisseurs/${saved.id}`);
    } catch (err) {
      setError(err);
      if (err instanceof ApiError && err.code.startsWith("supplier.") && err.code.includes("duplicate")) await checkDuplicates();
      setBusy(false);
    }
  }

  const blocking = duplicates.filter((d) => d.blocking);
  // En modification, un doublon probable déjà accepté n'est redemandé que si le nom, le téléphone ou l'email changent (même règle que le serveur).
  const signalsChanged = !supplier || (["name", "phone", "email"] as const).some((k) => values[k].trim() !== (supplier[k] ?? ""));
  const possible = signalsChanged ? duplicates.filter((d) => !d.blocking) : [];
  const duplicateError = error instanceof ApiError && (error.code === "supplier.duplicate" || error.code === "supplier.possible_duplicate");

  return (
    <form onSubmit={submit} noValidate className="wrap" style={{ display: "flex", flexWrap: "wrap", gap: 24, alignItems: "flex-start" }}>
      <div style={{ flex: "999 1 520px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s-id">
          <h2 id="s-id" className="sec-t">
            Identité
          </h2>
          <Input label="Nom usuel" name="name" required value={values.name} onChange={set("name")} onBlur={() => void checkDuplicates()} placeholder="Ex. : Quincaillerie Ndiaye" error={fieldError(error, "name")} />
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            <div style={{ flex: "2 1 260px", minWidth: 0 }}>
              <Input label="Raison sociale" name="legal_name" value={values.legal_name} onChange={set("legal_name")} error={fieldError(error, "legal_name")} />
            </div>
            <div style={{ flex: "1 1 180px", minWidth: 0 }}>
              <Select label="Forme juridique" name="legal_form" value={values.legal_form} onChange={set("legal_form")} error={fieldError(error, "legal_form")}>
                <option value="">—</option>
                {Object.entries(LEGAL_FORM_LABEL).map(([k, label]) => (
                  <option key={k} value={k}>
                    {label}
                  </option>
                ))}
              </Select>
            </div>
          </div>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 16 }}>
            <div style={{ flex: "1 1 220px", minWidth: 0 }}>
              <Input label="NINEA" name="ninea" value={values.ninea} onChange={set("ninea")} onBlur={() => void checkDuplicates()} placeholder="004567891 2G3" error={fieldError(error, "ninea")} />
            </div>
            <div style={{ flex: "1 1 220px", minWidth: 0 }}>
              <Input label="RCCM" name="rccm" value={values.rccm} onChange={set("rccm")} onBlur={() => void checkDuplicates()} placeholder="SN-DKR-2015-B-12345" error={fieldError(error, "rccm")} />
            </div>
          </div>
          <fieldset style={{ border: 0, margin: 0, padding: 0 }}>
            <legend className="lbl" style={{ marginBottom: 8 }}>
              Activités
            </legend>
            <div style={{ display: "flex", flexWrap: "wrap", gap: 6 }}>
              {Object.entries(SUPPLIER_ACTIVITY_LABEL).map(([k, label]) => {
                const on = activities.includes(k);
                return (
                  <button key={k} type="button" className={`tab ${on ? "on" : ""}`} aria-pressed={on} onClick={() => setActivities((a) => (on ? a.filter((x) => x !== k) : [...a, k]))}>
                    {label}
                  </button>
                );
              })}
            </div>
          </fieldset>
          <Textarea label="Notes internes" name="notes" value={values.notes} onChange={set("notes")} placeholder="Horaires, spécialités, qualité de service…" error={fieldError(error, "notes")} />
        </section>
      </div>

      <div style={{ flex: "1 1 300px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
        <section className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }} aria-labelledby="s-coord">
          <h2 id="s-coord" className="sec-t">
            Coordonnées
          </h2>
          <Input label="Téléphone" name="phone" type="tel" value={values.phone} onChange={set("phone")} onBlur={() => void checkDuplicates()} placeholder="+221 77 123 45 67" error={fieldError(error, "phone")} />
          <Input label="Email" name="email" type="email" value={values.email} onChange={set("email")} onBlur={() => void checkDuplicates()} hint="Les demandes de prix y sont envoyées." error={fieldError(error, "email")} />
          <Input label="Adresse" name="address" value={values.address} onChange={set("address")} error={fieldError(error, "address")} />
          <Input label="Ville" name="city" list="cities" value={values.city} onChange={set("city")} error={fieldError(error, "city")} />
          <datalist id="cities">
            {CITIES.filter((c) => c !== "Autre").map((c) => (
              <option key={c} value={c} />
            ))}
          </datalist>
        </section>
      </div>

      <div style={{ flex: "1 1 100%", display: "flex", flexDirection: "column", gap: 12, paddingTop: 16, borderTop: "1px solid #E5E5EA" }}>
        {blocking.map((d) => (
          <Banner key={d.id} tone="err" icon={<IconAlertCircle size={18} />}>
            Ce fournisseur existe déjà ({reasons(d)}) :{" "}
            <Link href={`/fournisseurs/${d.id}`} style={{ fontWeight: 600 }}>
              {d.name}
            </Link>
            {d.archived ? ", archivé : restaurez-le plutôt que d'en créer un nouveau." : "."}
          </Banner>
        ))}
        {blocking.length === 0 && possible.length > 0 && (
          <Banner tone="warn" icon={<IconWarn size={18} />}>
            <div>
              Fournisseur{possible.length > 1 ? "s" : ""} ressemblant{possible.length > 1 ? "s" : ""} :{" "}
              {possible.map((d, i) => (
                <span key={d.id}>
                  {i > 0 && " ; "}
                  <Link href={`/fournisseurs/${d.id}`} style={{ fontWeight: 600 }}>
                    {d.name}
                  </Link>{" "}
                  ({reasons(d)})
                </span>
              ))}
              .
            </div>
            <label style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 8, fontWeight: 600 }}>
              <input type="checkbox" checked={confirmed} onChange={(e) => setConfirmed(e.target.checked)} style={{ width: 18, height: 18 }} />
              Il s&apos;agit bien d&apos;un autre fournisseur
            </label>
          </Banner>
        )}
        {!duplicateError && <ErrorBanner error={error} />}
        <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, flexWrap: "wrap" }}>
          <Link href={supplier ? `/fournisseurs/${supplier.id}` : "/fournisseurs"} className="btn btn-g">
            Annuler
          </Link>
          <button type="submit" className="btn btn-p" disabled={busy || blocking.length > 0 || (possible.length > 0 && !confirmed)}>
            {busy ? "Enregistrement…" : supplier ? "Enregistrer" : "Créer le fournisseur"}
          </button>
        </div>
      </div>
    </form>
  );
}
