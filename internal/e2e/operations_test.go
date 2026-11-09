package e2e

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

// CF-019
func TestCF019_GlobalDashboard(t *testing.T) {
	tn := newTenant(t)
	p1 := tn.newProject(t)
	p2 := tn.newProject(t)
	tn.publishBudget(t, p1, "100", 10000) // 1 000 000
	tn.publishBudget(t, p2, "10", 50000)  //   500 000

	a := tn.approvedExpense(t, p1, 300000)
	tn.accountant.must(201, "POST", "/api/v1/expenses/"+a+"/payments", map[string]any{"amount": 100000, "paid_on": "2026-11-05", "method": "cash"})
	tn.approvedExpense(t, p2, 50000)
	pending := tn.newExpense(t, tn.worker, p1, 70000)
	tn.worker.must(200, "POST", "/api/v1/expenses/"+pending+"/submit", nil)
	tn.newExpense(t, tn.worker, p1, 999)           // brouillon : ne compte nulle part
	rejected := tn.newExpense(t, tn.worker, p2, 5) // rejetée : ne compte nulle part
	tn.worker.must(200, "POST", "/api/v1/expenses/"+rejected+"/submit", nil)
	tn.accountant.must(200, "POST", "/api/v1/expenses/"+rejected+"/reject", map[string]any{"comment": "Non"})

	d := tn.owner.must(200, "GET", "/api/v1/dashboard", nil).json()
	totals, _ := d["totals"].(map[string]any)
	want := map[string]float64{"budget": 1500000, "approved": 350000, "pending": 70000, "paid": 100000, "unpaid": 250000, "remaining": 1150000}
	for k, v := range want {
		if totals[k] != v {
			t.Errorf("%s = %v, attendu %v", k, totals[k], v)
		}
	}

	t.Run("totaux cohérents avec le détail par chantier", func(t *testing.T) {
		projects, _ := d["projects"].([]any)
		var budget, approved, pending, paid float64
		for _, raw := range projects {
			p, _ := raw.(map[string]any)
			budget += p["budget"].(float64)
			approved += p["approved"].(float64)
			pending += p["pending"].(float64)
			paid += p["paid"].(float64)
		}
		if budget != totals["budget"] || approved != totals["approved"] || pending != totals["pending"] || paid != totals["paid"] {
			t.Fatalf("somme des chantiers ≠ totaux : %v", d)
		}
	})

	t.Run("données limitées à l'organisation", func(t *testing.T) {
		other := newTenant(t)
		o := other.owner.must(200, "GET", "/api/v1/dashboard", nil).json()
		otherTotals, _ := o["totals"].(map[string]any)
		if otherTotals["budget"] != float64(0) || o["project_count"] != float64(0) {
			t.Fatalf("une organisation vide voit des données : %v", o)
		}
	})

	t.Run("un manager ne voit que ses chantiers", func(t *testing.T) {
		tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
			"reference": "H-" + unique(), "name": "Hors périmètre", "client_name": "Client", "address": "Dakar", "start_date": "2026-01-01", "status": "in_progress",
		})
		owner := tn.owner.must(200, "GET", "/api/v1/dashboard", nil).json()
		manager := tn.manager.must(200, "GET", "/api/v1/dashboard", nil).json()
		if owner["project_count"] != float64(3) || manager["project_count"] != float64(2) {
			t.Fatalf("owner=%v manager=%v", owner["project_count"], manager["project_count"])
		}
	})
}

// CF-020
func TestCF020_ProjectHealth(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)

	noBudget := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if noBudget.str("health") != "no_budget" {
		t.Fatalf("no_budget attendu : %s", noBudget.body)
	}

	v1 := tn.publishBudget(t, project, "100", 1000) // 100 000
	tn.approvedExpense(t, project, 79000)
	ok := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if ok.str("health") != "ok" || ok.num("remaining") != 21000 {
		t.Fatalf("sous les seuils : %s", ok.body)
	}

	// Une dépense en attente ne consomme pas le reste, mais reste visible à part.
	pending := tn.newExpense(t, tn.worker, project, 50000)
	tn.worker.must(200, "POST", "/api/v1/expenses/"+pending+"/submit", nil)
	still := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if still.num("remaining") != 21000 || still.num("pending") != 50000 || still.str("health") != "ok" {
		t.Fatalf("Pending séparées : %s", still.body)
	}

	tn.approvedExpense(t, project, 1000)
	warn := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if warn.str("health") != "warning" || warn.num("threshold_reached") != 80 {
		t.Fatalf("seuil de 80 %% : %s", warn.body)
	}

	tn.accountant.must(200, "POST", "/api/v1/expenses/"+pending+"/approve", nil)
	over := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if over.str("health") != "over" || over.num("overrun") != 30000 || over.num("remaining") != -30000 {
		t.Fatalf("dépassement identifiable : %s", over.body)
	}
	cats, _ := over.json()["categories"].([]any)
	if len(cats) != 1 {
		t.Fatalf("détail par catégorie : %s", over.body)
	}
	dash := tn.owner.must(200, "GET", "/api/v1/dashboard", nil).json()
	if dash["over_budget_count"] != float64(1) {
		t.Fatalf("le dashboard doit signaler le dépassement : %v", dash)
	}

	// La référence est toujours la dernière version publiée : un brouillon ne la change pas.
	draft := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+v1+"/revise", map[string]any{"reason": "Avenant"})
	lines, _ := draft.json()["lines"].([]any)
	line, _ := lines[0].(map[string]any)
	tn.manager.must(200, "PATCH", "/api/v1/budget-lines/"+line["id"].(string), map[string]any{"unit_price": 5000})
	if got := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil).num("budget"); got != 100000 {
		t.Fatalf("référence attendue = version publiée (100000) : %d", got)
	}
	tn.manager.must(200, "POST", "/api/v1/budget-versions/"+draft.str("id")+"/publish", nil)
	after := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if after.num("budget") != 500000 || after.str("health") != "ok" {
		t.Fatalf("nouvelle référence : %s", after.body)
	}
}

