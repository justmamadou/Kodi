package procurement

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/identity"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Service porte les cas d'usage des demandes de prix.
type Service struct {
	tx         *db.TxManager
	mail       mailer.Mailer
	appBaseURL string
}

// NewService construit le service ; appBaseURL sert à composer les liens des fournisseurs.
func NewService(tx *db.TxManager, mail mailer.Mailer, appBaseURL string) *Service {
	return &Service{tx: tx, mail: mail, appBaseURL: appBaseURL}
}

// Offer est une version d'offre et ses prix.
type Offer struct {
	db.ListOffersRow
	Lines []db.RfqOfferLine `json:"lines"`
}

// Detail est une demande de prix complète : lignes, fournisseurs consultés et toutes les offres reçues.
type Detail struct {
	db.GetRFQRow
	Reference string                   `json:"reference"`
	Open      bool                     `json:"open"` // accepte des offres
	Items     []db.RfqItem             `json:"items"`
	Suppliers []db.ListRFQSuppliersRow `json:"suppliers"`
	Offers    []Offer                  `json:"offers"` // toutes les versions, la plus récente d'abord
	// Comparaison sur critères et commande issue de la sélection (vue de l'entreprise seulement).
	Evaluation *Evaluation `json:"evaluation,omitempty"`
	Order      *OrderRef   `json:"order,omitempty"`
}

// Link est le lien de consultation d'un fournisseur : il n'est visible qu'au moment où il est émis.
type Link struct {
	SupplierID   uuid.UUID `json:"supplier_id"`
	SupplierName string    `json:"supplier_name"`
	Phone        *string   `json:"phone"`
	Email        *string   `json:"email"`
	URL          string    `json:"url"`
	Emailed      bool      `json:"emailed"`
}

// WithLinks accompagne une action qui émet des liens (publication, invitation, renouvellement).
type WithLinks struct {
	RFQ   Detail `json:"rfq"`
	Links []Link `json:"links"`
}

// List liste les demandes de prix visibles par l'acteur : toutes pour Owner et Finance, celles de
// ses chantiers pour les autres.
func (s *Service) List(ctx context.Context, a auth.Actor, projectID *uuid.UUID, status *string) ([]db.ListRFQsRow, error) {
	if err := rbac.Require(a, rbac.RFQView); err != nil {
		return nil, err
	}
	var out []db.ListRFQsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListRFQs(ctx, db.ListRFQsParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID, ProjectID: projectID, Status: status,
		})
		return err
	})
	return out, err
}

// Create crée une demande de prix en brouillon sur un chantier.
func (s *Service) Create(ctx context.Context, a auth.Actor, projectID uuid.UUID, in RFQInput) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return Detail{}, err
	}
	in.normalize()
	if err := in.Validate(); err != nil {
		return Detail{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Detail{}, err
	}
	var out Detail
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		if err := q.LockRFQNumbering(ctx, a.OrgID); err != nil {
			return err
		}
		rfq, err := q.CreateRFQ(ctx, db.CreateRFQParams{
			ID: id, OrgID: a.OrgID, ProjectID: projectID, Title: in.Title, Notes: in.Notes,
			ResponseDeadline: in.ResponseDeadline, DeliveryDate: in.DeliveryDate, CreatedBy: a.UserID,
		})
		if err != nil {
			return err
		}
		if err := s.writeContent(ctx, q, a, id, in); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.created", EntityType: "rfq", EntityID: &id, ProjectID: &projectID,
			After: map[string]any{"reference": Reference(rfq.Number), "title": in.Title, "items": len(in.Items), "suppliers": len(in.SupplierIDs)},
		}); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

// Update remplace le contenu d'un brouillon : objet, dates, lignes et fournisseurs consultés.
func (s *Service) Update(ctx context.Context, a auth.Actor, id uuid.UUID, in RFQInput) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return Detail{}, err
	}
	in.normalize()
	if err := in.Validate(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status != StatusDraft {
			return errNotDraft
		}
		if _, err := q.UpdateRFQDraft(ctx, db.UpdateRFQDraftParams{
			OrgID: a.OrgID, ID: id, Title: in.Title, Notes: in.Notes,
			ResponseDeadline: in.ResponseDeadline, DeliveryDate: in.DeliveryDate,
		}); err != nil {
			return err
		}
		if err := s.writeContent(ctx, q, a, id, in); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.updated", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			After: map[string]any{"title": in.Title, "items": len(in.Items), "suppliers": len(in.SupplierIDs)},
		}); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

