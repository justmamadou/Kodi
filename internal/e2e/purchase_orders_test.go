package e2e

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// E3 : comparaison sur critères, sélection, bons de commande, réceptions.

type awarded struct {
	rfq, offerA, offerB, supA, supB, emailA string
	cement, fer, transport                  string // lignes de la demande
}

// consultation crée une demande à trois lignes (ciment du catalogue, fer en saisie libre, transport),
// consulte deux fournisseurs, reçoit leurs offres et clôt la consultation.
func consultation(t *testing.T, tn *tenant, c *client, project, cementMaterial string) awarded {
	t.Helper()
	var a awarded
	a.emailA = "devis-" + unique() + "@example.test"
	a.supA = tn.supplier(t, "Quincaillerie Ndiaye", a.emailA)
	a.supB = tn.supplier(t, "Senbat", "")
	cement := item("Ciment CPJ 45", "100", "sac")
	cement["material_id"] = cementMaterial
	a.rfq = c.must(201, "POST", "/api/v1/projects/"+project+"/rfqs", rfqBody([]string{a.supA, a.supB},
		cement, item("Fer HA 10", "50", "barre"), map[string]any{"kind": "service", "label": "Transport", "quantity": "1", "unit": "forfait"})).str("id")
	tokens := map[string]string{}
	for _, l := range links(c.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/publish", nil)) {
		tokens[l["supplier_id"].(string)] = linkToken(t, l)
	}
	anon := newClient(t)
	items := listOf(anon.must(200, "POST", "/api/v1/supplier/rfq", map[string]any{"token": tokens[a.supA]}).json()["items"])
	a.cement, a.fer, a.transport = items[0]["id"].(string), items[1]["id"].(string), items[2]["id"].(string)
	price := func(item string, p int) map[string]any { return map[string]any{"rfq_item_id": item, "unit_price": p} }
	anon.must(201, "POST", "/api/v1/supplier/offers", map[string]any{"token": tokens[a.supA], "delivery_days": 5,
		"lines": []map[string]any{price(a.cement, 4800), price(a.fer, 5000), price(a.transport, 30000)}})
	a.offerA = anon.must(201, "POST", "/api/v1/supplier/offers", map[string]any{"token": tokens[a.supA], "delivery_days": 5, "payment_terms": "30 jours fin de mois",
		"lines": []map[string]any{price(a.cement, 4700), price(a.fer, 5000), price(a.transport, 30000)}}).str("id")
	a.offerB = anon.must(201, "POST", "/api/v1/supplier/offers", map[string]any{"token": tokens[a.supB], "delivery_days": 2,
		"lines": []map[string]any{price(a.cement, 4500), price(a.fer, 5200)}}).str("id")
	return a
}

func material(t *testing.T, tn *tenant, name, unit string) string {
	t.Helper()
	return tn.owner.must(201, "POST", "/api/v1/materials", map[string]any{"name": name + " " + unique(), "unit": unit}).str("id")
}

func onHand(t *testing.T, tn *tenant, project, materialID string) float64 {
	t.Helper()
	for _, row := range listOf(tn.owner.must(200, "GET", "/api/v1/projects/"+project+"/stock", nil).json()["items"]) {
		if row["material_id"] == materialID {
			v, _ := strconv.ParseFloat(row["on_hand"].(string), 64)
			return v
		}
	}
	return 0
}

