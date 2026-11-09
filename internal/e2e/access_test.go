package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// CF-005 : les permissions sont vérifiées côté serveur, les refus retournent 403.
func TestCF005_ServerSidePermissions(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)

	body := map[string]any{"reference": "X-" + unique(), "name": "Refus", "client_name": "Client", "address": "Dakar", "start_date": "2026-01-01"}
	forbidden := []struct {
		name   string
		c      *client
		method string
		path   string
		body   any
	}{
		{"chef de chantier ne crée pas de chantier", tn.worker, "POST", "/api/v1/projects", body},
		{"comptable ne crée pas de chantier", tn.accountant, "POST", "/api/v1/projects", body},
		{"lecteur ne crée pas de dépense", tn.viewer, "POST", "/api/v1/projects/" + project + "/expenses", map[string]any{"category_id": tn.categoryID, "description": "non", "amount": 1, "expense_date": "2026-11-03"}},
		{"comptable ne crée pas de budget", tn.accountant, "POST", "/api/v1/projects/" + project + "/budget/versions", nil},
		{"chef de chantier ne voit pas le dashboard", tn.worker, "GET", "/api/v1/dashboard", nil},
		{"chef de chantier ne voit pas les membres", tn.worker, "GET", "/api/v1/members", nil},
		{"manager ne modifie pas l'entreprise", tn.manager, "PATCH", "/api/v1/organization", map[string]any{"name": "Pirate"}},
		{"manager ne lit pas l'audit", tn.manager, "GET", "/api/v1/audit-logs", nil},
		{"manager n'invite pas", tn.manager, "POST", "/api/v1/invitations", map[string]any{"full_name": "Non", "email": "x@example.test", "role": "owner"}},
		{"manager n'exporte pas", tn.manager, "GET", "/api/v1/exports/projects/" + project + "/report.pdf", nil},
		{"lecteur ne pose pas de stock", tn.viewer, "POST", "/api/v1/projects/" + project + "/stock-movements", map[string]any{"movement_type": "in", "occurred_on": "2026-11-03", "items": []any{}}},
		{"chef de chantier ne gère pas le catalogue", tn.worker, "POST", "/api/v1/materials", map[string]any{"name": "Ciment", "unit": "sac"}},
		{"chef de chantier ne crée pas de tâche", tn.worker, "POST", "/api/v1/projects/" + project + "/tasks", map[string]any{"title": "Non"}},
		{"manager ne délègue pas d'approbation", tn.manager, "PUT", "/api/v1/projects/" + project + "/members/" + tn.workerID, map[string]any{"can_approve": true}},
	}
	for _, f := range forbidden {
		t.Run(f.name, func(t *testing.T) {
			expectCode(t, f.c.do(f.method, f.path, f.body), 403, "auth.forbidden")
		})
	}

	t.Run("hors affectation : 403", func(t *testing.T) {
		other := tn.owner.must(201, "POST", "/api/v1/projects", map[string]any{
			"reference": "Y-" + unique(), "name": "Autre chantier", "client_name": "Client", "address": "Dakar", "start_date": "2026-01-01",
		}).str("id")
		expectCode(t, tn.worker.do("GET", "/api/v1/projects/"+other, nil), 403, "auth.forbidden")
		expectCode(t, tn.manager.do("GET", "/api/v1/projects/"+other, nil), 403, "auth.forbidden")
		tn.accountant.must(200, "GET", "/api/v1/projects/"+other, nil) // Finance voit tout
		if items := tn.worker.must(200, "GET", "/api/v1/projects", nil).items(); len(items) != 1 {
			t.Fatalf("le chef de chantier doit voir son seul chantier, %d vus", len(items))
		}
	})
}

// CF-005 : le journal est append-only, y compris pour le rôle applicatif et en SQL direct.
func TestCF005_AuditTrailIsAppendOnly(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	expense := tn.approvedExpense(t, project, 5000)

	logs := tn.owner.must(200, "GET", "/api/v1/audit-logs?entity_id="+expense, nil).items()
	actions := map[string]bool{}
	for _, l := range logs {
		action, _ := l["action"].(string)
		actions[action] = true
	}
	for _, want := range []string{"expense.created", "expense.submitted", "expense.approved"} {
		if !actions[want] {
			t.Errorf("action %s absente du journal : %v", want, actions)
		}
	}

	ctx := context.Background()
	for _, stmt := range []string{"UPDATE audit_logs SET action = 'x'", "DELETE FROM audit_logs"} {
		// Même le rôle propriétaire est bloqué par le trigger.
		if _, err := env.ownerDB.Exec(ctx, stmt); err == nil || !strings.Contains(err.Error(), "append-only") {
			t.Errorf("%s : refus attendu, obtenu %v", stmt, err)
		}
		// Le rôle applicatif n'en a même pas le droit.
		tx, err := env.appPool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, stmt); err == nil || !strings.Contains(err.Error(), "42501") {
			t.Errorf("cf_app, %s : refus de privilège attendu, obtenu %v", stmt, err)
		}
		_ = tx.Rollback(ctx)
	}
}

