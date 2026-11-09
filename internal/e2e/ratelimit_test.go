package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/app"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
)

// CF-004 : les endpoints sensibles sont limités en débit, avec une configuration dédiée.
func TestCF004_RateLimitOnSensitiveEndpoints(t *testing.T) {
	var router chi.Router
	router, err := app.NewRouter(app.Deps{
		Config: config.Config{Env: "test", AppBaseURL: "http://app.test", AuthRatePerMinute: 3},
		Tx:     db.NewTxManager(env.appPool), Storage: env.store, Mailer: mailer.Noop{},
	})
	if err != nil {
		t.Fatal(err)
	}
	statuses := make([]int, 5)
	for i := range statuses {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.RemoteAddr = "203.0.113.7:1234"
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		statuses[i] = rec.Code
	}
	if statuses[2] == http.StatusTooManyRequests || statuses[3] != http.StatusTooManyRequests || statuses[4] != http.StatusTooManyRequests {
		t.Fatalf("3 essais puis 429 attendus : %v", statuses)
	}

	// Une autre adresse n'est pas pénalisée.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.8:1234"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("la limite doit être par adresse IP")
	}

	// X-Forwarded-For n'est pas écouté par défaut : on ne contourne pas la limite en le falsifiant.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "203.0.113.7:1234"
	req.Header.Set("X-Forwarded-For", "198.51.100.1")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("X-Forwarded-For ne doit pas contourner la limite : %d", rec.Code)
	}
}
