"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { AuthFrame, AuthHeading, authFormStyle, authInputStyle } from "@/components/AuthFrame";
import { IconAlertCircle, IconCheck } from "@/components/icons";
import { Banner } from "@/components/ui";
import { api } from "@/lib/api";
import { useLocationPart } from "@/lib/hooks";

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  // Retour de /nouveau-mot-de-passe.
  const resetDone = new URLSearchParams(useLocationPart("search") ?? "").has("reinitialise");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await api("/auth/login", { method: "POST", body: { login: form.get("login"), password: form.get("password"), remember: form.get("remember") === "on" } });
      router.replace("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Une erreur est survenue.");
      setBusy(false);
    }
  }

  return (
    <AuthFrame>
      <form onSubmit={submit} style={authFormStyle}>
        <AuthHeading title="Connexion">Accédez à l&apos;espace de votre entreprise.</AuthHeading>
        {resetDone && (
          <Banner tone="ok" icon={<IconCheck size={18} />}>
            Mot de passe modifié. Connectez-vous avec le nouveau : vos autres appareils ont été déconnectés.
          </Banner>
        )}
        <div className="f">
          <label htmlFor="login" style={{ fontSize: 14 }}>
            Email ou téléphone
          </label>
          <input id="login" name="login" className="inp" style={authInputStyle} autoComplete="username" required />
        </div>
        <div className="f">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 12 }}>
            <label htmlFor="password" style={{ fontSize: 14 }}>
              Mot de passe
            </label>
            <Link href="/mot-de-passe-oublie" style={{ fontSize: 14, fontWeight: 600, textDecoration: "none" }}>
              Mot de passe oublié ?
            </Link>
          </div>
          <input id="password" name="password" type="password" className="inp" style={authInputStyle} autoComplete="current-password" required />
        </div>
        <label style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 14 }}>
          <input type="checkbox" name="remember" defaultChecked style={{ width: 18, height: 18 }} /> Rester connecté sur cet appareil
        </label>
        <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
          {busy ? "Connexion…" : "Se connecter"}
        </button>
        {error && (
          <div role="alert" style={{ display: "flex", gap: 10, padding: "12px 14px", borderRadius: 10, background: "#FDECEA", color: "#B3261E", fontSize: 14 }}>
            <IconAlertCircle size={18} />
            <span>{error} Après 5 essais, la connexion est suspendue 15 minutes.</span>
          </div>
        )}
        <p style={{ margin: 0, textAlign: "center", fontSize: 15, color: "#6E6E73" }}>
          Nouvelle entreprise ?{" "}
          <Link href="/inscription" style={{ fontWeight: 600 }}>
            Créer un compte
          </Link>
        </p>
      </form>
    </AuthFrame>
  );
}