// CF-021
func TestCF021_Materials(t *testing.T) {
	tn := newTenant(t)
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"unit": "sac"}), 422, "validation.failed")
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"name": "Ciment"}), 422, "validation.failed")
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"name": "Ciment", "unit": "camion"}), 422, "validation.failed")
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"name": "Ciment", "unit": "sac", "sku": "espace interdit"}), 422, "validation.failed")

	m := tn.manager.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Ciment CPJ 45", "unit": "sac", "sku": "CIM-45"})
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"name": "Autre", "unit": "sac", "sku": "cim-45"}), 409, "material.duplicate")
	// Plusieurs matériaux sans SKU restent possibles.
	tn.manager.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Sable", "unit": "m3"})
	tn.manager.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Gravier", "unit": "m3"})
	// Le même SKU reste libre dans une autre organisation.
	newTenant(t).owner.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Ciment CPJ 45", "unit": "sac", "sku": "CIM-45"})

	// Un matériau archivé disparaît du catalogue, mais reste dans l'historique.
	project := tn.newProject(t)
	mv := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
		"movement_type": "in", "occurred_on": "2026-11-01", "items": []map[string]any{{"material_id": m.str("id"), "quantity": "10"}},
	})
	tn.manager.must(200, "POST", "/api/v1/stock-movements/"+mv.str("id")+"/post", nil)
	tn.manager.must(200, "POST", "/api/v1/materials/"+m.str("id")+"/archive", nil)

	for _, item := range tn.manager.must(200, "GET", "/api/v1/materials", nil).items() {
		if item["id"] == m.str("id") {
			t.Fatal("un matériau archivé ne doit plus apparaître au catalogue")
		}
	}
	if n := len(tn.manager.must(200, "GET", "/api/v1/materials?include_archived=true", nil).items()); n != 3 {
		t.Fatalf("3 matériaux attendus avec les archivés, %d", n)
	}
	if n := len(tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/history", nil).items()); n != 1 {
		t.Fatalf("l'historique doit conserver le matériau archivé : %d", n)
	}
	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
		"movement_type": "in", "occurred_on": "2026-11-02", "items": []map[string]any{{"material_id": m.str("id"), "quantity": "1"}},
	}), 422, "material.archived")
}

func (tn *tenant) material(t *testing.T) string {
	t.Helper()
	return tn.manager.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Article " + unique(), "unit": "sac"}).str("id")
}

// move crée et poste un mouvement.
func (tn *tenant) move(t *testing.T, c *client, project, material, kind, quantity string) response {
	t.Helper()
	body := map[string]any{"movement_type": kind, "occurred_on": "2026-11-01", "items": []map[string]any{{"material_id": material, "quantity": quantity}}}
	if kind == "adjustment" {
		body["reason"] = "Inventaire"
	}
	created := c.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", body)
	return c.do("POST", "/api/v1/stock-movements/"+created.str("id")+"/post", nil)
}

func (tn *tenant) onHand(t *testing.T, project, material string) string {
	t.Helper()
	for _, item := range tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock", nil).items() {
		if item["material_id"] == material {
			return item["on_hand"].(string)
		}
	}
	return "0"
}

