package supplier

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Service porte les cas d'usage du répertoire des fournisseurs.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// Duplicate est une fiche existante qui ressemble à celle saisie.
type Duplicate struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Archived bool      `json:"archived"`
	Blocking bool      `json:"blocking"` // même nom, NINEA ou RCCM : la création est refusée
	Reasons  []string  `json:"reasons"`  // same_name, same_ninea, same_rccm, same_phone, same_email, similar_name
}

// Detail est la fiche complète. Les conditions commerciales ne sont remplies que pour les rôles
// qui voient les prix.
type Detail struct {
	db.Supplier
	Contacts []db.SupplierContact     `json:"contacts"`
	Terms    *db.ListSupplierTermsRow `json:"terms"` // version en vigueur
}

// History regroupe les consultations (demandes de prix) et les achats (dépenses) du fournisseur, sur
// les chantiers visibles par l'acteur.
type History struct {
	Consultations []db.SupplierConsultationsRow `json:"consultations"`
	Purchases     []db.SupplierPurchasesRow     `json:"purchases"`
	Totals        db.SupplierPurchaseTotalsRow  `json:"totals"`
	Orders        []db.SupplierOrdersRow        `json:"orders"`
}

func fieldsOf(s db.Supplier) Fields {
	return Fields{
		Name: s.Name, Phone: s.Phone, Email: s.Email, LegalName: s.LegalName, LegalForm: s.LegalForm,
		NINEA: s.Ninea, RCCM: s.Rccm, Address: s.Address, City: s.City, Notes: s.Notes, Activities: s.Activities,
	}
}

func duplicatesOf(rows []db.FindSupplierDuplicatesRow) []Duplicate {
	out := make([]Duplicate, 0, len(rows))
	for _, r := range rows {
		d := Duplicate{ID: r.ID, Name: r.Name, Archived: !r.IsActive, Reasons: []string{}}
		for _, check := range []struct {
			reason string
			ok     bool
		}{
			{"same_name", r.SameName}, {"same_ninea", r.SameNinea}, {"same_rccm", r.SameRccm},
			{"same_phone", r.SamePhone}, {"same_email", r.SameEmail}, {"similar_name", r.SimilarName},
		} {
			if check.ok {
				d.Reasons = append(d.Reasons, check.reason)
			}
		}
		d.Blocking = r.SameName || r.SameNinea || r.SameRccm
		out = append(out, d)
	}
	return out
}

var reasonLabel = map[string]string{
	"same_name": "même nom", "same_ninea": "même NINEA", "same_rccm": "même RCCM",
	"same_phone": "même téléphone", "same_email": "même email", "similar_name": "nom très proche",
}

func describe(d Duplicate) string {
	reasons := make([]string, len(d.Reasons))
	for i, r := range d.Reasons {
		reasons[i] = reasonLabel[r]
	}
	out := "« " + d.Name + " » (" + strings.Join(reasons, ", ") + ")"
	if d.Archived {
		out += ", archivé"
	}
	return out
}

// checkDuplicates applique les règles de doublon dans la transaction d'écriture.
func checkDuplicates(ctx context.Context, q *db.Queries, orgID uuid.UUID, exclude *uuid.UUID, f Fields, confirmed bool) error {
	rows, err := q.FindSupplierDuplicates(ctx, db.FindSupplierDuplicatesParams{
		OrgID: orgID, ExcludeID: exclude, Name: f.Name, Ninea: f.NINEA, Rccm: f.RCCM, Phone: f.Phone, Email: f.Email,
	})
	if err != nil {
		return err
	}
	dups := duplicatesOf(rows)
	for _, d := range dups {
		if d.Blocking {
			msg := "Ce fournisseur existe déjà : " + describe(d) + "."
			if d.Archived {
				msg += " Restaurez-le plutôt que d'en créer un nouveau."
			}
			return apperr.Conflict("supplier.duplicate", msg)
		}
	}
	if len(dups) > 0 && !confirmed {
		return apperr.Conflict("supplier.possible_duplicate",
			"Un fournisseur ressemblant existe déjà : "+describe(dups[0])+". Confirmez qu'il s'agit d'un autre fournisseur.")
	}
	return nil
}

// duplicateConstraint traduit une course entre deux créations, rattrapée par les index uniques.
func duplicateConstraint(err error) error {
	if err != nil && apperr.Normalize(err).Code == "resource.duplicate" {
		return apperr.Conflict("supplier.duplicate", "Ce fournisseur existe déjà (même nom, NINEA ou RCCM).")
	}
	return err
}

