package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// E1 : demandes de prix, consultation des fournisseurs, réception des offres.

func (tn *tenant) supplier(t *testing.T, name, email string) string {
	t.Helper()
	// Noms et numéros distincts, et doublon probable assumé : ces tests ne portent pas sur les doublons (E2).
	body := map[string]any{"name": name + " " + unique(), "phone": "+221771234567", "confirm_not_duplicate": true}
	if email != "" {
		body["email"] = email
	}
	r := tn.owner.must(201, "POST", "/api/v1/suppliers", body)
	if email != "" && r.str("email") != strings.ToLower(email) {
		t.Fatalf("email du fournisseur non enregistré : %s", r.body)
	}
	return r.str("id")
}

func dayOffset(days int) string { return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02") }

func rfqBody(suppliers []string, items ...map[string]any) map[string]any {
	return map[string]any{
		"title": "Gros œuvre R+1", "notes": "Livraison sur chantier, accès camion.",
		"response_deadline": dayOffset(7), "delivery_date": dayOffset(20),
		"items": items, "supplier_ids": suppliers,
	}
}

func item(label, quantity, unit string) map[string]any {
	return map[string]any{"kind": "material", "label": label, "quantity": quantity, "unit": unit, "specifications": "Norme NF"}
}

func linkToken(t *testing.T, link map[string]any) string {
	t.Helper()
	url, _ := link["url"].(string)
	i := strings.Index(url, "/offre#")
	if !strings.HasPrefix(url, "http://app.test/offre#") || i < 0 {
		t.Fatalf("lien de consultation inattendu : %q", url)
	}
	return url[i+len("/offre#"):]
}

func links(r response) []map[string]any {
	raw, _ := r.json()["links"].([]any)
	out := make([]map[string]any, len(raw))
	for i, l := range raw {
		out[i], _ = l.(map[string]any)
	}
	return out
}

func listOf(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, len(raw))
	for i, x := range raw {
		out[i], _ = x.(map[string]any)
	}
	return out
}

func rfqAudit(t *testing.T, rfqID, action string) (count int, actorless int) {
	t.Helper()
	if err := env.ownerDB.QueryRow(context.Background(),
		"SELECT count(*), count(*) FILTER (WHERE actor_id IS NULL) FROM audit_logs WHERE entity_id = $1 AND action = $2",
		rfqID, action).Scan(&count, &actorless); err != nil {
		t.Fatal(err)
	}
	return count, actorless
}

