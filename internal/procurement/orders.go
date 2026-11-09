package procurement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
	"github.com/shopspring/decimal"
)

// Statuts d'un bon de commande, identiques à la contrainte CHECK de la base.
const (
	OrderDraft             = "draft"
	OrderSubmitted         = "submitted"
	OrderApproved          = "approved"
	OrderSent              = "sent"
	OrderPartiallyReceived = "partially_received"
	OrderReceived          = "received"
	OrderCancelled         = "cancelled"
)

// OrderReference est le numéro affiché d'un bon de commande : BC-0001.
func OrderReference(number int32) string { return fmt.Sprintf("BC-%04d", number) }

// OrderLine est une ligne de commande. Les prix restent vides pour qui ne voit pas les prix des
// fournisseurs (chef de chantier) : il réceptionne des quantités.
type OrderLine struct {
	ID                uuid.UUID        `json:"id"`
	Position          int16            `json:"position"`
	RFQItemID         *uuid.UUID       `json:"rfq_item_id"`
	MaterialID        *uuid.UUID       `json:"material_id"`
	Label             string           `json:"label"`
	Unit              string           `json:"unit"`
	QuantityOrdered   decimal.Decimal  `json:"quantity_ordered"`
	QuantityReceived  decimal.Decimal  `json:"quantity_received"`
	QuantityRemaining decimal.Decimal  `json:"quantity_remaining"`
	UnitPrice         *int64           `json:"unit_price"`
	TaxRate           *decimal.Decimal `json:"tax_rate"`
	Amount            *int64           `json:"amount"`
	TaxAmount         *int64           `json:"tax_amount"`
	Comment           *string          `json:"comment"`
}

// Receipt est une réception et ses quantités par ligne.
type Receipt struct {
	db.ListPOReceiptsRow
	Lines []db.PurchaseOrderReceiptLine `json:"lines"`
}

// OrderDetail est un bon de commande complet : lignes, réceptions et historique.
type OrderDetail struct {
	ID                   uuid.UUID            `json:"id"`
	Number               int32                `json:"number"`
	Reference            string               `json:"reference"`
	Status               string               `json:"status"`
	ProjectID            uuid.UUID            `json:"project_id"`
	ProjectName          string               `json:"project_name"`
	SupplierID           uuid.UUID            `json:"supplier_id"`
	SupplierName         string               `json:"supplier_name"`
	SupplierEmail        *string              `json:"supplier_email"`
	SupplierPhone        *string              `json:"supplier_phone"`
	RFQID                *uuid.UUID           `json:"rfq_id"`
	RFQReference         *string              `json:"rfq_reference"`
	OfferID              *uuid.UUID           `json:"offer_id"`
	Currency             string               `json:"currency"`
	ExpectedDeliveryDate *civil.Date          `json:"expected_delivery_date"`
	DeliveryAddress      *string              `json:"delivery_address"`
	PaymentTerms         *string              `json:"payment_terms"`
	Notes                *string              `json:"notes"`
	SubtotalAmount       *int64               `json:"subtotal_amount"`
	TaxAmount            *int64               `json:"tax_amount"`
	TotalAmount          *int64               `json:"total_amount"`
	CreatedBy            uuid.UUID            `json:"created_by"`
	CreatedByName        string               `json:"created_by_name"`
	CreatedAt            time.Time            `json:"created_at"`
	SubmittedAt          *time.Time           `json:"submitted_at"`
	ApprovedBy           *uuid.UUID           `json:"approved_by"`
	ApprovedAt           *time.Time           `json:"approved_at"`
	SentAt               *time.Time           `json:"sent_at"`
	CancelledAt          *time.Time           `json:"cancelled_at"`
	CancelReason         *string              `json:"cancel_reason"`
	Lines                []OrderLine          `json:"lines"`
	Receipts             []Receipt            `json:"receipts"`
	Events               []db.ListPOEventsRow `json:"events"`
	PricesVisible        bool                 `json:"prices_visible"`
}

// OrderSummary est une ligne de la liste des commandes.
type OrderSummary struct {
	db.ListPurchaseOrdersRow
	Reference   string `json:"reference"`
	TotalAmount *int64 `json:"total_amount"`
}

func pricesVisible(a auth.Actor) bool { return rbac.Can(a.Role, rbac.RFQView) }

func dakarDeliveryPlace(address string, city *string) *string {
	place := deliveryPlace(address, city)
	return &place
}

