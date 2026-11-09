package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// CF-009
func TestCF009_Projects(t *testing.T) {
	tn := newTenant(t)
	ref := "REF-" + unique()
	valid := map[string]any{"reference": ref, "name": "Villa Almadies", "client_name": "M. Sarr", "address": "Almadies", "start_date": "2026-03-03"}

	t.Run("champs obligatoires", func(t *testing.T) {
		r := tn.manager.do("POST", "/api/v1/projects", map[string]any{"name": "x"})
		expectCode(t, r, 422, "validation.failed")
		fields, _ := r.json()["fields"].(map[string]any)
		for _, f := range []string{"reference", "name", "client_name", "address", "start_date"} {
			if fields[f] == nil {
				t.Errorf("erreur attendue sur %s : %s", f, r.body)
			}
		}
		bad := map[string]any{"reference": "D-" + unique(), "name": "Dates", "client_name": "Client", "address": "Dakar", "start_date": "2026-03-03", "planned_end_date": "2026-01-01"}
		expectCode(t, tn.manager.do("POST", "/api/v1/projects", bad), 422, "validation.failed")
	})

	created := tn.manager.must(201, "POST", "/api/v1/projects", valid)
	id := created.str("id")

	t.Run("référence unique par organisation", func(t *testing.T) {
		dup := map[string]any{"reference": strings.ToLower(ref), "name": "Copie", "client_name": "Client", "address": "Dakar", "start_date": "2026-03-03"}
		expectCode(t, tn.manager.do("POST", "/api/v1/projects", dup), 409, "project.duplicate_reference")
		// La même référence reste possible dans une autre organisation.
		newTenant(t).owner.must(201, "POST", "/api/v1/projects", valid)
	})

	t.Run("un client ne peut pas fixer de champ serveur", func(t *testing.T) {
		evil := map[string]any{"reference": "E-" + unique(), "name": "Evil", "client_name": "Client", "address": "Dakar", "start_date": "2026-03-03", "organization_id": "00000000-0000-0000-0000-000000000000"}
		expectCode(t, tn.manager.do("POST", "/api/v1/projects", evil), 400, "request.invalid_json")
	})

	t.Run("verrou optimiste", func(t *testing.T) {
		expectCode(t, tn.manager.do("PATCH", "/api/v1/projects/"+id, map[string]any{"city": "Dakar"}), 428, "request.if_match_required")
		updated := tn.manager.must(200, "PATCH", "/api/v1/projects/"+id, map[string]any{"city": "Dakar"}, "If-Match", "1")
		if updated.num("row_version") != 2 {
			t.Fatalf("row_version attendu 2 : %s", updated.body)
		}
		expectCode(t, tn.manager.do("PATCH", "/api/v1/projects/"+id, map[string]any{"city": "Thiès"}, "If-Match", "1"), 409, "resource.stale")
	})

	t.Run("filtres, recherche, pagination", func(t *testing.T) {
		for i := range 3 {
			tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
				"reference": fmt.Sprintf("PAG-%d-%s", i, unique()), "name": fmt.Sprintf("Résidence %d", i),
				"client_name": "Client", "address": "Dakar", "start_date": "2026-03-03", "status": "planned",
			})
		}
		page1 := tn.owner.must(200, "GET", "/api/v1/projects?limit=2&q=Résidence", nil)
		if len(page1.items()) != 2 || page1.str("next_cursor") == "" {
			t.Fatalf("page 1 : %s", page1.body)
		}
		page2 := tn.owner.must(200, "GET", "/api/v1/projects?limit=2&q=Résidence&cursor="+page1.str("next_cursor"), nil)
		if len(page2.items()) != 1 || page2.json()["next_cursor"] != nil {
			t.Fatalf("page 2 : %s", page2.body)
		}
		if page1.items()[0]["id"] == page2.items()[0]["id"] {
			t.Fatal("les pages se chevauchent")
		}
		planned := tn.owner.must(200, "GET", "/api/v1/projects?status=planned", nil).items()
		if len(planned) != 3 {
			t.Fatalf("3 chantiers planifiés attendus, %d", len(planned))
		}
	})

	t.Run("un manager garde l'accès au chantier créé pour un autre", func(t *testing.T) {
		other := tn.manager.must(201, "POST", "/api/v1/projects", map[string]any{
			"reference": "M-" + unique(), "name": "Pour un autre", "client_name": "Client", "address": "Dakar",
			"start_date": "2026-03-03", "manager_id": tn.ownerID,
		})
		tn.manager.must(200, "GET", "/api/v1/projects/"+other.str("id"), nil)
	})
}

