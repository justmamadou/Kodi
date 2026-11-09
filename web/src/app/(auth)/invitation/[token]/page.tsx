"use client";

import { useParams, useRouter } from "next/navigation";
import { type FormEvent, useState } from "react";
import { IconHelmet } from "@/components/icons";
import { ErrorBanner, fieldError, Input } from "@/components/ui";
import { api } from "@/lib/api";

export default function InvitationPage() {
  const { token } = useParams<{ token: string }>();
  const router = useRouter();
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = new FormData(event.currentTarget);
    setBusy(true);
    setError(null);
    try {
      await api(`/auth/invitations/${encodeURIComponent(token)}/accept`, { method: "POST", body: { password: form.get("password"), full_name: form.get("full_name") || null } });
      router.replace("/");
    } catch (err) {
      setError(err);
      setBusy(false);
    }
  }

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
      <form onSubmit={submit} noValidate style={{ width: "100%", maxWidth: 440, marginTop: 28, background: "#FFFFFF", border: "1px solid #E5E5EA", borderRadius: 16, padding: 28, display: "flex", flexDirection: "column", gap: 20 }}>
        <div>
          <h1 style={{ margin: 0, fontSize: 28, fontWeight: 700, letterSpacing: "-0.02em" }}>Activer votre compte</h1>
          <p style={{ margin: "8px 0 0", fontSize: 15, color: "#6E6E73" }}>Choisissez votre mot de passe pour rejoindre votre entreprise.</p>
        </div>
        <Input label="Votre nom" name="full_name" autoComplete="name" hint="Laissez vide pour garder le nom indiqué dans l'invitation." />
        <Input label="Mot de passe" name="password" type="password" autoComplete="new-password" required hint="10 caractères minimum, dont un chiffre." error={fieldError(error, "password")} />
        <ErrorBanner error={error} />
        <button type="submit" className="btn btn-p" disabled={busy} style={{ height: 48, fontSize: 15 }}>
          {busy ? "Activation…" : "Activer mon compte"}
        </button>
      </form>
    </div>
  );
}
