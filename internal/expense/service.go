// Package expense gère les dépenses, leur workflow d'approbation et leurs paiements.
package expense

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/civil"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Statuts de validation (libellés des maquettes : Brouillon, À valider, Approuvée, Rejetée, Annulée).
const (
	StatusDraft     = "draft"
	StatusPending   = "pending"
	StatusApproved  = "approved"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
)

// Service porte les cas d'usage des dépenses.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// CreateInput décrit une nouvelle dépense. L'id, généré par le client, rend le renvoi idempotent.
type CreateInput struct {
	ID          *uuid.UUID  `json:"id"`
	CategoryID  uuid.UUID   `json:"category_id"`
	SupplierID  *uuid.UUID  `json:"supplier_id"`
	Description string      `json:"description"`
	Amount      int64       `json:"amount"`
	ExpenseDate *civil.Date `json:"expense_date"`
	// SupplierName permet de saisir un fournisseur par son nom, y compris hors ligne.
	SupplierName *string `json:"supplier_name"`
	Comment      *string `json:"comment"`
	// Déclaration du saisisseur : le paiement n'existe qu'une fois enregistré par Finance.
	DeclaredPaidOn        *civil.Date `json:"declared_paid_on"`
	DeclaredPaymentMethod *string     `json:"declared_payment_method"`
}

func validateCore(v *apperr.Validator, description string, amount int64, date *civil.Date) {
	n := len(strings.TrimSpace(description))
	v.Check(n >= 2 && n <= 500, "description", "La description doit contenir entre 2 et 500 caractères.")
	v.Check(amount > 0, "amount", "Le montant doit être strictement positif.")
	v.Check(date != nil, "expense_date", "La date est obligatoire.")
	if date != nil {
		v.Check(!date.After(time.Now().AddDate(0, 0, 1)), "expense_date", "La date ne peut pas être dans le futur.")
	}
}

func requireActiveCategory(ctx context.Context, q *db.Queries, orgID, id uuid.UUID) error {
	c, err := q.GetCategory(ctx, db.GetCategoryParams{OrgID: orgID, ID: id})
	var v apperr.Validator
	if err != nil {
		if !db.IsNoRows(err) {
			return err
		}
		v.Check(false, "category_id", "Catégorie introuvable.")
		return v.Err()
	}
	v.Check(c.IsActive, "category_id", "Cette catégorie est désactivée.")
	return v.Err()
}

