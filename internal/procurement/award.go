package procurement

import (
	"context"
	"math"
	"net/http"
	"sort"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Evaluation compare la dernière offre de chaque fournisseur sur trois critères pondérés :
//   - prix : sur les lignes qu'elle chiffre, somme des meilleurs montants / somme de ses montants ;
//   - délai : (plus court délai annoncé + 1) / (son délai + 1), 0 si non communiqué ;
//   - couverture : part des lignes de la demande qu'elle chiffre.
//
// Chaque critère vaut de 0 à 100 ; le score est leur moyenne pondérée.
type Evaluation struct {
	WeightPrice    int16            `json:"weight_price"`
	WeightDelivery int16            `json:"weight_delivery"`
	WeightCoverage int16            `json:"weight_coverage"`
	Offers         []EvaluatedOffer `json:"offers"` // du meilleur score au moins bon
}

// EvaluatedOffer est une offre notée.
type EvaluatedOffer struct {
	OfferID       uuid.UUID   `json:"offer_id"`
	SupplierID    uuid.UUID   `json:"supplier_id"`
	SupplierName  string      `json:"supplier_name"`
	Version       int32       `json:"version"`
	TotalAmount   int64       `json:"total_amount"`
	QuotedLines   int         `json:"quoted_lines"`
	TotalLines    int         `json:"total_lines"`
	DeliveryDays  *int16      `json:"delivery_days"`
	ValidUntil    *civil.Date `json:"valid_until"`
	Expired       bool        `json:"expired"`
	PaymentTerms  *string     `json:"payment_terms"`
	PriceScore    float64     `json:"price_score"`
	DeliveryScore float64     `json:"delivery_score"`
	CoverageScore float64     `json:"coverage_score"`
	Score         float64     `json:"score"`
	Rank          int         `json:"rank"`
	Selected      bool        `json:"selected"`
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// Evaluate note les dernières offres d'une demande. today sert à signaler une offre dont la validité est passée.
func Evaluate(items []db.RfqItem, offers []Offer, weights [3]int16, selected *uuid.UUID, today civil.Date) Evaluation {
	out := Evaluation{WeightPrice: weights[0], WeightDelivery: weights[1], WeightCoverage: weights[2], Offers: []EvaluatedOffer{}}
	// Les offres arrivent de la plus récente à la plus ancienne : la première vue par fournisseur est sa dernière version.
	seen := map[uuid.UUID]bool{}
	var latest []Offer
	for _, o := range offers {
		if !seen[o.RfqSupplierID] {
			seen[o.RfqSupplierID] = true
			latest = append(latest, o)
		}
	}
	best := map[uuid.UUID]int64{}
	var minDays *int16
	for _, o := range latest {
		for _, l := range o.Lines {
			if l.Amount == nil {
				continue
			}
			if b, ok := best[l.RfqItemID]; !ok || *l.Amount < b {
				best[l.RfqItemID] = *l.Amount
			}
		}
		if o.DeliveryDays != nil && (minDays == nil || *o.DeliveryDays < *minDays) {
			minDays = o.DeliveryDays
		}
	}
	for _, o := range latest {
		e := EvaluatedOffer{
			OfferID: o.ID, SupplierID: o.SupplierID, SupplierName: o.SupplierName, Version: o.Version, TotalAmount: o.TotalAmount,
			QuotedLines: len(o.Lines), TotalLines: len(items), DeliveryDays: o.DeliveryDays, ValidUntil: o.ValidUntil,
			PaymentTerms: o.PaymentTerms, Selected: selected != nil && *selected == o.ID,
		}
		e.Expired = o.ValidUntil != nil && o.ValidUntil.Before(today.Time)
		var bestSum, ownSum int64
		for _, l := range o.Lines {
			if l.Amount != nil {
				ownSum += *l.Amount
				bestSum += best[l.RfqItemID]
			}
		}
		switch {
		case len(o.Lines) == 0:
			e.PriceScore = 0
		case ownSum == 0:
			e.PriceScore = 100
		default:
			e.PriceScore = round1(100 * float64(bestSum) / float64(ownSum))
		}
		if o.DeliveryDays != nil && minDays != nil {
			e.DeliveryScore = round1(100 * float64(*minDays+1) / float64(*o.DeliveryDays+1))
		}
		if len(items) > 0 {
			e.CoverageScore = round1(100 * float64(len(o.Lines)) / float64(len(items)))
		}
		e.Score = round1((float64(weights[0])*e.PriceScore + float64(weights[1])*e.DeliveryScore + float64(weights[2])*e.CoverageScore) / 100)
		out.Offers = append(out.Offers, e)
	}
	sort.SliceStable(out.Offers, func(i, j int) bool {
		if out.Offers[i].Score != out.Offers[j].Score {
			return out.Offers[i].Score > out.Offers[j].Score
		}
		return out.Offers[i].TotalAmount < out.Offers[j].TotalAmount
	})
	for i := range out.Offers {
		out.Offers[i].Rank = i + 1
	}
	return out
}

// CriteriaInput porte les poids des critères, en pour cent.
type CriteriaInput struct {
	WeightPrice    int16 `json:"weight_price"`
	WeightDelivery int16 `json:"weight_delivery"`
	WeightCoverage int16 `json:"weight_coverage"`
}

// SetCriteria change la pondération des critères, tant qu'aucune offre n'est retenue.
func (s *Service) SetCriteria(ctx context.Context, a auth.Actor, id uuid.UUID, in CriteriaInput) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return Detail{}, err
	}
	var v apperr.Validator
	for field, w := range map[string]int16{"weight_price": in.WeightPrice, "weight_delivery": in.WeightDelivery, "weight_coverage": in.WeightCoverage} {
		v.Check(w >= 0 && w <= 100, field, "Chaque poids est compris entre 0 et 100.")
	}
	v.Check(in.WeightPrice+in.WeightDelivery+in.WeightCoverage == 100, "weight_price", "Les poids doivent totaliser 100 %.")
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status == StatusCancelled {
			return apperr.Conflict("rfq.already_ended", "Cette demande de prix est annulée.")
		}
		if rfq.AwardedOfferID != nil {
			return apperr.Conflict("rfq.already_awarded", "Une offre est déjà retenue : retirez la sélection pour changer les critères.")
		}
		if _, err := q.SetRFQCriteria(ctx, db.SetRFQCriteriaParams{
			OrgID: a.OrgID, ID: id, WeightPrice: in.WeightPrice, WeightDelivery: in.WeightDelivery, WeightCoverage: in.WeightCoverage,
		}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.criteria_changed", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			Before: map[string]any{"weight_price": rfq.WeightPrice, "weight_delivery": rfq.WeightDelivery, "weight_coverage": rfq.WeightCoverage},
			After:  in,
		}); err != nil {
			return err
		}
		out, err = s.detail(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

// AwardInput désigne l'offre retenue et, si besoin, la raison du choix.
type AwardInput struct {
	OfferID uuid.UUID `json:"offer_id"`
	Reason  *string   `json:"reason"`
}

// Award retient une offre : la consultation doit être close, l'offre doit être la dernière version
// du fournisseur, et aucune commande active ne doit déjà en découler.
func (s *Service) Award(ctx context.Context, a auth.Actor, id uuid.UUID, in AwardInput) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQAward); err != nil {
		return Detail{}, err
	}
	reason := trimmed(in.Reason)
	var v apperr.Validator
	v.Check(reason == nil || len(*reason) <= 1000, "reason", "Motif trop long (1 000 caractères au plus).")
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status != StatusClosed {
			return apperr.Conflict("rfq.not_closed", "Clôturez la consultation avant de retenir une offre : plus aucun fournisseur ne doit pouvoir répondre.")
		}
		if err := noActiveOrder(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		latest, err := q.IsLatestOffer(ctx, db.IsLatestOfferParams{OrgID: a.OrgID, ID: in.OfferID, RfqID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.New(http.StatusUnprocessableEntity, "offer.not_found", "Cette offre n'appartient pas à la demande.")
			}
			return err
		}
		if latest == nil || !*latest {
			return apperr.Conflict("offer.not_latest", "Le fournisseur a envoyé une version plus récente : retenez sa dernière offre.")
		}
		offer, err := q.GetOffer(ctx, db.GetOfferParams{OrgID: a.OrgID, ID: in.OfferID})
		if err != nil {
			return err
		}
		if _, err := q.AwardRFQ(ctx, db.AwardRFQParams{OrgID: a.OrgID, ID: id, OfferID: &in.OfferID, ActorID: &a.UserID, Reason: reason}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.awarded", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			Before: map[string]any{"offer_id": rfq.AwardedOfferID},
			After:  map[string]any{"offer_id": in.OfferID, "supplier_id": offer.SupplierID, "version": offer.Version, "total_amount": offer.TotalAmount, "reason": reason},
		}); err != nil {
			return err
		}
		out, err = s.detail(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

// Unaward retire la sélection, tant qu'aucune commande active n'en découle.
func (s *Service) Unaward(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQAward); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.AwardedOfferID == nil {
			return apperr.Conflict("rfq.not_awarded", "Aucune offre n'est retenue.")
		}
		if err := noActiveOrder(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		if _, err := q.ClearRFQAward(ctx, db.ClearRFQAwardParams{OrgID: a.OrgID, ID: id}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.award_withdrawn", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			Before: map[string]any{"offer_id": rfq.AwardedOfferID},
		}); err != nil {
			return err
		}
		out, err = s.detail(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

func noActiveOrder(ctx context.Context, q *db.Queries, orgID, rfqID uuid.UUID) error {
	order, err := q.ActiveOrderForRFQ(ctx, db.ActiveOrderForRFQParams{OrgID: orgID, RfqID: &rfqID})
	if db.IsNoRows(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return apperr.Conflict("rfq.order_exists", "La commande "+OrderReference(order.Number)+" découle de cette sélection : annulez-la d'abord.")
}

// detail charge une demande avec son évaluation et sa commande active, pour l'entreprise.
func (s *Service) detail(ctx context.Context, q *db.Queries, orgID, id uuid.UUID) (Detail, error) {
	out, err := load(ctx, q, orgID, id)
	if err != nil {
		return out, err
	}
	today, err := q.TodayInDakar(ctx)
	if err != nil {
		return out, err
	}
	ev := Evaluate(out.Items, out.Offers, [3]int16{out.WeightPrice, out.WeightDelivery, out.WeightCoverage}, out.AwardedOfferID, today)
	out.Evaluation = &ev
	order, err := q.ActiveOrderForRFQ(ctx, db.ActiveOrderForRFQParams{OrgID: orgID, RfqID: &id})
	switch {
	case err == nil:
		out.Order = &OrderRef{ID: order.ID, Number: order.Number, Reference: OrderReference(order.Number), Status: order.Status}
	case !db.IsNoRows(err):
		return out, err
	}
	return out, nil
}

// OrderRef désigne la commande issue d'une demande de prix.
type OrderRef struct {
	ID        uuid.UUID `json:"id"`
	Number    int32     `json:"number"`
	Reference string    `json:"reference"`
	Status    string    `json:"status"`
}
