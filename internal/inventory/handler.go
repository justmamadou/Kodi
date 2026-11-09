package inventory

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose le catalogue et le stock.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/materials", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		out, err := h.svc.ListMaterials(r.Context(), httpx.Actor(r), httpx.QueryString(r, "q"), r.URL.Query().Get("include_archived") == "true")
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/materials", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[MaterialInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.CreateMaterial(r.Context(), httpx.Actor(r), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Patch("/materials/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[MaterialInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateMaterial(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/materials/{id}/archive", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.ArchiveMaterial(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/projects/{id}/stock-movements", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[MovementInput](r)
		if err != nil {
			return err
		}
		out, created, err := h.svc.CreateMovement(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		return httpx.JSON(w, status, out)
	}))
	r.Post("/stock-movements/{id}/post", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.PostMovement(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/projects/{id}/stock", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Stock(r.Context(), httpx.Actor(r), id, httpx.QueryString(r, "q"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Get("/projects/{id}/stock/history", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		material, err := httpx.QueryUUID(r, "material_id")
		if err != nil {
			return err
		}
		from, err := httpx.QueryDate(r, "date_from")
		if err != nil {
			return err
		}
		to, err := httpx.QueryDate(r, "date_to")
		if err != nil {
			return err
		}
		offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)
		if err != nil || offset < 0 {
			offset = 0
		}
		out, err := h.svc.History(r.Context(), httpx.Actor(r), id, HistoryFilter{
			MaterialID: material, MovementType: httpx.QueryString(r, "movement_type"),
			DateFrom: from, DateTo: to, Limit: httpx.Limit(r), Offset: int32(offset),
		})
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Get("/projects/{id}/stock/reconciliation", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Reconciliation(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
}
