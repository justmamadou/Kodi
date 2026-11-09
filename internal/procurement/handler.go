package procurement

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/auth"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose les demandes de prix.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// PublicRoutes monte les routes des fournisseurs, sans session : le jeton du lien circule dans le
// corps de la requête, jamais dans l'URL (limitées en débit par l'appelant).
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Post("/supplier/rfq", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[TokenInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Portal(r.Context(), in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/supplier/offers", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		in, err := httpx.Decode[OfferInput](r)
		if err != nil {
			return err
		}
		out, created, err := h.svc.SubmitOffer(r.Context(), in)
		if err != nil {
			return err
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		return httpx.JSON(w, status, out)
	}))
}

// Routes monte les routes de l'entreprise.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/rfqs", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		project, err := httpx.QueryUUID(r, "project_id")
		if err != nil {
			return err
		}
		out, err := h.svc.List(r.Context(), httpx.Actor(r), project, httpx.QueryString(r, "status"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Post("/projects/{id}/rfqs", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[RFQInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Create(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Get("/rfqs/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Get(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Put("/rfqs/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[RFQInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Update(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/publish", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Publish(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/close", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Close(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/cancel", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[CancelInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Cancel(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/suppliers/{supplierId}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		supplierID, err := httpx.PathUUID(r, "supplierId")
		if err != nil {
			return err
		}
		out, err := h.svc.AddSupplier(r.Context(), httpx.Actor(r), id, supplierID)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Post("/rfqs/{id}/suppliers/{supplierId}/link", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		supplierID, err := httpx.PathUUID(r, "supplierId")
		if err != nil {
			return err
		}
		out, err := h.svc.RenewLink(r.Context(), httpx.Actor(r), id, supplierID)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Put("/rfqs/{id}/criteria", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[CriteriaInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.SetCriteria(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/award", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[AwardInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.Award(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Delete("/rfqs/{id}/award", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.Unaward(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/rfqs/{id}/purchase-order", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.CreateOrder(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusCreated, out)
	}))
	r.Get("/purchase-orders", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		project, err := httpx.QueryUUID(r, "project_id")
		if err != nil {
			return err
		}
		supplier, err := httpx.QueryUUID(r, "supplier_id")
		if err != nil {
			return err
		}
		out, err := h.svc.ListOrders(r.Context(), httpx.Actor(r), project, supplier, httpx.QueryString(r, "status"))
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}))
	r.Get("/purchase-orders/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.GetOrder(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Put("/purchase-orders/{id}", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[OrderInput](r)
		if err != nil {
			return err
		}
		out, err := h.svc.UpdateOrder(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/purchase-orders/{id}/submit", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.SubmitOrder(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	for path, fn := range map[string]func(*Service) func(context.Context, auth.Actor, uuid.UUID, CommentInput) (OrderDetail, error){
		"approve": func(s *Service) func(context.Context, auth.Actor, uuid.UUID, CommentInput) (OrderDetail, error) {
			return s.ApproveOrder
		},
		"return": func(s *Service) func(context.Context, auth.Actor, uuid.UUID, CommentInput) (OrderDetail, error) {
			return s.ReturnOrder
		},
		"cancel": func(s *Service) func(context.Context, auth.Actor, uuid.UUID, CommentInput) (OrderDetail, error) {
			return s.CancelOrder
		},
	} {
		action := fn(h.svc)
		r.Post("/purchase-orders/{id}/"+path, httpx.H(func(w http.ResponseWriter, r *http.Request) error {
			id, err := httpx.PathUUID(r, "id")
			if err != nil {
				return err
			}
			in, err := httpx.DecodeOptional[CommentInput](r)
			if err != nil {
				return err
			}
			out, err := action(r.Context(), httpx.Actor(r), id, in)
			if err != nil {
				return err
			}
			return httpx.JSON(w, http.StatusOK, out)
		}))
	}
	r.Post("/purchase-orders/{id}/send", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		out, err := h.svc.SendOrder(r.Context(), httpx.Actor(r), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, out)
	}))
	r.Post("/purchase-orders/{id}/receipts", httpx.H(func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathUUID(r, "id")
		if err != nil {
			return err
		}
		in, err := httpx.Decode[ReceiptInput](r)
		if err != nil {
			return err
		}
		out, created, err := h.svc.Receive(r.Context(), httpx.Actor(r), id, in)
		if err != nil {
			return err
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		return httpx.JSON(w, status, out)
	}))
}
