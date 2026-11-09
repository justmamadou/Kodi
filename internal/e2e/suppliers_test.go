package e2e

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// E2 : gestion des fournisseurs.

func randomPhone() string { return fmt.Sprintf("+22177%07d", rand.IntN(10_000_000)) }

func audits(t *testing.T, entityID, action string) int {
	t.Helper()
	n, _ := rfqAudit(t, entityID, action)
	return n
}

func TestE2_SupplierRecord(t *testing.T) {
	tn := newTenant(t)
	name := "Quincaillerie N'Diaye " + unique()
	created := tn.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{
		"name": "  " + name + " ", "legal_name": "Quincaillerie Ndiaye et Fils SARL", "legal_form": "sarl",
		"ninea": "004 567 891 2g3", "rccm": "sn-dkr-2015-b-12345", "phone": "77 123 45 67", "email": "Devis@Ndiaye.TEST",
		"address": "Route de Rufisque, km 4", "city": "Dakar", "activities": []string{"materials", "transport", "materials"},
		"notes": "Livre le samedi matin.",
	})
	j := created.json()
	id := j["id"].(string)
	if j["name"] != name || j["ninea"] != "0045678912G3" || j["rccm"] != "SN-DKR-2015-B-12345" || j["phone"] != "+221771234567" ||
		j["email"] != "devis@ndiaye.test" || fmt.Sprint(j["activities"]) != "[materials transport]" || j["is_active"] != true {
		t.Fatalf("fiche normalisée attendue : %s", created.body)
	}
	if strings.Contains(string(created.body), "name_key") {
		t.Error("la clé de doublon reste interne")
	}

	t.Run("saisie invalide, champ par champ", func(t *testing.T) {
		r := tn.owner.do("POST", "/api/v1/suppliers", map[string]any{
			"name": "X", "ninea": "ABC", "legal_form": "llc", "activities": []string{"peinture"}, "phone": "123", "email": "pas-un-email", "rccm": "?",
		})
		expectCode(t, r, 422, "validation.failed")
		fields, _ := r.json()["fields"].(map[string]any)
		for _, f := range []string{"name", "ninea", "legal_form", "activities", "phone", "email", "rccm"} {
			if fields[f] == nil {
				t.Errorf("erreur attendue sur %s : %s", f, r.body)
			}
		}
	})

	t.Run("modification partielle, effacement explicite, audit avant / après", func(t *testing.T) {
		updated := tn.manager.must(200, "PATCH", "/api/v1/suppliers/"+id, map[string]any{"city": "Rufisque", "notes": ""}).json()
		if updated["city"] != "Rufisque" || updated["notes"] != nil || updated["ninea"] != "0045678912G3" {
			t.Fatalf("seuls les champs envoyés changent : %v", updated)
		}
		var before, after string
		if err := env.ownerDB.QueryRow(context.Background(),
			"SELECT before->>'city', after->>'city' FROM audit_logs WHERE entity_id = $1 AND action = 'supplier.updated'", id).Scan(&before, &after); err != nil {
			t.Fatal(err)
		}
		if before != "Dakar" || after != "Rufisque" || audits(t, id, "supplier.created") != 1 {
			t.Fatalf("audit avant / après attendu : %q -> %q", before, after)
		}
	})

	t.Run("archivage : hors du répertoire et des consultations, historique conservé, restauration", func(t *testing.T) {
		archived := tn.accountant.must(200, "POST", "/api/v1/suppliers/"+id+"/archive", nil).json()
		if archived["is_active"] != false || archived["archived_at"] == nil {
			t.Fatalf("fiche archivée attendue : %v", archived)
		}
		expectCode(t, tn.owner.do("POST", "/api/v1/suppliers/"+id+"/archive", nil), 409, "supplier.already_archived")
		expectCode(t, tn.owner.do("PATCH", "/api/v1/suppliers/"+id, map[string]any{"city": "Thiès"}), 409, "supplier.archived")
		for _, s := range tn.owner.must(200, "GET", "/api/v1/suppliers", nil).items() {
			if s["id"] == id {
				t.Fatal("un fournisseur archivé sort du répertoire")
			}
		}
		found := false
		for _, s := range tn.owner.must(200, "GET", "/api/v1/suppliers?include_archived=true", nil).items() {
			found = found || s["id"] == id
		}
		if !found {
			t.Fatal("le répertoire complet garde les fournisseurs archivés")
		}
		project := tn.newProject(t)
		expectCode(t, tn.owner.do("POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{id})), 422, "supplier.not_found")

		// Saisie terrain par le nom : la fiche archivée est réutilisée, sans doublon ni réactivation.
		expense := tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", map[string]any{
			"category_id": tn.categoryID, "description": "Ciment", "amount": 10000, "expense_date": today(),
			"supplier_name": strings.ToUpper(strings.ReplaceAll(name, "'", " ")),
		})
		if expense.str("supplier_id") != id {
			t.Fatalf("la dépense doit réutiliser la fiche existante : %s", expense.body)
		}
		if tn.owner.must(200, "GET", "/api/v1/suppliers/"+id, nil).json()["is_active"] != false {
			t.Fatal("une saisie terrain ne réactive pas un fournisseur archivé")
		}

		tn.owner.must(200, "POST", "/api/v1/suppliers/"+id+"/restore", nil)
		expectCode(t, tn.owner.do("POST", "/api/v1/suppliers/"+id+"/restore", nil), 409, "supplier.not_archived")
		if audits(t, id, "supplier.archived") != 1 || audits(t, id, "supplier.restored") != 1 {
			t.Fatal("archivage et restauration au journal d'audit")
		}
	})
}

