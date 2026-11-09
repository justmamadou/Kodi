// Package rbac contient la matrice rôle × permission et le contrôle d'accès aux chantiers.
// La matrice est l'unique source de vérité ; elle est couverte par une table de vérité (rbac_test.go).
package rbac

import (
	"context"
	"slices"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
)

// Rôles initiaux (CF-008). Libellés des maquettes : Owner / Admin, Conducteur, Finance,
// Chef de chantier, Lecture seule.
const (
	RoleOwner      = "owner"
	RoleManager    = "manager"
	RoleAccountant = "accountant"
	RoleSiteWorker = "site_worker"
	RoleViewer     = "viewer"
)

// Roles liste les rôles attribuables.
var Roles = []string{RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer}

// IsRole indique si role est un rôle connu.
func IsRole(role string) bool { return slices.Contains(Roles, role) }

// Permission est une action contrôlée côté serveur.
type Permission string

// Permissions de l'application.
const (
	OrgManage       Permission = "org.manage"
	MembersManage   Permission = "members.manage"
	AuditRead       Permission = "audit.read"
	ProjectsViewAll Permission = "projects.view_all"
	ProjectCreate   Permission = "project.create"
	ProjectManage   Permission = "project.manage"
	BudgetView      Permission = "budget.view"
	BudgetEdit      Permission = "budget.edit"
	BudgetPublish   Permission = "budget.publish"
	ExpenseView     Permission = "expense.view"
	ExpenseCreate   Permission = "expense.create"
	ExpenseApprove  Permission = "expense.approve"
	PaymentRecord   Permission = "payment.record"
	FinanceView     Permission = "finance.view"
	DataExport      Permission = "data.export"
	CategoryManage  Permission = "category.manage"
	SupplierView    Permission = "supplier.view"
	SupplierCreate  Permission = "supplier.create"
	SupplierEdit    Permission = "supplier.edit"
	SupplierTrade   Permission = "supplier.commercial_view"
	CatalogManage   Permission = "catalog.manage"
	StockView       Permission = "stock.view"
	StockMove       Permission = "stock.move"
	TaskView        Permission = "task.view"
	TaskManage      Permission = "task.manage"
	TaskProgress    Permission = "task.progress"
	ReportView      Permission = "report.view"
	ReportWrite     Permission = "report.write"
	ReportReopen    Permission = "report.reopen"
	FileWrite       Permission = "file.write"
	FileRead        Permission = "file.read"
	RFQView         Permission = "rfq.view"
	RFQManage       Permission = "rfq.manage"
	RFQAward        Permission = "rfq.award"
	POView          Permission = "po.view"
	POManage        Permission = "po.manage"
	POApprove       Permission = "po.approve"
	POReceive       Permission = "po.receive"
)