// Create enregistre une dépense en brouillon. La devise est celle de l'entreprise (XOF par défaut).
func (s *Service) Create(ctx context.Context, a auth.Actor, projectID uuid.UUID, in CreateInput) (db.Expense, bool, error) {
	if err := rbac.Require(a, rbac.ExpenseCreate); err != nil {
		return db.Expense{}, false, err
	}
	var v apperr.Validator
	validateCore(&v, in.Description, in.Amount, in.ExpenseDate)
	v.Check(in.CategoryID != uuid.Nil, "category_id", "La catégorie est obligatoire.")
	if in.DeclaredPaymentMethod != nil {
		v.Check(paymentMethods[*in.DeclaredPaymentMethod], "declared_payment_method", "Moyen de paiement inconnu.")
	}
	if in.SupplierName != nil {
		n := len(strings.TrimSpace(*in.SupplierName))
		v.Check(n == 0 || (n >= 2 && n <= 160), "supplier_name", "Le nom du fournisseur doit contenir entre 2 et 160 caractères.")
	}
	if err := v.Err(); err != nil {
		return db.Expense{}, false, err
	}
	var id uuid.UUID
	if in.ID != nil {
		id = *in.ID
	} else {
		var err error
		if id, err = uuid.NewV7(); err != nil {
			return db.Expense{}, false, err
		}
	}
	var out db.Expense
	created := false
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		if err := requireActiveCategory(ctx, q, a.OrgID, in.CategoryID); err != nil {
			return err
		}
		org, err := q.GetOrganization(ctx, a.OrgID)
		if err != nil {
			return err
		}
		supplierID := in.SupplierID
		if supplierID == nil && in.SupplierName != nil && strings.TrimSpace(*in.SupplierName) != "" {
			newID, err := uuid.NewV7()
			if err != nil {
				return err
			}
			supplier, err := q.UpsertSupplierByName(ctx, db.UpsertSupplierByNameParams{
				ID: newID, OrgID: a.OrgID, Name: strings.TrimSpace(*in.SupplierName), CreatedBy: a.UserID,
			})
			if err != nil {
				return err
			}
			supplierID = &supplier.ID
		}
		out, err = q.CreateExpense(ctx, db.CreateExpenseParams{
			ID: id, OrgID: a.OrgID, ProjectID: projectID, CategoryID: in.CategoryID, SupplierID: supplierID,
			Description: strings.TrimSpace(in.Description), Amount: in.Amount, Currency: org.Currency,
			ExpenseDate: *in.ExpenseDate, Comment: in.Comment, DeclaredPaidOn: in.DeclaredPaidOn,
			DeclaredPaymentMethod: in.DeclaredPaymentMethod, CreatedBy: a.UserID,
		})
		if db.IsNoRows(err) {
			// Renvoi d'un id déjà connu. Il peut appartenir à un autre tenant : la RLS le rend alors invisible.
			out, err = q.GetExpense(ctx, db.GetExpenseParams{OrgID: a.OrgID, ID: id})
			if db.IsNoRows(err) {
				return apperr.Conflict("expense.id_conflict", "Cet identifiant de dépense n'est pas utilisable.")
			}
			return err
		}
		if err != nil {
			return err
		}
		created = true
		return audit.Record(ctx, q, a, audit.Entry{Action: "expense.created", EntityType: "expense", EntityID: &id, ProjectID: &projectID, After: out})
	})
	return out, created, err
}

// load charge une dépense et vérifie l'accès au chantier.
func load(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID, forUpdate bool) (db.Expense, rbac.ProjectAccess, error) {
	var e db.Expense
	var err error
	if forUpdate {
		e, err = q.GetExpenseForUpdate(ctx, db.GetExpenseForUpdateParams{OrgID: a.OrgID, ID: id})
	} else {
		e, err = q.GetExpense(ctx, db.GetExpenseParams{OrgID: a.OrgID, ID: id})
	}
	if err != nil {
		if db.IsNoRows(err) {
			return e, rbac.ProjectAccess{}, apperr.NotFound("expense")
		}
		return e, rbac.ProjectAccess{}, err
	}
	access, err := rbac.Project(ctx, q, a, e.ProjectID)
	return e, access, err
}

// Detail est une dépense avec son historique, ses paiements et ses justificatifs.
type Detail struct {
	db.Expense
	ProjectName   string                        `json:"project_name"`
	CategoryName  string                        `json:"category_name"`
	SupplierName  *string                       `json:"supplier_name"`
	CreatedByName string                        `json:"created_by_name"`
	Approvals     []db.ListExpenseApprovalsRow  `json:"approvals"`
	Payments      []db.ListPaymentsRow          `json:"payments"`
	Attachments   []db.Attachment               `json:"attachments"`
	Duplicates    []db.ListExpenseDuplicatesRow `json:"duplicates"`
}