func TestE3_ComparisonCriteriaAndAward(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	a := consultation(t, tn, tn.manager, project, material(t, tn, "Ciment CPJ 45", "sac"))

	ev, _ := tn.accountant.must(200, "GET", "/api/v1/rfqs/"+a.rfq, nil).json()["evaluation"].(map[string]any)
	offers := listOf(ev["offers"])
	if ev["weight_price"] != float64(60) || len(offers) != 2 {
		t.Fatalf("évaluation des deux dernières offres attendue : %v", ev)
	}
	// B : prix 98,6 (700 000 / 710 000), délai 100 (le plus court), couverture 66,7 → 94,2.
	// A : prix 97,3 (730 000 / 750 000), délai 50 ((2+1)/(5+1)), couverture 100 → 85,9.
	if offers[0]["offer_id"] != a.offerB || offers[0]["price_score"] != 98.6 || offers[0]["coverage_score"] != 66.7 || offers[0]["score"] != 94.2 ||
		offers[1]["offer_id"] != a.offerA || offers[1]["price_score"] != 97.3 || offers[1]["delivery_score"] != float64(50) || offers[1]["score"] != 85.9 {
		t.Fatalf("scores attendus B 94,2 puis A 85,9 : %v", offers)
	}

	t.Run("pondération modifiable, totalisant 100", func(t *testing.T) {
		expectCode(t, tn.manager.do("PUT", "/api/v1/rfqs/"+a.rfq+"/criteria", map[string]any{"weight_price": 50, "weight_delivery": 30, "weight_coverage": 30}), 422, "validation.failed")
		expectCode(t, tn.accountant.do("PUT", "/api/v1/rfqs/"+a.rfq+"/criteria", map[string]any{"weight_price": 0, "weight_delivery": 0, "weight_coverage": 100}), 403, "auth.forbidden")
		r := tn.manager.must(200, "PUT", "/api/v1/rfqs/"+a.rfq+"/criteria", map[string]any{"weight_price": 0, "weight_delivery": 0, "weight_coverage": 100})
		ranked := listOf(r.json()["evaluation"].(map[string]any)["offers"])
		if ranked[0]["offer_id"] != a.offerA || ranked[0]["score"] != float64(100) {
			t.Fatalf("sur la seule couverture, A passe devant : %v", ranked)
		}
	})

	t.Run("sélection : consultation close, dernière version, utilisateurs autorisés", func(t *testing.T) {
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerA}), 409, "rfq.not_closed")
		tn.manager.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/close", nil)
		var v1 string
		for _, o := range listOf(tn.owner.must(200, "GET", "/api/v1/rfqs/"+a.rfq, nil).json()["offers"]) {
			if o["supplier_id"] == a.supA && o["version"] == float64(1) {
				v1 = o["id"].(string)
			}
		}
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": v1}), 409, "offer.not_latest")
		expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": uuid.NewString()}), 422, "offer.not_found")
		for _, c := range []*client{tn.accountant, tn.worker, tn.viewer} {
			expectCode(t, c.do("POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerA}), 403, "auth.forbidden")
		}
		tn.manager.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerB})
		tn.manager.must(200, "DELETE", "/api/v1/rfqs/"+a.rfq+"/award", nil)
		r := tn.manager.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerA, "reason": "Seul à chiffrer le transport"}).json()
		if r["awarded_offer_id"] != a.offerA || r["award_reason"] != "Seul à chiffrer le transport" {
			t.Fatalf("offre A retenue attendue : %v", r)
		}
		for _, o := range listOf(r["evaluation"].(map[string]any)["offers"]) {
			if (o["offer_id"] == a.offerA) != (o["selected"] == true) {
				t.Fatalf("seule l'offre retenue est marquée : %v", o)
			}
		}
		expectCode(t, tn.manager.do("PUT", "/api/v1/rfqs/"+a.rfq+"/criteria", map[string]any{"weight_price": 100, "weight_delivery": 0, "weight_coverage": 0}), 409, "rfq.already_awarded")
		if audits(t, a.rfq, "rfq.awarded") != 2 || audits(t, a.rfq, "rfq.award_withdrawn") != 1 || audits(t, a.rfq, "rfq.criteria_changed") != 1 {
			t.Fatal("sélections, retrait et critères au journal d'audit")
		}
	})
}

