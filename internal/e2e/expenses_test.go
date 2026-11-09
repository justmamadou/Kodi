package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// CF-015
func TestCF015_CreateExpense(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	path := "/api/v1/projects/" + project + "/expenses"
	valid := map[string]any{"category_id": tn.categoryID, "description": "Ciment 50 sacs", "amount": 450000, "expense_date": "2026-11-03"}

	created := tn.accountant.must(201, "POST", path, valid)
	if created.str("status") != "draft" || created.str("currency") != "XOF" {
		t.Fatalf("Draft en XOF attendu : %s", created.body)
	}

	for name, body := range map[string]map[string]any{
		"montant nul":     {"category_id": tn.categoryID, "description": "Ciment", "amount": 0, "expense_date": "2026-11-03"},
		"montant négatif": {"category_id": tn.categoryID, "description": "Ciment", "amount": -5, "expense_date": "2026-11-03"},
		"sans catégorie":  {"description": "Ciment", "amount": 10, "expense_date": "2026-11-03"},
		"sans date":       {"category_id": tn.categoryID, "description": "Ciment", "amount": 10},
		"sans libellé":    {"category_id": tn.categoryID, "description": " ", "amount": 10, "expense_date": "2026-11-03"},
		"date future":     {"category_id": tn.categoryID, "description": "Ciment", "amount": 10, "expense_date": "2099-01-01"},
	} {
		t.Run(name, func(t *testing.T) {
			expectCode(t, tn.accountant.do("POST", path, body), 422, "validation.failed")
		})
	}

	t.Run("montant décimal refusé", func(t *testing.T) {
		bad := map[string]any{"category_id": tn.categoryID, "description": "Ciment", "amount": 10.5, "expense_date": "2026-11-03"}
		expectCode(t, tn.accountant.do("POST", path, bad), 400, "request.invalid_json")
	})

	t.Run("catégorie d'un autre tenant ou désactivée", func(t *testing.T) {
		other := newTenant(t)
		bad := map[string]any{"category_id": other.categoryID, "description": "Ciment", "amount": 10, "expense_date": "2026-11-03"}
		expectCode(t, tn.accountant.do("POST", path, bad), 422, "validation.failed")

		cats := tn.owner.must(200, "GET", "/api/v1/categories", nil).items()
		disabled, _ := cats[1]["id"].(string)
		tn.owner.must(200, "PATCH", "/api/v1/categories/"+disabled, map[string]any{"is_active": false})
		bad["category_id"] = disabled
		expectCode(t, tn.accountant.do("POST", path, bad), 422, "validation.failed")
	})

	t.Run("fournisseur d'un autre tenant refusé par la FK composite", func(t *testing.T) {
		other := newTenant(t)
		supplier := other.owner.must(201, "POST", "/api/v1/suppliers", map[string]any{"name": "Quincaillerie Ndiaye"}).str("id")
		bad := map[string]any{"category_id": tn.categoryID, "description": "Ciment", "amount": 10, "expense_date": "2026-11-03", "supplier_id": supplier}
		expectCode(t, tn.accountant.do("POST", path, bad), 422, "resource.invalid_reference")
	})

	t.Run("renvoi hors connexion du même id : pas de doublon", func(t *testing.T) {
		id := uuid.NewString()
		body := map[string]any{"id": id, "category_id": tn.categoryID, "description": "Offline", "amount": 77, "expense_date": "2026-11-03"}
		tn.worker.must(201, "POST", path, body)
		replay := tn.worker.must(200, "POST", path, body)
		if replay.str("id") != id {
			t.Fatalf("même dépense attendue : %s", replay.body)
		}
		var count int
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM expenses WHERE id = $1", id).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%d dépenses créées", count)
		}
	})

	t.Run("justificatif associable", func(t *testing.T) {
		expense := created.str("id")
		id := uuid.NewString()
		pre := tn.accountant.must(201, "POST", "/api/v1/attachments/presign", map[string]any{
			"id": id, "expense_id": expense, "kind": "receipt", "mime_type": "image/jpeg", "size_bytes": 4, "original_name": "recu.jpg",
		})
		att, _ := pre.json()["attachment"].(map[string]any)
		env.store.put(att["storage_key"].(string), []byte{0xFF, 0xD8, 0xFF, 0xE0})
		tn.accountant.must(200, "POST", "/api/v1/attachments/"+id+"/complete", nil)
		detail := tn.accountant.must(200, "GET", "/api/v1/expenses/"+expense, nil)
		if detail.json()["has_receipt"] != true {
			t.Fatalf("has_receipt attendu : %s", detail.body)
		}
		files, _ := detail.json()["attachments"].([]any)
		if len(files) != 1 {
			t.Fatalf("1 justificatif attendu : %s", detail.body)
		}
	})

	t.Run("seul l'auteur modifie son brouillon", func(t *testing.T) {
		expense := created.str("id")
		expectCode(t, tn.worker.do("PATCH", "/api/v1/expenses/"+expense, map[string]any{"amount": 5}, "If-Match", "1"), 403, "auth.forbidden")
		updated := tn.accountant.must(200, "PATCH", "/api/v1/expenses/"+expense, map[string]any{"amount": 460000}, "If-Match", "1")
		if updated.num("amount") != 460000 {
			t.Fatalf("montant : %s", updated.body)
		}
		expectCode(t, tn.accountant.do("PATCH", "/api/v1/expenses/"+expense, map[string]any{"amount": 5}, "If-Match", "1"), 409, "resource.stale")
	})
}

