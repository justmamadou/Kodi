// Package app assemble les modules en un routeur HTTP : c'est le seul endroit qui les connaît tous.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/budget"
	"github.com/justmamadou/Kodi/internal/expense"
	"github.com/justmamadou/Kodi/internal/fieldops"
	"github.com/justmamadou/Kodi/internal/files"
	"github.com/justmamadou/Kodi/internal/finance"
	"github.com/justmamadou/Kodi/internal/httpx"
	"github.com/justmamadou/Kodi/internal/identity"
	"github.com/justmamadou/Kodi/internal/inventory"
	"github.com/justmamadou/Kodi/internal/organization"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"github.com/justmamadou/Kodi/internal/platform/storage"
	"github.com/justmamadou/Kodi/internal/procurement"
	"github.com/justmamadou/Kodi/internal/project"
	"github.com/justmamadou/Kodi/internal/reporting"
	"github.com/justmamadou/Kodi/internal/supplier"
	"github.com/prometheus/client_golang/prometheus"
)

// Deps regroupe les dépendances externes de l'application.
type Deps struct {
	Config   config.Config
	Tx       *db.TxManager
	Storage  storage.Storage
	Mailer   mailer.Mailer
	Registry prometheus.Registerer
}

// NewRouter construit l'API complète.
func NewRouter(d Deps) (chi.Router, error) {
	identitySvc, err := identity.NewService(d.Tx, d.Mailer, d.Config.AppBaseURL)
	if err != nil {
		return nil, err
	}
	identityH := identity.NewHandler(identitySvc, d.Config.CookieSecure)
	expenseSvc := expense.NewService(d.Tx)
	financeSvc := finance.NewService(d.Tx)
	procurementH := procurement.NewHandler(procurement.NewService(d.Tx, d.Mailer, d.Config.AppBaseURL))

	var metrics *httpx.Metrics
	if d.Registry != nil {
		metrics = httpx.NewMetrics(d.Registry)
	}

	r := chi.NewRouter()
	r.Use(httpx.RequestContext(d.Config.TrustProxyHeaders, metrics))
	r.Use(httpx.SecurityHeaders)
	r.NotFound(httpx.H(func(http.ResponseWriter, *http.Request) error {
		return apperr.New(http.StatusNotFound, "route.not_found", "Cette route n'existe pas.")
	}))
	r.MethodNotAllowed(httpx.H(func(http.ResponseWriter, *http.Request) error {
		return apperr.New(http.StatusMethodNotAllowed, "route.method_not_allowed", "Méthode non autorisée sur cette route.")
	}))

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_ = httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := d.Tx.Ping(ctx); err != nil {
			return apperr.New(http.StatusServiceUnavailable, "service.not_ready", "La base de données ne répond pas.")
		}
		return httpx.JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}))

	r.Route("/api/v1", func(api chi.Router) {
		// Routes publiques sensibles : limitées en débit par adresse IP.
		api.Group(func(public chi.Router) {
			public.Use(httpx.NewRateLimiter(d.Config.AuthRatePerMinute).Middleware)
			identityH.PublicRoutes(public)
			procurementH.PublicRoutes(public)
		})
		// Tout le reste exige une session ; l'organisation en est déduite.
		api.Group(func(private chi.Router) {
			private.Use(identityH.RequireSession)
			private.Use(httpx.CSRF)
			private.Use(httpx.Idempotency(d.Tx))
			identityH.Routes(private)
			organization.NewHandler(organization.NewService(d.Tx, d.Mailer, d.Config.AppBaseURL)).Routes(private)
			project.NewHandler(project.NewService(d.Tx)).Routes(private)
			budget.NewHandler(budget.NewService(d.Tx)).Routes(private)
			expense.NewHandler(expenseSvc).Routes(private)
			finance.NewHandler(financeSvc).Routes(private)
			inventory.NewHandler(inventory.NewService(d.Tx)).Routes(private)
			fieldops.NewHandler(fieldops.NewService(d.Tx)).Routes(private)
			files.NewHandler(files.NewService(d.Tx, d.Storage)).Routes(private)
			procurementH.Routes(private)
			supplier.NewHandler(supplier.NewService(d.Tx)).Routes(private)
			reporting.NewHandler(reporting.NewService(d.Tx, expenseSvc, financeSvc)).Routes(private)
		})
	})
	return r, nil
}
