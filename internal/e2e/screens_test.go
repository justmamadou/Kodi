package e2e

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Données affichées par les maquettes : ces tests garantissent que les écrans montrent du réel.

func TestScreens_ProjectListCarriesFinancials(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	tn.publishBudget(t, project, "100", 1000)
	tn.approvedExpense(t, project, 58000)
	tn.manager.must(200, "POST", "/api/v1/projects/"+project+"/progress", map[string]any{"progress_pct": 45})

	item := tn.owner.must(200, "GET", "/api/v1/projects", nil).items()[0]
	if item["budget"].(float64) != 100000 || item["approved"].(float64) != 58000 {
		t.Fatalf("montants absents de la liste : %v", item)
	}
	if item["declared_progress_pct"].(float64) != 45 || item["manager_name"] == "" {
		t.Fatalf("avancement ou responsable absents : %v", item)
	}
	seen := tn.worker.must(200, "GET", "/api/v1/projects", nil).items()[0]
	if seen["budget"].(float64) != 0 || seen["approved"].(float64) != 0 {
		t.Fatalf("montants visibles par le chef de chantier : %v", seen)
	}
	if detail := tn.worker.must(200, "GET", "/api/v1/projects/"+project, nil); detail.str("manager_name") == "" || detail.str("id") != project {
		t.Fatalf("responsable absent du détail : %s", detail.body)
	}
	for _, m := range tn.owner.must(200, "GET", "/api/v1/members", nil).items() {
		if m["id"] == tn.managerID && len(m["project_names"].([]any)) != 1 {
			t.Fatalf("le chantier dirigé doit apparaître pour le conducteur : %v", m)
		}
	}
}

func TestScreens_ProgressAndSignal(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	tn.publishBudget(t, project, "100", 1000)
	tn.approvedExpense(t, project, 58000)
	path := "/api/v1/projects/" + project + "/progress"

	expectCode(t, tn.viewer.do("POST", path, map[string]any{"progress_pct": 10}), 403, "auth.forbidden")
	expectCode(t, tn.worker.do("POST", path, map[string]any{"progress_pct": 101}), 422, "validation.failed")
	if r := tn.worker.must(200, "POST", path, map[string]any{"progress_pct": 45}); r.num("declared_progress_pct") != 45 {
		t.Fatalf("avancement : %s", r.body)
	}
	fin := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil)
	if fin.str("signal") != "watch" {
		t.Fatalf("58 %% exposé pour 45 %% d'avancement : écart attendu : %s", fin.body)
	}
	tn.worker.must(200, "POST", path, map[string]any{"progress_pct": 60})
	if fin = tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/financials", nil); fin.str("signal") != "good" {
		t.Fatalf("signal : %s", fin.body)
	}
	expectCode(t, newTenant(t).owner.do("POST", path, map[string]any{"progress_pct": 1}), 404, "project.not_found")
}

func TestScreens_DashboardToProcessAndMonths(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	tn.publishBudget(t, project, "100", 10000)
	paid := tn.approvedExpense(t, project, 300000)
	tn.accountant.must(201, "POST", "/api/v1/expenses/"+paid+"/payments", map[string]any{"amount": 100000, "paid_on": today(), "method": "cash"})
	for range 2 { // deux saisies identiques : doublon possible
		id := tn.newExpense(t, tn.worker, project, 45000)
		tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
	}

	d := tn.owner.must(200, "GET", "/api/v1/dashboard?period=week", nil).json()
	if d["period"] != "week" || d["period_paid"] != float64(100000) {
		t.Fatalf("période : %v", d)
	}
	proc, _ := d["to_process"].(map[string]any)
	if proc["pending_count"] != float64(2) || proc["pending_amount"] != float64(90000) || proc["duplicate_count"] != float64(2) {
		t.Fatalf("à traiter : %v", proc)
	}
	missing, _ := proc["missing_reports"].([]any)
	if len(missing) != 1 {
		t.Fatalf("rapport manquant attendu : %v", proc)
	}
	months, _ := d["paid_by_month"].([]any)
	if len(months) != 5 {
		t.Fatalf("5 mois attendus, y compris sans paiement : %v", months)
	}
	last, _ := months[4].(map[string]any)
	if last["paid"] != float64(100000) {
		t.Fatalf("mois courant : %v", last)
	}
}