// CF-016
func TestCF016_SubmitExpense(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	id := tn.newExpense(t, tn.worker, project, 1000)

	expectCode(t, tn.manager.do("POST", "/api/v1/expenses/"+id+"/submit", nil), 403, "auth.forbidden")
	submitted := tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
	if submitted.str("status") != "pending" || submitted.str("submitted_at") == "" {
		t.Fatalf("Pending attendu : %s", submitted.body)
	}
	events := func() []any {
		raw, _ := tn.worker.must(200, "GET", "/api/v1/expenses/"+id, nil).json()["approvals"].([]any)
		return raw
	}
	if len(events()) != 1 {
		t.Fatalf("un événement attendu : %v", events())
	}

	t.Run("seule une Draft est soumise, sans second événement", func(t *testing.T) {
		expectCode(t, tn.worker.do("POST", "/api/v1/expenses/"+id+"/submit", nil), 409, "expense.not_draft")
		if len(events()) != 1 {
			t.Fatalf("le refus ne doit pas créer d'événement : %v", events())
		}
		// Le montant est verrouillé.
		expectCode(t, tn.worker.do("PATCH", "/api/v1/expenses/"+id, map[string]any{"amount": 1}, "If-Match", "2"), 409, "expense.not_draft")
	})

	t.Run("double soumission avec clé d'idempotence : même réponse", func(t *testing.T) {
		other := tn.newExpense(t, tn.worker, project, 2000)
		key := uuid.NewString()
		first := tn.worker.must(200, "POST", "/api/v1/expenses/"+other+"/submit", nil, "Idempotency-Key", key)
		replay := tn.worker.must(200, "POST", "/api/v1/expenses/"+other+"/submit", nil, "Idempotency-Key", key)
		if replay.header.Get("Idempotency-Replayed") != "true" || string(first.body) != string(replay.body) {
			t.Fatalf("réponse mémorisée attendue :\n%s\n%s", first.body, replay.body)
		}
		raw, _ := tn.worker.must(200, "GET", "/api/v1/expenses/"+other, nil).json()["approvals"].([]any)
		if len(raw) != 1 {
			t.Fatalf("un seul événement attendu, %d", len(raw))
		}
		// Réutiliser la clé pour une autre requête est refusé.
		third := tn.newExpense(t, tn.worker, project, 3000)
		expectCode(t, tn.worker.do("POST", "/api/v1/expenses/"+third+"/submit", nil, "Idempotency-Key", key), 422, "idempotency.key_reused")
	})

	t.Run("soumissions concurrentes : un seul événement", func(t *testing.T) {
		race := tn.newExpense(t, tn.worker, project, 4000)
		ok, statuses := concurrently(8, func(int) response {
			return tn.worker.do("POST", "/api/v1/expenses/"+race+"/submit", nil)
		})
		if ok != 1 {
			t.Fatalf("une seule soumission attendue, %d : %v", ok, statuses)
		}
		var count int
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM expense_approvals WHERE expense_id = $1 AND event = 'submitted'", race).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("%d événements", count)
		}
	})

	t.Run("une erreur annule tout : statut et événement vont ensemble", func(t *testing.T) {
		var orphans int
		if err := env.ownerDB.QueryRow(context.Background(), `
			SELECT count(*) FROM expenses e
			 WHERE e.status = 'pending'
			   AND NOT EXISTS (SELECT 1 FROM expense_approvals a WHERE a.expense_id = e.id AND a.to_status = 'pending')`).Scan(&orphans); err != nil {
			t.Fatal(err)
		}
		if orphans != 0 {
			t.Fatalf("%d dépenses Pending sans événement", orphans)
		}
	})
}