// CF-010
func TestCF010_ProjectMembers(t *testing.T) {
	tn := newTenant(t)
	project := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "MB-" + unique(), "name": "Équipe", "client_name": "Client", "address": "Dakar", "start_date": "2026-03-03",
	}).str("id")

	expectCode(t, tn.worker.do("GET", "/api/v1/projects/"+project, nil), 403, "auth.forbidden")
	tn.owner.must(200, "PUT", "/api/v1/projects/"+project+"/members/"+tn.workerID, map[string]any{})
	tn.worker.must(200, "GET", "/api/v1/projects/"+project, nil)
	if n := len(tn.owner.must(200, "GET", "/api/v1/projects/"+project+"/members", nil).items()); n != 1 {
		t.Fatalf("1 membre attendu, %d", n)
	}
	tn.owner.must(204, "DELETE", "/api/v1/projects/"+project+"/members/"+tn.workerID, nil)
	expectCode(t, tn.worker.do("GET", "/api/v1/projects/"+project, nil), 403, "auth.forbidden")

	// Seuls les utilisateurs actifs de l'organisation sont affectables.
	tn.owner.must(200, "PATCH", "/api/v1/members/"+tn.viewerID, map[string]any{"status": "suspended"})
	expectCode(t, tn.owner.do("PUT", "/api/v1/projects/"+project+"/members/"+tn.viewerID, map[string]any{}), 422, "validation.failed")
	invited := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "En attente", "email": "wait-" + unique() + "@example.test", "role": "viewer"})
	expectCode(t, tn.owner.do("PUT", "/api/v1/projects/"+project+"/members/"+invited.str("user_id"), map[string]any{}), 422, "validation.failed")
	other := newTenant(t)
	expectCode(t, tn.owner.do("PUT", "/api/v1/projects/"+project+"/members/"+other.workerID, map[string]any{}), 422, "validation.failed")
}

// CF-011
func TestCF011_PhasesAndStatusHistory(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)

	ids := make([]string, 3)
	for i, name := range []string{"Terrassement", "Gros œuvre", "Finitions"} {
		r := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/phases", map[string]any{"name": name})
		ids[i] = r.str("id")
		if r.num("position") != int64(i+1) {
			t.Fatalf("position attendue %d : %s", i+1, r.body)
		}
	}
	reordered := tn.manager.must(200, "PUT", "/api/v1/projects/"+project+"/phases/order", map[string]any{"phase_ids": []string{ids[2], ids[0], ids[1]}}).items()
	if reordered[0]["id"] != ids[2] || reordered[2]["id"] != ids[1] || reordered[0]["position"].(float64) != 1 {
		t.Fatalf("ordre inattendu : %v", reordered)
	}
	expectCode(t, tn.manager.do("PUT", "/api/v1/projects/"+project+"/phases/order", map[string]any{"phase_ids": []string{ids[0]}}), 422, "validation.failed")
	tn.manager.must(200, "PATCH", "/api/v1/projects/"+project+"/phases/"+ids[0], map[string]any{"status": "in_progress"})
	tn.manager.must(204, "DELETE", "/api/v1/projects/"+project+"/phases/"+ids[2], nil)
	rest := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/phases", nil).items()
	if len(rest) != 2 || rest[0]["position"].(float64) != 1 || rest[1]["position"].(float64) != 2 {
		t.Fatalf("la numérotation doit rester continue : %v", rest)
	}

	// Machine à états : in_progress -> completed -> archived, le reste est refusé.
	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/status", map[string]any{"status": "draft"}), 409, "project.invalid_transition")
	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/status", map[string]any{"status": "archived"}), 409, "project.invalid_transition")
	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/status", map[string]any{"status": "inconnu"}), 422, "validation.failed")
	tn.manager.must(200, "POST", "/api/v1/projects/"+project+"/status", map[string]any{"status": "completed", "comment": "Réception"})
	tn.manager.must(200, "POST", "/api/v1/projects/"+project+"/status", map[string]any{"status": "archived"})

	history := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/status-history", nil).items()
	if len(history) != 3 {
		t.Fatalf("3 transitions attendues, %d", len(history))
	}
	if history[1]["to_status"] != "completed" || history[1]["actor_id"] != tn.managerID || history[1]["created_at"] == nil || history[1]["comment"] != "Réception" {
		t.Fatalf("acteur/date/commentaire absents : %v", history[1])
	}

	// Un chantier archivé est en lecture seule.
	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/phases", map[string]any{"name": "Après"}), 409, "project.archived")
	expectCode(t, tn.worker.do("POST", "/api/v1/projects/"+project+"/expenses", map[string]any{"category_id": tn.categoryID, "description": "Après", "amount": 1, "expense_date": "2026-11-03"}), 409, "project.archived")
	tn.manager.must(200, "GET", "/api/v1/projects/"+project, nil)

	// Le trigger impose la machine à états même sans passer par le service.
	_, err := env.ownerDB.Exec(context.Background(), "UPDATE projects SET status = 'in_progress', archived_at = NULL WHERE id = $1", project)
	if err == nil || !strings.Contains(err.Error(), "invalid project transition") {
		t.Fatalf("le trigger aurait dû refuser : %v", err)
	}
}