// loadOrder charge une commande ; l'accès au chantier est vérifié par l'appelant.
func loadOrder(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (OrderDetail, error) {
	po, err := q.GetPO(ctx, db.GetPOParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return OrderDetail{}, apperr.NotFound("purchase_order")
		}
		return OrderDetail{}, err
	}
	prices := pricesVisible(a)
	out := OrderDetail{
		ID: po.ID, Number: po.Number, Reference: OrderReference(po.Number), Status: po.Status, ProjectID: po.ProjectID,
		ProjectName: po.ProjectName, SupplierID: po.SupplierID, SupplierName: po.SupplierName, SupplierEmail: po.SupplierEmail,
		SupplierPhone: po.SupplierPhone, RFQID: po.RfqID, OfferID: po.OfferID, Currency: po.Currency,
		ExpectedDeliveryDate: po.ExpectedDeliveryDate, DeliveryAddress: po.DeliveryAddress, Notes: po.Notes,
		CreatedBy: po.CreatedBy, CreatedByName: po.CreatedByName, CreatedAt: po.CreatedAt, SubmittedAt: po.SubmittedAt,
		ApprovedBy: po.ApprovedBy, ApprovedAt: po.ApprovedAt, SentAt: po.SentAt, CancelledAt: po.CancelledAt,
		CancelReason: po.CancelReason, PricesVisible: prices,
	}
	if po.RfqNumber != nil {
		ref := Reference(*po.RfqNumber)
		out.RFQReference = &ref
	}
	if prices {
		out.SubtotalAmount, out.TaxAmount, out.TotalAmount = &po.SubtotalAmount, &po.TaxAmount, &po.TotalAmount
		out.PaymentTerms = po.PaymentTerms
	}
	lines, err := q.ListPOLines(ctx, db.ListPOLinesParams{OrgID: a.OrgID, PurchaseOrderID: id})
	if err != nil {
		return out, err
	}
	out.Lines = make([]OrderLine, len(lines))
	for i, l := range lines {
		ol := OrderLine{
			ID: l.ID, Position: l.Position, RFQItemID: l.RfqItemID, MaterialID: l.MaterialID, Label: l.Label, Unit: l.Unit,
			QuantityOrdered: l.QuantityOrdered, QuantityReceived: l.QuantityReceived, Comment: l.Comment,
		}
		if l.QuantityRemaining != nil {
			ol.QuantityRemaining = *l.QuantityRemaining
		}
		if prices {
			price, rate := l.UnitPrice, l.TaxRate
			ol.UnitPrice, ol.TaxRate, ol.Amount, ol.TaxAmount = &price, &rate, l.Amount, l.TaxAmount
		}
		out.Lines[i] = ol
	}
	receipts, err := q.ListPOReceipts(ctx, db.ListPOReceiptsParams{OrgID: a.OrgID, PurchaseOrderID: id})
	if err != nil {
		return out, err
	}
	receiptLines, err := q.ListPOReceiptLines(ctx, db.ListPOReceiptLinesParams{OrgID: a.OrgID, PurchaseOrderID: id})
	if err != nil {
		return out, err
	}
	out.Receipts = make([]Receipt, len(receipts))
	for i, r := range receipts {
		out.Receipts[i] = Receipt{ListPOReceiptsRow: r, Lines: []db.PurchaseOrderReceiptLine{}}
		for _, rl := range receiptLines {
			if rl.ReceiptID == r.ID {
				out.Receipts[i].Lines = append(out.Receipts[i].Lines, rl)
			}
		}
	}
	if out.Events, err = q.ListPOEvents(ctx, db.ListPOEventsParams{OrgID: a.OrgID, PurchaseOrderID: id}); err != nil {
		return out, err
	}
	if !prices {
		// L'historique garde les montants dans ses changements : on ne les montre qu'aux rôles qui voient les prix.
		for i := range out.Events {
			out.Events[i].Changes = nil
		}
	}
	return out, nil
}

// event écrit un événement de la commande et l'entrée d'audit correspondante.
func event(ctx context.Context, q *db.Queries, a auth.Actor, po db.PurchaseOrder, name, from, to string, comment *string, changes any) error {
	var raw []byte
	if changes != nil {
		var err error
		if raw, err = json.Marshal(changes); err != nil {
			return err
		}
	}
	var fromPtr *string
	if from != "" {
		fromPtr = &from
	}
	if err := q.InsertPOEvent(ctx, db.InsertPOEventParams{
		OrgID: a.OrgID, PurchaseOrderID: po.ID, Event: name, FromStatus: fromPtr, ToStatus: to, ActorID: a.UserID, Comment: comment, Changes: raw,
	}); err != nil {
		return err
	}
	return audit.Record(ctx, q, a, audit.Entry{
		Action: "purchase_order." + name, EntityType: "purchase_order", EntityID: &po.ID, ProjectID: &po.ProjectID,
		Before: map[string]any{"status": from}, After: map[string]any{"status": to, "comment": comment, "changes": changes},
	})
}

// lockOrder verrouille la commande et vérifie l'accès à son chantier (écriture).
func lockOrder(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.PurchaseOrder, error) {
	po, err := q.LockPO(ctx, db.LockPOParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return po, apperr.NotFound("purchase_order")
		}
		return po, err
	}
	_, err = rbac.ProjectWritable(ctx, q, a, po.ProjectID)
	return po, err
}

