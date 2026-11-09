package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/auth"
)

// Les logs doivent être corrélables : l'acteur, authentifié plus bas dans la chaîne, y figure.
func TestRequestLogCarriesActor(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	org, user := uuid.New(), uuid.New()
	authenticated := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := auth.WithActor(r.Context(), auth.Actor{OrgID: org, UserID: user})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	handler := RequestContext(false, nil)(authenticated(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-Request-Id", "req-42")
	handler.ServeHTTP(rec, req)

	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log illisible : %v\n%s", err, buf.String())
	}
	if line["org_id"] != org.String() || line["user_id"] != user.String() {
		t.Fatalf("acteur absent du log : %v", line)
	}
	if line["request_id"] != "req-42" || line["status"] != float64(http.StatusTeapot) {
		t.Fatalf("corrélation incomplète : %v", line)
	}
	if rec.Header().Get("X-Request-Id") != "req-42" {
		t.Fatal("le request_id doit revenir au client")
	}
}

func TestPanicBecomesCleanError(t *testing.T) {
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	handler := RequestContext(false, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret interne")
	}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("statut %d", rec.Code)
	}
	if bytes.Contains(rec.Body.Bytes(), []byte("secret interne")) {
		t.Fatal("la cause d'une panic ne doit pas être exposée au client")
	}
}