func TestScreens_ExpenseCountsAndDeclaredPayment(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)

	// Saisie terrain : fournisseur par son nom, paiement simplement déclaré.
	body := map[string]any{
		"category_id": tn.categoryID, "description": "Gravier 10 m3", "amount": 250000, "expense_date": "2026-11-03",
		"supplier_name": "Carrière de Bargny", "comment": "Livré le matin",
		"declared_paid_on": "2026-11-03", "declared_payment_method": "wave",
	}
	created := tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", body)
	id := created.str("id")
	if created.str("declared_payment_method") != "wave" || created.num("amount_paid") != 0 {
		t.Fatalf("une déclaration n'est pas un paiement : %s", created.body)
	}
	other := tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", body2(tn, "Carrière de BARGNY"))
	if created.str("supplier_id") == "" || created.str("supplier_id") != other.str("supplier_id") {
		t.Fatalf("le même nom doit donner le même fournisseur : %s / %s", created.body, other.body)
	}
	bad := map[string]any{"category_id": tn.categoryID, "description": "Sable", "amount": 10, "expense_date": "2026-11-03", "declared_payment_method": "troc"}
	expectCode(t, tn.worker.do("POST", "/api/v1/projects/"+project+"/expenses", bad), 422, "validation.failed")

	tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
	detail := tn.accountant.must(200, "GET", "/api/v1/expenses/"+id, nil)
	if detail.str("project_name") == "" || detail.str("created_by_name") == "" || detail.str("supplier_name") != "Carrière de Bargny" {
		t.Fatalf("libellés du panneau de validation : %s", detail.body)
	}

	paid := tn.approvedExpense(t, project, 1000)
	tn.accountant.must(201, "POST", "/api/v1/expenses/"+paid+"/payments", map[string]any{"amount": 1000, "paid_on": "2026-11-05", "method": "cash"})
	open := tn.approvedExpense(t, project, 2000)
	rejected := tn.newExpense(t, tn.worker, project, 3000)
	tn.worker.must(200, "POST", "/api/v1/expenses/"+rejected+"/submit", nil)
	tn.accountant.must(200, "POST", "/api/v1/expenses/"+rejected+"/reject", map[string]any{"comment": "Non"})

	counts := tn.accountant.must(200, "GET", "/api/v1/expenses/counts", nil)
	if counts.num("pending") != 1 || counts.num("pending_amount") != 250000 || counts.num("paid") != 1 ||
		counts.num("approved") != 1 || counts.num("rejected") != 1 || counts.num("draft") != 1 || counts.num("all") != 5 {
		t.Fatalf("compteurs : %s", counts.body)
	}
	paidList := tn.accountant.must(200, "GET", "/api/v1/expenses?payment=paid", nil).items()
	if len(paidList) != 1 || paidList[0]["id"] != paid {
		t.Fatalf("onglet Payées : %v", paidList)
	}
	openList := tn.accountant.must(200, "GET", "/api/v1/expenses?status=approved&payment=unpaid", nil).items()
	if len(openList) != 1 || openList[0]["id"] != open {
		t.Fatalf("onglet Approuvées : %v", openList)
	}
	// Un autre tenant ne compte rien.
	if n := newTenant(t).owner.must(200, "GET", "/api/v1/expenses/counts", nil).num("all"); n != 0 {
		t.Fatalf("fuite de compteurs : %d", n)
	}
}

func body2(tn *tenant, supplier string) map[string]any {
	return map[string]any{"category_id": tn.categoryID, "description": "Sable", "amount": 12345, "expense_date": "2026-11-03", "supplier_name": supplier}
}

func TestScreens_DuplicatesShownForDecision(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	a := tn.newExpense(t, tn.worker, project, 450000)
	b := tn.newExpense(t, tn.worker, project, 450000)
	single := tn.newExpense(t, tn.worker, project, 1)

	rows := tn.accountant.must(200, "GET", "/api/v1/expenses?project_id="+project, nil).items()
	flags := map[string]bool{}
	for _, r := range rows {
		flags[r["id"].(string)], _ = r["possible_duplicate"].(bool)
	}
	if !flags[a] || !flags[b] || flags[single] {
		t.Fatalf("doublons : %v", flags)
	}
	dup, _ := tn.accountant.must(200, "GET", "/api/v1/expenses/"+a, nil).json()["duplicates"].([]any)
	if len(dup) != 1 || dup[0].(map[string]any)["id"] != b {
		t.Fatalf("détail des doublons : %v", dup)
	}
}

