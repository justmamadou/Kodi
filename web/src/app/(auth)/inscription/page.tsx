"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, type ReactNode, useState } from "react";
import { IconHelmet, IconInfo } from "@/components/icons";
import { ErrorBanner } from "@/components/ui";
import { api, ApiError } from "@/lib/api";
import { ACTIVITY_LABEL, CITIES } from "@/lib/format";

const H = 46;

function Step({ n, children }: { n: number; children: ReactNode }) {
  return (
    <legend style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 16, fontWeight: 700, marginBottom: 16 }}>
      <span style={{ width: 26, height: 26, borderRadius: "50%", background: "#1D1D1F", color: "#FFFFFF", fontSize: 13, display: "inline-flex", alignItems: "center", justifyContent: "center" }}>{n}</span>
      {children}
    </legend>
  );
}

function strength(password: string): number {
  let score = 0;
  if (password.length >= 10) score++;
  if (/\d/.test(password)) score++;
  if (/[a-zà-ÿ]/.test(password) && /[A-ZÀ-Ý]/.test(password)) score++;
  if (password.length >= 14 || /[^\p{L}\p{N}]/u.test(password)) score++;
  return score;
}

const STRENGTH_LABEL = ["Trop faible", "Faible", "Moyen", "Robuste", "Très robuste"];