// CF-022
func TestCF022_StockMovements(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	material := tn.material(t)

	t.Run("un brouillon n'a aucun effet", func(t *testing.T) {
		tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
			"movement_type": "in", "occurred_on": "2026-11-01", "items": []map[string]any{{"material_id": material, "quantity": "999"}},
		})
		if got := tn.onHand(t, project, material); got != "0" {
			t.Fatalf("solde %s", got)
		}
	})

	if r := tn.move(t, tn.worker, project, material, "in", "200"); r.status != 200 {
		t.Fatalf("entrée : %s", r.body)
	}
	if r := tn.move(t, tn.worker, project, material, "out", "50.5"); r.status != 200 {
		t.Fatalf("sortie : %s", r.body)
	}
	if got := tn.onHand(t, project, material); got != "149.5" {
		t.Fatalf("200 − 50,5 = 149,5 attendu : %s", got)
	}

	t.Run("une sortie ne rend pas le stock négatif", func(t *testing.T) {
		expectCode(t, tn.move(t, tn.worker, project, material, "out", "149.6"), 409, "stock.insufficient")
		if got := tn.onHand(t, project, material); got != "149.5" {
			t.Fatalf("le refus doit laisser le solde intact : %s", got)
		}
		fresh := tn.material(t)
		expectCode(t, tn.move(t, tn.worker, project, fresh, "out", "1"), 409, "stock.insufficient")
		expectCode(t, tn.move(t, tn.worker, project, material, "adjustment", "-1000"), 409, "stock.insufficient")
	})

	t.Run("atomicité : un article en défaut annule tout le mouvement", func(t *testing.T) {
		a, b := tn.material(t), tn.material(t)
		if r := tn.move(t, tn.manager, project, a, "in", "5"); r.status != 200 {
			t.Fatal(string(r.body))
		}
		mv := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
			"movement_type": "out", "occurred_on": "2026-11-01",
			"items": []map[string]any{{"material_id": a, "quantity": "5"}, {"material_id": b, "quantity": "1"}},
		})
		expectCode(t, tn.manager.do("POST", "/api/v1/stock-movements/"+mv.str("id")+"/post", nil), 409, "stock.insufficient")
		if got := tn.onHand(t, project, a); got != "5" {
			t.Fatalf("le premier article ne doit pas avoir bougé : %s", got)
		}
	})

	t.Run("mouvement posté immuable", func(t *testing.T) {
		mv := tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
			"movement_type": "in", "occurred_on": "2026-11-01", "items": []map[string]any{{"material_id": material, "quantity": "1"}},
		})
		tn.worker.must(200, "POST", "/api/v1/stock-movements/"+mv.str("id")+"/post", nil)
		expectCode(t, tn.worker.do("POST", "/api/v1/stock-movements/"+mv.str("id")+"/post", nil), 409, "stock.already_posted")
		ctx := context.Background()
		for _, stmt := range []string{
			"UPDATE stock_movements SET reason = 'x' WHERE id = '" + mv.str("id") + "'",
			"DELETE FROM stock_movements WHERE id = '" + mv.str("id") + "'",
			"UPDATE stock_movement_items SET quantity = 500 WHERE movement_id = '" + mv.str("id") + "'",
			"DELETE FROM stock_movement_items WHERE movement_id = '" + mv.str("id") + "'",
		} {
			if _, err := env.ownerDB.Exec(ctx, stmt); err == nil || !strings.Contains(err.Error(), "immutable") {
				t.Errorf("%s : refus attendu, obtenu %v", stmt, err)
			}
		}
	})

	t.Run("concurrence : 20 sorties de 10 sur un stock de 100", func(t *testing.T) {
		m := tn.material(t)
		if r := tn.move(t, tn.manager, project, m, "in", "100"); r.status != 200 {
			t.Fatal(string(r.body))
		}
		ids := make([]string, 20)
		for i := range ids {
			ids[i] = tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
				"movement_type": "out", "occurred_on": "2026-11-01", "items": []map[string]any{{"material_id": m, "quantity": "10"}},
			}).str("id")
		}
		ok, statuses := concurrently(len(ids), func(i int) response {
			return tn.manager.do("POST", "/api/v1/stock-movements/"+ids[i]+"/post", nil)
		})
		if ok != 10 {
			t.Fatalf("exactement 10 sorties attendues, %d : %v", ok, statuses)
		}
		for _, s := range statuses {
			if s != 200 && s != 409 {
				t.Fatalf("statut inattendu : %v", statuses)
			}
		}
		if got := tn.onHand(t, project, m); got != "0" {
			t.Fatalf("solde final %s", got)
		}
	})

	t.Run("concurrence : mouvements croisés sans interblocage", func(t *testing.T) {
		a, b := tn.material(t), tn.material(t)
		ok, statuses := concurrently(16, func(i int) response {
			items := []map[string]any{{"material_id": a, "quantity": "1"}, {"material_id": b, "quantity": "1"}}
			if i%2 == 0 {
				items[0], items[1] = items[1], items[0]
			}
			mv := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/stock-movements", map[string]any{
				"movement_type": "in", "occurred_on": "2026-11-01", "items": items,
			})
			return tn.manager.do("POST", "/api/v1/stock-movements/"+mv.str("id")+"/post", nil)
		})
		if ok != 16 {
			t.Fatalf("16 succès attendus, %d : %v", ok, statuses)
		}
		if got := tn.onHand(t, project, a); got != "16" {
			t.Fatalf("solde %s", got)
		}
	})

	t.Run("le stock est bien par chantier", func(t *testing.T) {
		other := tn.newProject(t)
		if got := tn.onHand(t, other, material); got != "0" {
			t.Fatalf("le stock ne doit pas se partager entre chantiers : %s", got)
		}
		expectCode(t, tn.move(t, tn.manager, other, material, "out", "1"), 409, "stock.insufficient")
	})
}

