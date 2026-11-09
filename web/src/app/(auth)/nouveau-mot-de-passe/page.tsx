"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { AuthFrame, AuthHeading, authFormStyle } from "@/components/AuthFrame";
import { IconAlertCircle } from "@/components/icons";
import { Banner, ErrorBanner, fieldError, Input, Loading } from "@/components/ui";
import { api, ApiError } from "@/lib/api";
import { useLocationPart } from "@/lib/hooks";

export default function NewPasswordPage() {
  const router = useRouter();
  // Le jeton arrive dans le fragment (#) : le navigateur ne l'envoie jamais au serveur.
  const hash = useLocationPart("hash");
  const token = hash === null ? null : hash.replace(/^#/, "");
  const [error, setError] = useState<unknown>(null);
  const [mismatch, setMismatch] = useState(false);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    const password = String(form.get("password") ?? "");
    setError(null);
    if (password !== form.get("confirm")) {
      setMismatch(true);
      return;
    }
    setMismatch(false);
    setBusy(true);
    try {
      await api("/auth/password-reset/confirm", { method: "POST", body: { token, password } });
      router.replace("/connexion?reinitialise=1");
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

  if (token === null) {
    return (
      <AuthFrame>
        <Loading />
      </AuthFrame>
    );
  }

  const expired = !token || (error instanceof ApiError && error.code === "password_reset.invalid");
  if (expired) {
    return (
      <AuthFrame>
        <div style={authFormStyle}>
          <AuthHeading title="Lien expiré" />
          <Banner tone="err" icon={<IconAlertCircle size={18} />}>
            Ce lien de réinitialisation n&apos;est plus valable : il a expiré, a déjà servi ou a été remplacé par une demande plus récente.
          </Banner>
          <Link href="/mot-de-passe-oublie" className="btn btn-p" style={{ height: 48, fontSize: 15 }}>
            Demander un nouveau lien
          </Link>
          <p style={{ margin: 0, textAlign: "center", fontSize: 15 }}>
            <Link href="/connexion" style={{ fontWeight: 600, textDecoration: "none" }}>
              Retour à la connexion
            </Link>
          </p>
        </div>
      </AuthFrame>
    );
  }

  return (
    <AuthFrame>
      <form onSubmit={submit} noValidate style={authFormStyle}>
        <AuthHeading title="Nouveau mot de passe">Choisissez le mot de passe de votre compte. Vos appareils connectés seront déconnectés.</AuthHeading>
        <Input label="Nouveau mot de passe" name="password" type="password" autoComplete="new-password" required hint="10 caractères minimum, dont un chiffre." error={fieldError(error, "password")} style={{ height: 48, fontSize: 15 }} />
        <Input label="Confirmer le mot de passe" name="confirm" type="password" autoComplete="new-password" required error={mismatch ? "Les deux mots de passe ne sont pas identiques." : undefined} style={{ height: 48, fontSize: 15 }} />
        {!fieldError(error, "password") && <ErrorBanner error={error} />}
        <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
          {busy ? "Enregistrement…" : "Enregistrer le mot de passe"}
        </button>
      </form>
    </AuthFrame>
  );
}