// Get retourne le détail d'une dépense.
func (s *Service) Get(ctx context.Context, a auth.Actor, id uuid.UUID) (Detail, error) {
	if err := rbac.Require(a, rbac.ExpenseView); err != nil {
		return Detail{}, err
	}
	var out Detail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, _, err := load(ctx, q, a, id, false)
		if err != nil {
			return err
		}
		out.Expense = e
		labels, err := q.GetExpenseLabels(ctx, db.GetExpenseLabelsParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		out.ProjectName, out.CategoryName = labels.ProjectName, labels.CategoryName
		out.SupplierName, out.CreatedByName = labels.SupplierName, labels.CreatedByName
		if out.Duplicates, err = q.ListExpenseDuplicates(ctx, db.ListExpenseDuplicatesParams{OrgID: a.OrgID, ID: id}); err != nil {
			return err
		}
		if out.Approvals, err = q.ListExpenseApprovals(ctx, db.ListExpenseApprovalsParams{OrgID: a.OrgID, ExpenseID: id}); err != nil {
			return err
		}
		if out.Payments, err = q.ListPayments(ctx, db.ListPaymentsParams{OrgID: a.OrgID, ExpenseID: id}); err != nil {
			return err
		}
		out.Attachments, err = q.ListExpenseAttachments(ctx, db.ListExpenseAttachmentsParams{OrgID: a.OrgID, ExpenseID: &id})
		return err
	})
	return out, err
}

// ListFilter décrit les filtres de dépenses (dont la file de validation : status=pending).
type ListFilter struct {
	ProjectID *uuid.UUID
	Status    *string
	// Payment : "paid" (soldées) ou "unpaid" (approuvées restant à payer).
	Payment    *string
	CategoryID *uuid.UUID
	DateFrom   *civil.Date
	DateTo     *civil.Date
	Q          *string
	CursorDate *civil.Date
	CursorID   *uuid.UUID
	Limit      int32
}

// List liste les dépenses visibles par l'acteur.
func (s *Service) List(ctx context.Context, a auth.Actor, f ListFilter) ([]db.ListExpensesRow, error) {
	if err := rbac.Require(a, rbac.ExpenseView); err != nil {
		return nil, err
	}
	var out []db.ListExpensesRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if f.ProjectID != nil {
			if _, err := rbac.Project(ctx, q, a, *f.ProjectID); err != nil {
				return err
			}
		}
		var err error
		var paymentStatus *string
		unpaidOnly := false
		if f.Payment != nil {
			switch *f.Payment {
			case "paid":
				paymentStatus = f.Payment
			case "unpaid":
				unpaidOnly = true
			}
		}
		out, err = q.ListExpenses(ctx, db.ListExpensesParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID, ProjectID: f.ProjectID,
			PaymentStatus: paymentStatus, UnpaidOnly: unpaidOnly,
			Status: f.Status, CategoryID: f.CategoryID, DateFrom: f.DateFrom, DateTo: f.DateTo, Q: f.Q,
			CursorDate: f.CursorDate, CursorID: f.CursorID, PageSize: f.Limit,
		})
		return err
	})
	return out, err
}

// Counts alimente les onglets de la file de dépenses.
type Counts struct {
	Pending       int64 `json:"pending"`
	PendingAmount int64 `json:"pending_amount"`
	Approved      int64 `json:"approved"`
	Paid          int64 `json:"paid"`
	Rejected      int64 `json:"rejected"`
	Draft         int64 `json:"draft"`
	All           int64 `json:"all"`
}

// Count compte les dépenses visibles par statut, sur les mêmes filtres que la liste.
func (s *Service) Count(ctx context.Context, a auth.Actor, f ListFilter) (Counts, error) {
	if err := rbac.Require(a, rbac.ExpenseView); err != nil {
		return Counts{}, err
	}
	var out Counts
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		rows, err := q.CountExpenses(ctx, db.CountExpensesParams{
			OrgID: a.OrgID, AllProjects: rbac.SeesAllProjects(a), UserID: a.UserID,
			ProjectID: f.ProjectID, CategoryID: f.CategoryID, DateFrom: f.DateFrom, DateTo: f.DateTo,
		})
		if err != nil {
			return err
		}
		for _, r := range rows {
			out.All += r.Total
			switch r.Status {
			case StatusPending:
				out.Pending += r.Total
				out.PendingAmount += r.Amount
			case StatusApproved:
				if r.PaymentStatus == "paid" {
					out.Paid += r.Total
				} else {
					out.Approved += r.Total
				}
			case StatusRejected:
				out.Rejected += r.Total
			case StatusDraft:
				out.Draft += r.Total
			}
		}
		return nil
	})
	return out, err
}