// CF-023
func TestCF023_StockBalancesAndHistory(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	cement := tn.material(t)
	sand := tn.material(t)
	for _, m := range []struct{ material, kind, quantity string }{
		{cement, "in", "100"}, {cement, "out", "30"}, {sand, "in", "12.5"}, {sand, "adjustment", "-2.5"},
	} {
		if r := tn.move(t, tn.worker, project, m.material, m.kind, m.quantity); r.status != 200 {
			t.Fatal(string(r.body))
		}
	}
	if got := tn.onHand(t, project, cement); got != "70" {
		t.Fatalf("ciment : %s", got)
	}
	if got := tn.onHand(t, project, sand); got != "10" {
		t.Fatalf("sable : %s", got)
	}

	history := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/history", nil).items()
	if len(history) != 4 {
		t.Fatalf("4 lignes d'historique attendues, %d", len(history))
	}
	for _, field := range []string{"occurred_on", "movement_type", "quantity", "author_id", "author_name", "material_name"} {
		if history[0][field] == nil {
			t.Errorf("champ %s absent de l'historique : %v", field, history[0])
		}
	}

	byMaterial := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/history?material_id="+cement, nil).items()
	byType := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/history?movement_type=adjustment", nil).items()
	none := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/history?date_from=2030-01-01", nil).items()
	if len(byMaterial) != 2 || len(byType) != 1 || len(none) != 0 {
		t.Fatalf("filtres : matériau=%d type=%d date=%d", len(byMaterial), len(byType), len(none))
	}

	t.Run("rapprochement : solde = recalcul depuis le journal", func(t *testing.T) {
		rows := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock/reconciliation", nil).items()
		if len(rows) != 2 {
			t.Fatalf("2 articles attendus, %d", len(rows))
		}
		for _, r := range rows {
			if r["on_hand"] != r["journal_total"] {
				t.Errorf("écart : %v", r)
			}
		}
	})
}

// CF-024
func TestCF024_Tasks(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	path := "/api/v1/projects/" + project + "/tasks"

	expectCode(t, tn.manager.do("POST", path, map[string]any{"title": "x"}), 422, "validation.failed")
	expectCode(t, tn.manager.do("POST", path, map[string]any{"title": "Coffrage", "priority": "absolue"}), 422, "validation.failed")

	task := tn.manager.must(201, "POST", path, map[string]any{
		"title": "Coffrage dalle R+1", "priority": "high", "due_date": "2026-12-01", "assignee_ids": []string{tn.workerID},
	})
	if task.str("priority") != "high" || task.str("due_date") != "2026-12-01" || task.str("project_id") != project {
		t.Fatalf("priorité/échéance/chantier : %s", task.body)
	}

	t.Run("responsables autorisés", func(t *testing.T) {
		// Actif mais hors de ce chantier.
		outsider := tn.invited(t, "site_worker")
		expectCode(t, tn.manager.do("POST", path, map[string]any{"title": "Refus", "assignee_ids": []string{outsider}}), 422, "validation.failed")
		// D'une autre organisation.
		other := newTenant(t)
		expectCode(t, tn.manager.do("POST", path, map[string]any{"title": "Refus", "assignee_ids": []string{other.workerID}}), 422, "validation.failed")
	})

	tn.manager.must(201, "POST", path, map[string]any{"title": "Ferraillage poteaux"})
	if n := len(tn.manager.must(200, "GET", path, nil).items()); n != 2 {
		t.Fatalf("2 tâches attendues, %d", n)
	}
	mine := tn.manager.must(200, "GET", path+"?assignee_id="+tn.workerID, nil).items()
	if len(mine) != 1 || mine[0]["id"] != task.str("id") {
		t.Fatalf("filtre responsable : %v", mine)
	}
	assignees, _ := mine[0]["assignee_ids"].([]any)
	if len(assignees) != 1 {
		t.Fatalf("responsables exposés : %v", mine[0])
	}
	if n := len(tn.manager.must(200, "GET", path+"?status=done", nil).items()); n != 0 {
		t.Fatalf("filtre statut : %d", n)
	}

	updated := tn.manager.must(200, "PATCH", "/api/v1/tasks/"+task.str("id"), map[string]any{"priority": "urgent", "assignee_ids": []string{}}, "If-Match", "1")
	if updated.str("priority") != "urgent" {
		t.Fatalf("mise à jour : %s", updated.body)
	}
	if ids, _ := updated.json()["assignee_ids"].([]any); len(ids) != 0 {
		t.Fatalf("responsables retirés attendus : %s", updated.body)
	}
	expectCode(t, tn.manager.do("PATCH", "/api/v1/tasks/"+task.str("id"), map[string]any{"priority": "low"}, "If-Match", "1"), 409, "resource.stale")
}

func (tn *tenant) invited(t *testing.T, role string) string {
	t.Helper()
	_, id := tn.invite(t, role)
	return id
}

