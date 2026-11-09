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
        .map((el) => `${el.tagName.toLowerCase()}#${el.id}.${String(el.className).slice(0, 40)} (${Math.round(el.getBoundingClientRect().right)} px)`),
    };
  });
  expect(overflow, `débordement horizontal sur ${where} : ${culprits.join(", ")}`).toBeLessThanOrEqual(0);
}

function inDays(days: number): string {
  const d = new Date();
  d.setDate(d.getDate() + days);
  return d.toISOString().slice(0, 10);
}

test("demande de prix : brouillon, publication, offre du fournisseur, comparatif, clôture", async ({ page, context, browser }, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  const id = `rfq-${testInfo.project.name}-${unique}`;
  const api = async (path: string, data: unknown) => {
    const csrf = (await context.cookies()).find((c) => c.name === "cf_csrf")?.value ?? "";
    const response = await page.request.post(`/api/v1${path}`, { data, headers: { "X-CSRF-Token": csrf, "Idempotency-Key": crypto.randomUUID() } });
    expect(response.ok(), `${path} : ${await response.text()}`).toBe(true);
    return response.json();
  };
  await api("/auth/signup", { organization_name: `Entreprise ${id}`, full_name: "Moussa Diop", email: `${id}@example.test`, password: PASSWORD });
  const project = await api("/projects", { reference: `CH-${id}`.slice(0, 40), name: "Villa Almadies", client_name: "M. Sarr", address: "Almadies, Dakar", status: "in_progress", start_date: "2026-01-15" });
  const supplierName = `Quincaillerie Ndiaye ${unique}`;
  const supplier = await api("/suppliers", { name: supplierName, phone: "+221771234567", email: `devis-${id}@example.test` });

  // Brouillon : objet, lignes (matériau et prestation), fournisseur, échéances.
  await page.goto("/achats");
  await expect(page.getByRole("heading", { name: "Demandes de prix" })).toBeVisible();
  await page.getByRole("link", { name: "Nouvelle demande" }).click();
  await expect(page.getByRole("heading", { name: "Nouvelle demande de prix" })).toBeVisible();
  await page.getByLabel("Objet").fill("Gros œuvre R+1");
  await page.getByLabel("Désignation").first().fill("Ciment CPJ 45");
  await page.getByLabel("Quantité").first().fill("200");
  await page.getByRole("button", { name: "Ajouter une ligne" }).click();
  await page.getByRole("group", { name: "Type de la ligne 2" }).getByRole("button", { name: "Prestation" }).click();
  await page.getByLabel("Désignation").nth(1).fill("Location bétonnière");
  await page.getByLabel("Quantité").nth(1).fill("2,5");
  await page.getByLabel("Unité").nth(1).selectOption("jour");
  await page.getByLabel("Ajouter un fournisseur").selectOption(supplier.id);
  await page.getByLabel("Date limite de réponse").fill(inDays(7));
  await noHorizontalOverflow(page, "nouvelle demande de prix");
  await page.getByRole("button", { name: "Enregistrer le brouillon" }).click();

  await expect(page.getByRole("heading", { name: "Gros œuvre R+1" })).toBeVisible();
  await expect(page.getByText("Brouillon", { exact: true })).toBeVisible();
  await expect(page.getByText("Location bétonnière")).toBeVisible();
  await noHorizontalOverflow(page, "brouillon");

  // Publication : le lien s'affiche une fois, avec l'envoi par email.
  await page.getByRole("button", { name: "Publier aux fournisseurs" }).click();
  await expect(page.getByRole("heading", { name: "Liens à transmettre" })).toBeVisible();
  await expect(page.getByText(`Envoyé à devis-${id}@example.test`)).toBeVisible();
  const link = await page.getByLabel(`Lien de ${supplierName}`).inputValue();
  expect(link).toMatch(/\/offre#[A-Za-z0-9_-]+$/);
  await noHorizontalOverflow(page, "liens à transmettre");

  // Le fournisseur, sans compte ni session, dépose son offre.
  const supplierContext = await browser.newContext({ viewport: page.viewportSize(), locale: "fr-FR" });
  const offerPage = await supplierContext.newPage();
  offerPage.on("pageerror", (err) => errors.push(err.message));
  const url = new URL(link);
  await offerPage.goto(url.pathname + url.hash);
  await expect(offerPage.getByRole("heading", { name: "Gros œuvre R+1" })).toBeVisible();
  await expect(offerPage.getByText("Ouverte aux offres")).toBeVisible();
  await offerPage.getByLabel("Prix unitaire (FCFA)").first().fill("4750");
  await offerPage.getByLabel("Prix unitaire (FCFA)").nth(1).fill("25000");
  await expect(offerPage.getByText("1 012 500 FCFA")).toBeVisible();
  await offerPage.getByLabel("Délai de livraison (jours)").fill("3");
  await offerPage.getByLabel("Votre nom").fill("Fatou Ndiaye");
  await noHorizontalOverflow(offerPage, "offre du fournisseur");
  await offerPage.getByRole("button", { name: "Envoyer mon offre" }).click();
  await expect(offerPage.getByText("Offre envoyée (version 1")).toBeVisible();
  await expect(offerPage.getByRole("heading", { name: "Vos offres envoyées" })).toBeVisible();

  // L'acheteur voit l'offre dans le comparatif et l'historique.
  await page.reload();
  await expect(page.getByRole("heading", { name: "Comparatif ligne par ligne" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Évaluation des offres" })).toBeVisible();
  await expect(page.getByRole("columnheader", { name: supplierName })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Historique des offres" })).toBeVisible();
  await expect(page.getByText("1 012 500 FCFA").first()).toBeVisible();
  await noHorizontalOverflow(page, "comparatif");

  // Clôture : le fournisseur ne peut plus envoyer d'offre.
  await page.getByRole("button", { name: "Clôturer la consultation" }).click();
  await expect(page.getByText("Clôturée", { exact: true })).toBeVisible();
  await offerPage.reload();
  await expect(offerPage.getByText("La consultation est close")).toBeVisible();
  await expect(offerPage.getByRole("button", { name: /Envoyer/ })).toHaveCount(0);

  await page.goto("/achats");
  await expect(page.getByRole("link", { name: "Gros œuvre R+1", exact: true })).toBeVisible();
  await noHorizontalOverflow(page, "liste des demandes");
  await supplierContext.close();
  expect(errors, "erreurs JavaScript").toEqual([]);
});