// CreateOrder génère un bon de commande en brouillon à partir de l'offre retenue d'une demande de prix.
func (s *Service) CreateOrder(ctx context.Context, a auth.Actor, rfqID uuid.UUID) (OrderDetail, error) {
	if err := rbac.Require(a, rbac.POManage); err != nil {
		return OrderDetail{}, err
	}
	poID, err := uuid.NewV7()
	if err != nil {
		return OrderDetail{}, err
	}
	var out OrderDetail
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, rfqID)
		if err != nil {
			return err
		}
		if rfq.AwardedOfferID == nil {
			return apperr.Conflict("rfq.not_awarded", "Retenez d'abord une offre pour générer la commande.")
		}
		if err := noActiveOrder(ctx, q, a.OrgID, rfqID); err != nil {
			return err
		}
		detail, err := load(ctx, q, a.OrgID, rfqID)
		if err != nil {
			return err
		}
		var offer *Offer
		for i := range detail.Offers {
			if detail.Offers[i].ID == *rfq.AwardedOfferID {
				offer = &detail.Offers[i]
			}
		}
		if offer == nil {
			return apperr.NotFound("offer")
		}
		if _, err := activeSupplier(ctx, q, a.OrgID, offer.SupplierID); err != nil {
			return apperr.Conflict("supplier.archived", "Le fournisseur retenu est archivé : restaurez-le pour lui passer commande.")
		}
		expected := rfq.DeliveryDate
		if expected == nil && offer.DeliveryDays != nil {
			today, err := q.TodayInDakar(ctx)
			if err != nil {
				return err
			}
			d := civil.Date{Time: today.AddDate(0, 0, int(*offer.DeliveryDays))}
			expected = &d
		}
		if err := q.LockPONumbering(ctx, a.OrgID); err != nil {
			return err
		}
		_, err = q.CreatePurchaseOrder(ctx, db.CreatePurchaseOrderParams{
			ID: poID, OrgID: a.OrgID, ProjectID: rfq.ProjectID, SupplierID: offer.SupplierID, RfqID: &rfqID, OfferID: &offer.ID,
			ExpectedDeliveryDate: expected, DeliveryAddress: dakarDeliveryPlace(detail.ProjectAddress, detail.ProjectCity),
			PaymentTerms: offer.PaymentTerms, CreatedBy: a.UserID,
		})
		if err != nil {
			if apperr.Normalize(err).Code == "resource.duplicate" {
				return apperr.Conflict("rfq.order_exists", "Une commande découle déjà de cette offre.")
			}
			return err
		}
		items := map[uuid.UUID]db.RfqItem{}
		for _, item := range detail.Items {
			items[item.ID] = item
		}
		lines := slices.Clone(offer.Lines)
		sort.Slice(lines, func(i, j int) bool { return items[lines[i].RfqItemID].Position < items[lines[j].RfqItemID].Position })
		for i, l := range lines {
			item := items[l.RfqItemID]
			lineID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			if err := q.InsertPOLine(ctx, db.InsertPOLineParams{
				ID: lineID, OrgID: a.OrgID, PurchaseOrderID: poID, Position: int16(i + 1), RfqItemID: &item.ID, MaterialID: item.MaterialID,
				Label: item.Label, Unit: item.Unit, QuantityOrdered: item.Quantity, UnitPrice: l.UnitPrice, TaxRate: decimal.Zero, Comment: l.Comment,
			}); err != nil {
				return err
			}
		}
		po, err := q.RecomputePOTotals(ctx, db.RecomputePOTotalsParams{OrgID: a.OrgID, ID: poID})
		if err != nil {
			return err
		}
		if err := event(ctx, q, a, po, "created", "", OrderDraft, nil, map[string]any{
			"rfq": Reference(rfq.Number), "offer_id": offer.ID, "lines": len(lines), "total_amount": po.TotalAmount,
		}); err != nil {
			return err
		}
		out, err = loadOrder(ctx, q, a, poID)
		return err
	})
	return out, err
}

// OrderLineInput est une ligne de commande en brouillon.
type OrderLineInput struct {
	RFQItemID  *uuid.UUID      `json:"rfq_item_id"`
	MaterialID *uuid.UUID      `json:"material_id"`
	Label      string          `json:"label"`
	Unit       string          `json:"unit"`
	Quantity   decimal.Decimal `json:"quantity"`
	UnitPrice  int64           `json:"unit_price"`
	TaxRate    decimal.Decimal `json:"tax_rate"`
	Comment    *string         `json:"comment"`
}

// OrderInput remplace le contenu d'un brouillon de commande.
type OrderInput struct {
	ExpectedDeliveryDate *civil.Date      `json:"expected_delivery_date"`
	DeliveryAddress      *string          `json:"delivery_address"`
	PaymentTerms         *string          `json:"payment_terms"`
	Notes                *string          `json:"notes"`
	Lines                []OrderLineInput `json:"lines"`
}