// UpdateInput décrit la modification d'un brouillon.
type UpdateInput struct {
	CategoryID  *uuid.UUID  `json:"category_id"`
	SupplierID  *uuid.UUID  `json:"supplier_id"`
	Description *string     `json:"description"`
	Amount      *int64      `json:"amount"`
	ExpenseDate *civil.Date `json:"expense_date"`
}

// Update modifie un brouillon : seuls son auteur et l'administrateur peuvent le faire.
func (s *Service) Update(ctx context.Context, a auth.Actor, id uuid.UUID, version int32, in UpdateInput) (db.Expense, error) {
	if err := rbac.Require(a, rbac.ExpenseCreate); err != nil {
		return db.Expense{}, err
	}
	var v apperr.Validator
	if in.Description != nil {
		n := len(strings.TrimSpace(*in.Description))
		v.Check(n >= 2 && n <= 500, "description", "La description doit contenir entre 2 et 500 caractères.")
	}
	if in.Amount != nil {
		v.Check(*in.Amount > 0, "amount", "Le montant doit être strictement positif.")
	}
	if err := v.Err(); err != nil {
		return db.Expense{}, err
	}
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if e.CreatedBy != a.UserID && a.Role != rbac.RoleOwner {
			return apperr.Forbidden()
		}
		if e.Status != StatusDraft {
			return apperr.Conflict("expense.not_draft", "Seule une dépense en brouillon peut être modifiée.")
		}
		if in.CategoryID != nil {
			if err := requireActiveCategory(ctx, q, a.OrgID, *in.CategoryID); err != nil {
				return err
			}
		}
		out, err = q.UpdateDraftExpense(ctx, db.UpdateDraftExpenseParams{
			OrgID: a.OrgID, ID: id, RowVersion: version, CategoryID: in.CategoryID, SupplierID: in.SupplierID,
			Description: in.Description, Amount: in.Amount, ExpenseDate: in.ExpenseDate,
		})
		if db.IsNoRows(err) {
			return apperr.StaleVersion()
		}
		return err
	})
	return out, err
}

func requireWritable(access rbac.ProjectAccess) error {
	if access.Status == "archived" {
		return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
	}
	return nil
}

func recordEvent(ctx context.Context, q *db.Queries, a auth.Actor, e db.Expense, event, from string, comment *string) error {
	if err := q.InsertExpenseApproval(ctx, db.InsertExpenseApprovalParams{
		OrgID: a.OrgID, ExpenseID: e.ID, Event: event, FromStatus: from, ToStatus: e.Status, ActorID: a.UserID, Comment: comment,
	}); err != nil {
		return err
	}
	return audit.Record(ctx, q, a, audit.Entry{
		Action: "expense." + event, EntityType: "expense", EntityID: &e.ID, ProjectID: &e.ProjectID,
		Before: map[string]string{"status": from},
		After:  map[string]any{"status": e.Status, "amount": e.Amount, "comment": comment},
	})
}

// Submit soumet un brouillon à validation. Le changement de statut et l'événement sont écrits dans
// la même transaction ; une double soumission échoue sans créer de second événement.
func (s *Service) Submit(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Expense, error) {
	if err := rbac.Require(a, rbac.ExpenseCreate); err != nil {
		return db.Expense{}, err
	}
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if e.CreatedBy != a.UserID && a.Role != rbac.RoleOwner {
			return apperr.Forbidden()
		}
		if e.Status != StatusDraft {
			return apperr.Conflict("expense.not_draft", "Seule une dépense en brouillon peut être soumise.")
		}
		out, err = q.SubmitExpense(ctx, db.SubmitExpenseParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			return err
		}
		return recordEvent(ctx, q, a, out, "submitted", e.Status, nil)
	})
	return out, err
}

