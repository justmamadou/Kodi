// Package budget gère les budgets versionnés : brouillon, publication immuable, révisions, comparaison.
package budget

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
	"github.com/shopspring/decimal"
)

// Service porte les cas d'usage budgétaires.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// CreateDraft ouvre la première version d'un chantier. Les suivantes passent par Revise (motif obligatoire).
func (s *Service) CreateDraft(ctx context.Context, a auth.Actor, projectID uuid.UUID) (db.BudgetVersion, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return db.BudgetVersion{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.BudgetVersion{}, err
	}
	var out db.BudgetVersion
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		if _, err := q.LockProject(ctx, db.LockProjectParams{OrgID: a.OrgID, ID: projectID}); err != nil {
			return err
		}
		no, err := q.NextBudgetVersionNo(ctx, db.NextBudgetVersionNoParams{OrgID: a.OrgID, ProjectID: projectID})
		if err != nil {
			return err
		}
		if no != 1 {
			return apperr.Conflict("budget.revision_required", "Ce chantier a déjà un budget : créez une révision en indiquant son motif.")
		}
		out, err = q.CreateBudgetVersion(ctx, db.CreateBudgetVersionParams{ID: id, OrgID: a.OrgID, ProjectID: projectID, VersionNo: no, CreatedBy: a.UserID})
		return err
	})
	return out, err
}

// VersionDetail est une version avec ses lignes.
type VersionDetail struct {
	db.BudgetVersion
	Lines []db.ListBudgetLinesRow `json:"lines"`
}

func (s *Service) loadVersion(ctx context.Context, q *db.Queries, a auth.Actor, id uuid.UUID) (db.BudgetVersion, rbac.ProjectAccess, error) {
	v, err := q.GetBudgetVersion(ctx, db.GetBudgetVersionParams{OrgID: a.OrgID, ID: id})
	if err != nil {
		if db.IsNoRows(err) {
			return v, rbac.ProjectAccess{}, apperr.NotFound("budget")
		}
		return v, rbac.ProjectAccess{}, err
	}
	access, err := rbac.Project(ctx, q, a, v.ProjectID)
	return v, access, err
}

// GetVersion retourne une version et ses lignes.
func (s *Service) GetVersion(ctx context.Context, a auth.Actor, id uuid.UUID) (VersionDetail, error) {
	if err := rbac.Require(a, rbac.BudgetView); err != nil {
		return VersionDetail{}, err
	}
	var out VersionDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		v, _, err := s.loadVersion(ctx, q, a, id)
		if err != nil {
			return err
		}
		lines, err := q.ListBudgetLines(ctx, db.ListBudgetLinesParams{OrgID: a.OrgID, BudgetVersionID: id})
		out = VersionDetail{BudgetVersion: v, Lines: lines}
		return err
	})
	return out, err
}