// Validate applique les règles d'une commande en brouillon.
func (in *OrderInput) Validate() error {
	in.DeliveryAddress, in.PaymentTerms, in.Notes = trimmed(in.DeliveryAddress), trimmed(in.PaymentTerms), trimmed(in.Notes)
	var v apperr.Validator
	v.Check(len(in.Lines) > 0, "lines", "Une commande compte au moins une ligne.")
	v.Check(len(in.Lines) <= maxItems, "lines", "Une commande compte au plus 200 lignes.")
	v.Check(in.DeliveryAddress == nil || len(*in.DeliveryAddress) <= 300, "delivery_address", "Adresse trop longue (300 caractères au plus).")
	v.Check(in.PaymentTerms == nil || len(*in.PaymentTerms) <= 500, "payment_terms", "Conditions trop longues (500 caractères au plus).")
	v.Check(in.Notes == nil || len(*in.Notes) <= 2000, "notes", "Notes trop longues (2 000 caractères au plus).")
	for i := range in.Lines {
		l := &in.Lines[i]
		l.Label = strings.TrimSpace(l.Label)
		l.Comment = trimmed(l.Comment)
		f := fmt.Sprintf("lines.%d", i)
		v.Check(len(l.Label) >= 2 && len(l.Label) <= 200, f+".label", "La désignation doit contenir entre 2 et 200 caractères.")
		v.Check(slices.Contains(Units, l.Unit), f+".unit", "Unité inconnue.")
		v.Check(l.Quantity.IsPositive() && l.Quantity.Equal(l.Quantity.Truncate(3)) && l.Quantity.LessThan(decimal.New(1, 11)), f+".quantity", "Quantité positive, 3 décimales au plus.")
		v.Check(l.UnitPrice >= 0 && l.UnitPrice < maxUnitPrice, f+".unit_price", "Prix unitaire invalide.")
		v.Check(!l.TaxRate.IsNegative() && l.TaxRate.LessThanOrEqual(decimal.NewFromInt(100)) && l.TaxRate.Equal(l.TaxRate.Truncate(2)), f+".tax_rate", "Taux de taxe entre 0 et 100 %, 2 décimales au plus.")
		v.Check(l.Comment == nil || len(*l.Comment) <= 500, f+".comment", "Commentaire trop long (500 caractères au plus).")
	}
	return v.Err()
}

// UpdateOrder remplace le contenu d'un brouillon ; l'ancienne et la nouvelle version sont historisées.
func (s *Service) UpdateOrder(ctx context.Context, a auth.Actor, id uuid.UUID, in OrderInput) (OrderDetail, error) {
	if err := rbac.Require(a, rbac.POManage); err != nil {
		return OrderDetail{}, err
	}
	if err := in.Validate(); err != nil {
		return OrderDetail{}, err
	}
	var out OrderDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := lockOrder(ctx, q, a, id)
		if err != nil {
			return err
		}
		if before.Status != OrderDraft {
			return apperr.Conflict("purchase_order.not_draft", "Seul un brouillon de commande se modifie.")
		}
		oldLines, err := q.ListPOLines(ctx, db.ListPOLinesParams{OrgID: a.OrgID, PurchaseOrderID: id})
		if err != nil {
			return err
		}
		for i, l := range in.Lines {
			if l.MaterialID == nil {
				continue
			}
			mat, err := q.GetActiveMaterial(ctx, db.GetActiveMaterialParams{OrgID: a.OrgID, ID: *l.MaterialID})
			if err != nil {
				if db.IsNoRows(err) {
					return apperr.New(http.StatusUnprocessableEntity, "material.not_found", fmt.Sprintf("Ligne %d : cet article n'est pas (ou plus) au catalogue.", i+1))
				}
				return err
			}
			if mat.Unit != l.Unit {
				return apperr.New(http.StatusUnprocessableEntity, "material.unit_mismatch",
					fmt.Sprintf("Ligne %d : l'unité doit être celle de l'article du catalogue (%s) pour entrer en stock.", i+1, mat.Unit))
			}
		}
		if _, err := q.UpdatePODraft(ctx, db.UpdatePODraftParams{
			OrgID: a.OrgID, ID: id, ExpectedDeliveryDate: in.ExpectedDeliveryDate, DeliveryAddress: in.DeliveryAddress,
			PaymentTerms: in.PaymentTerms, Notes: in.Notes,
		}); err != nil {
			return err
		}
		if err := q.DeletePOLines(ctx, db.DeletePOLinesParams{OrgID: a.OrgID, PurchaseOrderID: id}); err != nil {
			return err
		}
		for i, l := range in.Lines {
			lineID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			if err := q.InsertPOLine(ctx, db.InsertPOLineParams{
				ID: lineID, OrgID: a.OrgID, PurchaseOrderID: id, Position: int16(i + 1), RfqItemID: l.RFQItemID, MaterialID: l.MaterialID,
				Label: l.Label, Unit: l.Unit, QuantityOrdered: l.Quantity, UnitPrice: l.UnitPrice, TaxRate: l.TaxRate, Comment: l.Comment,
			}); err != nil {
				return err
			}
		}
		after, err := q.RecomputePOTotals(ctx, db.RecomputePOTotalsParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		changes := map[string]any{
			"before": map[string]any{"total_amount": before.TotalAmount, "tax_amount": before.TaxAmount, "lines": lineSummary(oldLines),
				"expected_delivery_date": before.ExpectedDeliveryDate, "payment_terms": before.PaymentTerms},
			"after": map[string]any{"total_amount": after.TotalAmount, "tax_amount": after.TaxAmount, "lines": inputSummary(in.Lines),
				"expected_delivery_date": after.ExpectedDeliveryDate, "payment_terms": after.PaymentTerms},
		}
		if err := event(ctx, q, a, after, "updated", OrderDraft, OrderDraft, nil, changes); err != nil {
			return err
		}
		out, err = loadOrder(ctx, q, a, id)
		return err
	})
	return out, err
}