// CF-017
func TestCF017_ApproveReject(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)

	pending := func(amount int64) string {
		id := tn.newExpense(t, tn.worker, project, amount)
		tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
		return id
	}

	t.Run("approbation historisée", func(t *testing.T) {
		id := pending(1000)
		r := tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/approve", map[string]any{"comment": "OK"})
		if r.str("status") != "approved" || r.str("decided_by") != tn.accountID || r.str("decided_at") == "" {
			t.Fatalf("décision : %s", r.body)
		}
		raw, _ := tn.owner.must(200, "GET", "/api/v1/expenses/"+id, nil).json()["approvals"].([]any)
		last, _ := raw[len(raw)-1].(map[string]any)
		if last["event"] != "approved" || last["actor_id"] != tn.accountID || last["created_at"] == nil {
			t.Fatalf("acteur/date/décision absents : %v", last)
		}
		// Seule Pending est décidée.
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 409, "expense.not_pending")
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/reject", map[string]any{"comment": "Non"}), 409, "expense.not_pending")
	})

	t.Run("une Draft ne se décide pas", func(t *testing.T) {
		id := tn.newExpense(t, tn.worker, project, 1000)
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 409, "expense.not_pending")
	})

	t.Run("le créateur ne peut pas approuver", func(t *testing.T) {
		id := tn.newExpense(t, tn.accountant, project, 1000)
		tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 403, "expense.self_approval")
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/reject", map[string]any{"comment": "Non"}), 403, "expense.self_approval")
		// Y compris pour l'Owner.
		mine := tn.newExpense(t, tn.owner, project, 1000)
		tn.owner.must(200, "POST", "/api/v1/expenses/"+mine+"/submit", nil)
		expectCode(t, tn.owner.do("POST", "/api/v1/expenses/"+mine+"/approve", nil), 403, "expense.self_approval")
		// Et la base le garantit aussi.
		_, err := env.ownerDB.Exec(context.Background(), "UPDATE expenses SET status = 'approved', decided_by = created_by, decided_at = now() WHERE id = $1", mine)
		if err == nil || !strings.Contains(err.Error(), "expenses_no_self_approval_chk") {
			t.Fatalf("la contrainte aurait dû refuser : %v", err)
		}
	})

	t.Run("rejet motivé", func(t *testing.T) {
		id := pending(1000)
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/reject", nil), 422, "validation.failed")
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/reject", map[string]any{"comment": "   "}), 422, "validation.failed")
		r := tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/reject", map[string]any{"comment": "Doublon de la saisie de 14:31"})
		if r.str("status") != "rejected" || r.str("decision_comment") != "Doublon de la saisie de 14:31" {
			t.Fatalf("rejet : %s", r.body)
		}
		// L'auteur corrige et resoumet.
		back := tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/back-to-draft", nil)
		if back.str("status") != "draft" {
			t.Fatalf("retour en brouillon : %s", back.body)
		}
		tn.worker.must(200, "POST", "/api/v1/expenses/"+id+"/submit", nil)
	})

	t.Run("droits d'approbation", func(t *testing.T) {
		id := pending(5000)
		expectCode(t, tn.worker.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 403, "auth.forbidden")
		expectCode(t, tn.viewer.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 403, "auth.forbidden")
		// Manager : uniquement délégué, et dans la limite de son plafond.
		expectCode(t, tn.manager.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 403, "auth.forbidden")
		tn.owner.must(200, "PUT", "/api/v1/projects/"+project+"/members/"+tn.managerID, map[string]any{"can_approve": true, "approval_limit": 1000})
		expectCode(t, tn.manager.do("POST", "/api/v1/expenses/"+id+"/approve", nil), 403, "expense.above_approval_limit")
		tn.owner.must(200, "PUT", "/api/v1/projects/"+project+"/members/"+tn.managerID, map[string]any{"can_approve": true, "approval_limit": 5000})
		tn.manager.must(200, "POST", "/api/v1/expenses/"+id+"/approve", nil)
	})

	t.Run("deux approbateurs simultanés : une seule décision", func(t *testing.T) {
		id := pending(1000)
		ok, statuses := concurrently(8, func(i int) response {
			if i%2 == 0 {
				return tn.accountant.do("POST", "/api/v1/expenses/"+id+"/approve", nil)
			}
			return tn.owner.do("POST", "/api/v1/expenses/"+id+"/reject", map[string]any{"comment": "Non"})
		})
		if ok != 1 {
			t.Fatalf("une seule décision attendue, %d : %v", ok, statuses)
		}
		var events int
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM expense_approvals WHERE expense_id = $1 AND event IN ('approved','rejected')", id).Scan(&events); err != nil {
			t.Fatal(err)
		}
		if events != 1 {
			t.Fatalf("%d événements de décision", events)
		}
	})

	t.Run("file de validation", func(t *testing.T) {
		tn2 := newTenant(t)
		p2 := tn2.newProject(t)
		a := tn2.newExpense(t, tn2.worker, p2, 1000)
		tn2.newExpense(t, tn2.worker, p2, 2000)
		tn2.worker.must(200, "POST", "/api/v1/expenses/"+a+"/submit", nil)
		inbox := tn2.accountant.must(200, "GET", "/api/v1/expenses?status=pending", nil).items()
		if len(inbox) != 1 || inbox[0]["id"] != a || inbox[0]["project_name"] == nil || inbox[0]["created_by_name"] == nil {
			t.Fatalf("inbox : %v", inbox)
		}
	})

	t.Run("machine à états imposée en base", func(t *testing.T) {
		id := pending(1000)
		tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/approve", nil)
		_, err := env.ownerDB.Exec(context.Background(), "UPDATE expenses SET status = 'pending', decided_at = NULL, decided_by = NULL WHERE id = $1", id)
		if err == nil || !strings.Contains(err.Error(), "invalid expense transition") {
			t.Fatalf("transition approved -> pending : %v", err)
		}
		_, err = env.ownerDB.Exec(context.Background(), "UPDATE expenses SET amount = 1 WHERE id = $1", id)
		if err == nil || !strings.Contains(err.Error(), "locked") {
			t.Fatalf("le montant d'une dépense décidée est verrouillé : %v", err)
		}
		_, err = env.ownerDB.Exec(context.Background(), "DELETE FROM expenses WHERE id = $1", id)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("une dépense ne se supprime pas : %v", err)
		}
	})
}

