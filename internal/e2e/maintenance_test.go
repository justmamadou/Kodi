package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/reporting"
)

// CF-004 : la session est glissante, et le cookie du navigateur suit.
func TestCF004_SlidingSessionRenewsCookie(t *testing.T) {
	id := unique()
	c := newClient(t)
	c.must(201, "POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": "Glissante " + id, "full_name": "Awa Fall", "email": "slide-" + id + "@example.test", "password": testPassword,
	})
	// Requête récente : on n'écrit ni en base ni dans les cookies à chaque lecture.
	if r := c.must(200, "GET", "/api/v1/auth/me", nil); len(r.header.Values("Set-Cookie")) != 0 {
		t.Fatalf("aucun renouvellement attendu : %v", r.header.Values("Set-Cookie"))
	}
	token := sessionToken(t, c)
	ctx := context.Background()
	if _, err := env.ownerDB.Exec(ctx, "UPDATE sessions SET last_seen_at = now() - interval '10 minutes', expires_at = now() + interval '1 hour' WHERE token_hash = sha256($1::bytea)", []byte(token)); err != nil {
		t.Fatal(err)
	}
	r := c.must(200, "GET", "/api/v1/auth/me", nil)
	cookies := strings.Join(r.header.Values("Set-Cookie"), "\n")
	if !strings.Contains(cookies, "cf_session="+token) || !strings.Contains(cookies, "HttpOnly") || !strings.Contains(cookies, "cf_csrf=") {
		t.Fatalf("cookies non prolongés : %s", cookies)
	}
	var hours float64
	if err := env.ownerDB.QueryRow(ctx, "SELECT extract(epoch FROM expires_at - now()) / 3600 FROM sessions WHERE token_hash = sha256($1::bytea)", []byte(token)).Scan(&hours); err != nil {
		t.Fatal(err)
	}
	if hours < 11 {
		t.Fatalf("expiration non prolongée : %.1f h", hours)
	}
	// Le jeton CSRF reste valable après le renouvellement.
	c.must(200, "PATCH", "/api/v1/organization", map[string]any{"city": "Dakar"})
}

// Maintenance : la purge retire ce qui a expiré, et seulement cela.
func TestPurgeExpired(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	ctx := context.Background()

	key := uuid.NewString()
	tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", map[string]any{
		"category_id": tn.categoryID, "description": "Ciment", "amount": 10, "expense_date": "2026-11-03",
	}, "Idempotency-Key", key)
	recent := uuid.NewString()
	tn.worker.must(201, "POST", "/api/v1/projects/"+project+"/expenses", map[string]any{
		"category_id": tn.categoryID, "description": "Sable", "amount": 20, "expense_date": "2026-11-03",
	}, "Idempotency-Key", recent)
	if _, err := env.ownerDB.Exec(ctx, "UPDATE idempotency_keys SET created_at = now() - interval '8 days' WHERE key = $1", key); err != nil {
		t.Fatal(err)
	}
	var ownerOrg string
	if err := env.ownerDB.QueryRow(ctx, "SELECT organization_id FROM projects WHERE id = $1", project).Scan(&ownerOrg); err != nil {
		t.Fatal(err)
	}
	var oldSession, liveSession string
	if err := env.ownerDB.QueryRow(ctx, `UPDATE sessions SET revoked_at = now() - interval '31 days'
		 WHERE id = (SELECT id FROM sessions WHERE organization_id = $1 AND user_id = $2 LIMIT 1) RETURNING id`, ownerOrg, tn.viewerID).Scan(&oldSession); err != nil {
		t.Fatal(err)
	}
	if err := env.ownerDB.QueryRow(ctx, "SELECT id FROM sessions WHERE organization_id = $1 AND user_id = $2 LIMIT 1", ownerOrg, tn.workerID).Scan(&liveSession); err != nil {
		t.Fatal(err)
	}

	// Avec le rôle applicatif, sans aucun tenant : la fonction est le seul chemin autorisé.
	keys, sessions, err := db.NewTxManager(env.appPool).PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if keys < 1 || sessions < 1 {
		t.Fatalf("purge : %d clés, %d sessions", keys, sessions)
	}
	count := func(query string, arg any) int {
		var n int
		if err := env.ownerDB.QueryRow(ctx, query, arg).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if count("SELECT count(*) FROM idempotency_keys WHERE key = $1", key) != 0 || count("SELECT count(*) FROM sessions WHERE id = $1", oldSession) != 0 {
		t.Fatal("ce qui a expiré doit disparaître")
	}
	if count("SELECT count(*) FROM idempotency_keys WHERE key = $1", recent) != 1 || count("SELECT count(*) FROM sessions WHERE id = $1", liveSession) != 1 {
		t.Fatal("ce qui est récent doit rester")
	}
}

// CF-028 : un export qui ne tient pas dans la limite échoue au lieu de produire des totaux faux.
func TestCF028_TooLargeExportFailsInsteadOfTruncating(t *testing.T) {
	tn := newTenant(t)
	project := tn.newProject(t)
	for range 3 {
		tn.newExpense(t, tn.owner, project, 100)
	}
	previous := reporting.MaxExportRows
	reporting.MaxExportRows = 2
	t.Cleanup(func() { reporting.MaxExportRows = previous })
	expectCode(t, tn.accountant.do("GET", "/api/v1/exports/expenses.xlsx?project_id="+project, nil), 422, "export.too_large")
	reporting.MaxExportRows = 3
	tn.accountant.must(200, "GET", "/api/v1/exports/expenses.xlsx?project_id="+project, nil)
}