// DecisionInput décrit une décision d'approbation.
type DecisionInput struct {
	Comment *string `json:"comment"`
}

// Approve approuve une dépense en attente. Personne n'approuve sa propre dépense.
func (s *Service) Approve(ctx context.Context, a auth.Actor, id uuid.UUID, in DecisionInput) (db.Expense, error) {
	if err := rbac.Require(a, rbac.ExpenseApprove); err != nil {
		return db.Expense{}, err
	}
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if err := rbac.CanApproveExpense(a, access, e.Amount); err != nil {
			return err
		}
		if e.CreatedBy == a.UserID {
			return apperr.New(http.StatusForbidden, "expense.self_approval", "Vous ne pouvez pas approuver votre propre dépense.")
		}
		if e.Status != StatusPending {
			return apperr.Conflict("expense.not_pending", "Seule une dépense en attente peut être approuvée.")
		}
		out, err = q.ApproveExpense(ctx, db.ApproveExpenseParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID, Comment: in.Comment})
		if err != nil {
			return err
		}
		return recordEvent(ctx, q, a, out, "approved", e.Status, in.Comment)
	})
	return out, err
}

// Reject rejette une dépense en attente ; le motif est obligatoire.
func (s *Service) Reject(ctx context.Context, a auth.Actor, id uuid.UUID, in DecisionInput) (db.Expense, error) {
	if err := rbac.Require(a, rbac.ExpenseApprove); err != nil {
		return db.Expense{}, err
	}
	var v apperr.Validator
	v.Check(in.Comment != nil && strings.TrimSpace(*in.Comment) != "", "comment", "Le motif du rejet est obligatoire.")
	if err := v.Err(); err != nil {
		return db.Expense{}, err
	}
	comment := strings.TrimSpace(*in.Comment)
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if err := rbac.CanApproveExpense(a, access, e.Amount); err != nil {
			return err
		}
		if e.CreatedBy == a.UserID {
			return apperr.New(http.StatusForbidden, "expense.self_approval", "Vous ne pouvez pas décider de votre propre dépense.")
		}
		if e.Status != StatusPending {
			return apperr.Conflict("expense.not_pending", "Seule une dépense en attente peut être rejetée.")
		}
		out, err = q.RejectExpense(ctx, db.RejectExpenseParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID, Comment: &comment})
		if err != nil {
			return err
		}
		return recordEvent(ctx, q, a, out, "rejected", e.Status, &comment)
	})
	return out, err
}

// BackToDraft retire une dépense en attente ou reprend une dépense rejetée (par son auteur).
func (s *Service) BackToDraft(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Expense, error) {
	if err := rbac.Require(a, rbac.ExpenseCreate); err != nil {
		return db.Expense{}, err
	}
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if e.CreatedBy != a.UserID {
			return apperr.Forbidden()
		}
		var event string
		switch e.Status {
		case StatusPending:
			event = "withdrawn"
		case StatusRejected:
			event = "reopened"
		default:
			return apperr.Conflict("expense.invalid_transition", "Cette dépense ne peut pas revenir en brouillon.")
		}
		out, err = q.ReturnExpenseToDraft(ctx, db.ReturnExpenseToDraftParams{OrgID: a.OrgID, ID: id, FromStatus: e.Status})
		if err != nil {
			return err
		}
		return recordEvent(ctx, q, a, out, event, e.Status, nil)
	})
	return out, err
}