func lineSummary(lines []db.PurchaseOrderLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fmt.Sprintf("%s : %s %s × %d (taxe %s %%)", l.Label, l.QuantityOrdered, l.Unit, l.UnitPrice, l.TaxRate)
	}
	return out
}

func inputSummary(lines []OrderLineInput) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = fmt.Sprintf("%s : %s %s × %d (taxe %s %%)", l.Label, l.Quantity, l.Unit, l.UnitPrice, l.TaxRate)
	}
	return out
}

// CommentInput porte un commentaire facultatif (approbation) ou obligatoire (renvoi, annulation).
type CommentInput struct {
	Comment *string `json:"comment"`
}

func checkComment(c *string, required bool, field string) (*string, error) {
	c = trimmed(c)
	var v apperr.Validator
	v.Check(!required || c != nil, field, "Indiquez le motif.")
	v.Check(c == nil || len(*c) <= 1000, field, "Texte trop long (1 000 caractères au plus).")
	return c, v.Err()
}

// transition applique une étape du workflow sous verrou, avec son événement.
func (s *Service) transition(ctx context.Context, a auth.Actor, id uuid.UUID, perm rbac.Permission, name string,
	apply func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error), comment *string) (OrderDetail, db.PurchaseOrder, error) {
	if err := rbac.Require(a, perm); err != nil {
		return OrderDetail{}, db.PurchaseOrder{}, err
	}
	var out OrderDetail
	var after db.PurchaseOrder
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := lockOrder(ctx, q, a, id)
		if err != nil {
			return err
		}
		if after, err = apply(q, before); err != nil {
			return err
		}
		if err := event(ctx, q, a, after, name, before.Status, after.Status, comment, nil); err != nil {
			return err
		}
		out, err = loadOrder(ctx, q, a, id)
		return err
	})
	return out, after, err
}

func wrongStatus(want string) error {
	return apperr.Conflict("purchase_order.invalid_status", want)
}

// SubmitOrder soumet le brouillon à l'approbation.
func (s *Service) SubmitOrder(ctx context.Context, a auth.Actor, id uuid.UUID) (OrderDetail, error) {
	out, _, err := s.transition(ctx, a, id, rbac.POManage, "submitted", func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error) {
		if po.Status != OrderDraft {
			return po, wrongStatus("Seul un brouillon peut être soumis.")
		}
		return q.SubmitPO(ctx, db.SubmitPOParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
	}, nil)
	return out, err
}

// ApproveOrder approuve une commande soumise. Son auteur ne peut pas l'approuver.
func (s *Service) ApproveOrder(ctx context.Context, a auth.Actor, id uuid.UUID, in CommentInput) (OrderDetail, error) {
	comment, err := checkComment(in.Comment, false, "comment")
	if err != nil {
		return OrderDetail{}, err
	}
	out, _, err := s.transition(ctx, a, id, rbac.POApprove, "approved", func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error) {
		if po.Status != OrderSubmitted {
			return po, wrongStatus("Seule une commande soumise peut être approuvée.")
		}
		if po.CreatedBy == a.UserID {
			return po, apperr.New(http.StatusForbidden, "purchase_order.self_approval", "Vous ne pouvez pas approuver une commande que vous avez préparée.")
		}
		return q.ApprovePO(ctx, db.ApprovePOParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
	}, comment)
	return out, err
}

// ReturnOrder renvoie une commande soumise en brouillon, avec un motif.
func (s *Service) ReturnOrder(ctx context.Context, a auth.Actor, id uuid.UUID, in CommentInput) (OrderDetail, error) {
	comment, err := checkComment(in.Comment, true, "comment")
	if err != nil {
		return OrderDetail{}, err
	}
	out, _, err := s.transition(ctx, a, id, rbac.POApprove, "returned", func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error) {
		if po.Status != OrderSubmitted {
			return po, wrongStatus("Seule une commande soumise peut être renvoyée en brouillon.")
		}
		return q.ReturnPO(ctx, db.ReturnPOParams{OrgID: a.OrgID, ID: id})
	}, comment)
	return out, err
}

// SendResult indique si le bon de commande est parti par email.
type SendResult struct {
	Order   OrderDetail `json:"order"`
	Emailed bool        `json:"emailed"`
}

// SendOrder marque la commande approuvée comme envoyée et l'adresse au fournisseur par email s'il en a un.
func (s *Service) SendOrder(ctx context.Context, a auth.Actor, id uuid.UUID) (SendResult, error) {
	out, _, err := s.transition(ctx, a, id, rbac.POManage, "sent", func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error) {
		if po.Status != OrderApproved {
			return po, wrongStatus("Seule une commande approuvée peut être envoyée au fournisseur.")
		}
		return q.SendPO(ctx, db.SendPOParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
	}, nil)
	if err != nil {
		return SendResult{}, err
	}
	res := SendResult{Order: out}
	if out.SupplierEmail == nil {
		return res, nil
	}
	// Après le commit, comme les liens de consultation : l'envoi n'est pas fiable, la commande reste envoyée.
	var orgName string
	if err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		orgName, err = q.GetOrganizationName(ctx, a.OrgID)
		return err
	}); err != nil {
		slog.ErrorContext(ctx, "nom de l'entreprise introuvable pour l'envoi de la commande", "error", err)
		return res, nil
	}
	full := out
	if !out.PricesVisible { // l'email porte toujours les prix
		full, err = s.orderForEmail(ctx, a, id)
		if err != nil {
			return res, nil
		}
	}
	if err := s.mail.Send(ctx, *out.SupplierEmail, "Bon de commande "+out.Reference+" — "+orgName, orderEmail(orgName, full)); err != nil {
		slog.ErrorContext(ctx, "envoi du bon de commande en échec", "error", err, "purchase_order_id", id)
		return res, nil
	}
	res.Emailed = true
	return res, nil
}

