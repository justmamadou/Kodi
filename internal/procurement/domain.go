// Package procurement gère les demandes de prix (RFQ) : consultation des fournisseurs d'un chantier
// et réception de leurs offres.
package procurement

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/shopspring/decimal"
)

// Statuts d'une demande de prix, identiques à la contrainte CHECK de la base.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusClosed    = "closed"
	StatusCancelled = "cancelled"
)

// Units liste les unités d'une ligne : celles du catalogue de matériaux, plus celles des prestations.
var Units = []string{"sac", "barre", "m3", "m2", "ml", "unite", "kg", "tonne", "litre", "rouleau", "boite", "forfait", "jour", "heure"}

const (
	maxItems     = 200
	maxSuppliers = 50
	maxUnitPrice = 1_000_000_000_000 // 10^12 FCFA : au-delà, c'est une erreur de saisie
)

// LinkPath est la page de la PWA où le fournisseur dépose son offre. Le jeton passe dans le
// fragment (#), que le navigateur n'envoie jamais au serveur.
const LinkPath = "/offre#"

// Reference est le numéro affiché d'une demande : DP-0001.
func Reference(number int32) string { return fmt.Sprintf("DP-%04d", number) }

// ItemInput décrit un matériau ou une prestation demandés.
type ItemInput struct {
	Kind           string          `json:"kind"` // material | service
	MaterialID     *uuid.UUID      `json:"material_id"`
	Label          string          `json:"label"`
	Quantity       decimal.Decimal `json:"quantity"`
	Unit           string          `json:"unit"`
	Specifications *string         `json:"specifications"`
	NeededBy       *civil.Date     `json:"needed_by"`
}

// RFQInput décrit le contenu d'une demande en brouillon : il est remplacé en entier à chaque enregistrement.
type RFQInput struct {
	Title            string      `json:"title"`
	Notes            *string     `json:"notes"`
	ResponseDeadline *civil.Date `json:"response_deadline"`
	DeliveryDate     *civil.Date `json:"delivery_date"`
	Items            []ItemInput `json:"items"`
	SupplierIDs      []uuid.UUID `json:"supplier_ids"`
}

