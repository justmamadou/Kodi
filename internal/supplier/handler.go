package supplier

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose le répertoire des fournisseurs.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func withID(fn func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error) http.HandlerFunc {
	return httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		return fn(w, r, id)
	})
}

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/suppliers", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.List(r.Context(), httpx.Actor(r), httpx.QueryString(r, "q"), httpx.QueryString(r, "activity"),
			r.URL.Query().Get("include_archived") == "true")
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/suppliers", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[Input](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Create(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	// POST : la saisie en cours (nom, NINEA, téléphone...) reste hors de l'URL et des journaux d'accès.
	r.Post("/suppliers/duplicates", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[DuplicateQuery](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Duplicates(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Get("/suppliers/{id}", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Get(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Patch("/suppliers/{id}", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[Input](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Update(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/suppliers/{id}/archive", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Archive(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/suppliers/{id}/restore", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Restore(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/suppliers/{id}/terms", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Terms(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/suppliers/{id}/terms", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[TermsInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.RecordTerms(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Get("/suppliers/{id}/history", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.History(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/suppliers/{id}/contacts", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[ContactInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.AddContact(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Patch("/supplier-contacts/{id}", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[ContactInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateContact(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Delete("/supplier-contacts/{id}", withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		if err := h.svc.DeleteContact(r.Context(), httpx.Actor(r), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
}