// CF-025
func TestCF025_TaskProgress(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	task := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/tasks", map[string]any{"title": "Coffrage", "assignee_ids": []string{tn.workerID}}).str("id")
	other := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/tasks", map[string]any{"title": "Autre tâche"}).str("id")
	progress := "/api/v1/tasks/" + task + "/progress"

	t.Run("un chef de chantier agit sur ses tâches uniquement", func(t *testing.T) {
		expectCode(t, tn.worker.do("POST", "/api/v1/tasks/"+other+"/progress", map[string]any{"progress_pct": 10}), 403, "auth.forbidden")
		expectCode(t, tn.viewer.do("POST", progress, map[string]any{"progress_pct": 10}), 403, "auth.forbidden")
	})

	expectCode(t, tn.worker.do("POST", progress, map[string]any{"progress_pct": 101}), 422, "validation.failed")
	expectCode(t, tn.worker.do("POST", progress, map[string]any{"progress_pct": -1}), 422, "validation.failed")
	expectCode(t, tn.worker.do("POST", progress, map[string]any{"status": "inconnu"}), 409, "task.invalid_transition")

	started := tn.worker.must(200, "POST", progress, map[string]any{"status": "in_progress", "progress_pct": 40})
	if started.num("progress_pct") != 40 || started.json()["completed_at"] != nil {
		t.Fatalf("en cours : %s", started.body)
	}
	done := tn.worker.must(200, "POST", progress, map[string]any{"status": "done"})
	if done.num("progress_pct") != 100 || done.str("completed_at") == "" {
		t.Fatalf("Done doit renseigner completed_at : %s", done.body)
	}
	expectCode(t, tn.worker.do("POST", progress, map[string]any{"status": "blocked"}), 409, "task.invalid_transition")

	reopened := tn.manager.must(200, "POST", progress, map[string]any{"status": "in_progress"})
	if reopened.json()["completed_at"] != nil || reopened.num("progress_pct") == 100 {
		t.Fatalf("réouverture : %s", reopened.body)
	}

	detail := tn.worker.must(200, "GET", "/api/v1/tasks/"+task, nil)
	history, _ := detail.json()["history"].([]any)
	if len(history) != 3 {
		t.Fatalf("3 transitions historisées attendues, %d : %s", len(history), detail.body)
	}
	first, _ := history[0].(map[string]any)
	if first["from_status"] != "todo" || first["to_status"] != "in_progress" || first["actor_id"] != tn.workerID {
		t.Fatalf("transition : %v", first)
	}

	_, err := env.ownerDB.Exec(context.Background(), "UPDATE tasks SET progress_pct = 150 WHERE id = $1", task)
	if err == nil {
		t.Fatal("la base doit refuser une progression hors de 0-100")
	}
}