func TestE2_DuplicateRules(t *testing.T) {
	tn := newTenant(t)
	phone := randomPhone()
	base := tn.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{
		"name": "Ets Diallo & Fils SARL", "ninea": "123456789", "rccm": "SN-THS-2020-A-777", "phone": phone,
	}).str("id")
	create := func(body map[string]any) response { return tn.owner.do("POST", "/api/v1/suppliers", body) }

	t.Run("bloquants : même nom (casse, accents, ponctuation), NINEA, RCCM — même confirmés", func(t *testing.T) {
		for _, body := range []map[string]any{
			{"name": "ÉTS DIALLO & FILS, S.A.R.L.", "confirm_not_duplicate": true},
			{"name": "Autre nom", "ninea": "123 456 789", "confirm_not_duplicate": true},
			{"name": "Encore un autre", "rccm": "sn-ths-2020-a-777", "confirm_not_duplicate": true},
		} {
			r := create(body)
			expectCode(t, r, 409, "supplier.duplicate")
			if !strings.Contains(r.str("detail"), "Ets Diallo & Fils SARL") {
				t.Errorf("le message nomme la fiche existante : %s", r.body)
			}
		}
	})

	t.Run("probables : même téléphone, même email de contact, nom proche — à confirmer", func(t *testing.T) {
		expectCode(t, create(map[string]any{"name": "Sotrac", "phone": phone}), 409, "supplier.possible_duplicate")
		tn.owner.must(201, "POST", "/api/v1/suppliers/"+base+"/contacts", map[string]any{"full_name": "Awa Diallo", "email": "awa@diallo.test"})
		expectCode(t, create(map[string]any{"name": "Comptoir du Sahel", "email": "AWA@diallo.test"}), 409, "supplier.possible_duplicate")
		expectCode(t, create(map[string]any{"name": "Diallo & Fils"}), 409, "supplier.possible_duplicate")

		dups := tn.owner.must(200, "POST", "/api/v1/suppliers/duplicates", map[string]any{"name": "Diallo & Fils", "phone": phone}).items()
		if len(dups) != 1 || dups[0]["id"] != base || dups[0]["blocking"] != false {
			t.Fatalf("un doublon probable attendu : %v", dups)
		}
		reasons, _ := dups[0]["reasons"].([]any)
		if !slices.Contains(reasons, any("same_phone")) || !slices.Contains(reasons, any("similar_name")) {
			t.Fatalf("raisons attendues : %v", reasons)
		}
		tn.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Sotrac", "phone": phone, "confirm_not_duplicate": true})
	})

	t.Run("modification contrôlée de la même façon", func(t *testing.T) {
		other := tn.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Senbat " + unique()}).str("id")
		expectCode(t, tn.owner.do("PATCH", "/api/v1/suppliers/"+other, map[string]any{"name": "ets diallo et fils sarl", "ninea": "123456789"}), 409, "supplier.duplicate")
		// Une fiche n'est pas son propre doublon.
		tn.owner.must(200, "PATCH", "/api/v1/suppliers/"+base, map[string]any{"name": "Ets Diallo & Fils SARL", "ninea": "123456789"})
	})

	t.Run("règles propres à chaque entreprise", func(t *testing.T) {
		other := newTenant(t)
		other.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Ets Diallo & Fils SARL", "ninea": "123456789", "phone": phone})
	})

	t.Run("créations concurrentes du même fournisseur : une seule passe", func(t *testing.T) {
		ok, statuses := concurrently(6, func(int) response {
			return create(map[string]any{"name": "Cimenterie du Cap " + t.Name()[:4]})
		})
		if ok != 1 {
			t.Fatalf("une seule création attendue : %v", statuses)
		}
	})
}

