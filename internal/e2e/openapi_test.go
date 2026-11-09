package e2e

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/app"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"gopkg.in/yaml.v3"
)

// Le contrat et le code ne peuvent pas diverger : toute route doit être documentée, et tout
// ce qui est documenté doit exister.
func TestOpenAPIMatchesRoutes(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, methods := range doc.Paths {
		for method := range methods {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	router, err := app.NewRouter(app.Deps{
		Config: config.Config{Env: "test", AppBaseURL: "http://app.test", AuthRatePerMinute: 1},
		Tx:     db.NewTxManager(env.appPool), Storage: env.store, Mailer: mailer.Noop{},
	})
	if err != nil {
		t.Fatal(err)
	}
	actual := map[string]bool{}
	err = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		actual[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var problems []string
	for route := range actual {
		if !documented[route] {
			problems = append(problems, "route absente du contrat : "+route)
		}
	}
	for route := range documented {
		if !actual[route] {
			problems = append(problems, "contrat sans route : "+route)
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
	if len(actual) < 80 {
		t.Fatalf("seulement %d routes détectées : le test ne voit pas l'API", len(actual))
	}
}