// List liste le répertoire. Les fournisseurs archivés n'apparaissent que sur demande.
func (s *Service) List(ctx context.Context, a auth.Actor, query, activity *string, includeArchived bool) ([]db.Supplier, error) {
	if err := rbac.Require(a, rbac.SupplierView); err != nil {
		return nil, err
	}
	var out []db.Supplier
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListSuppliers(ctx, db.ListSuppliersParams{OrgID: a.OrgID, Q: query, Activity: activity, IncludeArchived: includeArchived})
		return err
	})
	return out, err
}

// DuplicateQuery décrit une fiche en cours de saisie, pour signaler les doublons avant d'enregistrer.
type DuplicateQuery struct {
	Input
	ExcludeID *uuid.UUID `json:"exclude_id"`
}

// Duplicates retourne les fiches qui ressemblent à la saisie, avec les règles qui s'appliquent.
func (s *Service) Duplicates(ctx context.Context, a auth.Actor, in DuplicateQuery) ([]Duplicate, error) {
	if err := rbac.Require(a, rbac.SupplierCreate); err != nil {
		return nil, err
	}
	f, err := in.Merge(Fields{})
	if err != nil {
		return nil, err
	}
	var out []Duplicate
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rows, err := q.FindSupplierDuplicates(ctx, db.FindSupplierDuplicatesParams{
			OrgID: a.OrgID, ExcludeID: in.ExcludeID, Name: f.Name, Ninea: f.NINEA, Rccm: f.RCCM, Phone: f.Phone, Email: f.Email,
		})
		out = duplicatesOf(rows)
		return err
	})
	return out, err
}

// Create ajoute un fournisseur, après contrôle des doublons.
func (s *Service) Create(ctx context.Context, a auth.Actor, in Input) (db.Supplier, error) {
	if err := rbac.Require(a, rbac.SupplierCreate); err != nil {
		return db.Supplier{}, err
	}
	f, err := in.Merge(Fields{})
	if err != nil {
		return db.Supplier{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.Supplier{}, err
	}
	var out db.Supplier
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if err := checkDuplicates(ctx, q, a.OrgID, nil, f, in.ConfirmNotDuplicate); err != nil {
			return err
		}
		var err error
		out, err = q.CreateSupplier(ctx, db.CreateSupplierParams{
			ID: id, OrgID: a.OrgID, Name: f.Name, Phone: f.Phone, Email: f.Email, LegalName: f.LegalName, LegalForm: f.LegalForm,
			Ninea: f.NINEA, Rccm: f.RCCM, Address: f.Address, City: f.City, Activities: f.Activities, Notes: f.Notes, CreatedBy: a.UserID,
		})
		if err != nil {
			return duplicateConstraint(err)
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.created", EntityType: "supplier", EntityID: &id, After: out})
	})
	return out, err
}

func eq(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func lockEditable(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.Supplier, error) {
	sup, err := q.LockSupplier(ctx, db.LockSupplierParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return sup, apperr.NotFound("supplier")
		}
		return sup, err
	}
	if !sup.IsActive {
		return sup, apperr.Conflict("supplier.archived", "Ce fournisseur est archivé : restaurez-le pour le modifier.")
	}
	return sup, nil
}

// Update modifie la fiche ; les doublons sont contrôlés comme à la création.
func (s *Service) Update(ctx context.Context, a auth.Actor, id uuid.UUID, in Input) (db.Supplier, error) {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return db.Supplier{}, err
	}
	var out db.Supplier
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := lockEditable(ctx, q, a, id)
		if err != nil {
			return err
		}
		f, err := in.Merge(fieldsOf(before))
		if err != nil {
			return err
		}
		// Un doublon probable déjà accepté n'est redemandé que si ce qui le signale change.
		unchanged := f.Name == before.Name && eq(f.Phone, before.Phone) && eq(f.Email, before.Email)
		if err := checkDuplicates(ctx, q, a.OrgID, &id, f, in.ConfirmNotDuplicate || unchanged); err != nil {
			return err
		}
		out, err = q.UpdateSupplier(ctx, db.UpdateSupplierParams{
			OrgID: a.OrgID, ID: id, Name: f.Name, Phone: f.Phone, Email: f.Email, LegalName: f.LegalName, LegalForm: f.LegalForm,
			Ninea: f.NINEA, Rccm: f.RCCM, Address: f.Address, City: f.City, Activities: f.Activities, Notes: f.Notes,
		})
		if err != nil {
			return duplicateConstraint(err)
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.updated", EntityType: "supplier", EntityID: &id, Before: before, After: out})
	})
	return out, err
}

// Archive retire un fournisseur du répertoire et des nouvelles consultations. Son historique reste,
// et les liens déjà envoyés restent valables jusqu'à la clôture des demandes en cours.
func (s *Service) Archive(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Supplier, error) {
	return s.setArchived(ctx, a, id, true)
}

// Restore remet un fournisseur archivé dans le répertoire.
func (s *Service) Restore(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Supplier, error) {
	return s.setArchived(ctx, a, id, false)
}