// ListVersions retourne l'historique des versions d'un chantier.
func (s *Service) ListVersions(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]db.ListBudgetVersionsRow, error) {
	if err := rbac.Require(a, rbac.BudgetView); err != nil {
		return nil, err
	}
	var out []db.ListBudgetVersionsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListBudgetVersions(ctx, db.ListBudgetVersionsParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}

// LineInput décrit une ligne de budget. Le montant n'en fait pas partie : il est calculé par la base.
type LineInput struct {
	CategoryID *uuid.UUID       `json:"category_id"`
	Label      *string          `json:"label"`
	Quantity   *decimal.Decimal `json:"quantity"`
	Unit       *string          `json:"unit"`
	UnitPrice  *int64           `json:"unit_price"`
	Position   *int32           `json:"position"`
}

func validateLine(v *apperr.Validator, in LineInput, creating bool) {
	if creating {
		v.Check(in.CategoryID != nil, "category_id", "La catégorie est obligatoire.")
		v.Check(in.Label != nil, "label", "Le libellé est obligatoire.")
		v.Check(in.Quantity != nil, "quantity", "La quantité est obligatoire.")
		v.Check(in.Unit != nil, "unit", "L'unité est obligatoire.")
		v.Check(in.UnitPrice != nil, "unit_price", "Le prix unitaire est obligatoire.")
	}
	if in.Label != nil {
		n := len(strings.TrimSpace(*in.Label))
		v.Check(n >= 2 && n <= 200, "label", "Le libellé doit contenir entre 2 et 200 caractères.")
	}
	if in.Quantity != nil {
		v.Check(in.Quantity.IsPositive(), "quantity", "La quantité doit être strictement positive.")
		v.Check(in.Quantity.Equal(in.Quantity.Truncate(3)), "quantity", "La quantité accepte au plus 3 décimales.")
		v.Check(in.Quantity.LessThan(decimal.New(1, 11)), "quantity", "La quantité est trop grande.")
	}
	if in.Unit != nil {
		n := len(strings.TrimSpace(*in.Unit))
		v.Check(n >= 1 && n <= 20, "unit", "L'unité doit contenir entre 1 et 20 caractères.")
	}
	if in.UnitPrice != nil {
		v.Check(*in.UnitPrice >= 0, "unit_price", "Le prix unitaire ne peut pas être négatif.")
	}
}

func requireDraft(v db.BudgetVersion) error {
	if v.Status != "draft" {
		return apperr.Conflict("budget.version_immutable", "Seule une version en brouillon peut être modifiée.")
	}
	return nil
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

// AddLine ajoute une ligne à un brouillon et recalcule le total.
func (s *Service) AddLine(ctx context.Context, a auth.Actor, versionID uuid.UUID, in LineInput) (VersionDetail, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return VersionDetail{}, err
	}
	var v apperr.Validator
	validateLine(&v, in, true)
	if err := v.Err(); err != nil {
		return VersionDetail{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return VersionDetail{}, err
	}
	key, err := uuid.NewV7()
	if err != nil {
		return VersionDetail{}, err
	}
	position := int32(0)
	if in.Position != nil {
		position = *in.Position
	}
	var out VersionDetail
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		version, access, err := s.loadVersion(ctx, q, a, versionID)
		if err != nil {
			return err
		}
		if err := writable(access, version); err != nil {
			return err
		}
		if err := requireActiveCategory(ctx, q, a.OrgID, *in.CategoryID); err != nil {
			return err
		}
		if _, err := q.CreateBudgetLine(ctx, db.CreateBudgetLineParams{
			ID: id, OrgID: a.OrgID, BudgetVersionID: versionID, LineKey: key, CategoryID: *in.CategoryID,
			Label: strings.TrimSpace(*in.Label), Quantity: *in.Quantity, Unit: strings.TrimSpace(*in.Unit),
			UnitPrice: *in.UnitPrice, Position: position,
		}); err != nil {
			return err
		}
		out, err = reload(ctx, q, a.OrgID, versionID)
		return err
	})
	return out, err
}

func writable(access rbac.ProjectAccess, v db.BudgetVersion) error {
	if access.Status == "archived" {
		return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
	}
	return requireDraft(v)
}

func reload(ctx context.Context, q *db.Queries, orgID, versionID uuid.UUID) (VersionDetail, error) {
	v, err := q.RecalcBudgetTotal(ctx, db.RecalcBudgetTotalParams{OrgID: orgID, ID: versionID})
	if err != nil {
		return VersionDetail{}, err
	}
	lines, err := q.ListBudgetLines(ctx, db.ListBudgetLinesParams{OrgID: orgID, BudgetVersionID: versionID})
	if err != nil {
		return VersionDetail{}, err
	}
	return VersionDetail{BudgetVersion: v, Lines: lines}, nil
}