func (s *Service) orderForEmail(ctx context.Context, a auth.Actor, id uuid.UUID) (OrderDetail, error) {
	owner := a
	owner.Role = rbac.RoleOwner
	var out OrderDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = loadOrder(ctx, q, owner, id)
		return err
	})
	return out, err
}

func orderEmail(orgName string, o OrderDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Bonjour,\n\n%s vous adresse le bon de commande %s.\n\n", orgName, o.Reference)
	if o.DeliveryAddress != nil {
		fmt.Fprintf(&b, "Livraison : %s\n", *o.DeliveryAddress)
	}
	if o.ExpectedDeliveryDate != nil {
		fmt.Fprintf(&b, "Date de livraison souhaitée : %s\n", o.ExpectedDeliveryDate.Format("02/01/2006"))
	}
	b.WriteString("\n")
	for _, l := range o.Lines {
		fmt.Fprintf(&b, "- %s : %s %s", l.Label, l.QuantityOrdered.String(), l.Unit)
		if l.UnitPrice != nil && l.Amount != nil {
			fmt.Fprintf(&b, " × %d FCFA = %d FCFA HT", *l.UnitPrice, *l.Amount)
			if l.TaxRate != nil && l.TaxRate.IsPositive() {
				fmt.Fprintf(&b, " (taxe %s %%)", l.TaxRate.String())
			}
		}
		b.WriteString("\n")
	}
	if o.SubtotalAmount != nil && o.TaxAmount != nil && o.TotalAmount != nil {
		fmt.Fprintf(&b, "\nTotal HT : %d FCFA\nTaxes : %d FCFA\nTotal TTC : %d FCFA\n", *o.SubtotalAmount, *o.TaxAmount, *o.TotalAmount)
	}
	if o.PaymentTerms != nil {
		fmt.Fprintf(&b, "\nConditions de paiement : %s\n", *o.PaymentTerms)
	}
	if o.Notes != nil {
		fmt.Fprintf(&b, "\n%s\n", *o.Notes)
	}
	b.WriteString("\nMerci de rappeler cette référence sur votre bon de livraison.\n")
	return b.String()
}

// CancelOrder annule une commande tant que rien n'a été reçu.
func (s *Service) CancelOrder(ctx context.Context, a auth.Actor, id uuid.UUID, in CommentInput) (OrderDetail, error) {
	reason, err := checkComment(in.Comment, true, "comment")
	if err != nil {
		return OrderDetail{}, err
	}
	out, _, err := s.transition(ctx, a, id, rbac.POManage, "cancelled", func(q *db.Queries, po db.PurchaseOrder) (db.PurchaseOrder, error) {
		switch po.Status {
		case OrderDraft, OrderSubmitted, OrderApproved, OrderSent:
			return q.CancelPO(ctx, db.CancelPOParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID, Reason: reason})
		case OrderPartiallyReceived, OrderReceived:
			return po, wrongStatus("Une commande déjà réceptionnée, même en partie, ne s'annule plus.")
		}
		return po, wrongStatus("Cette commande est déjà annulée.")
	}, reason)
	return out, err
}

// GetOrder retourne une commande ; les prix sont masqués aux rôles qui ne les voient pas.
func (s *Service) GetOrder(ctx context.Context, a auth.Actor, id uuid.UUID) (OrderDetail, error) {
	if err := rbac.Require(a, rbac.POView); err != nil {
		return OrderDetail{}, err
	}
	var out OrderDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		if out, err = loadOrder(ctx, q, a, id); err != nil {
			return err
		}
		_, err = rbac.Project(ctx, q, a, out.ProjectID)
		return err
	})
	return out, err
}

// ListOrders liste les commandes des chantiers visibles.
func (s *Service) ListOrders(ctx context.Context, a auth.Actor, projectID, supplierID *uuid.UUID, status *string) ([]OrderSummary, error) {
	if err := rbac.Require(a, rbac.POView); err != nil {
		return nil, err
	}
	var rows []db.ListPurchaseOrdersRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		rows, err = q.ListPurchaseOrders(ctx, db.ListPurchaseOrdersParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID, ProjectID: projectID, SupplierID: supplierID, Status: status,
		})
		return err
	})
	prices := pricesVisible(a)
	out := make([]OrderSummary, len(rows))
	for i, r := range rows {
		out[i] = OrderSummary{ListPurchaseOrdersRow: r, Reference: OrderReference(r.Number)}
		if prices {
			total := r.TotalAmount
			out[i].TotalAmount = &total
		}
	}
	return out, err
}