func (s *Service) setArchived(ctx context.Context, a auth.Actor, id uuid.UUID, archive bool) (db.Supplier, error) {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return db.Supplier{}, err
	}
	var out db.Supplier
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		current, err := q.LockSupplier(ctx, db.LockSupplierParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("supplier")
			}
			return err
		}
		action := "supplier.archived"
		if archive {
			if !current.IsActive {
				return apperr.Conflict("supplier.already_archived", "Ce fournisseur est déjà archivé.")
			}
			out, err = q.ArchiveSupplier(ctx, db.ArchiveSupplierParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
		} else {
			if current.IsActive {
				return apperr.Conflict("supplier.not_archived", "Ce fournisseur n'est pas archivé.")
			}
			action = "supplier.restored"
			out, err = q.RestoreSupplier(ctx, db.RestoreSupplierParams{OrgID: a.OrgID, ID: id})
		}
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: action, EntityType: "supplier", EntityID: &id})
	})
	return out, err
}

// Get retourne la fiche, ses contacts et, selon le rôle, ses conditions commerciales en vigueur.
func (s *Service) Get(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	if err := rbac.Require(a, rbac.SupplierView); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		sup, err := q.GetSupplier(ctx, db.GetSupplierParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("supplier")
			}
			return err
		}
		out.Supplier = sup
		if out.Contacts, err = q.ListSupplierContacts(ctx, db.ListSupplierContactsParams{OrgID: a.OrgID, SupplierID: id}); err != nil {
			return err
		}
		if rbac.Can(a.Role, rbac.SupplierTrade) {
			terms, err := q.ListSupplierTerms(ctx, db.ListSupplierTermsParams{OrgID: a.OrgID, SupplierID: id})
			if err != nil {
				return err
			}
			if len(terms) > 0 {
				out.Terms = &terms[0]
			}
		}
		return nil
	})
	return out, err
}

func exists(ctx context.Context, q *db.Queries, orgID, id uuid.UUID) error {
	if _, err := q.GetSupplier(ctx, db.GetSupplierParams{OrgID: orgID, ID: id}); err != nil {
		if db.IsNoRows(err) {
			return apperr.NotFound("supplier")
		}
		return err
	}
	return nil
}

// Terms retourne toutes les versions des conditions commerciales, la plus récente d'abord.
func (s *Service) Terms(ctx context.Context, a auth.Actor, id uuid.UUID) ([]db.ListSupplierTermsRow, error) {
	if err := rbac.Require(a, rbac.SupplierTrade); err != nil {
		return nil, err
	}
	var out []db.ListSupplierTermsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if err := exists(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		var err error
		out, err = q.ListSupplierTerms(ctx, db.ListSupplierTermsParams{OrgID: a.OrgID, SupplierID: id})
		return err
	})
	return out, err
}

// RecordTerms enregistre de nouvelles conditions commerciales : une version de plus, l'ancienne reste.
func (s *Service) RecordTerms(ctx context.Context, a auth.Actor, id uuid.UUID, in TermsInput) (db.SupplierTerm, error) {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return db.SupplierTerm{}, err
	}
	if err := in.Validate(); err != nil {
		return db.SupplierTerm{}, err
	}
	termsID, err := uuid.NewV7()
	if err != nil {
		return db.SupplierTerm{}, err
	}
	var out db.SupplierTerm
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := lockEditable(ctx, q, a, id); err != nil {
			return err
		}
		version, err := q.NextSupplierTermsVersion(ctx, db.NextSupplierTermsVersionParams{OrgID: a.OrgID, SupplierID: id})
		if err != nil {
			return err
		}
		effective := in.EffectiveFrom
		if effective == nil {
			today, err := q.TodayInDakar(ctx)
			if err != nil {
				return err
			}
			effective = &today
		}
		out, err = q.InsertSupplierTerms(ctx, db.InsertSupplierTermsParams{
			ID: termsID, OrgID: a.OrgID, SupplierID: id, Version: version, EffectiveFrom: *effective,
			PaymentMethod: in.PaymentMethod, PaymentDelayDays: in.PaymentDelayDays, AdvancePct: in.AdvancePct,
			DiscountPct: in.DiscountPct, DeliveryDays: in.DeliveryDays, DeliveryIncluded: in.DeliveryIncluded,
			MinimumOrderAmount: in.MinimumOrderAmount, CreditLimit: in.CreditLimit, Notes: in.Notes, CreatedBy: a.UserID,
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.terms_recorded", EntityType: "supplier", EntityID: &id, After: out})
	})
	return out, err
}