// CF-026
func TestCF026_DailyReports(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	id := uuid.NewString()
	path := "/api/v1/daily-reports/" + id
	body := map[string]any{"project_id": project, "report_date": "2026-11-03", "work_done": "Coffrage de la dalle R+1 terminé", "workforce_count": 8, "weather": "sunny"}

	draft := tn.worker.must(200, "PUT", path, body)
	if draft.str("status") != "draft" || draft.str("report_date") != "2026-11-03" || draft.str("project_id") != project {
		t.Fatalf("brouillon : %s", draft.body)
	}

	t.Run("validation", func(t *testing.T) {
		for name, b := range map[string]map[string]any{
			"sans travaux":     {"project_id": project, "report_date": "2026-11-04", "work_done": " "},
			"sans date":        {"project_id": project, "work_done": "Travaux"},
			"météo inconnue":   {"project_id": project, "report_date": "2026-11-04", "work_done": "Travaux", "weather": "neige"},
			"effectif négatif": {"project_id": project, "report_date": "2026-11-04", "work_done": "Travaux", "workforce_count": -1},
		} {
			t.Run(name, func(t *testing.T) {
				expectCode(t, tn.worker.do("PUT", "/api/v1/daily-reports/"+uuid.NewString(), b), 422, "validation.failed")
			})
		}
		expectCode(t, tn.viewer.do("PUT", "/api/v1/daily-reports/"+uuid.NewString(), body), 403, "auth.forbidden")
	})

	t.Run("un seul rapport par chantier, date et équipe", func(t *testing.T) {
		expectCode(t, tn.worker.do("PUT", "/api/v1/daily-reports/"+uuid.NewString(), body), 409, "report.duplicate")
		team := map[string]any{"project_id": project, "report_date": "2026-11-03", "team_label": "Équipe B", "work_done": "Ferraillage"}
		tn.worker.must(200, "PUT", "/api/v1/daily-reports/"+uuid.NewString(), team)
	})

	t.Run("brouillon modifiable, sans écrasement silencieux", func(t *testing.T) {
		body["work_done"] = "Coffrage terminé, ferraillage commencé"
		updated := tn.worker.must(200, "PUT", path, body, "If-Match", "1")
		if updated.num("row_version") != 2 {
			t.Fatalf("row_version : %s", updated.body)
		}
		expectCode(t, tn.worker.do("PUT", path, body, "If-Match", "1"), 409, "resource.stale")
	})

	t.Run("liens vers les tâches du chantier", func(t *testing.T) {
		task := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/tasks", map[string]any{"title": "Coffrage"}).str("id")
		withTasks := map[string]any{"project_id": project, "report_date": "2026-11-03", "work_done": "Coffrage", "tasks": []map[string]any{{"task_id": task, "done_that_day": true}}}
		r := tn.worker.must(200, "PUT", path, withTasks)
		tasks, _ := r.json()["tasks"].([]any)
		if len(tasks) != 1 {
			t.Fatalf("tâche liée : %s", r.body)
		}
		otherProject := tn.newProject(t)
		foreign := tn.manager.must(201, "POST", "/api/v1/projects/"+otherProject+"/tasks", map[string]any{"title": "Ailleurs"}).str("id")
		withTasks["tasks"] = []map[string]any{{"task_id": foreign, "done_that_day": false}}
		expectCode(t, tn.worker.do("PUT", path, withTasks), 422, "validation.failed")
	})

	t.Run("photos consultables", func(t *testing.T) {
		attachment := uuid.NewString()
		pre := tn.worker.must(201, "POST", "/api/v1/attachments/presign", map[string]any{
			"id": attachment, "daily_report_id": id, "kind": "site_photo", "mime_type": "image/webp", "size_bytes": 16,
		})
		att, _ := pre.json()["attachment"].(map[string]any)
		env.store.put(att["storage_key"].(string), []byte("RIFFxxxxWEBPVP8 "))
		tn.worker.must(200, "POST", "/api/v1/attachments/"+attachment+"/complete", nil)
		detail := tn.manager.must(200, "GET", path, nil)
		files, _ := detail.json()["attachments"].([]any)
		if len(files) != 1 {
			t.Fatalf("1 photo attendue : %s", detail.body)
		}
		if url := tn.viewer.must(200, "GET", "/api/v1/attachments/"+attachment+"/url", nil).str("url"); url == "" {
			t.Fatal("lien de consultation attendu")
		}
	})

	t.Run("soumis verrouillé sauf action auditée", func(t *testing.T) {
		submitted := tn.worker.must(200, "POST", path+"/submit", nil)
		if submitted.str("status") != "submitted" || submitted.str("submitted_at") == "" {
			t.Fatalf("soumission : %s", submitted.body)
		}
		expectCode(t, tn.worker.do("POST", path+"/submit", nil), 409, "report.already_submitted")
		expectCode(t, tn.worker.do("PUT", path, map[string]any{"project_id": project, "report_date": "2026-11-03", "work_done": "Modifié après coup"}), 409, "report.locked")
		_, err := env.ownerDB.Exec(context.Background(), "UPDATE daily_reports SET work_done = 'falsifié' WHERE id = $1", id)
		if err == nil || !strings.Contains(err.Error(), "locked") {
			t.Fatalf("le trigger doit refuser : %v", err)
		}

		expectCode(t, tn.worker.do("POST", path+"/reopen", map[string]any{"reason": "Erreur"}), 403, "auth.forbidden")
		expectCode(t, tn.manager.do("POST", path+"/reopen", map[string]any{"reason": " "}), 422, "validation.failed")
		reopened := tn.manager.must(200, "POST", path+"/reopen", map[string]any{"reason": "Effectif erroné"})
		if reopened.str("status") != "draft" {
			t.Fatalf("réouverture : %s", reopened.body)
		}
		logs := tn.owner.must(200, "GET", "/api/v1/audit-logs?action=report.reopened&entity_id="+id, nil).items()
		if len(logs) != 1 {
			t.Fatalf("la réouverture doit être auditée : %d", len(logs))
		}
	})

	if n := len(tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/daily-reports?date_from=2026-11-01&date_to=2026-11-30", nil).items()); n != 2 {
		t.Fatalf("2 rapports attendus, %d", n)
	}
}