func TestE1_RFQLifecycle(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	cement := tn.owner.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Ciment CPJ 45 " + unique(), "unit": "sac"}).str("id")
	emailed := "devis-" + unique() + "@example.test"
	supA := tn.supplier(t, "Quincaillerie Ndiaye", emailed)
	supB := tn.supplier(t, "Sotrac", "")
	anon := newClient(t)

	cementLine := item("Ciment CPJ 45", "200", "sac")
	cementLine["material_id"] = cement
	created := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{supA},
		cementLine, map[string]any{"kind": "service", "label": "Location bétonnière", "quantity": "3", "unit": "jour"}))
	id := created.str("id")
	if created.str("status") != "draft" || created.str("reference") != "DP-0001" || created.str("project_id") != project {
		t.Fatalf("brouillon attendu, DP-0001 : %s", created.body)
	}

	t.Run("le brouillon se modifie en entier", func(t *testing.T) {
		updated := tn.manager.must(200, "PUT", "/api/v1/rfqs/"+id, rfqBody([]string{supA, supB},
			cementLine, item("Fer HA 10", "120", "barre"), map[string]any{"kind": "service", "label": "Location bétonnière", "quantity": "2.5", "unit": "jour", "needed_by": dayOffset(15)}))
		items := listOf(updated.json()["items"])
		if len(items) != 3 || items[1]["label"] != "Fer HA 10" || items[1]["quantity"] != "120" || items[2]["unit"] != "jour" ||
			items[2]["needed_by"] != dayOffset(15) || items[0]["specifications"] != "Norme NF" || items[0]["material_id"] != cement {
			t.Fatalf("lignes inattendues : %v", items)
		}
		if len(listOf(updated.json()["suppliers"])) != 2 || updated.str("response_deadline") != dayOffset(7) || updated.str("delivery_date") != dayOffset(20) {
			t.Fatalf("fournisseurs ou dates inattendus : %s", updated.body)
		}
	})

	t.Run("lignes invalides refusées", func(t *testing.T) {
		bad := rfqBody(nil, item("X", "0", "palette"))
		r := tn.manager.do("POST", "/api/v1/projects/"+project+"/rfqs", bad)
		expectCode(t, r, 422, "validation.failed")
		fields, _ := r.json()["fields"].(map[string]any)
		for _, f := range []string{"items.0.label", "items.0.quantity", "items.0.unit"} {
			if fields[f] == nil {
				t.Errorf("erreur attendue sur %s : %s", f, r.body)
			}
		}
		service := map[string]any{"kind": "service", "material_id": cement, "label": "Pose", "quantity": "1", "unit": "forfait"}
		expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/rfqs", rfqBody(nil, service)), 422, "validation.failed")
		expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{supA, supA})), 422, "validation.failed")
	})

	var tokenA, tokenB string
	t.Run("publication : un lien par fournisseur, par email quand c'est possible", func(t *testing.T) {
		before := len(env.mail.to(emailed))
		r := tn.manager.must(200, "POST", "/api/v1/rfqs/"+id+"/publish", nil)
		rfq, _ := r.json()["rfq"].(map[string]any)
		if rfq["status"] != "published" || rfq["open"] != true {
			t.Fatalf("demande publiée et ouverte attendue : %s", r.body)
		}
		for _, l := range links(r) {
			switch l["supplier_id"] {
			case supA:
				tokenA = linkToken(t, l)
				if l["emailed"] != true {
					t.Error("le fournisseur avec email doit recevoir le lien")
				}
			case supB:
				tokenB = linkToken(t, l)
				if l["emailed"] != false || l["phone"] != "+221771234567" {
					t.Errorf("sans email : lien à transmettre, téléphone fourni : %v", l)
				}
			}
		}
		if tokenA == "" || tokenB == "" {
			t.Fatalf("deux liens attendus : %s", r.body)
		}
		mails := env.mail.to(emailed)
		if len(mails) != before+1 || !strings.Contains(mails[len(mails)-1].body, "http://app.test/offre#"+tokenA) ||
			!strings.Contains(mails[len(mails)-1].subject, "DP-0001") {
			t.Fatalf("email de consultation attendu avec le lien : %v", mails)
		}
		var raw, hashed int
		hash := sha256.Sum256([]byte(tokenA))
		if err := env.ownerDB.QueryRow(context.Background(),
			"SELECT count(*) FILTER (WHERE token_hash = convert_to($1, 'UTF8')), count(*) FILTER (WHERE token_hash = $2) FROM rfq_suppliers",
			tokenA, hash[:]).Scan(&raw, &hashed); err != nil {
			t.Fatal(err)
		}
		if raw != 0 || hashed != 1 {
			t.Fatalf("jeton attendu haché en base (brut %d, haché %d)", raw, hashed)
		}

		// Contenu figé, publication unique.
		expectCode(t, tn.manager.do("PUT", "/api/v1/rfqs/"+id, rfqBody([]string{supA}, cementLine)), 409, "rfq.not_draft")
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+id+"/publish", nil), 409, "rfq.already_published")
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE rfq_items SET quantity = 1 WHERE rfq_id = $1", id); err == nil {
			t.Fatal("les lignes d'une demande publiée doivent être figées, même en SQL direct")
		}
	})

	var items []map[string]any
	t.Run("le fournisseur voit la demande, sans rien d'interne", func(t *testing.T) {
		r := anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": tokenA})
		v := r.json()
		items = listOf(v["items"])
		if v["reference"] != "DP-0001" || v["organization_name"] != tn.name || v["open"] != true || len(items) != 3 || items[0]["quantity"] != "200" {
			t.Fatalf("vue fournisseur inattendue : %s", r.body)
		}
		if !strings.HasPrefix(fmt.Sprint(v["supplier_name"]), "Quincaillerie Ndiaye") || v["delivery_place"] == "" {
			t.Fatalf("fournisseur et lieu de livraison attendus : %s", r.body)
		}
		for _, leak := range []string{"material_id", "client_name", "created_by", "ip", "supplier_email"} {
			if strings.Contains(string(r.body), leak) {
				t.Errorf("la vue fournisseur expose %q : %s", leak, r.body)
			}
		}
		expectCode(t, anon.do("POST", "/api/v1/supplier/rfq", map[string]any{"token": "inconnu"}), 410, "rfq_link.invalid")
	})

	offerID := uuid.NewString()
	t.Run("offres historisées : une version par envoi, renvoi sans doublon", func(t *testing.T) {
		offer := map[string]any{
			"token": tokenA, "id": offerID, "valid_until": dayOffset(30), "delivery_days": 3,
			"payment_terms": "50 % à la commande", "contact_name": "Fatou Ndiaye",
			"lines": []map[string]any{
				{"rfq_item_id": items[0]["id"], "unit_price": 4750, "comment": "Livré palettisé"},
				{"rfq_item_id": items[2]["id"], "unit_price": 25001},
			},
		}
		first := anon.must(201, "POST", "/api/v1/supplier/offers", offer)
		// 200 × 4 750 + 2,5 × 25 001 = 950 000 + 62 502,5 → arrondi au franc : 62 503.
		if first.num("version") != 1 || first.num("total_amount") != 1_012_503 || len(listOf(first.json()["lines"])) != 2 {
			t.Fatalf("offre v1 inattendue : %s", first.body)
		}
		again := anon.must(200, "POST", "/api/v1/supplier/offers", offer)
		if again.str("id") != offerID || again.num("version") != 1 {
			t.Fatalf("un renvoi doit retourner la même offre : %s", again.body)
		}
		delete(offer, "id")
		offer["lines"] = []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 4600}, {"rfq_item_id": items[1]["id"], "unit_price": 5200}}
		if v2 := anon.must(201, "POST", "/api/v1/supplier/offers", offer); v2.num("version") != 2 || v2.num("total_amount") != 200*4600+120*5200 {
			t.Fatalf("offre v2 inattendue : %s", v2.body)
		}
		anon.must(201, "POST", "/api/v1/supplier/offers", map[string]any{"token": tokenB, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 4900}}})

		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", map[string]any{"token": tokenA, "lines": []map[string]any{}}), 422, "validation.failed")
		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", map[string]any{"token": tokenA, "lines": []map[string]any{{"rfq_item_id": project, "unit_price": 1}}}), 422, "offer.unknown_item")
		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", map[string]any{"token": tokenA, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": -5}}}), 422, "validation.failed")

		detail := tn.owner.must(200, "GET", "/api/v1/rfqs/"+id, nil).json()
		offers := listOf(detail["offers"])
		if len(offers) != 3 || offers[0]["supplier_name"] == nil {
			t.Fatalf("3 versions attendues dans l'historique : %v", offers)
		}
		for _, s := range listOf(detail["suppliers"]) {
			want := map[string]float64{supA: 2, supB: 1}[s["supplier_id"].(string)]
			if s["offer_count"] != want {
				t.Errorf("offres de %v : %v, attendu %v", s["supplier_name"], s["offer_count"], want)
			}
		}
		// Un fournisseur ne voit que ses propres offres.
		mine := listOf(anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": tokenB}).json()["offers"])
		if len(mine) != 1 || mine[0]["supplier_id"] != supB {
			t.Fatalf("le fournisseur B ne doit voir que son offre : %v", mine)
		}
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE rfq_offers SET total_amount = 1 WHERE id = $1", offerID); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("une offre reçue est immuable : %v", err)
		}
	})

	t.Run("lien renouvelé, fournisseur ajouté après publication", func(t *testing.T) {
		renewed := tn.manager.must(200, "POST", "/api/v1/rfqs/"+id+"/suppliers/"+supB+"/link", nil)
		fresh := linkToken(t, links(renewed)[0])
		expectCode(t, anon.do("POST", "/api/v1/supplier/rfq", map[string]any{"token": tokenB}), 410, "rfq_link.invalid")
		anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": fresh})

		supC := tn.supplier(t, "Senbat", "")
		added := tn.manager.must(201, "POST", "/api/v1/rfqs/"+id+"/suppliers/"+supC, nil)
		if len(links(added)) != 1 || links(added)[0]["supplier_id"] != supC {
			t.Fatalf("un lien attendu pour le fournisseur ajouté : %s", added.body)
		}
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+id+"/suppliers/"+supC, nil), 409, "rfq.supplier_already_consulted")
	})

	t.Run("clôture : plus d'offre, l'historique reste", func(t *testing.T) {
		closed := tn.manager.must(200, "POST", "/api/v1/rfqs/"+id+"/close", nil)
		if closed.str("status") != "closed" || closed.json()["open"] != false || len(listOf(closed.json()["offers"])) != 3 {
			t.Fatalf("demande clôturée avec ses offres attendue : %s", closed.body)
		}
		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", map[string]any{"token": tokenA, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 1}}}), 409, "rfq.not_open")
		v := anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": tokenA}).json()
		if v["status"] != "closed" || v["open"] != false || len(listOf(v["offers"])) != 2 {
			t.Fatalf("le fournisseur voit la clôture et ses offres : %v", v)
		}
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+id+"/cancel", map[string]any{"reason": "x"}), 409, "rfq.already_ended")
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+id+"/close", nil), 409, "rfq.not_published")
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+id+"/suppliers/"+supB+"/link", nil), 409, "rfq.not_open")
	})

	t.Run("journal d'audit", func(t *testing.T) {
		for action, want := range map[string]int{
			"rfq.created": 1, "rfq.updated": 1, "rfq.published": 1, "rfq.offer_received": 3,
			"rfq.link_renewed": 1, "rfq.supplier_invited": 1, "rfq.closed": 1,
		} {
			if n, _ := rfqAudit(t, id, action); n != want {
				t.Errorf("%s : %d entrée(s), %d attendue(s)", action, n, want)
			}
		}
		if n, actorless := rfqAudit(t, id, "rfq.offer_received"); actorless != n {
			t.Error("une offre déposée par un fournisseur n'a pas d'acteur interne")
		}
	})
}

