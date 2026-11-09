"use client";

import Link from "next/link";
import { type FormEvent, useState } from "react";
import { AuthFrame, AuthHeading, authFormStyle, authInputStyle } from "@/components/AuthFrame";
import { IconBack, IconCheck, IconInfo } from "@/components/icons";
import { Banner, ErrorBanner, fieldError } from "@/components/ui";
import { api } from "@/lib/api";

export default function ForgotPasswordPage() {
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  const [sentTo, setSentTo] = useState<string | null>(null);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const login = String(new FormData(event.currentTarget).get("login") ?? "").trim();
    setBusy(true);
    setError(null);
    try {
      await api("/auth/password-reset", { method: "POST", body: { login } });
      setSentTo(login);
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }

  const back = (
    <p style={{ margin: 0, textAlign: "center", fontSize: 15 }}>
      <Link href="/connexion" style={{ fontWeight: 600, textDecoration: "none", display: "inline-flex", alignItems: "center", gap: 6 }}>
        <IconBack size={16} /> Retour à la connexion
      </Link>
    </p>
  );

  if (sentTo !== null) {
    // Même message que le compte existe ou non : l'écran ne doit rien révéler de plus que l'API.
    return (
      <AuthFrame>
        <div style={authFormStyle}>
          <AuthHeading title="Vérifiez vos emails" />
          <Banner tone="ok" icon={<IconCheck size={18} />}>
            Si un compte correspond à <strong style={{ overflowWrap: "anywhere" }}>{sentTo}</strong>, un lien pour choisir un nouveau mot de passe vient de lui être envoyé par email. Il est valable 30 minutes et ne sert qu&apos;une fois.
          </Banner>
          <p style={{ margin: 0, fontSize: 14, lineHeight: 1.5, color: "#6E6E73" }}>Rien reçu après quelques minutes ? Regardez dans les courriers indésirables, puis refaites une demande : seul le dernier lien reçu fonctionne.</p>
          <button type="button" className="btn btn-s" style={{ height: 48, fontSize: 15 }} onClick={() => setSentTo(null)}>
            Refaire une demande
          </button>
          {back}
        </div>
      </AuthFrame>
    );
  }

  return (
    <AuthFrame>
      <form onSubmit={submit} noValidate style={authFormStyle}>
        <AuthHeading title="Mot de passe oublié">Indiquez l&apos;email ou le téléphone de votre compte. Nous vous enverrons un lien pour en choisir un nouveau.</AuthHeading>
        <div className="f">
          <label htmlFor="login" style={{ fontSize: 14 }}>
            Email ou téléphone
          </label>
          <input id="login" name="login" className="inp" style={authInputStyle} autoComplete="username" required aria-invalid={fieldError(error, "login") ? true : undefined} />
          {fieldError(error, "login") && (
            <span role="alert" className="field-err">
              {fieldError(error, "login")}
            </span>
          )}
        </div>
        <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
          {busy ? "Envoi…" : "Recevoir le lien"}
        </button>
        {!fieldError(error, "login") && <ErrorBanner error={error} />}
        <Banner tone="info" icon={<IconInfo size={18} />}>
          Le lien part par email. Si votre compte n&apos;a qu&apos;un numéro de téléphone, contactez l&apos;administrateur de votre entreprise.
        </Banner>
        {back}
      </form>
    </AuthFrame>
  );
}