// writeContent remplace les lignes et les fournisseurs d'un brouillon.
func (s *Service) writeContent(ctx context.Context, q *db.Queries, a auth.Actor, rfqID uuid.UUID, in RFQInput) error {
	if err := q.DeleteRFQItems(ctx, db.DeleteRFQItemsParams{OrgID: a.OrgID, RfqID: rfqID}); err != nil {
		return err
	}
	for i, item := range in.Items {
		if item.MaterialID != nil {
			if _, err := q.GetActiveMaterial(ctx, db.GetActiveMaterialParams{OrgID: a.OrgID, ID: *item.MaterialID}); err != nil {
				if db.IsNoRows(err) {
					return apperr.New(http.StatusUnprocessableEntity, "material.not_found",
						fmt.Sprintf("Ligne %d : ce matériau n'est pas (ou plus) au catalogue.", i+1))
				}
				return err
			}
		}
		itemID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if err := q.InsertRFQItem(ctx, db.InsertRFQItemParams{
			ID: itemID, OrgID: a.OrgID, RfqID: rfqID, Position: int16(i + 1), Kind: item.Kind, MaterialID: item.MaterialID,
			Label: item.Label, Quantity: item.Quantity, Unit: item.Unit, Specifications: item.Specifications, NeededBy: item.NeededBy,
		}); err != nil {
			return err
		}
	}
	keep := in.SupplierIDs
	if keep == nil {
		keep = []uuid.UUID{}
	}
	if err := q.DeleteRFQSuppliersExcept(ctx, db.DeleteRFQSuppliersExceptParams{OrgID: a.OrgID, RfqID: rfqID, Keep: keep}); err != nil {
		return err
	}
	for _, supplierID := range in.SupplierIDs {
		if _, err := activeSupplier(ctx, q, a.OrgID, supplierID); err != nil {
			return err
		}
		rowID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		if _, err := q.AddRFQSupplier(ctx, db.AddRFQSupplierParams{
			ID: rowID, OrgID: a.OrgID, RfqID: rfqID, SupplierID: supplierID, InvitedBy: a.UserID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func activeSupplier(ctx context.Context, q *db.Queries, orgID, id uuid.UUID) (db.Supplier, error) {
	sup, err := q.GetActiveSupplier(ctx, db.GetActiveSupplierParams{OrgID: orgID, ID: id})
	if db.IsNoRows(err) {
		return sup, apperr.New(http.StatusUnprocessableEntity, "supplier.not_found", "Un fournisseur choisi est introuvable ou désactivé.")
	}
	return sup, err
}

// lockWritable verrouille la demande et vérifie que l'acteur peut écrire sur son chantier.
func lockWritable(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.Rfq, error) {
	rfq, err := q.LockRFQ(ctx, db.LockRFQParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return rfq, apperr.NotFound("rfq")
		}
		return rfq, err
	}
	_, err = rbac.ProjectWritable(ctx, q, a, rfq.ProjectID)
	return rfq, err
}

func load(ctx context.Context, q *db.Queries, orgID, id uuid.UUID) (Detail, error) {
	rfq, err := q.GetRFQ(ctx, db.GetRFQParams{OrgID: orgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return Detail{}, apperr.NotFound("rfq")
		}
		return Detail{}, err
	}
	out := Detail{GetRFQRow: rfq, Reference: Reference(rfq.Number)}
	if out.Open, err = q.RFQIsOpen(ctx, db.RFQIsOpenParams{OrgID: orgID, ID: id}); err != nil {
		return out, err
	}
	if out.Items, err = q.ListRFQItems(ctx, db.ListRFQItemsParams{OrgID: orgID, RfqID: id}); err != nil {
		return out, err
	}
	if out.Suppliers, err = q.ListRFQSuppliers(ctx, db.ListRFQSuppliersParams{OrgID: orgID, RfqID: id}); err != nil {
		return out, err
	}
	offers, err := q.ListOffers(ctx, db.ListOffersParams{OrgID: orgID, RfqID: id})
	if err != nil {
		return out, err
	}
	lines, err := q.ListOfferLines(ctx, db.ListOfferLinesParams{OrgID: orgID, RfqID: id})
	if err != nil {
		return out, err
	}
	byOffer := map[uuid.UUID][]db.RfqOfferLine{}
	for _, l := range lines {
		byOffer[l.OfferID] = append(byOffer[l.OfferID], l)
	}
	out.Offers = make([]Offer, len(offers))
	for i, o := range offers {
		ls := byOffer[o.ID]
		if ls == nil {
			ls = []db.RfqOfferLine{}
		}
		out.Offers[i] = Offer{ListOffersRow: o, Lines: ls}
	}
	return out, nil
}

// Get retourne une demande de prix et ses offres.
func (s *Service) Get(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQView); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		if out, err = s.detail(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		_, err = rbac.Project(ctx, q, a, out.ProjectID)
		return err
	})
	return out, err
}

// pendingLink est un lien émis dans la transaction, à envoyer après son commit.
type pendingLink struct {
	link  Link
	token string
}

// issueLink émet un nouveau jeton pour un fournisseur consulté ; l'ancien lien cesse aussitôt de fonctionner.
func (s *Service) issueLink(ctx context.Context, q *db.Queries, orgID, rfqSupplierID uuid.UUID, sup db.GetRFQSupplierRow) (pendingLink, error) {
	token, hash, err := identity.NewToken()
	if err != nil {
		return pendingLink{}, err
	}
	if err := q.IssueRFQLink(ctx, db.IssueRFQLinkParams{OrgID: orgID, ID: rfqSupplierID, TokenHash: hash}); err != nil {
		return pendingLink{}, err
	}
	return pendingLink{token: token, link: Link{
		SupplierID: sup.SupplierID, SupplierName: sup.SupplierName, Phone: sup.SupplierPhone, Email: sup.SupplierEmail,
		URL: s.appBaseURL + LinkPath + token,
	}}, nil
}

// sendLinks envoie les liens par email, après le commit : l'envoi n'est pas fiable, et l'acheteur
// garde de toute façon les liens à l'écran pour les transmettre autrement (WhatsApp, SMS).
func (s *Service) sendLinks(ctx context.Context, d Detail, orgName string, pending []pendingLink) []Link {
	links := make([]Link, len(pending))
	for i, p := range pending {
		links[i] = p.link
		if p.link.Email == nil {
			continue
		}
		var deadline string
		if d.ResponseDeadline != nil {
			deadline = d.ResponseDeadline.Format("02/01/2006")
		}
		body := "Bonjour,\n\n" + orgName + " vous consulte pour la demande de prix " + d.Reference + " : « " + d.Title + " ».\n\n" +
			"Réponse attendue au plus tard le " + deadline + ".\n\n" +
			"Consultez la demande et déposez votre offre ici (lien personnel, à ne pas transmettre) :\n" + p.link.URL + "\n"
		subject := "Demande de prix " + d.Reference + " — " + orgName
		if err := s.mail.Send(ctx, *p.link.Email, subject, body); err != nil {
			slog.ErrorContext(ctx, "envoi de la demande de prix en échec", "error", err, "rfq_id", d.ID)
			continue
		}
		links[i].Emailed = true
	}
	return links
}

// Publish publie la demande : son contenu est figé et chaque fournisseur reçoit son lien.
func (s *Service) Publish(ctx context.Context, a auth.Actor, id uuid.UUID) (WithLinks, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return WithLinks{}, err
	}
	var out Detail
	var orgName string
	var pending []pendingLink
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status != StatusDraft {
			return apperr.Conflict("rfq.already_published", "Cette demande de prix a déjà été publiée ou annulée.")
		}
		current, err := load(ctx, q, a.OrgID, id)
		if err != nil {
			return err
		}
		today, err := q.TodayInDakar(ctx)
		if err != nil {
			return err
		}
		var v apperr.Validator
		v.Check(len(current.Items) > 0, "items", "Ajoutez au moins un matériau ou une prestation avant de publier.")
		v.Check(len(current.Suppliers) > 0, "supplier_ids", "Choisissez au moins un fournisseur avant de publier.")
		v.Check(rfq.ResponseDeadline != nil, "response_deadline", "Indiquez la date limite de réponse avant de publier.")
		v.Check(rfq.ResponseDeadline == nil || !rfq.ResponseDeadline.Before(today.Time), "response_deadline", "La date limite de réponse est déjà passée.")
		if err := v.Err(); err != nil {
			return err
		}
		if _, err := q.PublishRFQ(ctx, db.PublishRFQParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID}); err != nil {
			return err
		}
		for _, sup := range current.Suppliers {
			row, err := q.GetRFQSupplier(ctx, db.GetRFQSupplierParams{OrgID: a.OrgID, RfqID: id, SupplierID: sup.SupplierID})
			if err != nil {
				return err
			}
			p, err := s.issueLink(ctx, q, a.OrgID, row.ID, row)
			if err != nil {
				return err
			}
			pending = append(pending, p)
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.published", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			After: map[string]any{"response_deadline": rfq.ResponseDeadline, "suppliers": len(pending), "items": len(current.Items)},
		}); err != nil {
			return err
		}
		if orgName, err = q.GetOrganizationName(ctx, a.OrgID); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	if err != nil {
		return WithLinks{}, err
	}
	return WithLinks{RFQ: out, Links: s.sendLinks(ctx, out, orgName, pending)}, nil
}