func TestE1_RFQPublicationRulesCancelAndDeadline(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	sup := tn.supplier(t, "Fournisseur", "")
	anon := newClient(t)

	empty := tn.owner.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", map[string]any{"title": "Carrelage"}).str("id")
	r := tn.owner.do("POST", "/api/v1/rfqs/"+empty+"/publish", nil)
	expectCode(t, r, 422, "validation.failed")
	fields, _ := r.json()["fields"].(map[string]any)
	if fields["items"] == nil || fields["supplier_ids"] == nil || fields["response_deadline"] == nil {
		t.Fatalf("publication sans lignes, fournisseurs ni date refusée champ par champ : %s", r.body)
	}
	body := rfqBody([]string{sup}, item("Carreaux 60x60", "80", "m2"))
	body["response_deadline"] = dayOffset(-1)
	tn.owner.must(200, "PUT", "/api/v1/rfqs/"+empty, body)
	expectCode(t, tn.owner.do("POST", "/api/v1/rfqs/"+empty+"/publish", nil), 422, "validation.failed")

	t.Run("annulation motivée, visible du fournisseur", func(t *testing.T) {
		expectCode(t, tn.owner.do("POST", "/api/v1/rfqs/"+empty+"/cancel", map[string]any{"reason": " "}), 422, "validation.failed")
		id := tn.owner.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{sup}, item("Peinture", "40", "litre"))).str("id")
		token := linkToken(t, links(tn.owner.must(200, "POST", "/api/v1/rfqs/"+id+"/publish", nil))[0])
		tn.owner.must(200, "POST", "/api/v1/rfqs/"+id+"/cancel", map[string]any{"reason": "Projet suspendu par le client"})
		v := anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": token}).json()
		if v["status"] != "cancelled" || v["cancel_reason"] != "Projet suspendu par le client" || v["open"] != false {
			t.Fatalf("annulation et motif attendus côté fournisseur : %v", v)
		}
		items := listOf(v["items"])
		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", map[string]any{"token": token, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 1}}}), 409, "rfq.not_open")
		if n, _ := rfqAudit(t, id, "rfq.cancelled"); n != 1 {
			t.Fatal("annulation absente du journal d'audit")
		}
	})

	t.Run("date limite incluse, puis plus d'offre", func(t *testing.T) {
		body := rfqBody([]string{sup}, item("Sable", "12", "m3"))
		body["response_deadline"] = dayOffset(0)
		id := tn.owner.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", body).str("id")
		token := linkToken(t, links(tn.owner.must(200, "POST", "/api/v1/rfqs/"+id+"/publish", nil))[0])
		items := listOf(anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": token}).json()["items"])
		offer := map[string]any{"token": token, "lines": []map[string]any{{"rfq_item_id": items[0]["id"], "unit_price": 15000}}}
		anon.must(201, "POST", "/api/v1/supplier/offers", offer)

		// Le contenu publié est figé par trigger : on le contourne ici pour simuler le lendemain.
		if _, err := env.ownerDB.Exec(context.Background(), fmt.Sprintf(
			"BEGIN; SET LOCAL session_replication_role = replica; UPDATE rfqs SET response_deadline = response_deadline - 1 WHERE id = '%s'; COMMIT;", id)); err != nil {
			t.Fatal(err)
		}
		expectCode(t, anon.do("POST", "/api/v1/supplier/offers", offer), 409, "rfq.not_open")
		if v := anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": token}).json(); v["open"] != false || v["status"] != "published" {
			t.Fatalf("échéance passée : demande publiée mais fermée aux offres : %v", v)
		}
		tn.owner.must(200, "POST", "/api/v1/rfqs/"+id+"/close", nil)
	})
}

