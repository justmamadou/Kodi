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

test("fournisseurs : fiche, doublons, contacts, conditions historisées, archivage", async ({ page }, testInfo) => {
  const errors: string[] = [];
  page.on("pageerror", (err) => errors.push(err.message));
  const id = `sup-${testInfo.project.name}-${unique}`;
  const signup = await page.request.post("/api/v1/auth/signup", {
    data: { organization_name: `Entreprise ${id}`, full_name: "Moussa Diop", email: `${id}@example.test`, password: PASSWORD },
  });
  expect(signup.ok(), await signup.text()).toBe(true);

  // Fiche légale et commerciale.
  await page.goto("/fournisseurs");
  await page.getByRole("link", { name: "Nouveau fournisseur" }).click();
  await expect(page.getByRole("heading", { name: "Nouveau fournisseur" })).toBeVisible();
  await page.getByLabel("Nom usuel").fill("Quincaillerie Ndiaye");
  await page.getByLabel("Raison sociale").fill("Quincaillerie Ndiaye et Fils");
  await page.getByLabel("Forme juridique").selectOption("sarl");
  await page.getByLabel("NINEA").fill("004 567 891 2g3");
  await page.getByRole("button", { name: "Matériaux" }).click();
  await page.getByLabel("Téléphone").fill("77 123 45 67");
  await page.getByLabel("Email").fill("devis@ndiaye.test");
  await noHorizontalOverflow(page, "nouveau fournisseur");
  await page.getByRole("button", { name: "Créer le fournisseur" }).click();
  await expect(page.getByRole("heading", { name: "Quincaillerie Ndiaye", level: 1 })).toBeVisible();
  await expect(page.getByText("0045678912G3")).toBeVisible();
  await expect(page.getByText("+221771234567")).toBeVisible();
  const supplierUrl = page.url();

  // Contacts.
  await page.getByRole("button", { name: "Ajouter un contact" }).click();
  await page.getByLabel(/^Nom/).fill("Fatou Ndiaye");
  await page.getByLabel("Fonction").fill("Commerciale");
  await page.getByLabel("Téléphone").fill("78 111 22 33");
  await page.getByLabel("Contact principal").check();
  await page.getByRole("button", { name: "Ajouter le contact" }).click();
  await expect(page.getByText("Commerciale · +221781112233")).toBeVisible();
  await expect(page.getByText("Principal", { exact: true })).toBeVisible();

  // Conditions commerciales : deux versions, la première reste.
  await page.getByRole("button", { name: "Renseigner les conditions" }).click();
  await page.getByLabel("Moyen de paiement").selectOption("wave");
  await page.getByLabel("Délai de paiement (jours)").fill("30");
  await page.getByRole("button", { name: "Enregistrer une nouvelle version" }).click();
  await expect(page.getByText("Paiement : Wave · à 30 jours")).toBeVisible();
  await page.getByRole("button", { name: "Nouvelles conditions" }).click();
  await page.getByLabel("Délai de paiement (jours)").fill("45");
  await page.getByRole("button", { name: "Enregistrer une nouvelle version" }).click();
  await expect(page.getByText("Paiement : Wave · à 45 jours")).toBeVisible();
  await page.getByRole("button", { name: "Voir l'historique" }).click();
  await expect(page.getByRole("cell", { name: "Paiement : Wave · à 30 jours" })).toBeVisible();
  await noHorizontalOverflow(page, "fiche fournisseur");

  // Doublons : bloquant sur le nom (casse et ponctuation ignorées), à confirmer sur le téléphone.
  await page.goto("/fournisseurs/nouveau");
  await page.getByLabel("Nom usuel").fill("QUINCAILLERIE N'DIAYE");
  await page.getByLabel("Raison sociale").click();
  await expect(page.getByText("Ce fournisseur existe déjà (même nom")).toBeVisible();
  await expect(page.getByRole("button", { name: "Créer le fournisseur" })).toBeDisabled();
  await page.getByLabel("Nom usuel").fill("Sotrac Matériaux");
  await page.getByLabel("Téléphone").fill("+221771234567");
  await page.getByLabel("Email").click();
  await expect(page.getByText("même téléphone")).toBeVisible();
  await expect(page.getByRole("button", { name: "Créer le fournisseur" })).toBeDisabled();
  await page.getByLabel("Il s'agit bien d'un autre fournisseur").check();
  await noHorizontalOverflow(page, "doublon probable");
  await page.getByRole("button", { name: "Créer le fournisseur" }).click();
  await expect(page.getByRole("heading", { name: "Sotrac Matériaux", level: 1 })).toBeVisible();

  // Archivage : la fiche quitte le répertoire actif, son historique reste.
  await page.goto(supplierUrl);
  await page.getByRole("button", { name: "Archiver" }).click();
  await expect(page.getByText("Archivé", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Historique" })).toBeVisible();
  await page.goto("/fournisseurs");
  await expect(page.getByRole("link", { name: "Sotrac Matériaux", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Quincaillerie Ndiaye", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: /Archivés/ }).click();
  await expect(page.getByRole("link", { name: "Quincaillerie Ndiaye", exact: true })).toBeVisible();
  await noHorizontalOverflow(page, "répertoire");

  expect(errors, "erreurs JavaScript").toEqual([]);
});