// AddSupplier consulte un fournisseur de plus. Sur une demande publiée et encore ouverte, son lien
// est émis aussitôt.
func (s *Service) AddSupplier(ctx context.Context, a auth.Actor, id, supplierID uuid.UUID) (WithLinks, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return WithLinks{}, err
	}
	var out Detail
	var orgName string
	var pending []pendingLink
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		switch rfq.Status {
		case StatusDraft:
		case StatusPublished:
			open, err := q.RFQIsOpen(ctx, db.RFQIsOpenParams{OrgID: a.OrgID, ID: id})
			if err != nil {
				return err
			}
			if !open {
				return errNotOpen
			}
		default:
			return errNotOpen
		}
		if _, err := activeSupplier(ctx, q, a.OrgID, supplierID); err != nil {
			return err
		}
		rowID, err := uuid.NewV7()
		if err != nil {
			return err
		}
		added, err := q.AddRFQSupplier(ctx, db.AddRFQSupplierParams{ID: rowID, OrgID: a.OrgID, RfqID: id, SupplierID: supplierID, InvitedBy: a.UserID})
		if err != nil {
			return err
		}
		if !added.Inserted {
			return apperr.Conflict("rfq.supplier_already_consulted", "Ce fournisseur est déjà consulté sur cette demande.")
		}
		if rfq.Status == StatusPublished {
			row, err := q.GetRFQSupplier(ctx, db.GetRFQSupplierParams{OrgID: a.OrgID, RfqID: id, SupplierID: supplierID})
			if err != nil {
				return err
			}
			p, err := s.issueLink(ctx, q, a.OrgID, row.ID, row)
			if err != nil {
				return err
			}
			pending = append(pending, p)
			if orgName, err = q.GetOrganizationName(ctx, a.OrgID); err != nil {
				return err
			}
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.supplier_invited", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			After: map[string]any{"supplier_id": supplierID},
		}); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	if err != nil {
		return WithLinks{}, err
	}
	return WithLinks{RFQ: out, Links: s.sendLinks(ctx, out, orgName, pending)}, nil
}