// CF-012
func TestCF012_BudgetDraft(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	versionID := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/budget/versions", nil).str("id")

	line := func(quantity string, price int64) map[string]any {
		return map[string]any{"category_id": tn.categoryID, "label": "Ciment", "quantity": quantity, "unit": "sac", "unit_price": price}
	}

	r := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/lines", line("10.5", 5000))
	if r.num("total_amount") != 52500 {
		t.Fatalf("total calculé attendu 52500 : %s", r.body)
	}
	r = tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/lines", line("3", 1000))
	if r.num("total_amount") != 55500 {
		t.Fatalf("total attendu 55500 : %s", r.body)
	}

	t.Run("le montant est toujours un entier arrondi par le serveur", func(t *testing.T) {
		r := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/lines", line("0.333", 1000))
		lines, _ := r.json()["lines"].([]any)
		last, _ := lines[len(lines)-1].(map[string]any)
		if last["amount"].(float64) != 333 {
			t.Fatalf("0,333 × 1000 = 333 attendu : %v", last)
		}
		lineID, _ := last["id"].(string)
		tn.manager.must(200, "DELETE", "/api/v1/budget-lines/"+lineID, nil)
	})

	t.Run("le client ne fournit ni total ni montant", func(t *testing.T) {
		bad := line("1", 100)
		bad["amount"] = 999999
		expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/lines", bad), 400, "request.invalid_json")
	})

	t.Run("validation des quantités, unités et prix", func(t *testing.T) {
		for name, body := range map[string]map[string]any{
			"quantité nulle":    line("0", 100),
			"quantité négative": line("-1", 100),
			"trop de décimales": line("1.2345", 100),
			"prix négatif":      line("1", -5),
			"unité vide":        {"category_id": tn.categoryID, "label": "Ciment", "quantity": "1", "unit": "  ", "unit_price": 1},
			"catégorie absente": {"label": "Ciment", "quantity": "1", "unit": "sac", "unit_price": 1},
		} {
			t.Run(name, func(t *testing.T) {
				expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/lines", body), 422, "validation.failed")
			})
		}
	})

	t.Run("modification et suppression recalculent le total", func(t *testing.T) {
		detail := tn.manager.must(200, "GET", "/api/v1/budget-versions/"+versionID, nil)
		lines, _ := detail.json()["lines"].([]any)
		first, _ := lines[0].(map[string]any)
		lineID, _ := first["id"].(string)
		updated := tn.manager.must(200, "PATCH", "/api/v1/budget-lines/"+lineID, map[string]any{"quantity": "20"})
		if updated.num("total_amount") != 103000 {
			t.Fatalf("total attendu 103000 : %s", updated.body)
		}
		deleted := tn.manager.must(200, "DELETE", "/api/v1/budget-lines/"+lineID, nil)
		if deleted.num("total_amount") != 3000 {
			t.Fatalf("total attendu 3000 : %s", deleted.body)
		}
	})

	t.Run("colonnes monétaires en BIGINT, jamais flottantes", func(t *testing.T) {
		rows, err := env.ownerDB.Query(context.Background(), `
			SELECT table_name || '.' || column_name FROM information_schema.columns
			 WHERE table_schema = 'public' AND data_type IN ('real','double precision','money')
			   AND table_name NOT LIKE 'audit_logs%'`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var col string
			if err := rows.Scan(&col); err != nil {
				t.Fatal(err)
			}
			t.Errorf("colonne flottante interdite : %s", col)
		}
		var dataType string
		if err := env.ownerDB.QueryRow(context.Background(), `SELECT data_type FROM information_schema.columns WHERE table_name = 'budget_lines' AND column_name = 'amount'`).Scan(&dataType); err != nil {
			t.Fatal(err)
		}
		if dataType != "bigint" {
			t.Fatalf("budget_lines.amount : %s", dataType)
		}
	})

	expectCode(t, tn.manager.do("POST", "/api/v1/projects/"+project+"/budget/versions", nil), 409, "budget.revision_required")
}