var matrix = map[Permission][]string{
	OrgManage:       {RoleOwner},
	MembersManage:   {RoleOwner},
	AuditRead:       {RoleOwner},
	ProjectsViewAll: {RoleOwner, RoleAccountant},
	ProjectCreate:   {RoleOwner, RoleManager},
	ProjectManage:   {RoleOwner, RoleManager},
	BudgetView:      {RoleOwner, RoleManager, RoleAccountant, RoleViewer},
	BudgetEdit:      {RoleOwner, RoleManager},
	BudgetPublish:   {RoleOwner, RoleManager},
	ExpenseView:     {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	ExpenseCreate:   {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker},
	ExpenseApprove:  {RoleOwner, RoleManager, RoleAccountant}, // manager : si délégué sur le chantier
	PaymentRecord:   {RoleOwner, RoleAccountant},
	FinanceView:     {RoleOwner, RoleManager, RoleAccountant},
	DataExport:      {RoleOwner, RoleAccountant},
	CategoryManage:  {RoleOwner},
	// Fournisseurs (E2), validé par le Product Owner : fiche et contacts pour tous ; création rapide aussi sur le terrain ;
	// conditions commerciales et historique (prix) comme les demandes de prix.
	SupplierView:   {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	SupplierCreate: {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker},
	SupplierEdit:   {RoleOwner, RoleManager, RoleAccountant},
	SupplierTrade:  {RoleOwner, RoleManager, RoleAccountant, RoleViewer},
	CatalogManage:  {RoleOwner, RoleManager},
	StockView:      {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	StockMove:      {RoleOwner, RoleManager, RoleSiteWorker},
	TaskView:       {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	TaskManage:     {RoleOwner, RoleManager},
	TaskProgress:   {RoleOwner, RoleManager, RoleSiteWorker}, // site_worker : si responsable de la tâche
	ReportView:     {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	ReportWrite:    {RoleOwner, RoleManager, RoleSiteWorker},
	ReportReopen:   {RoleOwner, RoleManager},
	FileWrite:      {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker},
	FileRead:       {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	// Demandes de prix (E1), validé par le Product Owner : les prix des fournisseurs restent hors de
	// portée du chef de chantier.
	RFQView:   {RoleOwner, RoleManager, RoleAccountant, RoleViewer},
	RFQManage: {RoleOwner, RoleManager},
	// Bons de commande (E3), validé par le Product Owner : le conducteur retient l'offre et prépare la commande, l'Owner ou Finance
	// l'approuve (jamais son auteur), la réception se fait sur le chantier. Le chef de chantier voit les
	// commandes de ses chantiers sans les prix.
	RFQAward:  {RoleOwner, RoleManager},
	POView:    {RoleOwner, RoleManager, RoleAccountant, RoleSiteWorker, RoleViewer},
	POManage:  {RoleOwner, RoleManager},
	POApprove: {RoleOwner, RoleAccountant},
	POReceive: {RoleOwner, RoleManager, RoleSiteWorker},
}

// Can indique si le rôle détient la permission.
func Can(role string, p Permission) bool { return slices.Contains(matrix[p], role) }

// Require retourne une erreur 403 si l'acteur n'a pas la permission.
func Require(a auth.Actor, p Permission) error {
	if !Can(a.Role, p) {
		return apperr.Forbidden()
	}
	return nil
}

// SeesAllProjects : Owner et Finance voient tous les chantiers, les autres ceux où ils sont affectés.
func SeesAllProjects(a auth.Actor) bool { return Can(a.Role, ProjectsViewAll) }

// ProjectAccess est le résultat du contrôle d'accès à un chantier.
type ProjectAccess struct {
	Status        string
	IsManager     bool
	CanApprove    bool
	ApprovalLimit *int64
}

// Project vérifie que l'acteur peut accéder au chantier : 404 si le chantier n'existe pas pour ce
// tenant (une autre organisation est indiscernable d'une absence), 403 s'il n'y est pas affecté.
func Project(ctx context.Context, q *db.Queries, a auth.Actor, projectID uuid.UUID) (ProjectAccess, error) {
	row, err := q.GetProjectAccess(ctx, db.GetProjectAccessParams{OrgID: a.OrgID, ProjectID: projectID, UserID: a.UserID})
	if err != nil {
		if db.IsNoRows(err) {
			return ProjectAccess{}, apperr.NotFound("project")
		}
		return ProjectAccess{}, err
	}
	isManager := row.ManagerID == a.UserID
	if !SeesAllProjects(a) && !isManager && !row.IsMember {
		return ProjectAccess{}, apperr.Forbidden()
	}
	return ProjectAccess{
		Status:        row.Status,
		IsManager:     isManager,
		CanApprove:    row.CanApprove,
		ApprovalLimit: row.ApprovalLimit,
	}, nil
}

// ProjectWritable ajoute au contrôle d'accès le refus d'écrire sur un chantier archivé (lecture seule).
func ProjectWritable(ctx context.Context, q *db.Queries, a auth.Actor, projectID uuid.UUID) (ProjectAccess, error) {
	access, err := Project(ctx, q, a, projectID)
	if err != nil {
		return access, err
	}
	if access.Status == "archived" {
		return access, apperr.Conflict("project.archived", "Ce chantier est archivé : il est en lecture seule.")
	}
	return access, nil
}

// CanApproveExpense applique la règle d'approbation : Owner et Finance sans restriction ;
// un manager uniquement s'il est délégué sur ce chantier et dans la limite de son plafond.
func CanApproveExpense(a auth.Actor, access ProjectAccess, amount int64) error {
	if err := Require(a, ExpenseApprove); err != nil {
		return err
	}
	if a.Role != RoleManager {
		return nil
	}
	if !access.CanApprove {
		return apperr.Forbidden()
	}
	if access.ApprovalLimit != nil && amount > *access.ApprovalLimit {
		return apperr.New(403, "expense.above_approval_limit", "Le montant dépasse votre plafond d'approbation sur ce chantier.")
	}
	return nil
}