// ReceiptLineInput est une quantité reçue sur une ligne de commande.
type ReceiptLineInput struct {
	LineID   uuid.UUID       `json:"line_id"`
	Quantity decimal.Decimal `json:"quantity"`
}

// ReceiptInput est une réception. L'identifiant, généré par l'appareil, rend le renvoi sans effet.
type ReceiptInput struct {
	ID              *uuid.UUID         `json:"id"`
	ReceivedOn      *civil.Date        `json:"received_on"`
	DeliveryNoteRef *string            `json:"delivery_note_ref"`
	Notes           *string            `json:"notes"`
	ToStock         bool               `json:"to_stock"`
	Lines           []ReceiptLineInput `json:"lines"`
}

// Receive enregistre une réception, partielle ou complète, et l'entre en stock si demandé : les lignes
// rattachées au catalogue alimentent un mouvement d'entrée posté sur le chantier.
func (s *Service) Receive(ctx context.Context, a auth.Actor, id uuid.UUID, in ReceiptInput) (OrderDetail, bool, error) {
	if err := rbac.Require(a, rbac.POReceive); err != nil {
		return OrderDetail{}, false, err
	}
	in.DeliveryNoteRef, in.Notes = trimmed(in.DeliveryNoteRef), trimmed(in.Notes)
	var v apperr.Validator
	v.Check(in.ReceivedOn != nil, "received_on", "La date de réception est obligatoire.")
	v.Check(in.ReceivedOn == nil || !in.ReceivedOn.After(time.Now().AddDate(0, 0, 1)), "received_on", "La date ne peut pas être dans le futur.")
	v.Check(in.DeliveryNoteRef == nil || len(*in.DeliveryNoteRef) <= 60, "delivery_note_ref", "Référence trop longue (60 caractères au plus).")
	v.Check(in.Notes == nil || len(*in.Notes) <= 1000, "notes", "Notes trop longues (1 000 caractères au plus).")
	v.Check(len(in.Lines) > 0, "lines", "Indiquez au moins une quantité reçue.")
	seen := map[uuid.UUID]bool{}
	for i, l := range in.Lines {
		f := fmt.Sprintf("lines.%d.quantity", i)
		v.Check(!seen[l.LineID], f, "Une ligne ne figure qu'une fois par réception.")
		seen[l.LineID] = true
		v.Check(l.Quantity.IsPositive() && l.Quantity.Equal(l.Quantity.Truncate(3)), f, "Quantité positive, 3 décimales au plus.")
	}
	if err := v.Err(); err != nil {
		return OrderDetail{}, false, err
	}
	receiptID := uuid.Nil
	if in.ID != nil {
		receiptID = *in.ID
	} else {
		var err error
		if receiptID, err = uuid.NewV7(); err != nil {
			return OrderDetail{}, false, err
		}
	}
	created := false
	var out OrderDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		po, err := lockOrder(ctx, q, a, id)
		if err != nil {
			return err
		}
		// Renvoi de la même réception : même réponse, rien n'est compté deux fois.
		if existing, err := q.GetPOReceipt(ctx, db.GetPOReceiptParams{OrgID: a.OrgID, ID: receiptID}); err == nil {
			if existing.PurchaseOrderID != id {
				return apperr.Conflict("receipt.id_conflict", "Cet identifiant de réception n'est pas utilisable.")
			}
			out, err = loadOrder(ctx, q, a, id)
			return err
		} else if !db.IsNoRows(err) {
			return err
		}
		if po.Status != OrderSent && po.Status != OrderPartiallyReceived {
			return wrongStatus("La commande doit avoir été envoyée au fournisseur, et pas entièrement reçue, pour enregistrer une réception.")
		}
		if in.DeliveryNoteRef != nil {
			dup, err := q.ReceiptNoteExists(ctx, db.ReceiptNoteExistsParams{OrgID: a.OrgID, PurchaseOrderID: id, Ref: *in.DeliveryNoteRef})
			if err != nil {
				return err
			}
			if dup {
				return apperr.Conflict("receipt.duplicate", "Ce bon de livraison est déjà enregistré sur cette commande.")
			}
		}
		lines, err := q.ListPOLines(ctx, db.ListPOLinesParams{OrgID: a.OrgID, PurchaseOrderID: id})
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]db.PurchaseOrderLine{}
		for _, l := range lines {
			byID[l.ID] = l
		}
		stock := map[uuid.UUID]decimal.Decimal{}
		for i, rl := range in.Lines {
			line, ok := byID[rl.LineID]
			if !ok {
				return apperr.New(http.StatusUnprocessableEntity, "receipt.unknown_line", fmt.Sprintf("La ligne %d ne correspond à aucune ligne de la commande.", i+1))
			}
			if line.QuantityRemaining == nil || rl.Quantity.GreaterThan(*line.QuantityRemaining) {
				return apperr.New(http.StatusUnprocessableEntity, "receipt.over_quantity",
					fmt.Sprintf("« %s » : %s reçus pour %s restant à recevoir.", line.Label, rl.Quantity.String(), line.QuantityRemaining.String()))
			}
			if line.MaterialID != nil {
				stock[*line.MaterialID] = stock[*line.MaterialID].Add(rl.Quantity)
			}
		}
		var movementID *uuid.UUID
		if in.ToStock && len(stock) > 0 {
			mid, err := s.stockIn(ctx, q, a, po, *in.ReceivedOn, in.DeliveryNoteRef, stock)
			if err != nil {
				return err
			}
			movementID = &mid
		}
		if _, err := q.InsertPOReceipt(ctx, db.InsertPOReceiptParams{
			ID: receiptID, OrgID: a.OrgID, PurchaseOrderID: id, ReceivedOn: *in.ReceivedOn, DeliveryNoteRef: in.DeliveryNoteRef,
			Notes: in.Notes, StockMovementID: movementID, ReceivedBy: a.UserID,
		}); err != nil {
			if apperr.Normalize(err).Code == "resource.duplicate" {
				return apperr.Conflict("receipt.duplicate", "Ce bon de livraison est déjà enregistré sur cette commande.")
			}
			return err
		}
		for _, rl := range in.Lines {
			if err := q.InsertPOReceiptLine(ctx, db.InsertPOReceiptLineParams{OrgID: a.OrgID, ReceiptID: receiptID, LineID: rl.LineID, Quantity: rl.Quantity}); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23514" {
					return apperr.New(http.StatusUnprocessableEntity, "receipt.over_quantity", "La quantité reçue dépasse la quantité restant à recevoir.")
				}
				return err
			}
		}
		full, err := q.POFullyReceived(ctx, db.POFullyReceivedParams{OrgID: a.OrgID, PurchaseOrderID: id})
		if err != nil {
			return err
		}
		status := OrderPartiallyReceived
		if full {
			status = OrderReceived
		}
		after := po
		if status != po.Status {
			if after, err = q.SetPOReceptionStatus(ctx, db.SetPOReceptionStatusParams{OrgID: a.OrgID, ID: id, Status: status}); err != nil {
				return err
			}
		}
		if err := event(ctx, q, a, after, "received", po.Status, after.Status, in.Notes, map[string]any{
			"receipt_id": receiptID, "delivery_note_ref": in.DeliveryNoteRef, "lines": in.Lines, "stock_movement_id": movementID,
		}); err != nil {
			return err
		}
		created = true
		out, err = loadOrder(ctx, q, a, id)
		return err
	})
	return out, created, err
}

