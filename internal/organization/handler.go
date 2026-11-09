package organization

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose l'organisation et ses membres.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/organization", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.Get(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Patch("/organization", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[UpdateInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Update(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/organization/settings", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.GetSettings(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Patch("/organization/settings", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[SettingsInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateSettings(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))

	r.Get("/categories", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.ListCategories(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/categories", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[CategoryInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.CreateCategory(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Patch("/categories/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[CategoryInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateCategory(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))

	r.Get("/directory", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.Directory(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Get("/members", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.ListMembers(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Patch("/members/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[UpdateMemberInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateMember(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/invitations", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.ListInvitations(r.Context(), httpx.Actor(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/invitations", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[InviteInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Invite(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Delete("/invitations/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		if err := h.svc.RevokeInvitation(r.Context(), httpx.Actor(r), id); err != nil {
			return err
		}
		w.WriteHeader(http.StatusNoContent)
		return nil
	}))
	r.Get("/audit-logs", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		entityID, err := httpx.QueryUUID(r, "entity_id")
		if err != nil {
			return err
		}
		out, err := h.svc.ListAudit(r.Context(), httpx.Actor(r), httpx.QueryString(r, "action"), entityID, httpx.Limit(r))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
}
