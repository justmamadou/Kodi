import { expect, type Page, test } from "@playwright/test";

const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;
const PASSWORD = "motdepasse-test-42";

async function noHorizontalOverflow(page: Page, where: string) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow, `débordement horizontal sur ${where}`).toBeLessThanOrEqual(0);
}

async function tapTargetsOk(page: Page, where: string) {
  // Les hauteurs des maquettes bureau (filtres, boutons de ligne) ne visent pas le doigt.
  if ((page.viewportSize()?.width ?? 0) >= 1024) return;
  const tooSmall = await page.evaluate(() =>
    [...document.querySelectorAll<HTMLElement>("main button, main input:not([type=checkbox]):not([type=radio]):not([type=file]), main select")]
      .filter((el) => {
        const box = el.getBoundingClientRect();
        return box.height > 0 && box.height < 40;
      })
      .map((el) => el.outerHTML.slice(0, 80)),
  );
  expect(tooSmall, `cibles tactiles trop petites sur ${where}`).toEqual([]);
}

async function check(page: Page, where: string) {
  await expect(page.locator('main [role="alert"]')).toHaveCount(0);
  await noHorizontalOverflow(page, where);
  await tapTargetsOk(page, where);
}

test("parcours essentiels, sans débordement ni erreur", async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));

  const id = `${testInfo.project.name}-${unique}`;

  // Inscription : les erreurs de formulaire sont explicites, champ par champ.
  await page.goto("/inscription");
  await page.getByRole("button", { name: "Créer mon entreprise" }).click();
  await expect(page.getByText("Le téléphone est obligatoire.")).toBeVisible();
  await noHorizontalOverflow(page, "inscription avec erreurs");

  await page.locator("#full_name").fill("Moussa Diop");
  await page.locator("#phone").fill(`77${String(Math.floor(Math.random() * 1e7)).padStart(7, "0")}`);
  await page.locator("#email").fill(`${id}@example.test`);
  await page.locator("#password").fill(PASSWORD);
  await page.locator("#organization_name").fill(`Entreprise ${id}`);
  await page.getByLabel("J'accepte les conditions").check();
  await page.getByRole("button", { name: "Créer mon entreprise" }).click();
  await expect(page.getByRole("heading", { name: "Bonjour, Moussa." })).toBeVisible();
  await check(page, "tableau de bord");

  // Chantier : informations, budget réparti et publié en une seule saisie.
  await page.goto("/chantiers/nouveau");
  await page.getByRole("button", { name: "Créer le chantier" }).click();
  await expect(page.getByText("Indiquez le budget total du chantier.")).toBeVisible();
  await page.getByLabel("Référence").fill(`CH-${id}`);
  await page.getByLabel("Nom du chantier").fill("Villa Almadies");
  await page.getByLabel("Client / maître d'ouvrage").fill("M. Sarr");
  await page.getByLabel("Adresse du chantier").fill("Almadies, Dakar");
  await page.locator("#bud").fill("500000");
  await page.getByLabel("Matériaux").fill("400000");
  await expect(page.getByText("Reste à répartir")).toBeVisible();
  await page.getByLabel("Divers").fill("100000");
  await expect(page.getByText("Répartition complète")).toBeVisible();
  await noHorizontalOverflow(page, "nouveau chantier");
  await page.getByRole("button", { name: "Créer le chantier" }).click();
  await expect(page.getByRole("heading", { name: "Villa Almadies" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Situation financière" })).toBeVisible();
  await expect(page.getByText("500 000").first()).toBeVisible();
  await check(page, "chantier");

  // Budget publié dès la création.
  await page.getByRole("tab", { name: "Budget" }).click();
  await expect(page.getByText("Publiée", { exact: true })).toBeVisible();
  await noHorizontalOverflow(page, "budget publié");

  // Chaque onglet s'affiche sans erreur.
  for (const tab of ["Vue d'ensemble", "Dépenses", "Matériaux", "Tâches", "Rapports journaliers", "Équipe", "Documents", "Historique"]) {
    await page.getByRole("tab", { name: tab }).click();
    await page.waitForTimeout(300);
    await check(page, `onglet ${tab}`);
  }
  await expect(page.getByText("Budget publié").first()).toBeVisible();

  // Dépense : le formulaire de terrain, sur le téléphone.
  await page.goto("/depenses/nouvelle");
  await page.getByRole("button", { name: "Soumettre" }).click();
  await expect(page.getByText("Choisissez une catégorie.")).toBeVisible();
  await noHorizontalOverflow(page, "dépense avec erreurs");
  await page.locator("#mt").fill("450000");
  await page.getByRole("button", { name: "Matériaux" }).click();
  await page.locator("#ds").fill("Ciment 50 sacs");
  await page.locator("#fo").fill("Quincaillerie Ndiaye");
  await page.getByRole("button", { name: "Oui, payée" }).click();
  await page.getByRole("button", { name: "Wave", exact: true }).click();
  await page.getByRole("button", { name: "Soumettre" }).click();
  await expect(page.getByRole("heading", { name: "Ciment 50 sacs" })).toBeVisible();

  // Validation : la dépense attend, et son auteur ne peut pas la décider lui-même.
  await page.goto("/depenses");
  await expect(page.getByRole("heading", { name: "Ciment 50 sacs" })).toBeVisible();
  await expect(page.getByText("une autre personne doit la valider")).toBeVisible();
  await expect(page.getByText("1 dépense à valider · 450 000 FCFA")).toBeVisible();
  await noHorizontalOverflow(page, "validation");

  // Rapport : brouillon local, puis publication.
  await page.goto("/rapports/nouveau");
  await page.locator("#tr").fill("Coffrage de la dalle terminé.");
  await page.getByRole("button", { name: "Ajouter un ouvrier" }).click();
  await page.getByRole("button", { name: "Ensoleillé" }).click();
  await expect(page.getByText("Brouillon enregistré")).toBeVisible();
  await noHorizontalOverflow(page, "rapport");
  await page.getByRole("button", { name: "Publier le rapport" }).click();
  await expect(page.getByRole("heading", { name: "Bonjour, Moussa." })).toBeVisible();

  // Rapports : le comparatif et les rapports publiés sont là.
  await page.goto("/rapports");
  await expect(page.getByRole("heading", { name: "Comparatif par chantier" })).toBeVisible();
  await expect(page.getByText("Coffrage de la dalle terminé.")).toBeVisible();

  for (const [path, title] of [
    ["/", "Bonjour, Moussa."],
    ["/depenses", "Dépenses"],
    ["/chantiers", "Chantiers"],
    ["/materiaux", "Matériaux"],
    ["/achats", "Demandes de prix"],
    ["/achats/nouvelle", "Nouvelle demande de prix"],
    ["/commandes", "Commandes"],
    ["/rapports", "Rapports"],
    ["/equipe", "Équipe et permissions"],
    ["/fournisseurs", "Fournisseurs"],
    ["/fournisseurs/nouveau", "Nouveau fournisseur"],
    ["/parametres", "Paramètres"],
    ["/alertes", "Alertes"],
    ["/profil", "Profil"],
    ["/ajouter", "Ajouter sur Villa Almadies"],
    ["/rapports/nouveau", "Rapport du jour"],
  ] as const) {
    await page.goto(path);
    await expect(page.getByRole("heading", { name: title, exact: true })).toBeVisible();
    await page.waitForTimeout(200);
    await check(page, path);
  }

  expect(errors, "erreurs JavaScript").toEqual([]);
});

test("hors connexion : la dépense attend sur le téléphone, puis part toute seule", async ({ page, context }, testInfo) => {
  const id = `off-${testInfo.project.name}-${unique}`;
  const api = async (path: string, data: unknown) => {
    const csrf = (await context.cookies()).find((c) => c.name === "cf_csrf")?.value ?? "";
    const response = await page.request.post(`/api/v1${path}`, { data, headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() } });
    expect(response.ok(), `${path} : ${await response.text()}`).toBe(true);
    return response.json();
  };
  await api("/auth/signup", {
    organization_name: `Entreprise ${id}`,
    full_name: "Moussa Diop",
    email: `${id}@example.test`,
    phone: `+22177${String(Math.floor(Math.random() * 1e7)).padStart(7, "0")}`,
    password: PASSWORD,
    activity_type: "villas",
    city: "Dakar",
  });
  const project = await api("/projects", { reference: `CH-${id}`.slice(0, 40), name: "Villa Almadies", client_name: "M. Sarr", address: "Dakar", status: "in_progress", start_date: "2026-01-15" });

  // Une visite en ligne suffit à garder le nécessaire sur l'appareil.
  await page.goto(`/depenses/nouvelle?project=${project.id}`);
  await expect(page.getByRole("button", { name: "Matériaux" })).toBeVisible();
  await page.evaluate(() => navigator.serviceWorker.ready);
  await page.goto("/ajouter");
  await expect(page.getByRole("heading", { name: "Ajouter sur Villa Almadies" })).toBeVisible();
  await page.goto(`/depenses/nouvelle?project=${project.id}`);
  await expect(page.getByRole("button", { name: "Matériaux" })).toBeVisible();

  await context.setOffline(true);
  await expect(page.getByText("Hors ligne")).toBeVisible();
  await page.locator("#mt").fill("120000");
  await page.getByRole("button", { name: "Matériaux" }).click();
  await page.locator("#ds").fill("Gravier 10 m3");
  await page.getByRole("button", { name: "Soumettre" }).click();
  await expect(page.getByText("En attente d'envoi")).toBeVisible();
  await expect(page.getByText("Dépense · Gravier 10 m3")).toBeVisible();

  await context.setOffline(false);
  await expect(page.getByText("En attente d'envoi")).toBeHidden({ timeout: 20_000 });
  const list = await (await page.request.get("/api/v1/expenses?status=pending")).json();
  expect(list.items.map((e: { description: string }) => e.description)).toEqual(["Gravier 10 m3"]);
});

test("l'application est installable", async ({ page, request }) => {
  const manifest = await (await request.get("/manifest.webmanifest")).json();
  expect(manifest.display).toBe("standalone");
  expect(manifest.start_url).toBe("/");
  expect(manifest.name).toBeTruthy();
  const sizes = manifest.icons.map((i: { sizes: string }) => i.sizes);
  expect(sizes).toContain("192x192");
  expect(sizes).toContain("512x512");
  expect(manifest.icons.some((i: { purpose: string }) => i.purpose === "maskable")).toBe(true);
  for (const icon of manifest.icons) {
    expect((await request.get(icon.src)).ok()).toBe(true);
  }
  await page.goto("/connexion");
  await expect(page.locator('link[rel="manifest"]')).toHaveCount(1);
  await expect
    .poll(() => page.evaluate(async () => (await navigator.serviceWorker.ready).active?.state))
    .toBe("activated");
});
