// Package inventory gère le catalogue de matériaux et le stock par chantier.
package inventory

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"slices"
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

// Units liste les unités acceptées, identiques à la contrainte CHECK de la base.
var Units = []string{"sac", "barre", "m3", "m2", "ml", "unite", "kg", "tonne", "litre", "rouleau", "boite"}

var skuRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,40}$`)

// Service porte les cas d'usage du stock.
type Service struct{ tx *db.TxManager }

// NewService construit le service.
func NewService(tx *db.TxManager) *Service { return &Service{tx: tx} }

// MaterialInput décrit un matériau.
type MaterialInput struct {
	Name              *string          `json:"name"`
	Unit              *string          `json:"unit"`
	SKU               *string          `json:"sku"`
	LowStockThreshold *decimal.Decimal `json:"low_stock_threshold"`
}

func validateMaterial(v *apperr.Validator, in MaterialInput, creating bool) {
	if creating {
		v.Check(in.Name != nil, "name", "Le nom est obligatoire.")
		v.Check(in.Unit != nil, "unit", "L'unité est obligatoire.")
	}
	if in.Name != nil {
		n := len(strings.TrimSpace(*in.Name))
		v.Check(n >= 2 && n <= 160, "name", "Le nom doit contenir entre 2 et 160 caractères.")
	}
	if in.Unit != nil {
		v.Check(slices.Contains(Units, *in.Unit), "unit", "Unité inconnue.")
	}
	if in.SKU != nil {
		v.Check(skuRe.MatchString(*in.SKU), "sku", "SKU invalide : lettres, chiffres, point, tiret ou souligné (40 caractères maximum).")
	}
	if in.LowStockThreshold != nil {
		v.Check(!in.LowStockThreshold.IsNegative(), "low_stock_threshold", "Le seuil de stock bas ne peut pas être négatif.")
	}
}

func duplicateMaterial(err error) error {
	if err != nil && apperr.Normalize(err).Code == "resource.duplicate" {
		return apperr.Conflict("material.duplicate", "Un matériau porte déjà ce SKU, ou ce nom avec cette unité.")
	}
	return err
}

// CreateMaterial ajoute un matériau au catalogue.
func (s *Service) CreateMaterial(ctx context.Context, a auth.Actor, in MaterialInput) (db.Material, error) {
	if err := rbac.Require(a, rbac.CatalogManage); err != nil {
		return db.Material{}, err
	}
	var v apperr.Validator
	validateMaterial(&v, in, true)
	if err := v.Err(); err != nil {
		return db.Material{}, err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return db.Material{}, err
	}
	var out db.Material
	err = s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.CreateMaterial(ctx, db.CreateMaterialParams{ID: id, OrgID: a.OrgID, Name: strings.TrimSpace(*in.Name), Unit: *in.Unit, Sku: in.SKU, LowStockThreshold: in.LowStockThreshold})
		return duplicateMaterial(err)
	})
	return out, err
}

// UpdateMaterial modifie un matériau.
func (s *Service) UpdateMaterial(ctx context.Context, a auth.Actor, id uuid.UUID, in MaterialInput) (db.Material, error) {
	if err := rbac.Require(a, rbac.CatalogManage); err != nil {
		return db.Material{}, err
	}
	var v apperr.Validator
	validateMaterial(&v, in, false)
	if err := v.Err(); err != nil {
		return db.Material{}, err
	}
	var out db.Material
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.UpdateMaterial(ctx, db.UpdateMaterialParams{OrgID: a.OrgID, ID: id, Name: in.Name, Unit: in.Unit, Sku: in.SKU, LowStockThreshold: in.LowStockThreshold})
		if db.IsNoRows(err) {
			return apperr.NotFound("material")
		}
		return duplicateMaterial(err)
	})
	return out, err
}

// ArchiveMaterial retire un matériau du catalogue ; il reste dans les soldes et l'historique.
func (s *Service) ArchiveMaterial(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Material, error) {
	if err := rbac.Require(a, rbac.CatalogManage); err != nil {
		return db.Material{}, err
	}
	var out db.Material
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ArchiveMaterial(ctx, db.ArchiveMaterialParams{OrgID: a.OrgID, ID: id})
		if db.IsNoRows(err) {
			return apperr.NotFound("material")
		}
		return err
	})
	return out, err
}

// ListMaterials liste le catalogue.
func (s *Service) ListMaterials(ctx context.Context, a auth.Actor, query *string, includeArchived bool) ([]db.Material, error) {
	if err := rbac.Require(a, rbac.StockView); err != nil {
		return nil, err
	}
	var out []db.Material
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		var err error
		out, err = q.ListMaterials(ctx, db.ListMaterialsParams{OrgID: a.OrgID, Q: query, IncludeArchived: includeArchived})
		return err
	})
	return out, err
}

// MovementItem est une ligne de mouvement.
type MovementItem struct {
	MaterialID uuid.UUID       `json:"material_id"`
	Quantity   decimal.Decimal `json:"quantity"`
}

// MovementInput décrit un mouvement de stock. L'id peut être généré par le client (idempotence).
type MovementInput struct {
	ID           *uuid.UUID     `json:"id"`
	MovementType string         `json:"movement_type"` // in | out | adjustment
	OccurredOn   *civil.Date    `json:"occurred_on"`
	Reason       *string        `json:"reason"`
	Items        []MovementItem `json:"items"`
}

// ValidateMovement applique les règles d'un mouvement : quantité positive sauf ajustement (signé,
// et motivé), au plus 3 décimales, chaque article une seule fois.
func ValidateMovement(in MovementInput) error {
	var v apperr.Validator
	valid := in.MovementType == "in" || in.MovementType == "out" || in.MovementType == "adjustment"
	v.Check(valid, "movement_type", "Le type doit être entrée, sortie ou ajustement.")
	v.Check(in.OccurredOn != nil, "occurred_on", "La date est obligatoire.")
	v.Check(len(in.Items) > 0, "items", "Au moins un article est requis.")
	if in.MovementType == "adjustment" {
		v.Check(in.Reason != nil && strings.TrimSpace(*in.Reason) != "", "reason", "Un ajustement doit être motivé.")
	}
	seen := map[uuid.UUID]bool{}
	for _, item := range in.Items {
		v.Check(!seen[item.MaterialID], "items", "Un article ne peut figurer qu'une fois par mouvement.")
		seen[item.MaterialID] = true
		v.Check(!item.Quantity.IsZero(), "items", "La quantité ne peut pas être nulle.")
		v.Check(item.Quantity.Equal(item.Quantity.Truncate(3)), "items", "La quantité accepte au plus 3 décimales.")
		v.Check(item.Quantity.Abs().LessThan(decimal.New(1, 11)), "items", "La quantité est trop grande.")
		if in.MovementType != "adjustment" {
			v.Check(item.Quantity.IsPositive(), "items", "La quantité doit être positive.")
		}
	}
	return v.Err()
}

// Delta traduit une ligne en variation de solde.
func Delta(movementType string, quantity decimal.Decimal) decimal.Decimal {
	if movementType == "out" {
		return quantity.Neg()
	}
	return quantity
}

// MovementDetail est un mouvement avec ses lignes.
type MovementDetail struct {
	db.StockMovement
	Items []db.ListStockMovementItemsRow `json:"items"`
}

// CreateMovement enregistre un mouvement en brouillon, sans effet sur le stock.
func (s *Service) CreateMovement(ctx context.Context, a auth.Actor, projectID uuid.UUID, in MovementInput) (MovementDetail, bool, error) {
	if err := rbac.Require(a, rbac.StockMove); err != nil {
		return MovementDetail{}, false, err
	}
	if err := ValidateMovement(in); err != nil {
		return MovementDetail{}, false, err
	}
	var id uuid.UUID
	if in.ID != nil {
		id = *in.ID
	} else {
		var err error
		if id, err = uuid.NewV7(); err != nil {
			return MovementDetail{}, false, err
		}
	}
	created := false
	var out MovementDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		m, err := q.CreateStockMovement(ctx, db.CreateStockMovementParams{
			ID: id, OrgID: a.OrgID, ProjectID: projectID, MovementType: in.MovementType,
			OccurredOn: *in.OccurredOn, Reason: in.Reason, CreatedBy: a.UserID,
		})
		if db.IsNoRows(err) {
			m, err = q.GetStockMovement(ctx, db.GetStockMovementParams{OrgID: a.OrgID, ID: id})
			if db.IsNoRows(err) {
				return apperr.Conflict("stock.id_conflict", "Cet identifiant de mouvement n'est pas utilisable.")
			}
			if err != nil {
				return err
			}
			if m.ProjectID != projectID {
				return apperr.Conflict("stock.id_conflict", "Cet identifiant de mouvement n'est pas utilisable.")
			}
		} else {
			if err != nil {
				return err
			}
			created = true
			for _, item := range in.Items {
				mat, err := q.GetMaterial(ctx, db.GetMaterialParams{OrgID: a.OrgID, ID: item.MaterialID})
				if err != nil {
					if db.IsNoRows(err) {
						return apperr.New(http.StatusUnprocessableEntity, "material.not_found", "Un article du mouvement est introuvable.")
					}
					return err
				}
				if mat.ArchivedAt != nil {
					return apperr.New(http.StatusUnprocessableEntity, "material.archived", "Un article archivé ne peut plus être mouvementé : "+mat.Name+".")
				}
				if err := q.InsertStockMovementItem(ctx, db.InsertStockMovementItemParams{
					OrgID: a.OrgID, MovementID: id, MaterialID: item.MaterialID, Quantity: item.Quantity,
				}); err != nil {
					return err
				}
			}
		}
		items, err := q.ListStockMovementItems(ctx, db.ListStockMovementItemsParams{OrgID: a.OrgID, MovementID: id})
		out = MovementDetail{StockMovement: m, Items: items}
		return err
	})
	return out, created, err
}

// PostMovement poste un mouvement : le solde et le statut changent atomiquement, puis il est immuable.
// Les lignes sont verrouillées dans l'ordre des articles, ce qui évite les interblocages, et le
// CHECK (on_hand >= 0) refuse toute sortie qui rendrait le stock négatif, même en concurrence.
func (s *Service) PostMovement(ctx context.Context, a auth.Actor, id uuid.UUID) (MovementDetail, error) {
	if err := rbac.Require(a, rbac.StockMove); err != nil {
		return MovementDetail{}, err
	}
	var out MovementDetail
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		m, err := q.GetStockMovementForUpdate(ctx, db.GetStockMovementForUpdateParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("stock_movement")
			}
			return err
		}
		if _, err := rbac.ProjectWritable(ctx, q, a, m.ProjectID); err != nil {
			return err
		}
		if m.Status != "draft" {
			return apperr.Conflict("stock.already_posted", "Ce mouvement est déjà posté : il est immuable.")
		}
		items, err := q.ListStockMovementItems(ctx, db.ListStockMovementItemsParams{OrgID: a.OrgID, MovementID: id})
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := q.EnsureStockRow(ctx, db.EnsureStockRowParams{OrgID: a.OrgID, ProjectID: m.ProjectID, MaterialID: item.MaterialID}); err != nil {
				return err
			}
			if _, err := q.ApplyStockDelta(ctx, db.ApplyStockDeltaParams{
				OrgID: a.OrgID, ProjectID: m.ProjectID, MaterialID: item.MaterialID,
				Delta: Delta(m.MovementType, item.Quantity), OccurredOn: m.OccurredOn,
			}); err != nil {
				var pgErr *pgconn.PgError
				if errors.As(err, &pgErr) && pgErr.Code == "23514" {
					return apperr.Conflict("stock.insufficient", "Stock insuffisant pour « "+item.MaterialName+" » sur ce chantier.")
				}
				return err
			}
		}
		posted, err := q.PostStockMovement(ctx, db.PostStockMovementParams{OrgID: a.OrgID, ID: id, ActorID: &a.UserID})
		if err != nil {
			return err
		}
		out = MovementDetail{StockMovement: posted, Items: items}
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "stock.posted", EntityType: "stock_movement", EntityID: &id, ProjectID: &m.ProjectID,
			After: map[string]any{"movement_type": m.MovementType, "items": items},
		})
	})
	return out, err
}

// StockView est l'écran de stock d'un chantier : les soldes et leur résumé.
type StockView struct {
	Items              []db.ListProjectStockRow `json:"items"`
	LowCount           int                      `json:"low_count"`
	MovementsThisMonth int64                    `json:"movements_this_month"`
}

// Stock retourne les soldes d'un chantier.
func (s *Service) Stock(ctx context.Context, a auth.Actor, projectID uuid.UUID, query *string) (StockView, error) {
	if err := rbac.Require(a, rbac.StockView); err != nil {
		return StockView{}, err
	}
	var out StockView
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out.Items, err = q.ListProjectStock(ctx, db.ListProjectStockParams{OrgID: a.OrgID, ProjectID: projectID, Q: query})
		if err != nil {
			return err
		}
		for _, row := range out.Items {
			if row.IsLow {
				out.LowCount++
			}
		}
		now := time.Now().UTC()
		monthStart := civil.Date{Time: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)}
		out.MovementsThisMonth, err = q.CountPostedMovementsSince(ctx, db.CountPostedMovementsSinceParams{OrgID: a.OrgID, ProjectID: projectID, Since: monthStart})
		return err
	})
	return out, err
}

// HistoryFilter décrit les filtres de l'historique de stock.
type HistoryFilter struct {
	MaterialID   *uuid.UUID
	MovementType *string
	DateFrom     *civil.Date
	DateTo       *civil.Date
	Limit        int32
	Offset       int32
}

// History retourne l'historique des mouvements postés d'un chantier.
func (s *Service) History(ctx context.Context, a auth.Actor, projectID uuid.UUID, f HistoryFilter) ([]db.ListStockHistoryRow, error) {
	if err := rbac.Require(a, rbac.StockView); err != nil {
		return nil, err
	}
	var out []db.ListStockHistoryRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListStockHistory(ctx, db.ListStockHistoryParams{
			OrgID: a.OrgID, ProjectID: projectID, MaterialID: f.MaterialID, MovementType: f.MovementType,
			DateFrom: f.DateFrom, DateTo: f.DateTo, PageSize: f.Limit, PageOffset: f.Offset,
		})
		return err
	})
	return out, err
}

// Reconciliation retourne le solde projeté et le recalcul depuis le journal, par article.
func (s *Service) Reconciliation(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]db.ReconcileProjectStockRow, error) {
	if err := rbac.Require(a, rbac.StockView); err != nil {
		return nil, err
	}
	var out []db.ReconcileProjectStockRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ReconcileProjectStock(ctx, db.ReconcileProjectStockParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}