// UpdateLine modifie une ligne d'un brouillon et recalcule le total.
func (s *Service) UpdateLine(ctx context.Context, a auth.Actor, lineID uuid.UUID, in LineInput) (VersionDetail, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return VersionDetail{}, err
	}
	var v apperr.Validator
	validateLine(&v, in, false)
	if err := v.Err(); err != nil {
		return VersionDetail{}, err
	}
	var out VersionDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		line, err := q.GetBudgetLine(ctx, db.GetBudgetLineParams{OrgID: a.OrgID, ID: lineID})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("budget_line")
			}
			return err
		}
		version, access, err := s.loadVersion(ctx, q, a, line.BudgetVersionID)
		if err != nil {
			return err
		}
		if err := writable(access, version); err != nil {
			return err
		}
		if in.CategoryID != nil {
			if err := requireActiveCategory(ctx, q, a.OrgID, *in.CategoryID); err != nil {
				return err
			}
		}
		if _, err := q.UpdateBudgetLine(ctx, db.UpdateBudgetLineParams{
			OrgID: a.OrgID, ID: lineID, CategoryID: in.CategoryID, Label: in.Label, Quantity: in.Quantity,
			Unit: in.Unit, UnitPrice: in.UnitPrice, Position: in.Position,
		}); err != nil {
			return err
		}
		out, err = reload(ctx, q, a.OrgID, line.BudgetVersionID)
		return err
	})
	return out, err
}

// DeleteLine retire une ligne d'un brouillon et recalcule le total.
func (s *Service) DeleteLine(ctx context.Context, a auth.Actor, lineID uuid.UUID) (VersionDetail, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return VersionDetail{}, err
	}
	var out VersionDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		line, err := q.GetBudgetLine(ctx, db.GetBudgetLineParams{OrgID: a.OrgID, ID: lineID})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("budget_line")
			}
			return err
		}
		version, access, err := s.loadVersion(ctx, q, a, line.BudgetVersionID)
		if err != nil {
			return err
		}
		if err := writable(access, version); err != nil {
			return err
		}
		if _, err := q.DeleteBudgetLine(ctx, db.DeleteBudgetLineParams{OrgID: a.OrgID, ID: lineID}); err != nil {
			return err
		}
		out, err = reload(ctx, q, a.OrgID, line.BudgetVersionID)
		return err
	})
	return out, err
}

// Publish publie une version en une seule transaction : l'ancienne version passe « superseded »,
// la nouvelle devient la référence du chantier. Le verrou sur le chantier sérialise deux publications.
func (s *Service) Publish(ctx context.Context, a auth.Actor, versionID uuid.UUID) (db.BudgetVersion, error) {
	if err := rbac.Require(a, rbac.BudgetPublish); err != nil {
		return db.BudgetVersion{}, err
	}
	var out db.BudgetVersion
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		version, _, err := s.loadVersion(ctx, q, a, versionID)
		if err != nil {
			return err
		}
		locked, err := q.LockProject(ctx, db.LockProjectParams{OrgID: a.OrgID, ID: version.ProjectID})
		if err != nil {
			return err
		}
		// Relu après le verrou : une publication concurrente a pu passer pendant l'attente.
		version, access, err := s.loadVersion(ctx, q, a, versionID)
		if err != nil {
			return err
		}
		if err := writable(access, version); err != nil {
			return err
		}
		count, err := q.CountBudgetLines(ctx, db.CountBudgetLinesParams{OrgID: a.OrgID, BudgetVersionID: versionID})
		if err != nil {
			return err
		}
		if count == 0 {
			return apperr.Conflict("budget.empty", "Un budget sans ligne ne peut pas être publié.")
		}
		if _, err := q.RecalcBudgetTotal(ctx, db.RecalcBudgetTotalParams{OrgID: a.OrgID, ID: versionID}); err != nil {
			return err
		}
		if _, err := q.SupersedePublishedBudget(ctx, db.SupersedePublishedBudgetParams{OrgID: a.OrgID, ProjectID: version.ProjectID}); err != nil {
			return err
		}
		out, err = q.PublishBudgetVersion(ctx, db.PublishBudgetVersionParams{OrgID: a.OrgID, ID: versionID, ActorID: &a.UserID})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.Conflict("budget.version_immutable", "Cette version a déjà été publiée ou abandonnée.")
			}
			return err
		}
		if err := q.SetProjectCurrentBudget(ctx, db.SetProjectCurrentBudgetParams{OrgID: a.OrgID, ProjectID: version.ProjectID, BudgetVersionID: &versionID}); err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "budget.published", EntityType: "budget_version", EntityID: &versionID, ProjectID: &version.ProjectID,
			Before: map[string]any{"previous_version_id": locked.CurrentBudgetVersionID},
			After:  map[string]any{"version_no": out.VersionNo, "total_amount": out.TotalAmount},
		})
	})
	return out, err
}