// RenewLink remplace le lien d'un fournisseur (perdu, ou transmis à la mauvaise personne).
func (s *Service) RenewLink(ctx context.Context, a auth.Actor, id, supplierID uuid.UUID) (WithLinks, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return WithLinks{}, err
	}
	var out Detail
	var orgName string
	var pending []pendingLink
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		open, err := q.RFQIsOpen(ctx, db.RFQIsOpenParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		if !open {
			return errNotOpen
		}
		row, err := q.GetRFQSupplier(ctx, db.GetRFQSupplierParams{OrgID: a.OrgID, RfqID: id, SupplierID: supplierID})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("rfq_supplier")
			}
			return err
		}
		p, err := s.issueLink(ctx, q, a.OrgID, row.ID, row)
		if err != nil {
			return err
		}
		pending = append(pending, p)
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.link_renewed", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			After: map[string]any{"supplier_id": supplierID},
		}); err != nil {
			return err
		}
		if orgName, err = q.GetOrganizationName(ctx, a.OrgID); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	if err != nil {
		return WithLinks{}, err
	}
	return WithLinks{RFQ: out, Links: s.sendLinks(ctx, out, orgName, pending)}, nil
}

// Close clôt la consultation : plus aucune offre n'est acceptée, celles reçues restent consultables.
func (s *Service) Close(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status != StatusPublished {
			return apperr.Conflict("rfq.not_published", "Seule une demande publiée peut être clôturée.")
		}
		if _, err := q.CloseRFQ(ctx, db.CloseRFQParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID}); err != nil {
			return err
		}
		if out, err = load(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.closed", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			After: map[string]any{"offers": len(out.Offers)},
		})
	})
	return out, err
}