func TestScreens_StockSummaryAndLowThreshold(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	low := tn.manager.must(201, "POST", "/api/v1/materials", map[string]any{"name": "Gravier " + unique(), "unit": "m3", "low_stock_threshold": "5"}).str("id")
	high := tn.material(t)
	expectCode(t, tn.manager.do("POST", "/api/v1/materials", map[string]any{"name": "Négatif", "unit": "m3", "low_stock_threshold": "-1"}), 422, "validation.failed")
	for _, m := range []struct{ material, kind, quantity string }{{low, "in", "60"}, {low, "out", "57"}, {high, "in", "10"}} {
		if r := tn.move(t, tn.manager, project, m.material, m.kind, m.quantity); r.status != 200 {
			t.Fatal(string(r.body))
		}
	}
	view := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/stock", nil)
	if view.num("low_count") != 1 {
		t.Fatalf("stock bas : %s", view.body)
	}
	for _, row := range view.items() {
		if row["material_id"] == low {
			if row["qty_in"] != "60" || row["qty_out"] != "57" || row["on_hand"] != "3" || row["is_low"] != true || row["last_movement_type"] != "out" {
				t.Fatalf("ligne de stock : %v", row)
			}
		}
	}
	// Le mouvement de test porte une date fixe : le compteur du mois dépend de la date du jour.
	if view.json()["movements_this_month"] == nil {
		t.Fatalf("compteur du mois absent : %s", view.body)
	}
}

func TestScreens_InvitationAppliesProjects(t *testing.T) {
	tn := newTenant(t)
	project := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "INV-" + unique(), "name": "Avec invités", "client_name": "Client", "address": "Dakar", "start_date": "2026-01-01",
	}).str("id")
	expectCode(t, tn.owner.do("POST", "/api/v1/invitations", map[string]any{
		"full_name": "Refus", "email": "inv-" + unique() + "@example.test", "role": "site_worker", "project_ids": []string{uuid.NewString()},
	}), 422, "validation.failed")

	r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{
		"full_name": "Pape Seck", "email": "pape-" + unique() + "@example.test", "role": "site_worker", "project_ids": []string{project},
	})
	// Tant que l'invitation n'est pas acceptée, la personne n'est pas affectée.
	var members int
	if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM project_members WHERE user_id = $1", r.str("user_id")).Scan(&members); err != nil {
		t.Fatal(err)
	}
	if members != 0 {
		t.Fatal("un compte invité ne doit pas encore être affecté")
	}
	c := newClient(t)
	c.must(200, "POST", "/api/v1/auth/invitations/"+tokenFrom(t, r.str("invite_url"))+"/accept", map[string]any{"password": testPassword})
	c.must(200, "GET", "/api/v1/projects/"+project, nil)

	list := tn.owner.must(200, "GET", "/api/v1/members", nil)
	if bytes.Contains(list.body, []byte("password_hash")) {
		t.Fatal("secret exposé")
	}
	found := false
	for _, m := range list.items() {
		if m["id"] == r.str("user_id") {
			names, _ := m["project_names"].([]any)
			found = len(names) == 1 && names[0] == "Avec invités"
		}
	}
	if !found {
		t.Fatalf("chantiers du membre : %s", list.body)
	}
}

func TestScreens_DirectoryIsMinimal(t *testing.T) {
	tn := newTenant(t)
	r := tn.manager.must(200, "GET", "/api/v1/directory", nil)
	if len(r.items()) != 5 {
		t.Fatalf("5 membres actifs attendus : %s", r.body)
	}
	if bytes.Contains(r.body, []byte("@example.test")) {
		t.Fatal("l'annuaire ne doit pas exposer les coordonnées")
	}
	expectCode(t, tn.worker.do("GET", "/api/v1/directory", nil), 403, "auth.forbidden")
	expectCode(t, tn.worker.do("GET", "/api/v1/members", nil), 403, "auth.forbidden")
}

func TestScreens_DocumentsAndHistory(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	id := uuid.NewString()
	pre := tn.worker.must(201, "POST", "/api/v1/attachments/presign", map[string]any{
		"id": id, "project_id": project, "kind": "site_photo", "mime_type": "image/jpeg", "size_bytes": 4, "original_name": "dalle.jpg",
	})
	att, _ := pre.json()["attachment"].(map[string]any)
	if n := len(tn.viewer.must(200, "GET", "/api/v1/projects/"+project+"/attachments", nil).items()); n != 0 {
		t.Fatalf("un dépôt non confirmé ne doit pas apparaître : %d", n)
	}
	env.store.put(att["storage_key"].(string), []byte{0xFF, 0xD8, 0xFF, 0xE0})
	tn.worker.must(200, "POST", "/api/v1/attachments/"+id+"/complete", nil)
	docs := tn.viewer.must(200, "GET", "/api/v1/projects/"+project+"/attachments", nil).items()
	if len(docs) != 1 || docs[0]["original_name"] != "dalle.jpg" || docs[0]["uploaded_by_name"] == "" {
		t.Fatalf("documents : %v", docs)
	}

	tn.publishBudget(t, project, "1", 100)
	history := tn.manager.must(200, "GET", "/api/v1/projects/"+project+"/history", nil).items()
	actions := map[string]bool{}
	for _, h := range history {
		actions[h["action"].(string)] = true
	}
	if !actions["budget.published"] || !actions["project.created"] {
		t.Fatalf("historique : %v", actions)
	}
	expectCode(t, tn.worker.do("GET", "/api/v1/projects/"+project+"/history", nil), 403, "auth.forbidden")
	other := newTenant(t)
	expectCode(t, other.owner.do("GET", "/api/v1/projects/"+project+"/history", nil), 404, "project.not_found")
	expectCode(t, other.owner.do("GET", "/api/v1/projects/"+project+"/attachments", nil), 404, "project.not_found")
}