// Cancel annule un brouillon (auteur ou administrateur) ou une dépense approuvée sans paiement
// (Owner ou Finance). Une dépense n'est jamais supprimée physiquement.
func (s *Service) Cancel(ctx context.Context, a auth.Actor, id uuid.UUID, in DecisionInput) (db.Expense, error) {
	var out db.Expense
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, id, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		switch e.Status {
		case StatusDraft:
			if e.CreatedBy != a.UserID && a.Role != rbac.RoleOwner {
				return apperr.Forbidden()
			}
		case StatusApproved:
			if err := rbac.Require(a, rbac.PaymentRecord); err != nil {
				return err
			}
			if e.AmountPaid > 0 {
				return apperr.Conflict("expense.has_payments", "Une dépense déjà payée ne peut pas être annulée.")
			}
		default:
			return apperr.Conflict("expense.invalid_transition", "Cette dépense ne peut pas être annulée.")
		}
		out, err = q.CancelExpense(ctx, db.CancelExpenseParams{OrgID: a.OrgID, ID: id, Comment: in.Comment})
		if err != nil {
			return err
		}
		return recordEvent(ctx, q, a, out, "cancelled", e.Status, in.Comment)
	})
	return out, err
}

// PaymentInput décrit un paiement.
type PaymentInput struct {
	Amount    int64       `json:"amount"`
	PaidOn    *civil.Date `json:"paid_on"`
	Method    string      `json:"method"`
	Reference *string     `json:"reference"`
}

var paymentMethods = map[string]bool{
	"cash": true, "wave": true, "orange_money": true, "free_money": true,
	"bank_transfer": true, "cheque": true, "other": true,
}

// RemainingAfter calcule le solde après un paiement et refuse tout dépassement.
func RemainingAfter(amount, paid, payment int64) (int64, bool) {
	if payment <= 0 || payment > amount-paid {
		return amount - paid, false
	}
	return amount - paid - payment, true
}

// PaymentResult est un paiement et le solde qui en résulte.
type PaymentResult struct {
	Payment   db.Payment `json:"payment"`
	Expense   db.Expense `json:"expense"`
	Remaining int64      `json:"remaining"`
}

// AddPayment enregistre un paiement. Le verrou FOR UPDATE sur la dépense sérialise les paiements
// concurrents : le cumul ne peut jamais dépasser le montant.
func (s *Service) AddPayment(ctx context.Context, a auth.Actor, expenseID uuid.UUID, in PaymentInput) (PaymentResult, error) {
	if err := rbac.Require(a, rbac.PaymentRecord); err != nil {
		return PaymentResult{}, err
	}
	var v apperr.Validator
	v.Check(in.Amount > 0, "amount", "Le montant doit être strictement positif.")
	v.Check(in.PaidOn != nil, "paid_on", "La date de paiement est obligatoire.")
	v.Check(paymentMethods[in.Method], "method", "Moyen de paiement inconnu.")
	if err := v.Err(); err != nil {
		return PaymentResult{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return PaymentResult{}, err
	}
	var out PaymentResult
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		e, access, err := load(ctx, q, a, expenseID, true)
		if err != nil {
			return err
		}
		if err := requireWritable(access); err != nil {
			return err
		}
		if e.Status != StatusApproved {
			return apperr.Conflict("expense.not_approved", "Seule une dépense approuvée peut être payée.")
		}
		remaining, ok := RemainingAfter(e.Amount, e.AmountPaid, in.Amount)
		if !ok {
			return apperr.Conflict("payment.exceeds_remaining", "Le paiement dépasse le solde restant de la dépense.")
		}
		out.Payment, err = q.InsertPayment(ctx, db.InsertPaymentParams{
			ID: id, OrgID: a.OrgID, ExpenseID: expenseID, Amount: in.Amount, PaidOn: *in.PaidOn,
			Method: in.Method, Reference: in.Reference, RecordedBy: a.UserID,
		})
		if err != nil {
			return err
		}
		out.Expense, err = q.ApplyPaymentToExpense(ctx, db.ApplyPaymentToExpenseParams{OrgID: a.OrgID, ID: expenseID, Delta: in.Amount})
		if err != nil {
			return err
		}
		out.Remaining = remaining
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "payment.recorded", EntityType: "expense", EntityID: &expenseID, ProjectID: &e.ProjectID,
			After: map[string]any{"amount": in.Amount, "method": in.Method, "remaining": remaining},
		})
	})
	return out, err
}
