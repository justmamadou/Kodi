package budget

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose les budgets.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{id}/budget/versions", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.ListVersions(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/projects/{id}/budget/versions", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.CreateDraft(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Get("/budget-versions/compare", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		from, err := httpx.QueryUUID(r, "from")
		if err != nil {
			return err
		}
		to, err := httpx.QueryUUID(r, "to")
		if err != nil {
			return err
		}
		if from == nil || to == nil {
			return apperr.New(http.StatusBadRequest, "request.invalid_filter", "Les paramètres from et to sont obligatoires.")
		}
		out, err := h.svc.Compare(r.Context(), httpx.Actor(r), *from, *to)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/budget-versions/{id}", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.GetVersion(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/budget-versions/{id}/lines", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[LineInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.AddLine(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Post("/budget-versions/{id}/publish", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Publish(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/budget-versions/{id}/discard", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Discard(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/budget-versions/{id}/revise", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[ReviseInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Revise(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Patch("/budget-lines/{id}", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[LineInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateLine(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Delete("/budget-lines/{id}", h.versionAction(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.DeleteLine(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
}

func (h *Handler) versionAction(fn func(http.ResponseWriter, *http.Request, uuid.UUID) error) http.HandlerFunc {
	return httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		return fn(w, r, id)
	})
}