func TestE1_RFQAccess(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	sup := tn.supplier(t, "Fournisseur", "")
	id := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{sup}, item("Gravier", "10", "m3"))).str("id")

	t.Run("rôles", func(t *testing.T) {
		expectCode(t, tn.worker.do("GET", "/api/v1/rfqs", nil), 403, "auth.forbidden")
		expectCode(t, tn.worker.do("GET", "/api/v1/rfqs/"+id, nil), 403, "auth.forbidden")
		for _, c := range []*client{tn.worker, tn.viewer, tn.accountant} {
			expectCode(t, c.do("POST", "/api/v1/projects/"+project+"/rfqs", rfqBody(nil)), 403, "auth.forbidden")
			expectCode(t, c.do("POST", "/api/v1/rfqs/"+id+"/publish", nil), 403, "auth.forbidden")
		}
		for _, c := range []*client{tn.viewer, tn.accountant, tn.owner} {
			list := c.must(200, "GET", "/api/v1/rfqs?project_id="+project, nil).items()
			if len(list) != 1 || list[0]["id"] != id || list[0]["item_count"] != float64(1) || list[0]["supplier_count"] != float64(1) {
				t.Fatalf("liste attendue avec la demande et ses compteurs : %v", list)
			}
			c.must(200, "GET", "/api/v1/rfqs/"+id, nil)
		}
	})

	t.Run("un conducteur ne gère que ses chantiers", func(t *testing.T) {
		other := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
			"reference": "CH-" + unique(), "name": "Immeuble " + unique(), "client_name": "SCI Baobab", "address": "Plateau, Dakar",
			"start_date": "2026-01-15", "status": "in_progress", "manager_id": tn.ownerID,
		}).str("id")
		expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+other+"/rfqs", rfqBody(nil)), 403, "auth.forbidden")
		ownerRFQ := tn.owner.must(201, "POST", "/api/v1/projects/"+other+"/rfqs", rfqBody(nil)).str("id")
		expectCode(t, tn.manager.do("GET", "/api/v1/rfqs/"+ownerRFQ, nil), 403, "auth.forbidden")
		for _, row := range tn.manager.must(200, "GET", "/api/v1/rfqs", nil).items() {
			if row["id"] == ownerRFQ {
				t.Fatal("la liste du conducteur ne montre pas les chantiers des autres")
			}
		}
	})

	t.Run("isolation entre entreprises", func(t *testing.T) {
		other := newTenant(t)
		expectCode(t, other.owner.do("GET", "/api/v1/rfqs/"+id, nil), 404, "")
		expectCode(t, other.owner.do("PUT", "/api/v1/rfqs/"+id, rfqBody(nil)), 404, "")
		expectCode(t, other.owner.do("POST", "/api/v1/rfqs/"+id+"/publish", nil), 404, "")
		expectCode(t, other.owner.do("POST", "/api/v1/projects/"+project+"/rfqs", rfqBody(nil)), 404, "project.not_found")
		if len(other.owner.must(200, "GET", "/api/v1/rfqs", nil).items()) != 0 {
			t.Fatal("une autre entreprise ne voit aucune demande")
		}
		otherSupplier := other.supplier(t, "Étranger", "")
		otherProject := other.newProject(t)
		expectCode(t, other.owner.do("POST", "/api/v1/projects/"+otherProject+"/rfqs", rfqBody([]string{sup})), 422, "supplier.not_found")
		expectCode(t, tn.owner.do("POST", "/api/v1/rfqs/"+id+"/suppliers/"+otherSupplier, nil), 422, "supplier.not_found")
	})
}

// La numérotation DP-0001, DP-0002... reste sans trou ni doublon sous créations concurrentes.
func TestE1_RFQNumberingIsSequential(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	var refs []string
	ok, statuses := concurrently(8, func(int) response {
		r := tn.owner.do("POST", "/api/v1/projects/"+project+"/rfqs", map[string]any{"title": "Consultation"})
		if r.status == 201 {
			counterMu.Lock()
			refs = append(refs, r.str("reference"))
			counterMu.Unlock()
		}
		return r
	})
	if ok != 8 {
		t.Fatalf("8 créations attendues : %v", statuses)
	}
	sort.Strings(refs)
	for i, ref := range refs {
		if want := fmt.Sprintf("DP-%04d", i+1); ref != want {
			t.Fatalf("numéros attendus DP-0001 à DP-0008 : %v", refs)
		}
	}
}