// History retourne les consultations et les achats du fournisseur, sur les chantiers visibles.
func (s *Service) History(ctx context.Context, a auth.Actor, id uuid.UUID) (History, error) {
	if err := rbac.Require(a, rbac.SupplierTrade); err != nil {
		return History{}, err
	}
	var out History
	all := rbac.SeesAllProjects(a)
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if err := exists(ctx, q, a.OrgID, id); err != nil {
			return err
		}
		var err error
		if out.Consultations, err = q.SupplierConsultations(ctx, db.SupplierConsultationsParams{OrgID: a.OrgID, SupplierID: id, AllProjects: all, UserID: a.UserID}); err != nil {
			return err
		}
		if out.Purchases, err = q.SupplierPurchases(ctx, db.SupplierPurchasesParams{OrgID: a.OrgID, SupplierID: &id, AllProjects: all, UserID: a.UserID}); err != nil {
			return err
		}
		if out.Orders, err = q.SupplierOrders(ctx, db.SupplierOrdersParams{OrgID: a.OrgID, SupplierID: id, AllProjects: all, UserID: a.UserID}); err != nil {
			return err
		}
		out.Totals, err = q.SupplierPurchaseTotals(ctx, db.SupplierPurchaseTotalsParams{OrgID: a.OrgID, SupplierID: &id, AllProjects: all, UserID: a.UserID})
		return err
	})
	return out, err
}

// AddContact ajoute un contact ; un nouveau contact principal remplace le précédent.
func (s *Service) AddContact(ctx context.Context, a auth.Actor, supplierID uuid.UUID, in ContactInput) (db.SupplierContact, error) {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return db.SupplierContact{}, err
	}
	c, err := in.Normalize()
	if err != nil {
		return db.SupplierContact{}, err
	}
	contactID, err := uuid.NewV7()
	if err != nil {
		return db.SupplierContact{}, err
	}
	var out db.SupplierContact
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := lockEditable(ctx, q, a, supplierID); err != nil {
			return err
		}
		if c.IsPrimary {
			if err := q.ClearPrimaryContact(ctx, db.ClearPrimaryContactParams{OrgID: a.OrgID, SupplierID: supplierID}); err != nil {
				return err
			}
		}
		var err error
		out, err = q.CreateSupplierContact(ctx, db.CreateSupplierContactParams{
			ID: contactID, OrgID: a.OrgID, SupplierID: supplierID, FullName: c.FullName, JobTitle: c.JobTitle,
			Phone: c.Phone, Email: c.Email, IsPrimary: c.IsPrimary, Notes: c.Notes, CreatedBy: a.UserID,
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.contact_added", EntityType: "supplier", EntityID: &supplierID, After: out})
	})
	return out, err
}

// contactForEdit retrouve le contact et verrouille son fournisseur, qui doit être actif.
func contactForEdit(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.SupplierContact, error) {
	c, err := q.GetSupplierContact(ctx, db.GetSupplierContactParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return c, apperr.NotFound("supplier_contact")
		}
		return c, err
	}
	_, err = lockEditable(ctx, q, a, c.SupplierID)
	return c, err
}

// UpdateContact remplace un contact.
func (s *Service) UpdateContact(ctx context.Context, a auth.Actor, id uuid.UUID, in ContactInput) (db.SupplierContact, error) {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return db.SupplierContact{}, err
	}
	c, err := in.Normalize()
	if err != nil {
		return db.SupplierContact{}, err
	}
	var out db.SupplierContact
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := contactForEdit(ctx, q, a, id)
		if err != nil {
			return err
		}
		if c.IsPrimary && !before.IsPrimary {
			if err := q.ClearPrimaryContact(ctx, db.ClearPrimaryContactParams{OrgID: a.OrgID, SupplierID: before.SupplierID}); err != nil {
				return err
			}
		}
		out, err = q.UpdateSupplierContact(ctx, db.UpdateSupplierContactParams{
			OrgID: a.OrgID, ID: id, FullName: c.FullName, JobTitle: c.JobTitle, Phone: c.Phone, Email: c.Email, IsPrimary: c.IsPrimary, Notes: c.Notes,
		})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.contact_updated", EntityType: "supplier", EntityID: &before.SupplierID, Before: before, After: out})
	})
	return out, err
}

// DeleteContact retire un contact ; le journal d'audit en garde la trace.
func (s *Service) DeleteContact(ctx context.Context, a auth.Actor, id uuid.UUID) error {
	if err := rbac.Require(a, rbac.SupplierEdit); err != nil {
		return err
	}
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		before, err := contactForEdit(ctx, q, a, id)
		if err != nil {
			return err
		}
		if _, err := q.DeleteSupplierContact(ctx, db.DeleteSupplierContactParams{OrgID: a.OrgID, ID: id}); err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "supplier.contact_removed", EntityType: "supplier", EntityID: &before.SupplierID, Before: before})
	})
}