// Discard abandonne un brouillon ; il reste visible dans l'historique.
func (s *Service) Discard(ctx context.Context, a auth.Actor, versionID uuid.UUID) (db.BudgetVersion, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return db.BudgetVersion{}, err
	}
	var out db.BudgetVersion
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		version, access, err := s.loadVersion(ctx, q, a, versionID)
		if err != nil {
			return err
		}
		if err := writable(access, version); err != nil {
			return err
		}
		out, err = q.DiscardBudgetVersion(ctx, db.DiscardBudgetVersionParams{OrgID: a.OrgID, ID: versionID})
		if err != nil {
			return err
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "budget.discarded", EntityType: "budget_version", EntityID: &versionID, ProjectID: &version.ProjectID})
	})
	return out, err
}

// ReviseInput décrit une révision.
type ReviseInput struct {
	Reason string `json:"reason"`
}

// Revise duplique une version publiée ou remplacée vers un nouveau brouillon, avec son motif.
// Les dépenses gardent leur budget_version_id : rien n'est réaffecté implicitement.
func (s *Service) Revise(ctx context.Context, a auth.Actor, fromVersionID uuid.UUID, in ReviseInput) (VersionDetail, error) {
	if err := rbac.Require(a, rbac.BudgetEdit); err != nil {
		return VersionDetail{}, err
	}
	reason := strings.TrimSpace(in.Reason)
	var v apperr.Validator
	v.Check(len(reason) >= 3, "reason", "Le motif de la révision est obligatoire.")
	if err := v.Err(); err != nil {
		return VersionDetail{}, err
	}
	newID, err := uuid.NewV7()
	if err != nil {
		return VersionDetail{}, err
	}
	revisionID, err := uuid.NewV7()
	if err != nil {
		return VersionDetail{}, err
	}
	var out VersionDetail
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		from, access, err := s.loadVersion(ctx, q, a, fromVersionID)
		if err != nil {
			return err
		}
		if access.Status == "archived" {
			return apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
		}
		if from.Status != "published" && from.Status != "superseded" {
			return apperr.Conflict("budget.not_published", "Seule une version publiée peut être révisée.")
		}
		if _, err := q.LockProject(ctx, db.LockProjectParams{OrgID: a.OrgID, ID: from.ProjectID}); err != nil {
			return err
		}
		no, err := q.NextBudgetVersionNo(ctx, db.NextBudgetVersionNoParams{OrgID: a.OrgID, ProjectID: from.ProjectID})
		if err != nil {
			return err
		}
		if _, err := q.CreateBudgetVersion(ctx, db.CreateBudgetVersionParams{ID: newID, OrgID: a.OrgID, ProjectID: from.ProjectID, VersionNo: no, CreatedBy: a.UserID}); err != nil {
			if apperr.Normalize(err).Code == "resource.duplicate" {
				return apperr.Conflict("budget.draft_exists", "Un brouillon est déjà en cours sur ce chantier.")
			}
			return err
		}
		if err := q.CopyBudgetLines(ctx, db.CopyBudgetLinesParams{OrgID: a.OrgID, FromVersionID: fromVersionID, ToVersionID: newID}); err != nil {
			return err
		}
		if _, err := q.CreateBudgetRevision(ctx, db.CreateBudgetRevisionParams{
			ID: revisionID, OrgID: a.OrgID, ProjectID: from.ProjectID, FromVersionID: fromVersionID, ToVersionID: newID, Reason: reason, CreatedBy: a.UserID,
		}); err != nil {
			return err
		}
		if err := audit.Record(ctx, q, a, audit.Entry{
			Action: "budget.revision_created", EntityType: "budget_version", EntityID: &newID, ProjectID: &from.ProjectID,
			After: map[string]any{"from_version_id": fromVersionID, "reason": reason},
		}); err != nil {
			return err
		}
		out, err = reload(ctx, q, a.OrgID, newID)
		return err
	})
	return out, err
}