// CF-027
func TestCF027_Attachments(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	expense := tn.newExpense(t, tn.worker, project, 1000)
	presign := func(c *client, extra map[string]any) response {
		body := map[string]any{"id": uuid.NewString(), "expense_id": expense, "kind": "receipt", "mime_type": "image/jpeg", "size_bytes": 1000}
		for k, v := range extra {
			body[k] = v
		}
		return c.do("POST", "/api/v1/attachments/presign", body)
	}

	t.Run("type et taille contrôlés à la demande", func(t *testing.T) {
		expectCode(t, presign(tn.worker, map[string]any{"mime_type": "text/html"}), 422, "validation.failed")
		expectCode(t, presign(tn.worker, map[string]any{"mime_type": "image/svg+xml"}), 422, "validation.failed")
		expectCode(t, presign(tn.worker, map[string]any{"size_bytes": 26 * 1024 * 1024}), 422, "validation.failed")
		expectCode(t, presign(tn.worker, map[string]any{"size_bytes": 0}), 422, "validation.failed")
	})

	id := uuid.NewString()
	pre := presign(tn.worker, map[string]any{"id": id, "original_name": "../../recu.jpg"})
	if pre.status != 201 || pre.str("upload_url") == "" || pre.num("expires_in") != 300 {
		t.Fatalf("URL pré-signée de 5 minutes attendue : %s", pre.body)
	}
	att, _ := pre.json()["attachment"].(map[string]any)
	key := att["storage_key"].(string)
	var orgID string
	if err := env.ownerDB.QueryRow(context.Background(), "SELECT organization_id FROM projects WHERE id = $1", project).Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "org/"+orgID+"/proj/"+project+"/") {
		t.Fatalf("clé objet non cloisonnée : %s", key)
	}
	if att["original_name"] != "recu.jpg" || att["status"] != "pending" {
		t.Fatalf("métadonnées : %v", att)
	}

	t.Run("le rattachement est déduit de la ressource", func(t *testing.T) {
		other := tn.newProject(t)
		expectCode(t, presign(tn.worker, map[string]any{"project_id": other}), 422, "validation.failed")
	})

	t.Run("confirmation sur les octets réellement reçus", func(t *testing.T) {
		expectCode(t, tn.worker.do("POST", "/api/v1/attachments/"+id+"/complete", nil), 409, "attachment.not_uploaded")
		expectCode(t, tn.worker.do("GET", "/api/v1/attachments/"+id+"/url", nil), 409, "attachment.not_uploaded")
		env.store.put(key, []byte("<html><script>alert(1)</script></html>"))
		expectCode(t, tn.worker.do("POST", "/api/v1/attachments/"+id+"/complete", nil), 422, "attachment.type_mismatch")
		env.store.put(key, make([]byte, 26*1024*1024))
		expectCode(t, tn.worker.do("POST", "/api/v1/attachments/"+id+"/complete", nil), 422, "attachment.invalid_size")
		env.store.put(key, []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00})
		done := tn.worker.must(200, "POST", "/api/v1/attachments/"+id+"/complete", nil)
		if done.str("status") != "uploaded" || done.num("size_bytes") != 5 {
			t.Fatalf("la taille réelle doit remplacer la taille annoncée : %s", done.body)
		}
	})

	t.Run("accès contrôlé par ressource", func(t *testing.T) {
		tn.worker.must(200, "GET", "/api/v1/attachments/"+id+"/url", nil)
		outsider, _ := tn.invite(t, "site_worker")
		expectCode(t, outsider.do("GET", "/api/v1/attachments/"+id+"/url", nil), 403, "auth.forbidden")
		expectCode(t, newTenant(t).owner.do("GET", "/api/v1/attachments/"+id+"/url", nil), 404, "attachment.not_found")
	})

	t.Run("seul le fichier fait partie des métadonnées, pas le lien permanent", func(t *testing.T) {
		detail := tn.worker.must(200, "GET", "/api/v1/expenses/"+expense, nil)
		if strings.Contains(string(detail.body), "s3.test") {
			t.Fatal("aucune URL ne doit être stockée ou exposée dans les métadonnées")
		}
	})
}

// CF-028
func TestCF028_Exports(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	other := tn.newProject(t)
	tn.publishBudget(t, project, "100", 10000)
	tn.approvedExpense(t, project, 300000)
	tn.approvedExpense(t, project, 25000)
	tn.approvedExpense(t, other, 7000)
	draft := tn.owner.must(201, "POST", "/api/v1/projects/"+project+"/expenses", map[string]any{
		"category_id": tn.categoryID, "description": "=HYPERLINK(\"http://evil\")", "amount": 111, "expense_date": "2026-11-03",
	})
	_ = draft

	t.Run("permissions", func(t *testing.T) {
		for _, c := range []*client{tn.manager, tn.worker, tn.viewer} {
			expectCode(t, c.do("GET", "/api/v1/exports/expenses.xlsx", nil), 403, "auth.forbidden")
			expectCode(t, c.do("GET", "/api/v1/exports/projects/"+project+"/report.pdf", nil), 403, "auth.forbidden")
		}
		expectCode(t, tn.accountant.do("GET", "/api/v1/exports/expenses.xlsx?project_id="+uuid.NewString(), nil), 404, "project.not_found")
	})

	r := tn.accountant.must(200, "GET", "/api/v1/exports/expenses.xlsx?project_id="+project+"&status=approved", nil)
	if !strings.Contains(r.header.Get("Content-Disposition"), ".xlsx") {
		t.Fatalf("en-têtes : %v", r.header)
	}
	file, err := excelize.OpenReader(bytes.NewReader(r.body))
	if err != nil {
		t.Fatalf("Excel illisible : %v", err)
	}
	defer func() { _ = file.Close() }()
	rows, err := file.GetRows("Dépenses")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("filtres respectés, totaux identiques aux données", func(t *testing.T) {
		if len(rows) != 4 { // en-tête, 2 dépenses approuvées du chantier, total
			t.Fatalf("%d lignes : %v", len(rows), rows)
		}
		total := rows[3]
		if total[5] != "Total" {
			t.Fatalf("ligne de total : %v", total)
		}
		value, err := file.GetCellValue("Dépenses", "G4")
		if err != nil {
			t.Fatal(err)
		}
		if strings.ReplaceAll(value, ",", "") != "325000" {
			t.Fatalf("total Excel = %q, attendu 325000", value)
		}
		formula, err := file.GetCellFormula("Dépenses", "G4")
		if err != nil || !strings.Contains(formula, "SUM(G2:G3)") {
			t.Fatalf("total recalculable attendu : %q (%v)", formula, err)
		}
		api := tn.accountant.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
		if api.num("approved") != 325000 {
			t.Fatalf("l'export doit coïncider avec l'API : %s", api.body)
		}
	})

	t.Run("montants numériques, exploitables", func(t *testing.T) {
		kind, err := file.GetCellType("Dépenses", "G2")
		if err != nil {
			t.Fatal(err)
		}
		if kind == excelize.CellTypeInlineString || kind == excelize.CellTypeSharedString {
			t.Fatalf("un montant ne doit pas être une chaîne : %v", kind)
		}
	})

	t.Run("formule malveillante neutralisée", func(t *testing.T) {
		all := tn.accountant.must(200, "GET", "/api/v1/exports/expenses.xlsx?project_id="+project, nil)
		f, err := excelize.OpenReader(bytes.NewReader(all.body))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		lines, _ := f.GetRows("Dépenses")
		found := false
		for i, line := range lines {
			if len(line) > 4 && strings.Contains(line[4], "HYPERLINK") {
				found = true
				if !strings.HasPrefix(line[4], "'") {
					t.Fatalf("injection de formule non neutralisée : %q", line[4])
				}
				cell := "E" + string(rune('1'+i))
				if formula, _ := f.GetCellFormula("Dépenses", cell); formula != "" {
					t.Fatalf("cellule interprétée comme formule : %q", formula)
				}
			}
		}
		if !found {
			t.Fatal("ligne de test absente")
		}
	})

	t.Run("PDF", func(t *testing.T) {
		pdf := tn.accountant.must(200, "GET", "/api/v1/exports/projects/"+project+"/report.pdf", nil)
		if !bytes.HasPrefix(pdf.body, []byte("%PDF-")) || pdf.header.Get("Content-Type") != "application/pdf" {
			t.Fatalf("PDF invalide : %q", pdf.body[:min(20, len(pdf.body))])
		}
		if len(pdf.body) < 1000 {
			t.Fatalf("PDF suspect (%d octets)", len(pdf.body))
		}
	})

	t.Run("chaque export est audité", func(t *testing.T) {
		logs := tn.owner.must(200, "GET", "/api/v1/audit-logs?action=export.created", nil).items()
		if len(logs) < 3 {
			t.Fatalf("exports audités : %d", len(logs))
		}
	})
}

