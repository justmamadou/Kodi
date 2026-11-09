package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/platform/migrate"
)

// CF-001
func TestCF001_Healthz(t *testing.T) {
	c := newClient(t)
	if r := c.must(200, "GET", "/healthz", nil); r.str("status") != "ok" {
		t.Fatalf("/healthz : %s", r.body)
	}
	c.must(200, "GET", "/readyz", nil)
	if r := c.must(200, "GET", "/healthz", nil); r.header.Get("X-Request-Id") == "" {
		t.Fatal("chaque réponse doit porter un request_id")
	}
}

// CF-002 : le cycle complet up / down / up doit fonctionner sur une base vierge.
func TestCF002_MigrationsUpDown(t *testing.T) {
	ctx := context.Background()
	name := "cf_migr_" + strings.ReplaceAll(unique(), "-", "")
	if _, err := env.ownerDB.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = env.ownerDB.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })
	url := strings.Replace(env.ownerURL, env.ownerURL[strings.LastIndex(env.ownerURL, "/"):], "/"+name+"?sslmode=disable", 1)
	if err := migrate.Up(url); err != nil {
		t.Fatalf("up : %v", err)
	}
	if err := migrate.Down(url); err != nil {
		t.Fatalf("down : %v", err)
	}
	if err := migrate.Up(url); err != nil {
		t.Fatalf("up après down : %v", err)
	}
}

// CF-002, CF-003 : aucune table portant organization_id ne peut échapper à la RLS.
func TestCF002_EveryTenantTableHasRLS(t *testing.T) {
	rows, err := env.ownerDB.Query(context.Background(), `
		SELECT c.relname
		  FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'organization_id' AND NOT a.attisdropped
		 WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND NOT c.relispartition
		   AND NOT (c.relrowsecurity AND c.relforcerowsecurity
		            AND EXISTS (SELECT 1 FROM pg_policy p WHERE p.polrelid = c.oid))`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		t.Errorf("table %s : organization_id sans RLS forcée", name)
	}
}