// CF-013
func TestCF013_PublishImmutable(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	versionID := tn.manager.must(201, "POST", "/api/v1/projects/"+project+"/budget/versions", nil).str("id")

	expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/publish", nil), 409, "budget.empty")
	lineResp := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/lines", map[string]any{
		"category_id": tn.categoryID, "label": "Ciment", "quantity": "100", "unit": "sac", "unit_price": 5000,
	})
	lines, _ := lineResp.json()["lines"].([]any)
	lineID, _ := lines[0].(map[string]any)["id"].(string)

	expectCode(t, tn.accountant.do("POST", "/api/v1/budget-versions/"+versionID+"/publish", nil), 403, "auth.forbidden")
	published := tn.manager.must(200, "POST", "/api/v1/budget-versions/"+versionID+"/publish", nil)
	if published.str("status") != "published" || published.str("published_by") != tn.managerID || published.str("published_at") == "" {
		t.Fatalf("auteur et date attendus : %s", published.body)
	}
	if got := tn.owner.must(200, "GET", "/api/v1/projects/"+project, nil).str("current_budget_version_id"); got != versionID {
		t.Fatalf("référence du chantier : %s", got)
	}

	t.Run("version publiée non modifiable via l'API", func(t *testing.T) {
		expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/lines", map[string]any{"category_id": tn.categoryID, "label": "Ajout", "quantity": "1", "unit": "sac", "unit_price": 1}), 409, "budget.version_immutable")
		expectCode(t, tn.manager.do("PATCH", "/api/v1/budget-lines/"+lineID, map[string]any{"quantity": "1"}), 409, "budget.version_immutable")
		expectCode(t, tn.manager.do("DELETE", "/api/v1/budget-lines/"+lineID, nil), 409, "budget.version_immutable")
		expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/publish", nil), 409, "budget.version_immutable")
		expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+versionID+"/discard", nil), 409, "budget.version_immutable")
	})

	t.Run("version publiée non modifiable en SQL direct", func(t *testing.T) {
		ctx := context.Background()
		for name, stmt := range map[string]string{
			"total":          "UPDATE budget_versions SET total_amount = 1 WHERE id = '" + versionID + "'",
			"auteur":         "UPDATE budget_versions SET published_at = now() WHERE id = '" + versionID + "'",
			"suppression":    "DELETE FROM budget_versions WHERE id = '" + versionID + "'",
			"ligne":          "UPDATE budget_lines SET unit_price = 1 WHERE id = '" + lineID + "'",
			"ligne retirée":  "DELETE FROM budget_lines WHERE id = '" + lineID + "'",
			"retour arrière": "UPDATE budget_versions SET status = 'draft', published_at = NULL, published_by = NULL WHERE id = '" + versionID + "'",
		} {
			if _, err := env.ownerDB.Exec(ctx, stmt); err == nil || !strings.Contains(err.Error(), "immutable") && !strings.Contains(err.Error(), "cannot be deleted") {
				t.Errorf("%s : refus attendu par le trigger, obtenu %v", name, err)
			}
		}
	})

	t.Run("publications concurrentes : une seule version publiée", func(t *testing.T) {
		draft := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+versionID+"/revise", map[string]any{"reason": "Hausse du ciment"}).str("id")
		ok, statuses := concurrently(8, func(int) response {
			return tn.manager.do("POST", "/api/v1/budget-versions/"+draft+"/publish", nil)
		})
		if ok != 1 {
			t.Fatalf("une seule publication attendue, %d réussies : %v", ok, statuses)
		}
		for _, s := range statuses {
			if s != 200 && s != 409 {
				t.Fatalf("un perdant doit recevoir 409 : %v", statuses)
			}
		}
		var count int
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM budget_versions WHERE project_id = $1 AND status = 'published'", project).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%d versions publiées", count)
		}
	})
}

