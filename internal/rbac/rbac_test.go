package rbac

import (
	"testing"

	"github.com/justmamadou/Kodi/internal/auth"
)

// Table de vérité exhaustive : toute évolution de la matrice doit être un choix explicite.
func TestMatrix(t *testing.T) {
	want := map[Permission]map[string]bool{
		OrgManage:       {RoleOwner: true},
		MembersManage:   {RoleOwner: true},
		AuditRead:       {RoleOwner: true},
		ProjectsViewAll: {RoleOwner: true, RoleAccountant: true},
		ProjectCreate:   {RoleOwner: true, RoleManager: true},
		ProjectManage:   {RoleOwner: true, RoleManager: true},
		BudgetView:      {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleViewer: true},
		BudgetEdit:      {RoleOwner: true, RoleManager: true},
		BudgetPublish:   {RoleOwner: true, RoleManager: true},
		ExpenseView:     {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		ExpenseCreate:   {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true},
		ExpenseApprove:  {RoleOwner: true, RoleManager: true, RoleAccountant: true},
		PaymentRecord:   {RoleOwner: true, RoleAccountant: true},
		FinanceView:     {RoleOwner: true, RoleManager: true, RoleAccountant: true},
		DataExport:      {RoleOwner: true, RoleAccountant: true},
		CategoryManage:  {RoleOwner: true},
		SupplierView:    {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		SupplierCreate:  {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true},
		SupplierEdit:    {RoleOwner: true, RoleManager: true, RoleAccountant: true},
		SupplierTrade:   {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleViewer: true},
		CatalogManage:   {RoleOwner: true, RoleManager: true},
		StockView:       {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		StockMove:       {RoleOwner: true, RoleManager: true, RoleSiteWorker: true},
		TaskView:        {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		TaskManage:      {RoleOwner: true, RoleManager: true},
		TaskProgress:    {RoleOwner: true, RoleManager: true, RoleSiteWorker: true},
		ReportView:      {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		ReportWrite:     {RoleOwner: true, RoleManager: true, RoleSiteWorker: true},
		ReportReopen:    {RoleOwner: true, RoleManager: true},
		FileWrite:       {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true},
		FileRead:        {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		RFQView:         {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleViewer: true},
		RFQManage:       {RoleOwner: true, RoleManager: true},
		RFQAward:        {RoleOwner: true, RoleManager: true},
		POView:          {RoleOwner: true, RoleManager: true, RoleAccountant: true, RoleSiteWorker: true, RoleViewer: true},
		POManage:        {RoleOwner: true, RoleManager: true},
		POApprove:       {RoleOwner: true, RoleAccountant: true},
		POReceive:       {RoleOwner: true, RoleManager: true, RoleSiteWorker: true},
	}
	if len(want) != len(matrix) {
		t.Fatalf("la table couvre %d permissions, la matrice %d", len(want), len(matrix))
	}
	for perm, roles := range want {
		for _, role := range Roles {
			if got := Can(role, perm); got != roles[role] {
				t.Errorf("Can(%s, %s) = %v, attendu %v", role, perm, got, roles[role])
			}
		}
	}
	if Can("inconnu", ExpenseView) {
		t.Error("un rôle inconnu ne doit rien pouvoir faire")
	}
}

func TestCanApproveExpense(t *testing.T) {
	limit := int64(100)
	tests := []struct {
		name   string
		role   string
		access ProjectAccess
		amount int64
		ok     bool
	}{
		{"owner sans délégation", RoleOwner, ProjectAccess{}, 1_000_000, true},
		{"finance sans délégation", RoleAccountant, ProjectAccess{}, 1_000_000, true},
		{"manager non délégué", RoleManager, ProjectAccess{}, 10, false},
		{"manager délégué sans plafond", RoleManager, ProjectAccess{CanApprove: true}, 1_000_000, true},
		{"manager au plafond", RoleManager, ProjectAccess{CanApprove: true, ApprovalLimit: &limit}, 100, true},
		{"manager au-dessus du plafond", RoleManager, ProjectAccess{CanApprove: true, ApprovalLimit: &limit}, 101, false},
		{"chef de chantier même délégué", RoleSiteWorker, ProjectAccess{CanApprove: true}, 10, false},
		{"lecture seule", RoleViewer, ProjectAccess{CanApprove: true}, 10, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CanApproveExpense(auth.Actor{Role: tt.role}, tt.access, tt.amount)
			if (err == nil) != tt.ok {
				t.Fatalf("erreur = %v, autorisation attendue = %v", err, tt.ok)
			}
		})
	}
}
