package project

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/httpx"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// Handler expose les chantiers.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects", httpx.H(h.list))
	r.Post("/projects", httpx.H(h.create))
	r.Get("/projects/{id}", httpx.H(h.get))
	r.Patch("/projects/{id}", httpx.H(h.update))
	r.Post("/projects/{id}/status", httpx.H(h.transition))
	r.Post("/projects/{id}/progress", httpx.H(h.progress))
	r.Get("/projects/{id}/history", httpx.H(h.activity))
	r.Get("/projects/{id}/status-history", httpx.H(h.history))
	r.Get("/projects/{id}/members", httpx.H(h.listMembers))
	r.Put("/projects/{id}/members/{userId}", httpx.H(h.assignMember))
	r.Delete("/projects/{id}/members/{userId}", httpx.H(h.removeMember))
	r.Get("/projects/{id}/phases", httpx.H(h.listPhases))
	r.Post("/projects/{id}/phases", httpx.H(h.createPhase))
	r.Put("/projects/{id}/phases/order", httpx.H(h.reorderPhases))
	r.Patch("/projects/{id}/phases/{phaseId}", httpx.H(h.updatePhase))
	r.Delete("/projects/{id}/phases/{phaseId}", httpx.H(h.deletePhase))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	manager, err := httpx.QueryUUID(r, "manager_id")
	if err != nil {
		return err
	}
	var cursor *uuid.UUID
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parts, err := httpx.DecodeCursor(raw, 1)
		if err != nil {
			return err
		}
		id, err := uuid.Parse(parts[0])
		if err != nil {
			return apperr.New(http.StatusBadRequest, "request.invalid_cursor", "Curseur de pagination invalide.")
		}
		cursor = &id
	}
	limit := httpx.Limit(r)
	items, err := h.svc.List(r.Context(), httpx.Actor(r), ListFilter{
		Status: httpx.QueryString(r, "status"), ManagerID: manager, Q: httpx.QueryString(r, "q"), Cursor: cursor, Limit: limit + 1,
	})
	if err != nil {
		return err
	}
	page := httpx.Page[db.ListProjectsRow]{Items: items}
	if len(items) > int(limit) {
		page.Items = items[:limit]
		page.NextCursor = httpx.EncodeCursor(page.Items[limit-1].ID.String())
	}
	return httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	in, err := httpx.Decode[CreateInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.Create(r.Context(), httpx.Actor(r), in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.Get(r.Context(), httpx.Actor(r), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	version, err := httpx.IfMatch(r)
	if err != nil {
		return err
	}
	in, err := httpx.Decode[UpdateInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.Update(r.Context(), httpx.Actor(r), id, version, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[TransitionInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.Transition(r.Context(), httpx.Actor(r), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) progress(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[ProgressInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.SetProgress(r.Context(), httpx.Actor(r), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) activity(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.Activity(r.Context(), httpx.Actor(r), id, httpx.Limit(r))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.History(r.Context(), httpx.Actor(r), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.ListMembers(r.Context(), httpx.Actor(r), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) assignMember(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	userID, err := httpx.PathUUID(r, "userId")
	if err != nil {
		return err
	}
	in, err := httpx.DecodeOptional[MemberInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.AssignMember(r.Context(), httpx.Actor(r), id, userID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	userID, err := httpx.PathUUID(r, "userId")
	if err != nil {
		return err
	}
	if err := h.svc.RemoveMember(r.Context(), httpx.Actor(r), id, userID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listPhases(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	out, err := h.svc.ListPhases(r.Context(), httpx.Actor(r), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createPhase(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[PhaseInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.CreatePhase(r.Context(), httpx.Actor(r), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, out)
}

func (h *Handler) reorderPhases(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[struct {
		PhaseIDs []uuid.UUID `json:"phase_ids"`
	}](r)
	if err != nil {
		return err
	}
	out, err := h.svc.ReorderPhases(r.Context(), httpx.Actor(r), id, in.PhaseIDs)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) updatePhase(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	phaseID, err := httpx.PathUUID(r, "phaseId")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[PhaseInput](r)
	if err != nil {
		return err
	}
	out, err := h.svc.UpdatePhase(r.Context(), httpx.Actor(r), id, phaseID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) deletePhase(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	phaseID, err := httpx.PathUUID(r, "phaseId")
	if err != nil {
		return err
	}
	if err := h.svc.DeletePhase(r.Context(), httpx.Actor(r), id, phaseID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