// CF-006
func TestCF006_Organization(t *testing.T) {
	tn := newTenant(t)
	org := tn.owner.must(200, "GET", "/api/v1/organization", nil)
	if org.str("currency") != "XOF" || org.str("timezone") != "Africa/Dakar" {
		t.Fatalf("XOF et Africa/Dakar attendus par défaut : %s", org.body)
	}
	if org.str("slug") == "" {
		t.Fatal("un slug doit être dérivé du nom")
	}
	updated := tn.owner.must(200, "PATCH", "/api/v1/organization", map[string]any{"city": "Dakar"})
	if updated.str("city") != "Dakar" {
		t.Fatalf("mise à jour non appliquée : %s", updated.body)
	}

	other := newTenant(t)
	expectCode(t, other.owner.do("PATCH", "/api/v1/organization", map[string]any{"slug": org.str("slug")}), 409, "organization.duplicate")
	expectCode(t, other.owner.do("PATCH", "/api/v1/organization", map[string]any{"name": strings.ToUpper(tn.name)}), 409, "organization.duplicate")
	expectCode(t, other.owner.do("PATCH", "/api/v1/organization", map[string]any{"slug": "Slug Invalide"}), 422, "validation.failed")
	expectCode(t, newClient(t).do("POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": tn.name, "full_name": "Copie", "email": "copie-" + unique() + "@example.test", "password": testPassword,
	}), 409, "signup.conflict")

	settings := tn.owner.must(200, "PATCH", "/api/v1/organization/settings", map[string]any{"budget_alert_thresholds": []int{90, 70}})
	thresholds, _ := settings.json()["budget_alert_thresholds"].([]any)
	if len(thresholds) != 2 || thresholds[0].(float64) != 70 {
		t.Fatalf("seuils triés attendus : %s", settings.body)
	}
	expectCode(t, tn.owner.do("PATCH", "/api/v1/organization/settings", map[string]any{"budget_alert_thresholds": []int{}}), 422, "validation.failed")
	expectCode(t, tn.accountant.do("PATCH", "/api/v1/organization/settings", map[string]any{"budget_alert_thresholds": []int{80}}), 403, "auth.forbidden")
}