func TestScreens_ReportsOverview(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	tn.publishBudget(t, project, "100", 1000)
	e := tn.approvedExpense(t, project, 5000)
	tn.accountant.must(201, "POST", "/api/v1/expenses/"+e+"/payments", map[string]any{"amount": 2000, "paid_on": "2026-11-05", "method": "cash"})
	// Le lundi de la semaine en cours : la semaine suivie va du lundi au samedi, un rapport du dimanche n'y entre pas.
	now := time.Now().UTC()
	monday := now.AddDate(0, 0, -((int(now.Weekday()) + 6) % 7)).Format(time.DateOnly)
	report := uuid.NewString()
	tn.worker.must(200, "PUT", "/api/v1/daily-reports/"+report, map[string]any{"project_id": project, "report_date": monday, "work_done": "Coffrage", "workforce_count": 8})
	tn.worker.must(200, "POST", "/api/v1/daily-reports/"+report+"/submit", nil)

	o := tn.owner.must(200, "GET", "/api/v1/reports/overview", nil).json()
	cats, _ := o["paid_by_category"].([]any)
	if len(cats) != 1 || cats[0].(map[string]any)["paid"] != float64(2000) {
		t.Fatalf("catégories : %v", cats)
	}
	days, _ := o["week_days"].([]any)
	if len(days) != 6 {
		t.Fatalf("lundi à samedi attendus : %v", days)
	}
	week, _ := o["week_projects"].([]any)
	reported, _ := week[0].(map[string]any)["reported"].([]any)
	if len(reported) != 1 || reported[0] != monday {
		t.Fatalf("rapports de la semaine : %v", week)
	}
	latest, _ := o["latest_reports"].([]any)
	if len(latest) != 1 || latest[0].(map[string]any)["author_name"] == "" {
		t.Fatalf("derniers rapports : %v", latest)
	}
	expectCode(t, tn.worker.do("GET", "/api/v1/reports/overview", nil), 403, "auth.forbidden")
	if p, _ := newTenant(t).owner.must(200, "GET", "/api/v1/reports/overview", nil).json()["projects"].([]any); len(p) != 0 {
		t.Fatalf("fuite inter-tenant : %v", p)
	}
	pdf := tn.accountant.must(200, "GET", "/api/v1/exports/portfolio.pdf", nil)
	if !bytes.HasPrefix(pdf.body, []byte("%PDF-")) {
		t.Fatal("PDF invalide")
	}
	expectCode(t, tn.manager.do("GET", "/api/v1/exports/portfolio.pdf", nil), 403, "auth.forbidden")
}

func TestScreens_SignupCompanyDetails(t *testing.T) {
	id := unique()
	c := newClient(t)
	c.must(201, "POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": "Diop Bâtiment " + id, "full_name": "Moussa Diop", "email": "m-" + id + "@example.test", "phone": fmt.Sprintf("+22177%07d", time.Now().UnixNano()%10000000),
		"password": testPassword, "company_phone": "+221338204510", "activity_type": "villas", "city": "Dakar", "address": "Sacré-Cœur 3",
	})
	org := c.must(200, "GET", "/api/v1/organization", nil)
	if org.str("activity_type") != "villas" || org.str("city") != "Dakar" || org.str("phone") != "+221338204510" || org.str("address") != "Sacré-Cœur 3" {
		t.Fatalf("coordonnées de l'entreprise : %s", org.body)
	}
}

func today() string {
	var d string
	if err := env.ownerDB.QueryRow(context.Background(), "SELECT to_char(now() AT TIME ZONE 'UTC', 'YYYY-MM-DD')").Scan(&d); err != nil {
		panic(err)
	}
	return d
}
