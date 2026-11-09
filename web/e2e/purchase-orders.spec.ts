import { expect, type Page, test } from "@playwright/test";

const PASSWORD = "motdepasse-test-42";
const unique = `${Date.now()}-${Math.floor(Math.random() * 1e6)}`;

async function noHorizontalOverflow(page: Page, where: string) {
  const { overflow, culprits } = await page.evaluate(() => {
    const width = document.documentElement.clientWidth;
    return {
      overflow: document.documentElement.scrollWidth - width,
      culprits: [...document.querySelectorAll("body *")]
        .filter((el) => el.getBoundingClientRect().right > width + 0.5)
        .slice(0, 5)
        .map((el) => `${el.tagName.toLowerCase()}.${String(el.className).slice(0, 40)} (${Math.round(el.getBoundingClientRect().right)} px)`),
    };
  });
  expect(overflow, `débordement horizontal sur ${where} : ${culprits.join(", ")}`).toBeLessThanOrEqual(0);
}

function inDays(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return d.toISOString().slice(0, 10);
}

test("commande : offre retenue, bon de commande, approbation par Finance, envoi, réceptions", async ({ page, context, browser }, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  const id = `po-${testInfo.project.name}-${unique}`;
  const api = async (path: string, data?: unknown) => {
    const csrf = (await context.cookies()).find((c) => c.name === "cf_csrf")?.value ?? "";
    const response = await page.request.post(`/api/v1${path}`, { data, headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() } });
    expect(response.ok(), `${path} : ${await response.text()}`).toBe(true);
    return response.json();
  };

  // Préparation : une consultation close avec une offre.
  await api("/auth/signup", { organization_name: `Entreprise ${id}`, full_name: "Moussa Diop", email: `${id}@example.test`, password: PASSWORD });
  const project = await api("/projects", { reference: `CH-${id}`.slice(0, 40), name: "Villa Almadies", client_name: "M. Sarr", address: "Almadies, Dakar", status: "in_progress", start_date: "2026-01-15" });
  const cement = await api("/materials", { name: "Ciment CPJ 45", unit: "sac" });
  const supplier = await api("/suppliers", { name: "Quincaillerie Ndiaye", email: `devis-${id}@example.test` });
  const rfq = await api(`/projects/${project.id}/rfqs`, {
    title: "Ciment R+1", response_deadline: inDays(5), delivery_date: inDays(10), supplier_ids: [supplier.id],
    items: [{ kind: "material", material_id: cement.id, label: "Ciment CPJ 45", quantity: "100", unit: "sac" }],
  });
  const published = await api(`/rfqs/${rfq.id}/publish`);
  const token = new URL(published.links[0].url).hash.slice(1);
  const view = await api("/supplier/rfq", { token });
  await api("/supplier/offers", { token, delivery_days: 3, payment_terms: "50 % à la commande", lines: [{ rfq_item_id: view.items[0].id, unit_price: 4750 }] });
  await api(`/rfqs/${rfq.id}/close`);
  const invite = await api("/invitations", { full_name: "Awa Fall", email: `finance-${id}@example.test`, role: "accountant" });

  // Sélection de l'offre sur critères, puis génération de la commande.
  await page.goto(`/achats/${rfq.id}`);
  await expect(page.getByRole("heading", { name: "Évaluation des offres" })).toBeVisible();
  await page.getByRole("button", { name: "Retenir", exact: true }).click();
  await page.getByLabel(/Motif du choix/).fill("Seule offre, prix conforme au budget");
  await page.getByRole("button", { name: "Retenir cette offre" }).click();
  await expect(page.getByText(/Offre retenue le/)).toBeVisible();
  await noHorizontalOverflow(page, "évaluation des offres");
  await page.getByRole("button", { name: "Générer le bon de commande" }).click();
  await expect(page.getByRole("heading", { name: "BC-0001 · Quincaillerie Ndiaye" })).toBeVisible();
  await expect(page.getByText("Brouillon", { exact: true })).toBeVisible();

  // Brouillon : TVA, puis soumission.
  await page.getByRole("button", { name: "Appliquer la TVA 18 % partout" }).click();
  await page.getByRole("button", { name: "Enregistrer le brouillon" }).click();
  await expect(page.getByText("TTC 560 500 FCFA")).toBeVisible();
  await noHorizontalOverflow(page, "brouillon de commande");
  await page.getByRole("button", { name: "Soumettre à l'approbation" }).click();
  await expect(page.getByText("À approuver", { exact: true })).toBeVisible();
  await expect(page.getByText("une autre personne doit l'approuver")).toBeVisible();
  const orderUrl = page.url();

  // Finance approuve.
  const finance = await browser.newContext({ baseURL: testInfo.project.use.baseURL, viewport: page.viewportSize(), locale: "fr-FR" });
  const accepted = await finance.request.post(`/api/v1/auth/invitations/${invite.invite_url.split("/").pop()}/accept`, { data: { password: PASSWORD } });
  expect(accepted.ok(), await accepted.text()).toBe(true);
  const financePage = await finance.newPage();
  financePage.on("pageerror", (err) => errors.push(err.message));
  await financePage.goto(new URL(orderUrl).pathname);
  await financePage.getByRole("button", { name: "Approuver" }).click();
  await financePage.getByLabel("Commentaire d'approbation").fill("Budget OK");
  await financePage.getByRole("button", { name: "Confirmer" }).click();
  await expect(financePage.locator(".bdg", { hasText: /^Approuvée$/ })).toBeVisible();
  await finance.close();

  // Envoi, puis deux réceptions dont une partielle.
  await page.reload();
  await page.getByRole("button", { name: "Envoyer au fournisseur" }).click();
  await expect(page.getByText(`Bon de commande envoyé par email à devis-${id}@example.test.`)).toBeVisible();
  await page.getByLabel(/^Reçu/).fill("40");
  await page.getByLabel("N° du bon de livraison").fill("BL-001");
  await noHorizontalOverflow(page, "réception");
  await page.getByRole("button", { name: "Enregistrer la réception" }).click();
  await expect(page.getByText("Reçue en partie", { exact: true })).toBeVisible();
  await expect(page.getByText("Entrée en stock")).toBeVisible();
  await expect(page.getByText("Reste 60 sac sur 100")).toBeVisible();
  await page.getByLabel("N° du bon de livraison").fill("BL-002");
  await page.getByRole("button", { name: "Enregistrer la réception" }).click();
  await expect(page.getByText("Reçue", { exact: true })).toBeVisible();
  await expect(page.getByText("Réception enregistrée").first()).toBeVisible();
  await noHorizontalOverflow(page, "commande reçue");

  await page.goto("/commandes");
  await expect(page.getByRole("link", { name: "BC-0001 · Quincaillerie Ndiaye" })).toBeVisible();
  await noHorizontalOverflow(page, "liste des commandes");
  expect(errors, "erreurs JavaScript").toEqual([]);
});