func TestE2_ContactsAndCommercialTerms(t *testing.T) {
	tn := newTenant(t)
	id := tn.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Sococim " + unique()}).str("id")

	t.Run("contacts : un seul principal, joignable, audités", func(t *testing.T) {
		first := tn.manager.must(201, "POST", "/api/v1/suppliers/"+id+"/contacts", map[string]any{
			"full_name": "Fatou Ndiaye", "job_title": "Commerciale", "phone": "78 111 22 33", "is_primary": true,
		})
		if first.str("phone") != "+221781112233" {
			t.Fatalf("téléphone normalisé attendu : %s", first.body)
		}
		second := tn.manager.must(201, "POST", "/api/v1/suppliers/"+id+"/contacts", map[string]any{
			"full_name": "Ibrahima Sarr", "email": "i.sarr@sococim.test", "is_primary": true,
		}).str("id")
		expectCode(t, tn.manager.do("POST", "/api/v1/suppliers/"+id+"/contacts", map[string]any{"full_name": "Sans moyen"}), 422, "validation.failed")

		contacts := listOf(tn.worker.must(200, "GET", "/api/v1/suppliers/"+id, nil).json()["contacts"])
		if len(contacts) != 2 || contacts[0]["id"] != second || contacts[0]["is_primary"] != true || contacts[1]["is_primary"] != false {
			t.Fatalf("le dernier contact principal remplace le précédent : %v", contacts)
		}
		tn.manager.must(200, "PATCH", "/api/v1/supplier-contacts/"+first.str("id"), map[string]any{"full_name": "Fatou Ndiaye", "phone": "+221781112233", "job_title": "Directrice commerciale"})
		tn.manager.must(204, "DELETE", "/api/v1/supplier-contacts/"+second, nil)
		if audits(t, id, "supplier.contact_added") != 2 || audits(t, id, "supplier.contact_updated") != 1 || audits(t, id, "supplier.contact_removed") != 1 {
			t.Fatal("contacts ajoutés, modifiés et retirés au journal d'audit")
		}
	})

	t.Run("conditions commerciales historisées", func(t *testing.T) {
		v1 := tn.accountant.must(201, "POST", "/api/v1/suppliers/"+id+"/terms", map[string]any{
			"payment_method": "wave", "payment_delay_days": 30, "discount_pct": "2.5", "delivery_days": 2, "delivery_included": true,
		})
		if v1.num("version") != 1 || v1.str("discount_pct") != "2.5" || v1.str("effective_from") != today() {
			t.Fatalf("version 1 attendue, en vigueur aujourd'hui : %s", v1.body)
		}
		v2 := tn.accountant.must(201, "POST", "/api/v1/suppliers/"+id+"/terms", map[string]any{
			"payment_method": "bank_transfer", "payment_delay_days": 45, "minimum_order_amount": 500000, "effective_from": "2027-01-01",
		})
		if v2.num("version") != 2 {
			t.Fatalf("version 2 attendue : %s", v2.body)
		}
		history := tn.owner.must(200, "GET", "/api/v1/suppliers/"+id+"/terms", nil).items()
		if len(history) != 2 || history[0]["version"] != float64(2) || history[1]["payment_method"] != "wave" || history[0]["created_by_name"] == "" {
			t.Fatalf("historique complet attendu : %v", history)
		}
		current, _ := tn.owner.must(200, "GET", "/api/v1/suppliers/"+id, nil).json()["terms"].(map[string]any)
		if current["version"] != float64(2) {
			t.Fatalf("la fiche montre la dernière version : %v", current)
		}
		expectCode(t, tn.accountant.do("POST", "/api/v1/suppliers/"+id+"/terms", map[string]any{}), 422, "validation.failed")
		expectCode(t, tn.accountant.do("POST", "/api/v1/suppliers/"+id+"/terms", map[string]any{"discount_pct": "101"}), 422, "validation.failed")
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE supplier_terms SET payment_delay_days = 1 WHERE supplier_id = $1", id); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("les conditions passées sont immuables : %v", err)
		}
		if audits(t, id, "supplier.terms_recorded") != 2 {
			t.Fatal("chaque version au journal d'audit")
		}
	})

	t.Run("droits", func(t *testing.T) {
		// Chef de chantier : fiche et contacts oui, prix et historique non, création rapide oui.
		detail := tn.worker.must(200, "GET", "/api/v1/suppliers/"+id, nil).json()
		if detail["terms"] != nil || len(listOf(detail["contacts"])) != 1 {
			t.Fatalf("contacts visibles, conditions masquées pour le chef de chantier : %v", detail)
		}
		expectCode(t, tn.worker.do("GET", "/api/v1/suppliers/"+id+"/terms", nil), 403, "auth.forbidden")
		expectCode(t, tn.worker.do("GET", "/api/v1/suppliers/"+id+"/history", nil), 403, "auth.forbidden")
		expectCode(t, tn.worker.do("PATCH", "/api/v1/suppliers/"+id, map[string]any{"city": "Dakar"}), 403, "auth.forbidden")
		expectCode(t, tn.worker.do("POST", "/api/v1/suppliers/"+id+"/contacts", map[string]any{"full_name": "X Y", "email": "x@y.test"}), 403, "auth.forbidden")
		expectCode(t, tn.worker.do("POST", "/api/v1/suppliers/"+id+"/archive", nil), 403, "auth.forbidden")
		tn.worker.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Dépôt du quartier " + unique()})
		// Lecture seule : tout voir, rien changer.
		if tn.viewer.must(200, "GET", "/api/v1/suppliers/"+id, nil).json()["terms"] == nil {
			t.Fatal("la lecture seule voit les conditions commerciales")
		}
		tn.viewer.must(200, "GET", "/api/v1/suppliers/"+id+"/history", nil)
		expectCode(t, tn.viewer.do("POST", "/api/v1/suppliers", map[string]any{"name": "Interdit " + unique()}), 403, "auth.forbidden")
		expectCode(t, tn.viewer.do("POST", "/api/v1/suppliers/"+id+"/terms", map[string]any{"delivery_days": 1}), 403, "auth.forbidden")
	})
}

