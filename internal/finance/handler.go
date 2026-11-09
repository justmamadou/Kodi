package finance

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose le tableau de bord financier.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/dashboard", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.Dashboard(r.Context(), httpx.Actor(r), r.URL.Query().Get("period"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/reports/overview", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		projectID, err := httpx.QueryUUID(r, "project_id")
		if err != nil {
			return err
		}
		out, err := h.svc.Overview(r.Context(), httpx.Actor(r), projectID)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/projects/{id}/financials", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Project(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
}