// CancelInput porte le motif d'annulation, communiqué aux fournisseurs qui ouvrent leur lien.
type CancelInput struct {
	Reason string `json:"reason"`
}

// Cancel annule une demande en brouillon ou publiée.
func (s *Service) Cancel(ctx context.Context, a auth.Actor, id uuid.UUID, in CancelInput) (Detail, error) {
	if err := rbac.Require(a, rbac.RFQManage); err != nil {
		return Detail{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	var v apperr.Validator
	v.Check(reason != "", "reason", "Indiquez le motif de l'annulation.")
	v.Check(len(reason) <= 500, "reason", "Motif trop long (500 caractères au plus).")
	if err := v.Err(); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rfq, err := lockWritable(ctx, q, a, id)
		if err != nil {
			return err
		}
		if rfq.Status != StatusDraft && rfq.Status != StatusPublished {
			return apperr.Conflict("rfq.already_ended", "Cette demande de prix est déjà clôturée ou annulée.")
		}
		if _, err := q.CancelRFQ(ctx, db.CancelRFQParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID, Reason: &reason}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "rfq.cancelled", EntityType: "rfq", EntityID: &id, ProjectID: &rfq.ProjectID,
			Before: map[string]any{"status": rfq.Status}, After: map[string]any{"reason": reason},
		}); err != nil {
			return err
		}
		out, err = load(ctx, q, a.OrgID, id)
		return err
	})
	return out, err
}

// PortalView est ce que voit un fournisseur avec son lien : la demande et ses propres offres, rien d'autre.
type PortalView struct {
	OrganizationName string       `json:"organization_name"`
	SupplierName     string       `json:"supplier_name"`
	Reference        string       `json:"reference"`
	Title            string       `json:"title"`
	Notes            *string      `json:"notes"`
	Status           string       `json:"status"`
	Open             bool         `json:"open"`
	CancelReason     *string      `json:"cancel_reason"`
	ResponseDeadline *civil.Date  `json:"response_deadline"`
	DeliveryDate     *civil.Date  `json:"delivery_date"`
	DeliveryPlace    string       `json:"delivery_place"`
	Items            []PortalItem `json:"items"`
	Offers           []Offer      `json:"offers"` // les siennes, la plus récente d'abord
}

// PortalItem est une ligne de la demande, sans lien vers le catalogue interne.
type PortalItem struct {
	ID             uuid.UUID   `json:"id"`
	Position       int16       `json:"position"`
	Kind           string      `json:"kind"`
	Label          string      `json:"label"`
	Quantity       string      `json:"quantity"`
	Unit           string      `json:"unit"`
	Specifications *string     `json:"specifications"`
	NeededBy       *civil.Date `json:"needed_by"`
}