func TestE2_ConsultationAndPurchaseHistory(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	sup := tn.supplier(t, "Senbat", "")
	rfq := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{sup}, item("Gravier", "10", "m3"))).str("id")
	token := linkToken(t, links(tn.manager.must(200, "POST", "/api/v1/rfqs/"+rfq+"/publish", nil))[0])
	items := listOf(newClient(t).must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": token}).json()["items"])
	newClient(t).must(201, "POST", "/api/v1/supplier/offers", map[string]any{"token": token, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 18000}}})

	purchase := func(c *client, projectID string, amount int64) string {
		id := c.must(201, "POST", "/api/v1/projects/"+projectID+"/expenses", map[string]any{
			"category_id": tn.categoryID, "description": "Gravier 10 m3", "amount": amount, "expense_date": today(), "supplier_id": sup,
		}).str("id")
		c.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
		return id
	}
	approved := purchase(tn.worker, project, 180000)
	tn.accountant.must(200, "POST", "/api/v1/expenses/"+approved+"/approve", nil)
	purchase(tn.worker, project, 25000) // en attente

	// Un chantier dont le conducteur n'est pas responsable.
	elsewhere := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "CH-" + unique(), "name": "Immeuble " + unique(), "client_name": "SCI Baobab", "address": "Plateau, Dakar",
		"start_date": "2026-01-15", "status": "in_progress", "manager_id": tn.ownerID,
	}).str("id")
	purchase(tn.owner, elsewhere, 99000)

	h := tn.owner.must(200, "GET", "/api/v1/suppliers/"+sup+"/history", nil).json()
	consultations := listOf(h["consultations"])
	if len(consultations) != 1 || consultations[0]["rfq_id"] != rfq || consultations[0]["offer_count"] != float64(1) || consultations[0]["last_offer_total"] != float64(180000) {
		t.Fatalf("consultation et dernière offre attendues : %v", consultations)
	}
	totals, _ := h["totals"].(map[string]any)
	if len(listOf(h["purchases"])) != 3 || totals["purchase_count"] != float64(3) || totals["approved_amount"] != float64(180000) {
		t.Fatalf("trois achats, dont 180 000 approuvés : %v", h)
	}
	mine := tn.manager.must(200, "GET", "/api/v1/suppliers/"+sup+"/history", nil).json()
	if len(listOf(mine["purchases"])) != 2 || len(listOf(mine["consultations"])) != 1 {
		t.Fatalf("le conducteur ne voit que l'historique de ses chantiers : %v", mine)
	}
}