// CF-032 : le parcours critique complet, du signup au paiement, en passant par les bons rôles.
func TestCF032_CriticalJourney(t *testing.T) {
	tn := newTenant(t)

	project := tn.manager.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "VILLA-" + unique(), "name": "Villa Almadies", "client_name": "M. et Mme Sarr",
		"address": "Almadies, Dakar", "start_date": "2026-03-03", "status": "in_progress",
	}).str("id")
	tn.manager.must(200, "PUT", "/api/v1/projects/"+project+"/members/"+tn.workerID, map[string]any{})

	budget := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/budget/versions", nil).str("id")
	tn.manager.must(201, "POST", "/api/v1/budget-versions/"+budget+"/lines", map[string]any{
		"category_id": tn.categoryID, "label": "Ciment CPJ 45", "quantity": "1000", "unit": "sac", "unit_price": 5000,
	})
	tn.manager.must(200, "POST", "/api/v1/budget-versions/"+budget+"/publish", nil)

	// Le chef de chantier saisit depuis son téléphone, avec un id client et une clé d'idempotence.
	expenseID := uuid.NewString()
	key := uuid.NewString()
	expense := map[string]any{"id": expenseID, "category_id": tn.categoryID, "description": "Ciment 50 sacs", "amount": 450000, "expense_date": "2026-11-03"}
	tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", expense, "Idempotency-Key", key)
	replay := tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", expense, "Idempotency-Key", key)
	if replay.header.Get("Idempotency-Replayed") != "true" {
		t.Fatal("le rejeu doit retourner la réponse mémorisée")
	}
	tn.worker.must(200, "POST", "/api/v1/expenses/"+expenseID+"/submit", nil)

	// Tant qu'elle n'est pas approuvée, elle reste à part.
	fin := tn.owner.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if fin.num("pending") != 450000 || fin.num("approved") != 0 || fin.num("remaining") != 5000000 {
		t.Fatalf("avant approbation : %s", fin.body)
	}

	inbox := tn.accountant.must(200, "GET", "/api/v1/expenses?status=pending", nil).items()
	if len(inbox) != 1 || inbox[0]["id"] != expenseID {
		t.Fatalf("file de validation : %v", inbox)
	}
	tn.accountant.must(200, "POST", "/api/v1/expenses/"+expenseID+"/approve", nil)
	tn.accountant.must(201, "POST", "/api/v1/expenses/"+expenseID+"/payments", map[string]any{"amount": 200000, "paid_on": "2026-11-05", "method": "wave"})

	fin = tn.owner.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if fin.num("pending") != 0 || fin.num("approved") != 450000 || fin.num("paid") != 200000 || fin.num("remaining") != 4550000 {
		t.Fatalf("après approbation : %s", fin.body)
	}

	// Traçabilité de bout en bout.
	logs := tn.owner.must(200, "GET", "/api/v1/audit-logs?entity_id="+expenseID, nil).items()
	seen := map[string]bool{}
	for _, l := range logs {
		seen[l["action"].(string)] = true
	}
	for _, want := range []string{"expense.created", "expense.submitted", "expense.approved", "payment.recorded"} {
		if !seen[want] {
			t.Errorf("%s absent de l'audit : %v", want, seen)
		}
	}
}