// TokenInput porte le jeton d'un lien de consultation.
type TokenInput struct {
	Token string `json:"token"`
}

func (s *Service) resolveLink(ctx context.Context, token string) (db.RFQLink, error) {
	if token == "" || len(token) > 128 {
		return db.RFQLink{}, errLink
	}
	link, err := s.tx.FindRFQLink(ctx, identity.HashToken(token))
	if err != nil {
		if db.IsNoRows(err) {
			return link, errLink
		}
		return link, err
	}
	return link, nil
}

// Portal retourne la demande vue par le fournisseur à qui appartient le lien.
func (s *Service) Portal(ctx context.Context, in TokenInput) (PortalView, error) {
	link, err := s.resolveLink(ctx, in.Token)
	if err != nil {
		return PortalView{}, err
	}
	var out PortalView
	err = s.tx.InTenantTx(ctx, link.OrganizationID, func(q *db.Queries) error {
		d, err := load(ctx, q, link.OrganizationID, link.RFQID)
		if err != nil {
			return err
		}
		orgName, err := q.GetOrganizationName(ctx, link.OrganizationID)
		if err != nil {
			return err
		}
		out = PortalView{
			OrganizationName: orgName, Reference: d.Reference, Title: d.Title, Notes: d.Notes, Status: d.Status,
			Open: d.Open, CancelReason: d.CancelReason, ResponseDeadline: d.ResponseDeadline, DeliveryDate: d.DeliveryDate,
			DeliveryPlace: deliveryPlace(d.ProjectAddress, d.ProjectCity), Items: make([]PortalItem, len(d.Items)), Offers: []Offer{},
		}
		for i, item := range d.Items {
			out.Items[i] = PortalItem{
				ID: item.ID, Position: item.Position, Kind: item.Kind, Label: item.Label, Quantity: item.Quantity.String(),
				Unit: item.Unit, Specifications: item.Specifications, NeededBy: item.NeededBy,
			}
		}
		for _, sup := range d.Suppliers {
			if sup.ID == link.RFQSupplierID {
				out.SupplierName = sup.SupplierName
			}
		}
		for _, o := range d.Offers {
			if o.RfqSupplierID == link.RFQSupplierID {
				out.Offers = append(out.Offers, o)
			}
		}
		return nil
	})
	return out, err
}

func deliveryPlace(address string, city *string) string {
	if city == nil || *city == "" || strings.Contains(strings.ToLower(address), strings.ToLower(*city)) {
		return address
	}
	return address + ", " + *city
}