// LineDiff compare une ligne entre deux versions.
type LineDiff struct {
	LineKey    uuid.UUID `json:"line_key"`
	Label      string    `json:"label"`
	Category   string    `json:"category"`
	Change     string    `json:"change"` // added | removed | changed | unchanged
	FromAmount int64     `json:"from_amount"`
	ToAmount   int64     `json:"to_amount"`
	Delta      int64     `json:"delta"`
}

// Comparison est le résultat de la comparaison de deux versions.
type Comparison struct {
	FromVersionNo int32      `json:"from_version_no"`
	ToVersionNo   int32      `json:"to_version_no"`
	FromTotal     int64      `json:"from_total"`
	ToTotal       int64      `json:"to_total"`
	Delta         int64      `json:"delta"`
	Lines         []LineDiff `json:"lines"`
}

// DiffLines compare deux jeux de lignes par leur line_key, conservé d'une version à l'autre.
func DiffLines(from, to []db.ListBudgetLinesRow) []LineDiff {
	before := make(map[uuid.UUID]db.ListBudgetLinesRow, len(from))
	for _, l := range from {
		before[l.LineKey] = l
	}
	out := make([]LineDiff, 0, len(to))
	seen := make(map[uuid.UUID]bool, len(to))
	for _, l := range to {
		seen[l.LineKey] = true
		d := LineDiff{LineKey: l.LineKey, Label: l.Label, Category: l.CategoryName, ToAmount: l.Amount}
		prev, existed := before[l.LineKey]
		switch {
		case !existed:
			d.Change = "added"
		case prev.Amount == l.Amount && prev.Label == l.Label && prev.CategoryID == l.CategoryID &&
			prev.Unit == l.Unit && prev.UnitPrice == l.UnitPrice && prev.Quantity.Equal(l.Quantity):
			d.Change = "unchanged"
			d.FromAmount = prev.Amount
		default:
			d.Change = "changed"
			d.FromAmount = prev.Amount
		}
		d.Delta = d.ToAmount - d.FromAmount
		out = append(out, d)
	}
	for _, l := range from {
		if !seen[l.LineKey] {
			out = append(out, LineDiff{
				LineKey: l.LineKey, Label: l.Label, Category: l.CategoryName, Change: "removed",
				FromAmount: l.Amount, Delta: -l.Amount,
			})
		}
	}
	return out
}

// Compare compare deux versions d'un même chantier.
func (s *Service) Compare(ctx context.Context, a auth.Actor, fromID, toID uuid.UUID) (Comparison, error) {
	if err := rbac.Require(a, rbac.BudgetView); err != nil {
		return Comparison{}, err
	}
	var out Comparison
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		from, _, err := s.loadVersion(ctx, q, a, fromID)
		if err != nil {
			return err
		}
		to, _, err := s.loadVersion(ctx, q, a, toID)
		if err != nil {
			return err
		}
		if from.ProjectID != to.ProjectID {
			return apperr.New(422, "budget.different_projects", "Ces deux versions n'appartiennent pas au même chantier.")
		}
		fromLines, err := q.ListBudgetLines(ctx, db.ListBudgetLinesParams{OrgID: a.OrgID, BudgetVersionID: fromID})
		if err != nil {
			return err
		}
		toLines, err := q.ListBudgetLines(ctx, db.ListBudgetLinesParams{OrgID: a.OrgID, BudgetVersionID: toID})
		if err != nil {
			return err
		}
		out = Comparison{
			FromVersionNo: from.VersionNo, ToVersionNo: to.VersionNo,
			FromTotal: from.TotalAmount, ToTotal: to.TotalAmount, Delta: to.TotalAmount - from.TotalAmount,
			Lines: DiffLines(fromLines, toLines),
		}
		return nil
	})
	return out, err
}
