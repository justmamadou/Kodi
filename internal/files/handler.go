package files

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose les pièces jointes.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{id}/attachments", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.ListProject(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/attachments/presign", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[PresignInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Presign(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Post("/attachments/{id}/complete", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Complete(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/attachments/{id}/url", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		url, ttl, err := h.svc.DownloadURL(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"url": url, "expires_in": ttl})
	}))
}