// SubmitOffer enregistre une offre du fournisseur : une nouvelle version à chaque envoi, tant que la
// demande est publiée et que la date limite n'est pas passée.
func (s *Service) SubmitOffer(ctx context.Context, in OfferInput) (Offer, bool, error) {
	if err := in.Validate(); err != nil {
		return Offer{}, false, err
	}
	link, err := s.resolveLink(ctx, in.Token)
	if err != nil {
		return Offer{}, false, err
	}
	offerID := uuid.Nil
	if in.ID != nil {
		offerID = *in.ID
	} else if offerID, err = uuid.NewV7(); err != nil {
		return Offer{}, false, err
	}
	orgID := link.OrganizationID
	var out Offer
	created := false
	err = s.tx.InTenantTx(ctx, orgID, func(q *db.Queries) error {
		rfq, err := q.LockRFQShared(ctx, db.LockRFQSharedParams{OrgID: orgID, ID: link.RFQID})
		if err != nil {
			return err
		}
		// Renvoi d'une offre déjà enregistrée : même réponse, aucun doublon.
		if existing, err := q.GetOffer(ctx, db.GetOfferParams{OrgID: orgID, ID: offerID}); err == nil {
			if existing.RfqSupplierID != link.RFQSupplierID {
				return apperr.Conflict("offer.id_conflict", "Cet identifiant d'offre n'est pas utilisable.")
			}
			out, err = s.offer(ctx, q, orgID, rfq.ID, offerID)
			return err
		} else if !db.IsNoRows(err) {
			return err
		}
		open, err := q.RFQIsOpen(ctx, db.RFQIsOpenParams{OrgID: orgID, ID: rfq.ID})
		if err != nil {
			return err
		}
		if !open {
			return errNotOpen
		}
		if _, err := q.LockRFQSupplierByID(ctx, db.LockRFQSupplierByIDParams{OrgID: orgID, ID: link.RFQSupplierID}); err != nil {
			return err
		}
		items, err := q.ListRFQItems(ctx, db.ListRFQItemsParams{OrgID: orgID, RfqID: rfq.ID})
		if err != nil {
			return err
		}
		byID := map[uuid.UUID]db.RfqItem{}
		for _, item := range items {
			byID[item.ID] = item
		}
		var total int64
		for i, line := range in.Lines {
			item, ok := byID[line.RFQItemID]
			if !ok {
				return apperr.New(http.StatusUnprocessableEntity, "offer.unknown_item", fmt.Sprintf("La ligne %d ne correspond à aucune ligne de la demande.", i+1))
			}
			total += LineAmount(item.Quantity, line.UnitPrice)
		}
		version, err := q.NextOfferVersion(ctx, db.NextOfferVersionParams{OrgID: orgID, RfqSupplierID: link.RFQSupplierID})
		if err != nil {
			return err
		}
		var ip *string
		if meta := auth.MetaFrom(ctx); meta.IP != "" {
			ip = &meta.IP
		}
		if _, err := q.InsertOffer(ctx, db.InsertOfferParams{
			ID: offerID, OrgID: orgID, RfqID: rfq.ID, RfqSupplierID: link.RFQSupplierID, SupplierID: link.SupplierID,
			Version: version, TotalAmount: total, ValidUntil: in.ValidUntil, DeliveryDays: in.DeliveryDays,
			PaymentTerms: in.PaymentTerms, Notes: in.Notes, ContactName: in.ContactName, Ip: ip,
		}); err != nil {
			return err
		}
		var stored int64
		for _, line := range in.Lines {
			amount, err := q.InsertOfferLine(ctx, db.InsertOfferLineParams{
				OrgID: orgID, OfferID: offerID, RfqItemID: line.RFQItemID, Quantity: byID[line.RFQItemID].Quantity,
				UnitPrice: line.UnitPrice, Comment: line.Comment,
			})
			if err != nil {
				return err
			}
			if amount != nil {
				stored += *amount
			}
		}
		// Le total est calculé avant l'insertion (l'offre est immuable) : il doit égaler la somme de la base.
		if stored != total {
			return fmt.Errorf("total d'offre incohérent : %d calculé, %d en base", total, stored)
		}
		if err := audit.RecordExternal(ctx, q, orgID, audit.Entry{
			Action: "rfq.offer_received", EntityType: "rfq", EntityID: &rfq.ID, ProjectID: &rfq.ProjectID,
			After: map[string]any{"offer_id": offerID, "supplier_id": link.SupplierID, "version": version, "total_amount": total},
		}); err != nil {
			return err
		}
		created = true
		out, err = s.offer(ctx, q, orgID, rfq.ID, offerID)
		return err
	})
	return out, created, err
}

func (s *Service) offer(ctx context.Context, q *db.Queries, orgID, rfqID, offerID uuid.UUID) (Offer, error) {
	offers, err := q.ListOffers(ctx, db.ListOffersParams{OrgID: orgID, RfqID: rfqID})
	if err != nil {
		return Offer{}, err
	}
	lines, err := q.ListOfferLines(ctx, db.ListOfferLinesParams{OrgID: orgID, RfqID: rfqID})
	if err != nil {
		return Offer{}, err
	}
	for _, o := range offers {
		if o.ID != offerID {
			continue
		}
		out := Offer{ListOffersRow: o, Lines: []db.RfqOfferLine{}}
		for _, l := range lines {
			if l.OfferID == offerID {
				out.Lines = append(out.Lines, l)
			}
		}
		return out, nil
	}
	return Offer{}, apperr.NotFound("offer")
}