func trimmed(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// normalize retire les espaces superflus et vide les champs facultatifs laissés blancs.
func (in *RFQInput) normalize() {
	in.Title = strings.TrimSpace(in.Title)
	in.Notes = trimmed(in.Notes)
	for i := range in.Items {
		in.Items[i].Label = strings.TrimSpace(in.Items[i].Label)
		in.Items[i].Specifications = trimmed(in.Items[i].Specifications)
	}
}

// Validate applique les règles d'un brouillon. Les exigences de publication (au moins une ligne et
// un fournisseur, date limite à venir) sont vérifiées à la publication seulement.
func (in RFQInput) Validate() error {
	var v apperr.Validator
	v.Check(len(in.Title) >= 2 && len(in.Title) <= 160, "title", "L'objet doit contenir entre 2 et 160 caractères.")
	v.Check(in.Notes == nil || len(*in.Notes) <= 4000, "notes", "Les précisions sont trop longues (4 000 caractères au plus).")
	v.Check(len(in.Items) <= maxItems, "items", "Une demande compte au plus 200 lignes.")
	for i, item := range in.Items {
		field := fmt.Sprintf("items.%d", i)
		v.Check(item.Kind == "material" || item.Kind == "service", field+".kind", "Le type doit être matériau ou prestation.")
		v.Check(item.Kind == "material" || item.MaterialID == nil, field+".material_id", "Une prestation ne se rattache pas au catalogue de matériaux.")
		v.Check(len(item.Label) >= 2 && len(item.Label) <= 200, field+".label", "La désignation doit contenir entre 2 et 200 caractères.")
		v.Check(item.Quantity.IsPositive(), field+".quantity", "La quantité doit être positive.")
		v.Check(item.Quantity.Equal(item.Quantity.Truncate(3)), field+".quantity", "La quantité accepte au plus 3 décimales.")
		v.Check(item.Quantity.LessThan(decimal.New(1, 11)), field+".quantity", "La quantité est trop grande.")
		v.Check(slices.Contains(Units, item.Unit), field+".unit", "Unité inconnue.")
		v.Check(item.Specifications == nil || len(*item.Specifications) <= 2000, field+".specifications", "Les spécifications sont trop longues (2 000 caractères au plus).")
	}
	v.Check(len(in.SupplierIDs) <= maxSuppliers, "supplier_ids", "Une demande consulte au plus 50 fournisseurs.")
	seen := map[uuid.UUID]bool{}
	for _, id := range in.SupplierIDs {
		v.Check(!seen[id], "supplier_ids", "Un fournisseur ne peut être consulté qu'une fois par demande.")
		seen[id] = true
	}
	return v.Err()
}

// OfferLineInput est le prix proposé pour une ligne de la demande. Une ligne absente n'est pas chiffrée.
type OfferLineInput struct {
	RFQItemID uuid.UUID `json:"rfq_item_id"`
	UnitPrice int64     `json:"unit_price"` // FCFA hors taxes, entier
	Comment   *string   `json:"comment"`
}

// OfferInput est l'offre déposée par un fournisseur avec son lien.
type OfferInput struct {
	Token        string           `json:"token"`
	ID           *uuid.UUID       `json:"id"` // généré par la page : un double envoi n'a qu'un effet
	Lines        []OfferLineInput `json:"lines"`
	ValidUntil   *civil.Date      `json:"valid_until"`
	DeliveryDays *int16           `json:"delivery_days"`
	PaymentTerms *string          `json:"payment_terms"`
	Notes        *string          `json:"notes"`
	ContactName  *string          `json:"contact_name"`
}

// Validate applique les règles d'une offre, indépendamment de la demande.
func (in *OfferInput) Validate() error {
	in.PaymentTerms, in.Notes, in.ContactName = trimmed(in.PaymentTerms), trimmed(in.Notes), trimmed(in.ContactName)
	var v apperr.Validator
	v.Check(len(in.Lines) > 0, "lines", "Indiquez au moins un prix.")
	v.Check(len(in.Lines) <= maxItems, "lines", "Trop de lignes.")
	seen := map[uuid.UUID]bool{}
	for i := range in.Lines {
		in.Lines[i].Comment = trimmed(in.Lines[i].Comment)
		line := in.Lines[i]
		field := fmt.Sprintf("lines.%d", i)
		v.Check(!seen[line.RFQItemID], field, "Une ligne ne peut être chiffrée qu'une fois.")
		seen[line.RFQItemID] = true
		v.Check(line.UnitPrice >= 0 && line.UnitPrice < maxUnitPrice, field+".unit_price", "Prix unitaire invalide.")
		v.Check(line.Comment == nil || len(*line.Comment) <= 500, field+".comment", "Commentaire trop long (500 caractères au plus).")
	}
	v.Check(in.DeliveryDays == nil || (*in.DeliveryDays >= 0 && *in.DeliveryDays <= 365), "delivery_days", "Le délai de livraison doit être compris entre 0 et 365 jours.")
	v.Check(in.PaymentTerms == nil || len(*in.PaymentTerms) <= 500, "payment_terms", "Conditions de paiement trop longues (500 caractères au plus).")
	v.Check(in.Notes == nil || len(*in.Notes) <= 2000, "notes", "Remarques trop longues (2 000 caractères au plus).")
	v.Check(in.ContactName == nil || len(*in.ContactName) <= 160, "contact_name", "Nom trop long (160 caractères au plus).")
	return v.Err()
}

// LineAmount est le montant d'une ligne, arrondi au franc comme la colonne générée de la base.
func LineAmount(quantity decimal.Decimal, unitPrice int64) int64 {
	return quantity.Mul(decimal.NewFromInt(unitPrice)).Round(0).IntPart()
}

var (
	errNotDraft = apperr.Conflict("rfq.not_draft", "Cette demande de prix est publiée : son contenu ne peut plus changer.")
	errNotOpen  = apperr.Conflict("rfq.not_open", "Cette demande de prix n'accepte plus d'offres.")
	errLink     = apperr.New(http.StatusGone, "rfq_link.invalid", "Ce lien de consultation n'est plus valable. Demandez-en un nouveau à votre client.")
)