// Un fournisseur n'existe que dans l'entreprise qui l'a enregistré : aucune autre ne le voit, ni ses
// contacts, ses conditions ou son historique ; et le lien d'un fournisseur ne montre que sa demande.
func TestE2_SupplierIsolation(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	sup := a.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Fournisseur privé " + unique(), "ninea": "987654321"}).str("id")
	contact := a.owner.must(201, "POST", "/api/v1/suppliers/"+sup+"/contacts", map[string]any{"full_name": "Moussa Fall", "phone": randomPhone()}).str("id")
	a.owner.must(201, "POST", "/api/v1/suppliers/"+sup+"/terms", map[string]any{"payment_delay_days": 30})

	person := map[string]any{"full_name": "Intrus", "email": "x@y.test"}
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/suppliers/" + sup, nil}, {"PATCH", "/suppliers/" + sup, map[string]any{"city": "X"}},
		{"POST", "/suppliers/" + sup + "/archive", nil}, {"POST", "/suppliers/" + sup + "/restore", nil},
		{"GET", "/suppliers/" + sup + "/terms", nil}, {"POST", "/suppliers/" + sup + "/terms", map[string]any{"payment_delay_days": 1}},
		{"GET", "/suppliers/" + sup + "/history", nil}, {"POST", "/suppliers/" + sup + "/contacts", person},
		{"PATCH", "/supplier-contacts/" + contact, person}, {"DELETE", "/supplier-contacts/" + contact, nil},
	} {
		if r := b.owner.do(call.method, "/api/v1"+call.path, call.body); r.status != 404 {
			t.Errorf("%s %s depuis une autre entreprise : %d, 404 attendu", call.method, call.path, r.status)
		}
	}
	for _, s := range b.owner.must(200, "GET", "/api/v1/suppliers?include_archived=true&q=priv", nil).items() {
		if s["id"] == sup {
			t.Fatal("le répertoire d'une autre entreprise ne contient pas ce fournisseur")
		}
	}
	if dups := b.owner.must(200, "POST", "/api/v1/suppliers/duplicates", map[string]any{"name": "Fournisseur privé", "ninea": "987654321"}).items(); len(dups) != 0 {
		t.Fatalf("le contrôle des doublons ne révèle rien d'une autre entreprise : %v", dups)
	}
}