// stockIn crée et poste un mouvement d'entrée sur le chantier, comme le module de stock : soldes
// verrouillés dans l'ordre des articles, mouvement immuable une fois posté.
func (s *Service) stockIn(ctx context.Context, q *db.Queries, a auth.Actor, po db.PurchaseOrder, on civil.Date, note *string, qty map[uuid.UUID]decimal.Decimal) (uuid.UUID, error) {
	if err := rbac.Require(a, rbac.StockMove); err != nil {
		return uuid.Nil, apperr.New(http.StatusForbidden, "auth.forbidden", "Vous ne pouvez pas faire entrer de marchandises en stock.")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, err
	}
	reason := "Réception " + OrderReference(po.Number)
	if note != nil {
		reason += " · BL " + *note
	}
	if _, err := q.CreateStockMovement(ctx, db.CreateStockMovementParams{
		ID: id, OrgID: a.OrgID, ProjectID: po.ProjectID, MovementType: "in", OccurredOn: on, Reason: &reason, CreatedBy: a.UserID,
	}); err != nil {
		return uuid.Nil, err
	}
	materials := make([]uuid.UUID, 0, len(qty))
	for m := range qty {
		materials = append(materials, m)
	}
	sort.Slice(materials, func(i, j int) bool { return materials[i].String() < materials[j].String() })
	for _, m := range materials {
		if _, err := q.GetActiveMaterial(ctx, db.GetActiveMaterialParams{OrgID: a.OrgID, ID: m}); err != nil {
			if db.IsNoRows(err) {
				return uuid.Nil, apperr.New(http.StatusUnprocessableEntity, "material.archived", "Un article de la commande a été archivé : il ne peut plus entrer en stock.")
			}
			return uuid.Nil, err
		}
		if err := q.InsertStockMovementItem(ctx, db.InsertStockMovementItemParams{OrgID: a.OrgID, MovementID: id, MaterialID: m, Quantity: qty[m]}); err != nil {
			return uuid.Nil, err
		}
		if err := q.EnsureStockRow(ctx, db.EnsureStockRowParams{OrgID: a.OrgID, ProjectID: po.ProjectID, MaterialID: m}); err != nil {
			return uuid.Nil, err
		}
		if _, err := q.ApplyStockDelta(ctx, db.ApplyStockDeltaParams{OrgID: a.OrgID, ProjectID: po.ProjectID, MaterialID: m, Delta: qty[m], OccurredOn: on}); err != nil {
			return uuid.Nil, err
		}
	}
	if _, err := q.PostStockMovement(ctx, db.PostStockMovementParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID}); err != nil {
		return uuid.Nil, err
	}
	return id, audit.Record(ctx, q, a, audit.Entry{
		Action: "stock.posted", EntityType: "stock_movement", EntityID: &id, ProjectID: &po.ProjectID,
		After: map[string]any{"movement_type": "in", "purchase_order": OrderReference(po.Number), "items": qty},
	})
}
