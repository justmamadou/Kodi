package fieldops

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose les tâches et rapports.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{id}/tasks", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		assignee, err := httpx.QueryUUID(r, "assignee_id")
		if err != nil {
			return err
		}
		out, err := h.svc.ListTasks(r.Context(), httpx.Actor(r), id, TaskFilter{Status: httpx.QueryString(r, "status"), AssigneeID: assignee})
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/projects/{id}/tasks", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[TaskInput](r)
		if err != nil {
			return err
		}
		out, created, err := h.svc.CreateTask(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		return httpx.JSON(w, status, out)
	}))
	r.Get("/tasks/{id}", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.GetTask(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Patch("/tasks/{id}", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		version, err := httpx.IfMatch(r)
		if err != nil {
			return err
		}
		in, err := httpx.Decode[TaskUpdateInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateTask(r.Context(), httpx.Actor(r), id, version, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/tasks/{id}/progress", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[ProgressInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateProgress(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))

	r.Get("/projects/{id}/daily-reports", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		from, err := httpx.QueryDate(r, "date_from")
		if err != nil {
			return err
		}
		to, err := httpx.QueryDate(r, "date_to")
		if err != nil {
			return err
		}
		out, err := h.svc.ListReports(r.Context(), httpx.Actor(r), id, from, to)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Put("/daily-reports/{id}", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		// If-Match est facultatif à la création, mais protège toute mise à jour d'un écrasement silencieux.
		var base *int32
		if raw := strings.Trim(r.Header.Get("If-Match"), `"`); raw != "" {
			n, err := strconv.ParseInt(raw, 10, 32)
			if err != nil {
				return apperr.New(http.StatusBadRequest, "request.invalid_if_match", "En-tête If-Match invalide.")
			}
			v := int32(n)
			base = &v
		}
		in, err := httpx.Decode[ReportInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.SaveReport(r.Context(), httpx.Actor(r), id, base, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/daily-reports/{id}", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.GetReport(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/daily-reports/{id}/submit", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.SubmitReport(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/daily-reports/{id}/reopen", h.withID(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[ReopenInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.ReopenReport(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
}

func (h *Handler) withID(fn func(http.ResponseWriter, *http.Request, uuid.UUID) error) http.HandlerFunc {
	return httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		return fn(w, r, id)
	})
}
