import { expect, type APIRequestContext, type Page, test } from "@playwright/test";

// Les emails de développement arrivent dans Mailpit (SMTP_ADDR de .env.example).
const MAILPIT = process.env.MAILPIT_URL ?? "http://localhost:8025";
const PASSWORD = "motdepasse-test-42";
const NEW_PASSWORD = "nouveau-secret-2026";
const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

async function noHorizontalOverflow(page: Page, where: string) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow, `débordement horizontal sur ${where}`).toBeLessThanOrEqual(0);
}

/** Attend l'email de réinitialisation envoyé à `to` et en retourne le lien. */
async function resetLink(request: APIRequestContext, to: string): Promise<string> {
  let link = "";
  await expect
    .poll(
      async () => {
        const search = await request.get(`${MAILPIT}/api/v1/search`, { params: { query: `to:"${to}"` } });
        if (!search.ok()) return "";
        const { messages } = (await search.json()) as { messages: { ID: string }[] };
        const latest = messages[0];
        if (!latest) return "";
        const message = await (await request.get(`${MAILPIT}/api/v1/message/${latest.ID}`)).json();
        link = (message.Text as string).match(/https?:\/\/\S+\/nouveau-mot-de-passe#[A-Za-z0-9_-]+/)?.[0] ?? "";
        return link;
      },
      { message: `email de réinitialisation pour ${to} (Mailpit sur ${MAILPIT})`, timeout: 15_000 },
    )
    .not.toBe("");
  const url = new URL(link);
  return url.pathname + url.hash;
}

test("mot de passe oublié : lien par email, nouveau mot de passe, connexion", async ({ page, context }, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  const email = `reset-${testInfo.project.name}-${unique}@example.test`;

  const signup = await page.request.post("/api/v1/auth/signup", {
    data: { organization_name: `Entreprise reset ${testInfo.project.name} ${unique}`, full_name: "Moussa Diop", email, password: PASSWORD },
  });
  expect(signup.ok(), await signup.text()).toBe(true);
  await context.clearCookies();

  // Le lien est à l'emplacement de la maquette, à côté du libellé du mot de passe.
  await page.goto("/connexion");
  await page.getByRole("link", { name: "Mot de passe oublié ?" }).click();
  await expect(page.getByRole("heading", { name: "Mot de passe oublié" })).toBeVisible();
  await noHorizontalOverflow(page, "demande de réinitialisation");

  await page.getByLabel("Email ou téléphone").fill(email);
  await page.getByRole("button", { name: "Recevoir le lien" }).click();
  await expect(page.getByRole("heading", { name: "Vérifiez vos emails" })).toBeVisible();
  await noHorizontalOverflow(page, "demande envoyée");

  const link = await resetLink(page.request, email);
  await page.goto(link);
  await expect(page.getByRole("heading", { name: "Nouveau mot de passe" })).toBeVisible();
  await noHorizontalOverflow(page, "nouveau mot de passe");

  await page.getByLabel("Nouveau mot de passe").fill(NEW_PASSWORD);
  await page.getByLabel("Confirmer le mot de passe").fill(`${NEW_PASSWORD}-autre`);
  await page.getByRole("button", { name: "Enregistrer le mot de passe" }).click();
  await expect(page.getByText("Les deux mots de passe ne sont pas identiques.")).toBeVisible();

  await page.getByLabel("Confirmer le mot de passe").fill(NEW_PASSWORD);
  await page.getByRole("button", { name: "Enregistrer le mot de passe" }).click();
  await expect(page.getByText("Mot de passe modifié.")).toBeVisible();
  await expect(page).toHaveURL(/\/connexion\?reinitialise=1$/);

  await page.getByLabel("Email ou téléphone").fill(email);
  await page.getByLabel("Mot de passe", { exact: true }).fill(NEW_PASSWORD);
  await page.getByRole("button", { name: "Se connecter" }).click();
  await expect(page.getByRole("heading", { name: "Bonjour, Moussa." })).toBeVisible();

  // Usage unique : le même lien ne sert plus.
  await page.goto(link);
  await page.getByLabel("Nouveau mot de passe").fill(`${NEW_PASSWORD}-2`);
  await page.getByLabel("Confirmer le mot de passe").fill(`${NEW_PASSWORD}-2`);
  await page.getByRole("button", { name: "Enregistrer le mot de passe" }).click();
  await expect(page.getByRole("heading", { name: "Lien expiré" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Demander un nouveau lien" })).toBeVisible();

  expect(errors, "erreurs JavaScript").toEqual([]);
});

test("mot de passe oublié : un lien sans jeton renvoie vers une nouvelle demande", async ({ page }) => {
  // Sans appel à l'API : la réponse identique pour un compte inconnu est vérifiée par internal/e2e.
  await page.goto("/nouveau-mot-de-passe");
  await expect(page.getByRole("heading", { name: "Lien expiré" })).toBeVisible();
  await page.getByRole("link", { name: "Demander un nouveau lien" }).click();
  await expect(page.getByRole("heading", { name: "Mot de passe oublié" })).toBeVisible();
});