// CF-007
func TestCF007_Invitations(t *testing.T) {
	tn := newTenant(t)
	email := "invite-" + unique() + "@example.test"
	invite := func() response {
		return tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "Cheikh Mbaye", "email": email, "role": "site_worker"})
	}

	t.Run("le jeton n'est stocké que haché", func(t *testing.T) {
		token := tokenFrom(t, invite().str("invite_url"))
		var count int
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM invitations WHERE token_hash = convert_to($1, 'UTF8')", token).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("jeton brut en base")
		}
	})

	t.Run("renvoyer remplace le lien précédent", func(t *testing.T) {
		old := tokenFrom(t, invite().str("invite_url"))
		fresh := tokenFrom(t, invite().str("invite_url"))
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/invitations/"+old+"/accept", map[string]any{"password": testPassword}), 410, "invitation.invalid")
		c := newClient(t)
		c.must(200, "POST", "/api/v1/auth/invitations/"+fresh+"/accept", map[string]any{"password": testPassword})
		c.must(200, "GET", "/api/v1/auth/me", nil)

		// Usage unique.
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/invitations/"+fresh+"/accept", map[string]any{"password": testPassword}), 410, "invitation.invalid")
		// Déjà membre : on ne peut plus l'inviter.
		expectCode(t, tn.owner.do("POST", "/api/v1/invitations", map[string]any{"full_name": "Cheikh Mbaye", "email": email, "role": "site_worker"}), 409, "member.already_exists")
	})

	t.Run("invitation expirée", func(t *testing.T) {
		r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "Expiré", "email": "exp-" + unique() + "@example.test", "role": "viewer"})
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE invitations SET expires_at = now() - interval '1 minute' WHERE id = $1", r.str("id")); err != nil {
			t.Fatal(err)
		}
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/invitations/"+tokenFrom(t, r.str("invite_url"))+"/accept", map[string]any{"password": testPassword}), 410, "invitation.invalid")
	})

	t.Run("invitation révoquée", func(t *testing.T) {
		r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "Révoqué", "email": "rev-" + unique() + "@example.test", "role": "viewer"})
		tn.owner.must(204, "DELETE", "/api/v1/invitations/"+r.str("id"), nil)
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/invitations/"+tokenFrom(t, r.str("invite_url"))+"/accept", map[string]any{"password": testPassword}), 410, "invitation.invalid")
	})

	t.Run("deux acceptations concurrentes : une seule passe", func(t *testing.T) {
		r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "Course", "email": "race-" + unique() + "@example.test", "role": "viewer"})
		token := tokenFrom(t, r.str("invite_url"))
		ok, statuses := concurrently(6, func(int) response {
			return newClient(t).do("POST", "/api/v1/auth/invitations/"+token+"/accept", map[string]any{"password": testPassword})
		})
		if ok != 1 {
			t.Fatalf("une seule acceptation attendue, %d réussies : %v", ok, statuses)
		}
	})

	t.Run("utilisateur lié à une seule organisation", func(t *testing.T) {
		other := newTenant(t)
		r := other.owner.do("POST", "/api/v1/invitations", map[string]any{"full_name": "Doublon", "email": email, "role": "viewer"})
		expectCode(t, r, 409, "member.identifier_taken")
	})

	t.Run("compte suspendu non authentifiable, sessions révoquées", func(t *testing.T) {
		suspendedEmail := "susp-" + unique() + "@example.test"
		r := tn.owner.must(201, "POST", "/api/v1/invitations", map[string]any{"full_name": "Suspendu", "email": suspendedEmail, "role": "site_worker"})
		c := newClient(t)
		c.must(200, "POST", "/api/v1/auth/invitations/"+tokenFrom(t, r.str("invite_url"))+"/accept", map[string]any{"password": testPassword})
		c.must(200, "GET", "/api/v1/auth/me", nil)

		tn.owner.must(200, "PATCH", "/api/v1/members/"+r.str("user_id"), map[string]any{"status": "suspended"})
		expectCode(t, c.do("GET", "/api/v1/auth/me", nil), 401, "auth.unauthenticated")
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": suspendedEmail, "password": testPassword}), 401, "auth.invalid_credentials")

		tn.owner.must(200, "PATCH", "/api/v1/members/"+r.str("user_id"), map[string]any{"status": "active"})
		newClient(t).must(200, "POST", "/api/v1/auth/login", map[string]any{"login": suspendedEmail, "password": testPassword})
	})

	t.Run("les secrets ne sortent jamais de l'API", func(t *testing.T) {
		members := tn.owner.must(200, "GET", "/api/v1/members", nil)
		body := string(members.body)
		for _, secret := range []string{"password_hash", "argon2", "token_hash", "failed_login"} {
			if strings.Contains(body, secret) {
				t.Errorf("%q présent dans la réponse des membres", secret)
			}
		}
	})
}

// CF-008
func TestCF008_Roles(t *testing.T) {
	tn := newTenant(t)
	r := tn.owner.must(200, "PATCH", "/api/v1/members/"+tn.workerID, map[string]any{"role": "manager"})
	if r.str("role") != "manager" {
		t.Fatalf("rôle non appliqué : %s", r.body)
	}
	// Le nouveau rôle s'applique immédiatement, sans reconnexion.
	tn.worker.must(201, "POST", "/api/v1/projects", map[string]any{
		"reference": "Z-" + unique(), "name": "Promu", "client_name": "Client", "address": "Dakar", "start_date": "2026-01-01",
	})

	logs := tn.owner.must(200, "GET", "/api/v1/audit-logs?action=member.role_changed&entity_id="+tn.workerID, nil).items()
	if len(logs) != 1 {
		t.Fatalf("le changement de rôle doit être audité une fois : %d", len(logs))
	}
	if !strings.Contains(string(mustJSON(t, logs[0])), `"manager"`) {
		t.Fatalf("avant/après absents de l'audit : %v", logs[0])
	}

	expectCode(t, tn.manager.do("PATCH", "/api/v1/members/"+tn.workerID, map[string]any{"role": "owner"}), 403, "auth.forbidden")
	expectCode(t, tn.owner.do("PATCH", "/api/v1/members/"+tn.workerID, map[string]any{"role": "empereur"}), 422, "validation.failed")
	expectCode(t, tn.owner.do("PATCH", "/api/v1/members/"+uuid.NewString(), map[string]any{"role": "viewer"}), 404, "member.not_found")

	t.Run("dernier Owner protégé", func(t *testing.T) {
		expectCode(t, tn.owner.do("PATCH", "/api/v1/members/"+tn.ownerID, map[string]any{"role": "viewer"}), 409, "member.last_owner")
		expectCode(t, tn.owner.do("PATCH", "/api/v1/members/"+tn.ownerID, map[string]any{"status": "suspended"}), 409, "member.last_owner")
		tn.owner.must(200, "PATCH", "/api/v1/members/"+tn.accountID, map[string]any{"role": "owner"})
		tn.owner.must(200, "PATCH", "/api/v1/members/"+tn.ownerID, map[string]any{"role": "accountant"})
	})
}
