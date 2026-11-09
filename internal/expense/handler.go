package expense

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/httpx"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// Handler expose les dépenses.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/expenses", httpx.H(h.list))
	r.Get("/expenses/counts", httpx.H(h.counts))
	r.Post("/projects/{id}/expenses", httpx.H(h.create))
	r.Get("/expenses/{id}", h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Get(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Patch("/expenses/{id}", h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
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
	}))
	r.Post("/expenses/{id}/submit", h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.Submit(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/expenses/{id}/approve", h.decision(h.svc.Approve))
	r.Post("/expenses/{id}/reject", h.decision(h.svc.Reject))
	r.Post("/expenses/{id}/cancel", h.decision(h.svc.Cancel))
	r.Post("/expenses/{id}/back-to-draft", h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		out, err := h.svc.BackToDraft(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/expenses/{id}/payments", h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.Decode[PaymentInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.AddPayment(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
}

func (h *Handler) action(fn func(http.ResponseWriter, *http.Request, uuid.UUID) error) http.HandlerFunc {
	return httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		return fn(w, r, id)
	})
}

func (h *Handler) decision(fn decisionFunc) http.HandlerFunc {
	return h.action(func(w http.ResponseWriter, r *http.Request, id uuid.UUID) error {
		in, err := httpx.DecodeOptional[DecisionInput](r)
		if err != nil {
			return err
		}
		out, err := fn(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	in, err := httpx.Decode[CreateInput](r)
	if err != nil {
		return err
	}
	out, created, err := h.svc.Create(r.Context(), httpx.Actor(r), projectID, in)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return httpx.JSON(w, status, out)
}

func (h *Handler) counts(w http.ResponseWriter, r *http.Request) error {
	f, err := filterFrom(r)
	if err != nil {
		return err
	}
	out, err := h.svc.Count(r.Context(), httpx.Actor(r), f)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, out)
}

func filterFrom(r *http.Request) (ListFilter, error) {
	projectID, err := httpx.QueryUUID(r, "project_id")
	if err != nil {
		return ListFilter{}, err
	}
	categoryID, err := httpx.QueryUUID(r, "category_id")
	if err != nil {
		return ListFilter{}, err
	}
	dateFrom, err := httpx.QueryDate(r, "date_from")
	if err != nil {
		return ListFilter{}, err
	}
	dateTo, err := httpx.QueryDate(r, "date_to")
	if err != nil {
		return ListFilter{}, err
	}
	return ListFilter{
		ProjectID: projectID, Status: httpx.QueryString(r, "status"), Payment: httpx.QueryString(r, "payment"),
		CategoryID: categoryID, DateFrom: dateFrom, DateTo: dateTo, Q: httpx.QueryString(r, "q"),
	}, nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	f, err := filterFrom(r)
	if err != nil {
		return err
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parts, err := httpx.DecodeCursor(raw, 2)
		if err != nil {
			return err
		}
		d, errDate := civil.Parse(parts[0])
		id, errID := uuid.Parse(parts[1])
		if errDate != nil || errID != nil {
			return apperr.New(http.StatusBadRequest, "request.invalid_cursor", "Curseur de pagination invalide.")
		}
		f.CursorDate, f.CursorID = &d, &id
	}
	limit := httpx.Limit(r)
	f.Limit = limit + 1
	items, err := h.svc.List(r.Context(), httpx.Actor(r), f)
	if err != nil {
		return err
	}
	page := httpx.Page[db.ListExpensesRow]{Items: items}
	if len(items) > int(limit) {
		page.Items = items[:limit]
		last := page.Items[limit-1]
		page.NextCursor = httpx.EncodeCursor(last.ExpenseDate.String(), last.ID.String())
	}
	return httpx.JSON(w, http.StatusOK, page)
}