// CF-002 : hors organizations/plans, chaque table métier porte organization_id.
func TestCF002_TenantOwnedTablesCarryOrganizationID(t *testing.T) {
	rows, err := env.ownerDB.Query(context.Background(), `
		SELECT c.relname FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relkind IN ('r','p') AND NOT c.relispartition
		   AND c.relname NOT IN ('organizations','schema_migrations')
		   AND NOT EXISTS (SELECT 1 FROM pg_attribute a
		                    WHERE a.attrelid = c.oid AND a.attname = 'organization_id' AND NOT a.attisdropped)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		t.Errorf("table %s sans organization_id", name)
	}
}

// CF-002 : la FK composite refuse une référence inter-tenant, même sans passer par l'API.
func TestCF002_CompositeForeignKeyBlocksCrossTenantReference(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	projectA := a.newProject(t)
	ctx := context.Background()
	var orgA string
	if err := env.ownerDB.QueryRow(ctx, "SELECT organization_id FROM projects WHERE id = $1", projectA).Scan(&orgA); err != nil {
		t.Fatal(err)
	}
	// Superutilisateur : ni la RLS ni l'application ne protègent ici, seule la FK composite agit.
	_, err := env.ownerDB.Exec(ctx, `INSERT INTO expenses (id, organization_id, project_id, category_id, description, amount, currency, expense_date, created_by)
		VALUES ($1, $2, $3, $4, 'fuite', 10, 'XOF', '2026-11-01', $5)`, uuid.New(), orgA, projectA, b.categoryID, a.ownerID)
	if err == nil || !strings.Contains(err.Error(), "23503") {
		t.Fatalf("la catégorie d'un autre tenant aurait dû être refusée : %v", err)
	}
}

// CF-003 : le rôle applicatif ne voit rien sans tenant, et rien d'autre que le sien avec.
func TestCF003_RLSFailsClosed(t *testing.T) {
	a := newTenant(t)
	a.newProject(t)
	ctx := context.Background()

	var count int
	if err := env.appPool.QueryRow(ctx, "SELECT count(*) FROM projects").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("sans tenant, cf_app voit %d chantiers", count)
	}
	for _, table := range []string{"users", "expenses", "sessions", "audit_logs", "organizations"} {
		if err := env.appPool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("sans tenant, cf_app voit %d lignes de %s", count, table)
		}
	}
	// Une partition d'audit lue directement contournerait la RLS : elle doit être interdite.
	err := env.appPool.QueryRow(ctx, "SELECT count(*) FROM audit_logs_2026_11").Scan(&count)
	if err == nil || !strings.Contains(err.Error(), "42501") {
		t.Fatalf("cf_app ne doit pas lire une partition directement : %v", err)
	}
}

// CF-003 : aucun accès inter-tenant sur les projets, budgets, dépenses et fichiers.
func TestCF003_CrossTenantAccessDenied(t *testing.T) {
	a, b := newTenant(t), newTenant(t)
	projectA := a.newProject(t)
	versionA := a.publishBudget(t, projectA, "100", 5000)
	expenseA := a.newExpense(t, a.worker, projectA, 10000)

	attachmentID := uuid.NewString()
	pre := a.worker.must(201, "POST", "/api/v1/attachments/presign", map[string]any{
		"id": attachmentID, "expense_id": expenseA, "kind": "receipt", "mime_type": "image/jpeg", "size_bytes": 4,
	})
	att, _ := pre.json()["attachment"].(map[string]any)
	env.store.put(att["storage_key"].(string), []byte{0xFF, 0xD8, 0xFF, 0xE0})
	a.worker.must(200, "POST", "/api/v1/attachments/"+attachmentID+"/complete", nil)

	// L'Owner de B a tous les droits chez lui : seul le tenant peut le bloquer.
	attacker := b.owner
	denied := []struct {
		method, path string
		body         any
		headers      []string
	}{
		{"GET", "/api/v1/projects/" + projectA, nil, nil},
		{"PATCH", "/api/v1/projects/" + projectA, map[string]any{"name": "Piraté"}, []string{"If-Match", "1"}},
		{"POST", "/api/v1/projects/" + projectA + "/status", map[string]any{"status": "completed"}, nil},
		{"GET", "/api/v1/projects/" + projectA + "/members", nil, nil},
		{"PUT", "/api/v1/projects/" + projectA + "/members/" + b.ownerID, map[string]any{}, nil},
		{"GET", "/api/v1/projects/" + projectA + "/phases", nil, nil},
		{"GET", "/api/v1/projects/" + projectA + "/financials", nil, nil},
		{"GET", "/api/v1/projects/" + projectA + "/budget/versions", nil, nil},
		{"POST", "/api/v1/projects/" + projectA + "/budget/versions", nil, nil},
		{"GET", "/api/v1/budget-versions/" + versionA, nil, nil},
		{"POST", "/api/v1/budget-versions/" + versionA + "/revise", map[string]any{"reason": "piratage"}, nil},
		{"POST", "/api/v1/budget-versions/" + versionA + "/publish", nil, nil},
		{"GET", "/api/v1/expenses/" + expenseA, nil, nil},
		{"PATCH", "/api/v1/expenses/" + expenseA, map[string]any{"amount": 1}, []string{"If-Match", "1"}},
		{"POST", "/api/v1/expenses/" + expenseA + "/submit", nil, nil},
		{"POST", "/api/v1/expenses/" + expenseA + "/approve", nil, nil},
		{"POST", "/api/v1/expenses/" + expenseA + "/payments", map[string]any{"amount": 1, "paid_on": "2026-11-05", "method": "cash"}, nil},
		{"POST", "/api/v1/projects/" + projectA + "/expenses", map[string]any{"category_id": b.categoryID, "description": "fuite", "amount": 10, "expense_date": "2026-11-03"}, nil},
		{"GET", "/api/v1/attachments/" + attachmentID + "/url", nil, nil},
		{"POST", "/api/v1/attachments/" + attachmentID + "/complete", nil, nil},
		{"POST", "/api/v1/attachments/presign", map[string]any{"id": uuid.NewString(), "expense_id": expenseA, "kind": "receipt", "mime_type": "image/jpeg", "size_bytes": 4}, nil},
		{"GET", "/api/v1/projects/" + projectA + "/stock", nil, nil},
		{"GET", "/api/v1/projects/" + projectA + "/tasks", nil, nil},
		{"GET", "/api/v1/projects/" + projectA + "/daily-reports", nil, nil},
		{"GET", "/api/v1/exports/projects/" + projectA + "/report.pdf", nil, nil},
		{"GET", "/api/v1/exports/expenses.xlsx?project_id=" + projectA, nil, nil},
	}
	for _, d := range denied {
		r := attacker.do(d.method, d.path, d.body, d.headers...)
		if r.status != 404 {
			t.Errorf("%s %s : statut %d, 404 attendu (indiscernable d'une absence)\n%s", d.method, d.path, r.status, r.body)
		}
	}

	// Les listes ne laissent rien fuiter.
	if items := attacker.must(200, "GET", "/api/v1/projects", nil).items(); len(items) != 0 {
		t.Errorf("B voit %d chantiers de A", len(items))
	}
	if items := attacker.must(200, "GET", "/api/v1/expenses", nil).items(); len(items) != 0 {
		t.Errorf("B voit %d dépenses de A", len(items))
	}

	// Une catégorie de A utilisée par B est refusée, y compris avec un id client déjà pris.
	projectB := b.newProject(t)
	r := b.owner.do("POST", "/api/v1/projects/"+projectB+"/expenses", map[string]any{
		"category_id": a.categoryID, "description": "fuite", "amount": 10, "expense_date": "2026-11-03",
	})
	expectCode(t, r, 422, "validation.failed")
	r = b.owner.do("POST", "/api/v1/projects/"+projectB+"/expenses", map[string]any{
		"id": expenseA, "category_id": b.categoryID, "description": "collision", "amount": 10, "expense_date": "2026-11-03",
	})
	expectCode(t, r, 409, "expense.id_conflict")

	// Un utilisateur de A n'est pas affectable chez B.
	r = b.owner.do("PUT", "/api/v1/projects/"+projectB+"/members/"+a.workerID, map[string]any{})
	expectCode(t, r, 422, "validation.failed")

	// La tentative d'accès n'a rien modifié.
	check := a.owner.must(200, "GET", "/api/v1/expenses/"+expenseA, nil)
	if check.str("status") != "draft" || check.num("amount") != 10000 {
		t.Fatalf("la dépense de A a été altérée : %s", check.body)
	}
}

// CF-004
func TestCF004_Authentication(t *testing.T) {
	id := unique()
	email := "auth-" + id + "@example.test"
	owner := newClient(t)
	owner.must(201, "POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": "Auth " + id, "full_name": "Awa Fall", "email": email, "password": testPassword,
	})

	t.Run("mot de passe jamais en clair", func(t *testing.T) {
		var hash string
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT password_hash FROM users WHERE email = $1", email).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(hash, "$argon2id$") || strings.Contains(hash, testPassword) {
			t.Fatalf("hash inattendu : %s", hash)
		}
	})

	t.Run("le jeton de session n'est stocké que haché", func(t *testing.T) {
		var count int
		cookie := sessionToken(t, owner)
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT count(*) FROM sessions WHERE encode(token_hash, 'escape') = $1", cookie).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("le jeton brut est présent en base")
		}
	})

	t.Run("erreur identique pour un email connu ou inconnu", func(t *testing.T) {
		known := newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": email, "password": "mauvais-mot-de-passe-1"})
		unknown := newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": "personne-" + id + "@example.test", "password": "mauvais-mot-de-passe-1"})
		expectCode(t, known, 401, "auth.invalid_credentials")
		expectCode(t, unknown, 401, "auth.invalid_credentials")
		if known.str("detail") != unknown.str("detail") {
			t.Fatal("les messages d'erreur diffèrent : l'existence d'un email est révélée")
		}
	})

	t.Run("cookie HttpOnly", func(t *testing.T) {
		r := newClient(t).must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword})
		var found bool
		for _, c := range r.header.Values("Set-Cookie") {
			if strings.HasPrefix(c, "cf_session=") {
				found = true
				if !strings.Contains(c, "HttpOnly") || !strings.Contains(c, "SameSite=Lax") {
					t.Fatalf("cookie de session mal protégé : %s", c)
				}
			}
		}
		if !found {
			t.Fatal("cookie de session absent")
		}
	})

	t.Run("déconnexion révoque la session", func(t *testing.T) {
		c := newClient(t)
		c.must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword})
		c.must(200, "GET", "/api/v1/auth/me", nil)
		token := sessionToken(t, c)
		c.must(204, "POST", "/api/v1/auth/logout", nil)
		replay := newClient(t)
		req := replay.do("GET", "/api/v1/auth/me", nil, "Cookie", "cf_session="+token)
		expectCode(t, req, 401, "auth.unauthenticated")
	})

	t.Run("révocation d'une autre session", func(t *testing.T) {
		phone := newClient(t)
		phone.must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword})
		sessions := owner.must(200, "GET", "/api/v1/auth/sessions", nil).items()
		if len(sessions) < 2 {
			t.Fatalf("au moins 2 sessions attendues, %d", len(sessions))
		}
		for _, s := range sessions {
			if _, leaked := s["token_hash"]; leaked {
				t.Fatal("le hash du jeton ne doit pas être exposé")
			}
		}
		var phoneSession string
		phoneToken := sessionToken(t, phone)
		if err := env.ownerDB.QueryRow(context.Background(), "SELECT id FROM sessions WHERE token_hash = sha256($1::bytea)", []byte(phoneToken)).Scan(&phoneSession); err != nil {
			t.Fatal(err)
		}
		owner.must(204, "DELETE", "/api/v1/auth/sessions/"+phoneSession, nil)
		expectCode(t, phone.do("GET", "/api/v1/auth/me", nil), 401, "auth.unauthenticated")
		owner.must(200, "GET", "/api/v1/auth/me", nil)
	})

	t.Run("session expirée refusée", func(t *testing.T) {
		c := newClient(t)
		c.must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword})
		token := sessionToken(t, c)
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE sessions SET expires_at = now() - interval '1 minute' WHERE token_hash = sha256($1::bytea)", []byte(token)); err != nil {
			t.Fatal(err)
		}
		expectCode(t, c.do("GET", "/api/v1/auth/me", nil), 401, "auth.unauthenticated")
	})

	t.Run("verrouillage après 5 échecs, même avec le bon mot de passe", func(t *testing.T) {
		lockID := unique()
		lockEmail := "lock-" + lockID + "@example.test"
		newClient(t).must(201, "POST", "/api/v1/auth/signup", map[string]any{
			"organization_name": "Lock " + lockID, "full_name": "Awa Fall", "email": lockEmail, "password": testPassword,
		})
		for range 5 {
			expectCode(t, newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": lockEmail, "password": "mauvais-mot-de-passe-1"}), 401, "auth.invalid_credentials")
		}
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": lockEmail, "password": testPassword}), 401, "auth.invalid_credentials")
		// Le verrou expiré, le compte fonctionne à nouveau.
		if _, err := env.ownerDB.Exec(context.Background(), "UPDATE users SET locked_until = now() - interval '1 minute' WHERE email = $1", lockEmail); err != nil {
			t.Fatal(err)
		}
		newClient(t).must(200, "POST", "/api/v1/auth/login", map[string]any{"login": lockEmail, "password": testPassword})
	})

	t.Run("règle de mot de passe", func(t *testing.T) {
		r := newClient(t).do("POST", "/api/v1/auth/signup", map[string]any{
			"organization_name": "Faible " + unique(), "full_name": "Awa Fall", "email": "faible-" + unique() + "@example.test", "password": "court",
		})
		expectCode(t, r, 422, "validation.failed")
		fields, _ := r.json()["fields"].(map[string]any)
		if fields["password"] == nil {
			t.Fatalf("erreur de champ attendue : %s", r.body)
		}
	})

	t.Run("mutation sans jeton CSRF refusée", func(t *testing.T) {
		token := sessionToken(t, owner)
		bare := newClient(t)
		r := bare.do("POST", "/api/v1/auth/logout", nil, "Cookie", "cf_session="+token)
		expectCode(t, r, 403, "auth.csrf")
	})

	t.Run("route privée sans session", func(t *testing.T) {
		expectCode(t, newClient(t).do("GET", "/api/v1/projects", nil), 401, "auth.unauthenticated")
	})
}

func sessionToken(t *testing.T, c *client) string {
	t.Helper()
	u := mustURL(t)
	for _, cookie := range c.http.Jar.Cookies(u) {
		if cookie.Name == "cf_session" {
			return cookie.Value
		}
	}
	t.Fatal("cookie de session absent")
	return ""
}