// CF-018
func TestCF018_Payments(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	pay := func(c *client, id string, amount int64) response {
		return c.do("POST", "/api/v1/expenses/"+id+"/payments", map[string]any{"amount": amount, "paid_on": "2026-11-05", "method": "wave", "reference": "TX-1"})
	}

	t.Run("uniquement sur dépense approuvée", func(t *testing.T) {
		draft := tn.newExpense(t, tn.worker, project, 1000)
		expectCode(t, pay(tn.accountant, draft, 100), 409, "expense.not_approved")
		tn.worker.must(200, "POST", "/api/v1/expenses/"+draft+"/submit", nil)
		expectCode(t, pay(tn.accountant, draft, 100), 409, "expense.not_approved")
		tn.accountant.must(200, "POST", "/api/v1/expenses/"+draft+"/reject", map[string]any{"comment": "Non"})
		expectCode(t, pay(tn.accountant, draft, 100), 409, "expense.not_approved")
	})

	t.Run("droits", func(t *testing.T) {
		id := tn.approvedExpense(t, project, 1000)
		expectCode(t, pay(tn.manager, id, 100), 403, "auth.forbidden")
		expectCode(t, pay(tn.worker, id, 100), 403, "auth.forbidden")
	})

	t.Run("paiements partiels, solde exact, cumul plafonné", func(t *testing.T) {
		id := tn.approvedExpense(t, project, 1000)
		r := pay(tn.accountant, id, 300)
		if r.status != 201 || r.num("remaining") != 700 {
			t.Fatalf("solde 700 attendu : %s", r.body)
		}
		exp, _ := r.json()["expense"].(map[string]any)
		if exp["payment_status"] != "partially_paid" || exp["amount_paid"].(float64) != 300 {
			t.Fatalf("statut de paiement : %v", exp)
		}
		expectCode(t, pay(tn.accountant, id, 701), 409, "payment.exceeds_remaining")
		expectCode(t, pay(tn.accountant, id, 0), 422, "validation.failed")
		r = pay(tn.accountant, id, 700)
		if r.status != 201 || r.num("remaining") != 0 {
			t.Fatalf("solde 0 attendu : %s", r.body)
		}
		exp, _ = r.json()["expense"].(map[string]any)
		if exp["payment_status"] != "paid" {
			t.Fatalf("statut : %v", exp)
		}
		expectCode(t, pay(tn.accountant, id, 1), 409, "payment.exceeds_remaining")

		// Historique complet et immuable.
		history, _ := tn.accountant.must(200, "GET", "/api/v1/expenses/"+id, nil).json()["payments"].([]any)
		if len(history) != 2 {
			t.Fatalf("2 paiements historisés attendus, %d", len(history))
		}
		_, err := env.ownerDB.Exec(context.Background(), "UPDATE payments SET amount = 1 WHERE expense_id = $1", id)
		if err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Fatalf("un paiement est immuable : %v", err)
		}
		// Une dépense payée ne s'annule pas.
		expectCode(t, tn.accountant.do("POST", "/api/v1/expenses/"+id+"/cancel", nil), 409, "expense.has_payments")
	})

	t.Run("concurrence : le cumul ne dépasse jamais le montant", func(t *testing.T) {
		id := tn.approvedExpense(t, project, 1000)
		ok, statuses := concurrently(20, func(int) response { return pay(tn.accountant, id, 100) })
		if ok != 10 {
			t.Fatalf("exactement 10 paiements de 100 attendus sur 1000, %d : %v", ok, statuses)
		}
		for _, s := range statuses {
			if s != 201 && s != 409 {
				t.Fatalf("statut inattendu en concurrence : %v", statuses)
			}
		}
		var paid, sum int64
		if err := env.ownerDB.QueryRow(context.Background(),
			"SELECT e.amount_paid, COALESCE((SELECT SUM(amount) FROM payments p WHERE p.expense_id = e.id), 0) FROM expenses e WHERE e.id = $1", id).Scan(&paid, &sum); err != nil {
			t.Fatal(err)
		}
		if paid != 1000 || sum != 1000 {
			t.Fatalf("cumul : amount_paid=%d, somme des paiements=%d", paid, sum)
		}
	})

	t.Run("annulation d'une approuvée non payée", func(t *testing.T) {
		id := tn.approvedExpense(t, project, 1000)
		expectCode(t, tn.worker.do("POST", "/api/v1/expenses/"+id+"/cancel", nil), 403, "auth.forbidden")
		r := tn.accountant.must(200, "POST", "/api/v1/expenses/"+id+"/cancel", map[string]any{"comment": "Erreur"})
		if r.str("status") != "cancelled" {
			t.Fatalf("annulation : %s", r.body)
		}
	})
}
