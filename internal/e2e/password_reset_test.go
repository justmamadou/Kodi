package e2e

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justmamadou/Kodi/internal/app"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
)

// fakeMailer garde les emails envoyés pour que les tests y lisent les liens.
type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
}

type sentMail struct{ to, subject, body string }

var _ mailer.Mailer = (*fakeMailer)(nil)

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMail{to: to, subject: subject, body: body})
	return nil
}

func (f *fakeMailer) to(addr string) []sentMail {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sentMail
	for _, m := range f.sent {
		if m.to == addr {
			out = append(out, m)
		}
	}
	return out
}

// waitMails attend que n emails soient partis vers addr : la demande est traitée en arrière-plan.
func (f *fakeMailer) waitMails(t *testing.T, addr string, n int) []sentMail {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := f.to(addr)
		if len(got) >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d email(s) attendu(s) pour %s, %d reçu(s)", n, addr, len(got))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

var resetLinkRe = regexp.MustCompile(`http://app\.test/nouveau-mot-de-passe#([A-Za-z0-9_-]+)`)

func resetToken(t *testing.T, m sentMail) string {
	t.Helper()
	match := resetLinkRe.FindStringSubmatch(m.body)
	if match == nil {
		t.Fatalf("lien de réinitialisation absent de l'email :\n%s", m.body)
	}
	return match[1]
}

// resetAccount crée une entreprise dont l'Owner a l'email retourné, connecté sur le client retourné.
func resetAccount(t *testing.T) (email, userID string, c *client) {
	t.Helper()
	id := unique()
	email = "reset-" + id + "@example.test"
	c = newClient(t)
	r := c.must(201, "POST", "/api/v1/auth/signup", map[string]any{
		"organization_name": "Entreprise " + id, "full_name": "Awa Ndiaye", "email": email, "password": testPassword,
	})
	user, _ := r.json()["user"].(map[string]any)
	userID, _ = user["id"].(string)
	return email, userID, c
}

func requestReset(t *testing.T, login string) response {
	t.Helper()
	return newClient(t).must(202, "POST", "/api/v1/auth/password-reset", map[string]any{"login": login})
}

func auditCount(t *testing.T, userID, action string) int {
	t.Helper()
	var n int
	if err := env.ownerDB.QueryRow(context.Background(),
		"SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = $2", userID, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func waitAudit(t *testing.T, userID, action string, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for auditCount(t, userID, action) < n {
		if time.Now().After(deadline) {
			t.Fatalf("entrée d'audit %q attendue pour %s", action, userID)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

const newPassword = "nouveau-secret-2026"

func TestPasswordReset(t *testing.T) {
	t.Run("la réponse ne dit pas si le compte existe", func(t *testing.T) {
		email, _, _ := resetAccount(t)
		known := requestReset(t, email)
		unknown := requestReset(t, "inconnu-"+unique()+"@example.test")
		unknownPhone := requestReset(t, "+221 70 000 00 00")
		if string(known.body) != string(unknown.body) || string(known.body) != string(unknownPhone.body) {
			t.Fatalf("réponses différentes :\n%s\n%s\n%s", known.body, unknown.body, unknownPhone.body)
		}
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/password-reset", map[string]any{"login": "  "}), 422, "validation.failed")
	})

	t.Run("lien par email : usage unique, haché en base, sessions révoquées, audit", func(t *testing.T) {
		email, userID, first := resetAccount(t)
		second := newClient(t)
		second.must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword})

		requestReset(t, strings.ToUpper(email))
		mail := env.mail.waitMails(t, email, 1)[0]
		token := resetToken(t, mail)

		var raw, hashed int
		hash := sha256.Sum256([]byte(token))
		if err := env.ownerDB.QueryRow(context.Background(),
			"SELECT count(*) FILTER (WHERE token_hash = convert_to($1, 'UTF8')), count(*) FILTER (WHERE token_hash = $2) FROM password_resets",
			token, hash[:]).Scan(&raw, &hashed); err != nil {
			t.Fatal(err)
		}
		if raw != 0 || hashed != 1 {
			t.Fatalf("jeton attendu haché en base (brut : %d, haché : %d)", raw, hashed)
		}

		anon := newClient(t)
		expectCode(t, anon.do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": token, "password": "court"}), 422, "validation.failed")
		anon.must(204, "POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": token, "password": newPassword})

		// Toutes les sessions du compte sont terminées.
		expectCode(t, first.do("GET", "/api/v1/auth/me", nil), 401, "auth.unauthenticated")
		expectCode(t, second.do("GET", "/api/v1/auth/me", nil), 401, "auth.unauthenticated")

		expectCode(t, newClient(t).do("POST", "/api/v1/auth/login", map[string]any{"login": email, "password": testPassword}), 401, "auth.invalid_credentials")
		newClient(t).must(200, "POST", "/api/v1/auth/login", map[string]any{"login": email, "password": newPassword})

		expectCode(t, newClient(t).do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": token, "password": newPassword + "x"}), 410, "password_reset.invalid")

		if auditCount(t, userID, "password_reset.requested") != 1 || auditCount(t, userID, "password.reset") != 1 {
			t.Fatal("demande et changement doivent figurer au journal d'audit")
		}
	})

	t.Run("un nouveau lien remplace le précédent", func(t *testing.T) {
		email, _, _ := resetAccount(t)
		requestReset(t, email)
		old := resetToken(t, env.mail.waitMails(t, email, 1)[0])
		requestReset(t, email)
		fresh := resetToken(t, env.mail.waitMails(t, email, 2)[1])
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": old, "password": newPassword}), 410, "password_reset.invalid")
		newClient(t).must(204, "POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": fresh, "password": newPassword})
	})

	t.Run("lien expiré ou inconnu", func(t *testing.T) {
		email, userID, _ := resetAccount(t)
		requestReset(t, email)
		token := resetToken(t, env.mail.waitMails(t, email, 1)[0])
		if _, err := env.ownerDB.Exec(context.Background(),
			"UPDATE password_resets SET expires_at = now() - interval '1 minute' WHERE user_id = $1", userID); err != nil {
			t.Fatal(err)
		}
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": token, "password": newPassword}), 410, "password_reset.invalid")
		expectCode(t, newClient(t).do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": "inconnu", "password": newPassword}), 410, "password_reset.invalid")
	})

	t.Run("deux confirmations concurrentes : une seule passe", func(t *testing.T) {
		email, _, _ := resetAccount(t)
		requestReset(t, email)
		token := resetToken(t, env.mail.waitMails(t, email, 1)[0])
		ok, statuses := concurrently(6, func(int) response {
			return newClient(t).do("POST", "/api/v1/auth/password-reset/confirm", map[string]any{"token": token, "password": newPassword})
		})
		if ok != 1 {
			t.Fatalf("une seule confirmation attendue, %d réussies : %v", ok, statuses)
		}
	})

	t.Run("plafond de 3 demandes par compte et par heure, même en rafale", func(t *testing.T) {
		email, _, _ := resetAccount(t)
		concurrently(6, func(int) response { return requestReset(t, email) })
		env.mail.waitMails(t, email, 3)
		time.Sleep(500 * time.Millisecond)
		if n := len(env.mail.to(email)); n != 3 {
			t.Fatalf("3 emails attendus, %d envoyés", n)
		}
	})

	t.Run("compte sans email : rien n'est envoyé, la demande est tracée", func(t *testing.T) {
		id := unique()
		phone := fmt.Sprintf("+22177%07d", rand.IntN(10_000_000))
		c := newClient(t)
		r := c.must(201, "POST", "/api/v1/auth/signup", map[string]any{
			"organization_name": "Entreprise " + id, "full_name": "Ibou Faye", "phone": phone, "password": testPassword,
		})
		user, _ := r.json()["user"].(map[string]any)
		userID, _ := user["id"].(string)
		// Saisi avec des espaces, comme sur un téléphone.
		requestReset(t, phone[:4]+" "+phone[4:6]+" "+phone[6:])
		waitAudit(t, userID, "password_reset.requested", 1)
		var channel string
		var links int
		if err := env.ownerDB.QueryRow(context.Background(),
			`SELECT (SELECT after->>'channel' FROM audit_logs WHERE entity_id = $1 AND action = 'password_reset.requested'),
			        (SELECT count(*) FROM password_resets WHERE user_id = $1)`, userID).Scan(&channel, &links); err != nil {
			t.Fatal(err)
		}
		if channel != "none" || links != 0 {
			t.Fatalf("aucun lien attendu sans email (canal %q, %d lien(s))", channel, links)
		}
		c.must(200, "GET", "/api/v1/auth/me", nil) // une simple demande ne déconnecte personne
	})
}

// Les deux routes publiques sont soumises à la limite par adresse IP.
func TestPasswordResetRateLimitedPerIP(t *testing.T) {
	router, err := app.NewRouter(app.Deps{
		Config: config.Config{Env: "test", AppBaseURL: "http://app.test", AuthRatePerMinute: 2},
		Tx:     db.NewTxManager(env.appPool), Storage: env.store, Mailer: &fakeMailer{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var last int
	for _, path := range []string{"/api/v1/auth/password-reset", "/api/v1/auth/password-reset/confirm", "/api/v1/auth/password-reset"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.20:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		last = rec.Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("429 attendu après 2 essais, obtenu %d", last)
	}
}