// CF-014
func TestCF014_Revisions(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	v1 := tn.publishBudget(t, project, "100", 5000) // 500 000
	expense := tn.approvedExpense(t, project, 20000)

	expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+v1+"/revise", map[string]any{"reason": " "}), 422, "validation.failed")
	draft := tn.manager.must(201, "POST", "/api/v1/budget-versions/"+v1+"/revise", map[string]any{"reason": "Hausse du ciment"})
	v2 := draft.str("id")
	if draft.num("version_no") != 2 || draft.num("total_amount") != 500000 {
		t.Fatalf("la révision doit dupliquer le budget : %s", draft.body)
	}
	expectCode(t, tn.manager.do("POST", "/api/v1/budget-versions/"+v1+"/revise", map[string]any{"reason": "Autre"}), 409, "budget.draft_exists")

	lines, _ := draft.json()["lines"].([]any)
	copied, _ := lines[0].(map[string]any)
	tn.manager.must(200, "PATCH", "/api/v1/budget-lines/"+copied["id"].(string), map[string]any{"unit_price": 6000})
	tn.manager.must(201, "POST", "/api/v1/budget-versions/"+v2+"/lines", map[string]any{
		"category_id": tn.categoryID, "label": "Fer à béton", "quantity": "10", "unit": "barre", "unit_price": 1000,
	})

	// Tant que la révision n'est pas publiée, la référence ne bouge pas.
	before := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if before.num("budget") != 500000 {
		t.Fatalf("la référence reste la version publiée : %s", before.body)
	}
	tn.manager.must(200, "POST", "/api/v1/budget-versions/"+v2+"/publish", nil)

	cmp := tn.manager.must(200, "GET", "/api/v1/budget-versions/compare?from="+v1+"&to="+v2, nil)
	if cmp.num("from_total") != 500000 || cmp.num("to_total") != 610000 || cmp.num("delta") != 110000 {
		t.Fatalf("totaux : %s", cmp.body)
	}
	changes := map[string]string{}
	diffLines, _ := cmp.json()["lines"].([]any)
	for _, raw := range diffLines {
		d, _ := raw.(map[string]any)
		changes[d["label"].(string)] = d["change"].(string)
	}
	if changes["Ciment CPJ 45"] != "changed" || changes["Fer à béton"] != "added" {
		t.Fatalf("diff par ligne : %v", changes)
	}

	versions := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/budget/versions", nil).items()
	if len(versions) != 2 || versions[0]["status"] != "published" || versions[1]["status"] != "superseded" {
		t.Fatalf("ancienne version conservée attendue : %v", versions)
	}
	if versions[0]["revision_reason"] != "Hausse du ciment" {
		t.Fatalf("motif attendu : %v", versions[0])
	}
	old := tn.manager.must(200, "GET", "/api/v1/budget-versions/"+v1, nil)
	if old.num("total_amount") != 500000 {
		t.Fatalf("l'ancienne version a changé : %s", old.body)
	}

	// Les dépenses historiques ne sont pas réaffectées implicitement.
	e := tn.owner.must(200, "GET", "/api/v1/expenses/"+expense, nil)
	if e.str("budget_version_id") != v1 {
		t.Fatalf("la dépense doit rester rattachée à la v1 : %s", e.body)
	}
	after := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if after.num("budget") != 610000 || after.num("approved") != 20000 {
		t.Fatalf("la référence suit la nouvelle version, pas les dépenses : %s", after.body)
	}
	var superseded int
	if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM budget_revisions WHERE to_version_id = $1 AND reason = 'Hausse du ciment'", v2).Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if superseded != 1 {
		t.Fatal("la raison doit être enregistrée dans budget_revisions")
	}
}