func TestE3_PurchaseOrderWorkflow(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	cementMat := material(t, tn, "Ciment CPJ 45", "sac")
	a := consultation(t, tn, tn.manager, project, cementMat)
	tn.manager.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/close", nil)
	expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil), 409, "rfq.not_awarded")
	tn.manager.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerA})

	expectCode(t, tn.worker.do("POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil), 403, "auth.forbidden")
	po := tn.manager.must(201, "POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil).json()
	id := po["id"].(string)
	lines := listOf(po["lines"])
	if po["reference"] != "BC-0001" || po["status"] != "draft" || po["supplier_id"] != a.supA || po["rfq_reference"] != "DP-0001" ||
		po["total_amount"] != float64(750000) || po["payment_terms"] != "30 jours fin de mois" || po["expected_delivery_date"] != dayOffset(20) ||
		len(lines) != 3 || lines[0]["material_id"] != cementMat || lines[0]["quantity_ordered"] != "100" || lines[0]["unit_price"] != float64(4700) {
		t.Fatalf("commande générée depuis l'offre retenue attendue : %v", po)
	}
	expectCode(t, tn.manager.do("POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil), 409, "rfq.order_exists")
	expectCode(t, tn.manager.do("DELETE", "/api/v1/rfqs/"+a.rfq+"/award", nil), 409, "rfq.order_exists")

	ferMat := material(t, tn, "Fer HA 10", "barre")
	line := func(l map[string]any, tax string, mat any, unit string) map[string]any {
		return map[string]any{"rfq_item_id": l["rfq_item_id"], "material_id": mat, "label": l["label"], "unit": unit,
			"quantity": l["quantity_ordered"], "unit_price": l["unit_price"], "tax_rate": tax}
	}
	t.Run("brouillon : taxes, rattachement au catalogue, totaux recalculés", func(t *testing.T) {
		bad := map[string]any{"lines": []map[string]any{line(lines[0], "18", cementMat, "sac"), line(lines[1], "18", ferMat, "kg"), line(lines[2], "0", nil, "forfait")}}
		expectCode(t, tn.manager.do("PUT", "/api/v1/purchase-orders/"+id, bad), 422, "material.unit_mismatch")
		updated := tn.manager.must(200, "PUT", "/api/v1/purchase-orders/"+id, map[string]any{
			"expected_delivery_date": dayOffset(10), "payment_terms": "30 jours fin de mois", "notes": "Livrer avant 10 h",
			"lines": []map[string]any{line(lines[0], "18", cementMat, "sac"), line(lines[1], "18", ferMat, "barre"), line(lines[2], "0", nil, "forfait")},
		}).json()
		// HT 470 000 + 250 000 + 30 000 ; TVA 18 % sur les deux premières : 84 600 + 45 000.
		if updated["subtotal_amount"] != float64(750000) || updated["tax_amount"] != float64(129600) || updated["total_amount"] != float64(879600) {
			t.Fatalf("totaux recalculés attendus : %v", updated)
		}
		lines = listOf(updated["lines"])
	})

	t.Run("workflow : soumission, renvoi motivé, approbation par un autre, envoi", func(t *testing.T) {
		tn.manager.must(200, "POST", "/api/v1/purchase-orders/"+id+"/submit", nil)
		expectCode(t, tn.manager.do("PUT", "/api/v1/purchase-orders/"+id, map[string]any{"lines": []map[string]any{line(lines[0], "0", nil, "sac")}}), 409, "purchase_order.not_draft")
		expectCode(t, tn.manager.do("POST", "/api/v1/purchase-orders/"+id+"/approve", nil), 403, "auth.forbidden")
		expectCode(t, tn.accountant.do("POST", "/api/v1/purchase-orders/"+id+"/return", nil), 422, "validation.failed")
		if r := tn.accountant.must(200, "POST", "/api/v1/purchase-orders/"+id+"/return", map[string]any{"comment": "Vérifier le prix du fer"}); r.str("status") != "draft" {
			t.Fatalf("renvoi en brouillon attendu : %s", r.body)
		}
		tn.manager.must(200, "POST", "/api/v1/purchase-orders/"+id+"/submit", nil)
		expectCode(t, tn.manager.do("POST", "/api/v1/purchase-orders/"+id+"/send", nil), 409, "purchase_order.invalid_status")
		approved := tn.accountant.must(200, "POST", "/api/v1/purchase-orders/"+id+"/approve", map[string]any{"comment": "OK budget"})
		if approved.str("status") != "approved" || approved.str("approved_by") != tn.accountID {
			t.Fatalf("approbation attendue : %s", approved.body)
		}
		before := len(env.mail.to(a.emailA))
		sent := tn.manager.must(200, "POST", "/api/v1/purchase-orders/"+id+"/send", nil).json()
		mails := env.mail.to(a.emailA)
		if sent["emailed"] != true || len(mails) != before+1 || !strings.Contains(mails[len(mails)-1].subject, "BC-0001") ||
			!strings.Contains(mails[len(mails)-1].body, "Total TTC : 879600 FCFA") {
			t.Fatalf("bon de commande envoyé par email attendu : %v", mails)
		}
		events := listOf(tn.owner.must(200, "GET", "/api/v1/purchase-orders/"+id, nil).json()["events"])
		var names []string
		for _, e := range events {
			names = append(names, e["event"].(string))
		}
		if strings.Join(names, ",") != "created,updated,submitted,returned,submitted,approved,sent" || events[3]["comment"] != "Vérifier le prix du fer" {
			t.Fatalf("historique complet attendu : %v", names)
		}
		if events[1]["changes"] == nil || audits(t, id, "purchase_order.approved") != 1 {
			t.Fatal("modification historisée avec son contenu, approbation au journal d'audit")
		}
	})

	t.Run("le chef de chantier voit la commande, sans les prix", func(t *testing.T) {
		seen := tn.worker.must(200, "GET", "/api/v1/purchase-orders/"+id, nil).json()
		l := listOf(seen["lines"])
		if seen["prices_visible"] != false || seen["total_amount"] != nil || l[0]["unit_price"] != nil || l[0]["quantity_ordered"] != "100" {
			t.Fatalf("quantités sans prix attendues : %v", seen)
		}
		for _, e := range listOf(seen["events"]) {
			if e["changes"] != nil {
				t.Fatal("l'historique ne révèle pas les montants au chef de chantier")
			}
		}
		if row := tn.worker.must(200, "GET", "/api/v1/purchase-orders?project_id="+project, nil).items()[0]; row["total_amount"] != nil {
			t.Fatalf("liste sans montant attendue : %v", row)
		}
		orders := listOf(tn.owner.must(200, "GET", "/api/v1/suppliers/"+a.supA+"/history", nil).json()["orders"])
		if len(orders) != 1 || orders[0]["id"] != id {
			t.Fatalf("la commande figure dans l'historique du fournisseur : %v", orders)
		}
	})

	t.Run("réceptions partielles puis complète, entrée en stock, sans double comptage", func(t *testing.T) {
		receipt := uuid.NewString()
		first := map[string]any{"id": receipt, "received_on": today(), "delivery_note_ref": "BL-001", "to_stock": true,
			"lines": []map[string]any{{"line_id": lines[0]["id"], "quantity": "40"}, {"line_id": lines[1]["id"], "quantity": "50"}}}
		expectCode(t, tn.accountant.do("POST", "/api/v1/purchase-orders/"+id+"/receipts", first), 403, "auth.forbidden")
		r := tn.worker.must(201, "POST", "/api/v1/purchase-orders/"+id+"/receipts", first).json()
		if r["status"] != "partially_received" || listOf(r["lines"])[0]["quantity_remaining"] != "60" || listOf(r["lines"])[1]["quantity_remaining"] != "0" {
			t.Fatalf("réception partielle attendue : %v", r)
		}
		if onHand(t, tn, project, cementMat) != 40 || onHand(t, tn, project, ferMat) != 50 {
			t.Fatal("la réception alimente le stock du chantier")
		}
		// Le même envoi (réseau instable, double clic) ne compte qu'une fois.
		tn.worker.must(200, "POST", "/api/v1/purchase-orders/"+id+"/receipts", first)
		dup := first
		dup["id"] = uuid.NewString()
		dup["delivery_note_ref"] = " bl-001 "
		expectCode(t, tn.worker.do("POST", "/api/v1/purchase-orders/"+id+"/receipts", dup), 409, "receipt.duplicate")
		over := map[string]any{"received_on": today(), "lines": []map[string]any{{"line_id": lines[0]["id"], "quantity": "61"}}}
		expectCode(t, tn.worker.do("POST", "/api/v1/purchase-orders/"+id+"/receipts", over), 422, "receipt.over_quantity")
		if onHand(t, tn, project, cementMat) != 40 {
			t.Fatal("ni le renvoi ni les refus ne touchent au stock")
		}
		expectCode(t, tn.manager.do("POST", "/api/v1/purchase-orders/"+id+"/cancel", map[string]any{"comment": "Trop tard"}), 409, "purchase_order.invalid_status")

		last := tn.worker.must(201, "POST", "/api/v1/purchase-orders/"+id+"/receipts", map[string]any{
			"received_on": today(), "delivery_note_ref": "BL-002", "to_stock": false,
			"lines": []map[string]any{{"line_id": lines[0]["id"], "quantity": "60"}, {"line_id": lines[2]["id"], "quantity": "1"}},
		}).json()
		if last["status"] != "received" || len(listOf(last["receipts"])) != 2 {
			t.Fatalf("commande entièrement reçue attendue : %v", last)
		}
		if onHand(t, tn, project, cementMat) != 40 {
			t.Fatal("une réception hors stock ne change pas le stock")
		}
		expectCode(t, tn.worker.do("POST", "/api/v1/purchase-orders/"+id+"/receipts", over), 409, "purchase_order.invalid_status")

		// Cohérence : reçu = somme des réceptions, restant = commandé - reçu, partout.
		var incoherent int
		if err := env.ownerDB.QueryRow(context.Background(), `
			SELECT count(*) FROM purchase_order_lines l
			 WHERE l.purchase_order_id = $1
			   AND (l.quantity_received <> COALESCE((SELECT sum(quantity) FROM purchase_order_receipt_lines rl WHERE rl.line_id = l.id), 0)
			        OR l.quantity_remaining <> l.quantity_ordered - l.quantity_received OR l.quantity_remaining <> 0)`, id).Scan(&incoherent); err != nil {
			t.Fatal(err)
		}
		if incoherent != 0 {
			t.Fatalf("%d ligne(s) incohérente(s)", incoherent)
		}
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE purchase_order_lines SET quantity_received = 200 WHERE purchase_order_id = $1", id); err == nil {
			t.Fatal("la quantité reçue ne peut ni dépasser la commande ni changer hors réception")
		}
	})
}

// Approbation par un autre que l'auteur, et réceptions concurrentes : une seule passe.
func TestE3_SeparationOfDutiesAndConcurrentReceipts(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	a := consultation(t, tn, tn.owner, project, material(t, tn, "Ciment", "sac"))
	tn.owner.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/close", nil)
	tn.owner.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerB})
	id := tn.owner.must(201, "POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil).str("id")
	tn.owner.must(200, "POST", "/api/v1/purchase-orders/"+id+"/submit", nil)
	expectCode(t, tn.owner.do("POST", "/api/v1/purchase-orders/"+id+"/approve", nil), 403, "purchase_order.self_approval")
	tn.accountant.must(200, "POST", "/api/v1/purchase-orders/"+id+"/approve", nil)
	if _, err := env.ownerDB.Exec(context.Background(), "UPDATE purchase_orders SET approved_by = created_by WHERE id = $1", id); err == nil {
		t.Fatal("la base refuse aussi l'auto-approbation")
	}
	sent := tn.owner.must(200, "POST", "/api/v1/purchase-orders/"+id+"/send", nil).json()
	if sent["emailed"] != false {
		t.Fatal("sans email, la commande est à transmettre autrement")
	}
	line := listOf(sent["order"].(map[string]any)["lines"])[0]
	ok, statuses := concurrently(5, func(int) response {
		return tn.worker.do("POST", "/api/v1/purchase-orders/"+id+"/receipts", map[string]any{
			"received_on": today(), "to_stock": true, "lines": []map[string]any{{"line_id": line["id"], "quantity": line["quantity_ordered"]}},
		})
	})
	if ok != 1 {
		t.Fatalf("une seule réception de la quantité totale attendue : %v", statuses)
	}

	t.Run("annulation motivée, offre de nouveau disponible", func(t *testing.T) {
		b := consultation(t, tn, tn.owner, project, material(t, tn, "Sable", "m3"))
		tn.owner.must(200, "POST", "/api/v1/rfqs/"+b.rfq+"/close", nil)
		tn.owner.must(200, "POST", "/api/v1/rfqs/"+b.rfq+"/award", map[string]any{"offer_id": b.offerA})
		po := tn.owner.must(201, "POST", "/api/v1/rfqs/"+b.rfq+"/purchase-order", nil).json()
		if po["reference"] != "BC-0002" {
			t.Fatalf("numérotation BC-0002 attendue : %v", po["reference"])
		}
		expectCode(t, tn.owner.do("POST", "/api/v1/purchase-orders/"+po["id"].(string)+"/cancel", map[string]any{}), 422, "validation.failed")
		tn.owner.must(200, "POST", "/api/v1/purchase-orders/"+po["id"].(string)+"/cancel", map[string]any{"comment": "Le client reporte"})
		tn.owner.must(200, "DELETE", "/api/v1/rfqs/"+b.rfq+"/award", nil)
		tn.owner.must(200, "POST", "/api/v1/rfqs/"+b.rfq+"/award", map[string]any{"offer_id": b.offerB})
		if tn.owner.must(201, "POST", "/api/v1/rfqs/"+b.rfq+"/purchase-order", nil).str("reference") != "BC-0003" {
			t.Fatal("une nouvelle commande prend le numéro suivant")
		}
	})
}

func TestE3_PurchaseOrderIsolation(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	a := consultation(t, tn, tn.owner, project, material(t, tn, "Ciment", "sac"))
	tn.owner.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/close", nil)
	tn.owner.must(200, "POST", "/api/v1/rfqs/"+a.rfq+"/award", map[string]any{"offer_id": a.offerA})
	id := tn.owner.must(201, "POST", "/api/v1/rfqs/"+a.rfq+"/purchase-order", nil).str("id")

	other := newTenant(t)
	for _, call := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/purchase-orders/" + id, nil}, {"POST", "/purchase-orders/" + id + "/submit", nil},
		{"POST", "/purchase-orders/" + id + "/cancel", map[string]any{"comment": "x"}},
		{"POST", "/purchase-orders/" + id + "/receipts", map[string]any{"received_on": today(), "lines": []map[string]any{{"line_id": id, "quantity": "1"}}}},
		{"POST", "/rfqs/" + a.rfq + "/award", map[string]any{"offer_id": a.offerB}}, {"POST", "/rfqs/" + a.rfq + "/purchase-order", nil},
	} {
		if r := other.owner.do(call.method, "/api/v1"+call.path, call.body); r.status != 404 {
			t.Errorf("%s %s depuis une autre entreprise : %d, 404 attendu\n%s", call.method, call.path, r.status, r.body)
		}
	}
	if len(other.owner.must(200, "GET", "/api/v1/purchase-orders", nil).items()) != 0 {
		t.Fatal("une autre entreprise ne voit aucune commande")
	}
}