export default function SignupPage() {
  const router = useRouter();
  const [error, setError] = useState<unknown>(null);
  const [local, setLocal] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(false);
  const [password, setPassword] = useState("");

  const fieldErr = (name: string) => local[name] ?? (error instanceof ApiError ? error.fields[name] : undefined);
  const err = (name: string) => {
    const message = fieldErr(name);
    return message ? (
      <span role="alert" className="field-err">
        {message}
      </span>
    ) : null;
  };

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const problems: Record<string, string> = {};
    if (!String(form.get("phone")).trim()) problems.phone = "Le téléphone est obligatoire.";
    if (!String(form.get("email")).trim()) problems.email = "L'email est obligatoire.";
    if (form.get("terms") !== "on") problems.terms = "Vous devez accepter les conditions pour continuer.";
    setLocal(problems);
    if (Object.keys(problems).length > 0) {
      setError(null);
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const digits = String(form.get("phone")).replace(/[\s.-]/g, "");
      await api("/auth/signup", {
        method: "POST",
        body: {
          organization_name: form.get("organization_name"),
          full_name: form.get("full_name"),
          email: form.get("email"),
          phone: digits.startsWith("+") ? digits : `+221${digits}`,
          password: form.get("password"),
          company_phone: form.get("company_phone") || null,
          activity_type: form.get("activity_type"),
          city: form.get("city"),
          address: form.get("address") || null,
        },
      });
      router.replace("/");
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  const score = strength(password);
  return (
    <div style={{ minHeight: "100dvh", display: "flex", flexDirection: "column", alignItems: "center", padding: "40px 24px 56px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <div style={{ width: 40, height: 40, borderRadius: 10, background: "#FFC21A", color: "#1D1D1F", display: "flex", alignItems: "center", justifyContent: "center" }}>
          <IconHelmet size={22} width={2} />
        </div>
        <div style={{ fontSize: 20, fontWeight: 700 }}>
          Kodi
        </div>
      </div>
      <form onSubmit={submit} noValidate className="signup-form" style={{ width: "100%", maxWidth: 720, marginTop: 28, background: "#FFFFFF", border: "1px solid #E5E5EA", borderRadius: 16, padding: 36, display: "flex", flexDirection: "column", gap: 28 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 28, fontWeight: 700, letterSpacing: "-0.02em" }}>Créer votre espace entreprise</h1>
          <p style={{ margin: "8px 0 0", fontSize: 15, color: "#6E6E73" }}>Vous deviendrez administrateur de l&apos;organisation et pourrez inviter vos équipes ensuite.</p>
        </div>
        <fieldset style={{ border: 0, margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 16 }}>
          <Step n={1}>Votre compte</Step>
          <div className="g2">
            <div className="f">
              <label htmlFor="full_name" style={{ fontSize: 14 }}>Nom et prénom <span className="req">*</span></label>
              <input id="full_name" name="full_name" className="inp" style={{ height: H }} autoComplete="name" aria-invalid={fieldErr("full_name") ? true : undefined} />
              {err("full_name")}
            </div>
            <div className="f">
              <label htmlFor="phone" style={{ fontSize: 14 }}>Téléphone <span className="req">*</span></label>
              <div style={{ display: "flex", gap: 8 }}>
                <span style={{ height: H, padding: "0 12px", border: "1px solid #D2D2D7", borderRadius: 10, display: "flex", alignItems: "center", fontWeight: 600, background: "#F5F5F7" }}>+221</span>
                <input id="phone" name="phone" type="tel" className="inp" style={{ height: H }} autoComplete="tel-national" placeholder="77 123 45 67" aria-invalid={fieldErr("phone") ? true : undefined} />
              </div>
              {err("phone")}
            </div>
          </div>
          <div className="f">
            <label htmlFor="email" style={{ fontSize: 14 }}>Email professionnel <span className="req">*</span></label>
            <input id="email" name="email" type="email" className="inp" style={{ height: H }} autoComplete="email" aria-invalid={fieldErr("email") ? true : undefined} />
            {fieldErr("email") ? err("email") : <span className="hint">Une adresse Gmail ou Yahoo est acceptée.</span>}
          </div>
          <div className="f">
            <label htmlFor="password" style={{ fontSize: 14 }}>Mot de passe <span className="req">*</span></label>
            <input id="password" name="password" type="password" className="inp" style={{ height: H }} autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} aria-invalid={fieldErr("password") ? true : undefined} />
            <div style={{ display: "flex", gap: 6, marginTop: 4 }} aria-hidden="true">
              {[0, 1, 2, 3].map((i) => (
                <span key={i} style={{ flex: 1, height: 4, borderRadius: 2, background: i < score ? (score >= 3 ? "#1A8F4C" : "#C76A00") : "#E5E5EA" }} />
              ))}
            </div>
            {fieldErr("password") ? err("password") : <span className="hint">{password ? `${STRENGTH_LABEL[score]} · ` : ""}10 caractères minimum, dont un chiffre.</span>}
          </div>
        </fieldset>
        <div style={{ height: 1, background: "#E5E5EA" }} />
        <fieldset style={{ border: 0, margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 16 }}>
          <Step n={2}>Votre entreprise</Step>
          <div className="g2">
            <div className="f">
              <label htmlFor="organization_name" style={{ fontSize: 14 }}>Nom de l&apos;entreprise <span className="req">*</span></label>
              <input id="organization_name" name="organization_name" className="inp" style={{ height: H }} aria-invalid={fieldErr("organization_name") ? true : undefined} />
              {err("organization_name")}
            </div>
            <div className="f">
              <label htmlFor="company_phone" style={{ fontSize: 14 }}>Téléphone de l&apos;entreprise</label>
              <input id="company_phone" name="company_phone" type="tel" className="inp" style={{ height: H }} placeholder="+221 33 820 45 10" />
            </div>
          </div>
          <div className="g2">
            <div className="f">
              <label htmlFor="activity_type" style={{ fontSize: 14 }}>Type d&apos;activité BTP <span className="req">*</span></label>
              <select id="activity_type" name="activity_type" className="inp" style={{ height: H }} defaultValue="villas">
                {Object.entries(ACTIVITY_LABEL).map(([value, label]) => <option key={value} value={value}>{label}</option>)}
              </select>
            </div>
            <div className="f">
              <label htmlFor="city" style={{ fontSize: 14 }}>Ville <span className="req">*</span></label>
              <select id="city" name="city" className="inp" style={{ height: H }} defaultValue="Dakar">
                {CITIES.map((c) => <option key={c}>{c}</option>)}
              </select>
            </div>
          </div>
          <div className="f">
            <label htmlFor="address" style={{ fontSize: 14 }}>Adresse</label>
            <input id="address" name="address" className="inp" style={{ height: H }} />
          </div>
          <div className="info" style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 10, fontSize: 14 }}>
            <IconInfo size={18} />
            <span>Pays : Sénégal · Devise : FCFA (XOF) · Fuseau : Africa/Dakar. Modifiable dans Paramètres.</span>
          </div>
        </fieldset>
        <div className="f">
          <label style={{ display: "flex", alignItems: "flex-start", gap: 10, fontSize: 14, color: "#48484A", fontWeight: 400 }}>
            <input type="checkbox" name="terms" style={{ width: 18, height: 18, marginTop: 1 }} /> J&apos;accepte les conditions d&apos;utilisation et la politique de confidentialité.
          </label>
          {err("terms")}
        </div>
        <ErrorBanner error={error} />
        <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
          {busy ? "Création…" : "Créer mon entreprise"}
        </button>
        <p style={{ margin: 0, textAlign: "center", fontSize: 15, color: "#6E6E73" }}>
          Déjà inscrit ?{" "}
          <Link href="/connexion" style={{ fontWeight: 600 }}>
            Se connecter
          </Link>
        </p>
      </form>
      <style>{`@media (max-width: 600px) { .signup-form { padding: 20px !important; } }`}</style>
    </div>
  );
}
