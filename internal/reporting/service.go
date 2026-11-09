// Package reporting produit les exports Excel et PDF. Ils passent par les mêmes requêtes et les mêmes
// contrôles que l'API : mêmes filtres, mêmes permissions, mêmes totaux.
package reporting

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/expense"
	"github.com/justmamadou/Kodi/internal/finance"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/rbac"
	"github.com/xuri/excelize/v2"
)

// MaxExportRows borne un export synchrone ; au-delà il faudra un job asynchrone (voir docs/DECISIONS.md).
var MaxExportRows = 50_000

// StatusLabel traduit un statut de dépense pour les documents exportés.
var StatusLabel = map[string]string{
	"draft": "Brouillon", "pending": "À valider", "approved": "Approuvée", "rejected": "Rejetée", "cancelled": "Annulée",
}

// Service porte les exports.
type Service struct {
	tx       *db.TxManager
	expenses *expense.Service
	finance  *finance.Service
}

// NewService construit le service.
func NewService(tx *db.TxManager, expenses *expense.Service, fin *finance.Service) *Service {
	return &Service{tx: tx, expenses: expenses, finance: fin}
}

func (s *Service) loadExpenses(ctx context.Context, a auth.Actor, f expense.ListFilter) ([]db.ListExpensesRow, error) {
	var all []db.ListExpensesRow
	const pageSize = 1000
	f.Limit = pageSize
	for {
		page, err := s.expenses.List(ctx, a, f)
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if len(all) > MaxExportRows {
			// Un export tronqué aurait des totaux faux : on refuse plutôt que de mentir.
			return nil, apperr.New(http.StatusUnprocessableEntity, "export.too_large",
				fmt.Sprintf("Trop de lignes pour un export (plus de %d). Affinez les filtres.", MaxExportRows))
		}
		if len(page) < pageSize {
			return all, nil
		}
		last := page[len(page)-1]
		f.CursorDate, f.CursorID = &last.ExpenseDate, &last.ID
	}
}

func (s *Service) auditExport(ctx context.Context, a auth.Actor, kind string, projectID *uuid.UUID, rows int) error {
	return s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		return audit.Record(ctx, q, a, audit.Entry{
			Action: "export.created", EntityType: "export", ProjectID: projectID,
			After: map[string]any{"type": kind, "rows": rows},
		})
	})
}

// sanitizeCell neutralise l'injection de formule : Excel exécuterait un texte commençant par = + - @.
func sanitizeCell(s string) string {
	if s != "" && strings.ContainsAny(s[:1], "=+-@\t\r") {
		return "'" + s
	}
	return s
}

// ExpensesXLSX exporte les dépenses filtrées dans un classeur exploitable : montants numériques,
// filtres automatiques, volets figés et total calculé par une formule.
func (s *Service) ExpensesXLSX(ctx context.Context, a auth.Actor, f expense.ListFilter) ([]byte, error) {
	if err := rbac.Require(a, rbac.DataExport); err != nil {
		return nil, err
	}
	rows, err := s.loadExpenses(ctx, a, f)
	if err != nil {
		return nil, err
	}
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()
	const sheet = "Dépenses"
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	headers := []string{"Date", "Chantier", "Catégorie", "Fournisseur", "Description", "Statut", "Montant (FCFA)", "Payé (FCFA)", "Saisie par"}
	bold, err := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, err
	}
	money, err := file.NewStyle(&excelize.Style{NumFmt: 3}) // #,##0
	if err != nil {
		return nil, err
	}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := file.SetCellValue(sheet, cell, h); err != nil {
			return nil, err
		}
	}
	if err := file.SetRowStyle(sheet, 1, 1, bold); err != nil {
		return nil, err
	}
	for i, r := range rows {
		supplier := ""
		if r.SupplierName != nil {
			supplier = *r.SupplierName
		}
		values := []any{
			r.ExpenseDate.Time, sanitizeCell(r.ProjectName), sanitizeCell(r.CategoryName), sanitizeCell(supplier),
			sanitizeCell(r.Description), StatusLabel[r.Status], r.Amount, r.AmountPaid, sanitizeCell(r.CreatedByName),
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if err := file.SetSheetRow(sheet, cell, &values); err != nil {
			return nil, err
		}
	}
	last := len(rows) + 1
	totalRow := last + 1
	if err := file.SetCellValue(sheet, fmt.Sprintf("F%d", totalRow), "Total"); err != nil {
		return nil, err
	}
	var totalAmount, totalPaid int64
	for _, r := range rows {
		totalAmount += r.Amount
		totalPaid += r.AmountPaid
	}
	// La valeur en cache est écrite avec la formule : le total est juste même sans recalcul à l'ouverture.
	for col, total := range map[string]int64{"G": totalAmount, "H": totalPaid} {
		cell := fmt.Sprintf("%s%d", col, totalRow)
		formula := fmt.Sprintf("SUM(%s2:%s%d)", col, col, max(last, 2))
		if err := file.SetCellFormula(sheet, cell, formula); err != nil {
			return nil, err
		}
		if err := file.SetCellValue(sheet, cell, total); err != nil {
			return nil, err
		}
		if err := file.SetCellFormula(sheet, cell, formula); err != nil {
			return nil, err
		}
	}
	if err := file.SetRowStyle(sheet, totalRow, totalRow, bold); err != nil {
		return nil, err
	}
	if err := file.SetColStyle(sheet, "G:H", money); err != nil {
		return nil, err
	}
	dateStyle, err := file.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil {
		return nil, err
	}
	if err := file.SetColStyle(sheet, "A", dateStyle); err != nil {
		return nil, err
	}
	if err := file.SetColWidth(sheet, "A", "A", 12); err != nil {
		return nil, err
	}
	if err := file.SetColWidth(sheet, "B", "D", 24); err != nil {
		return nil, err
	}
	if err := file.SetColWidth(sheet, "E", "E", 40); err != nil {
		return nil, err
	}
	if err := file.SetColWidth(sheet, "F", "I", 16); err != nil {
		return nil, err
	}
	if err := file.AutoFilter(sheet, fmt.Sprintf("A1:I%d", max(last, 1)), nil); err != nil {
		return nil, err
	}
	if err := file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := file.Write(&buf); err != nil {
		return nil, err
	}
	if err := s.auditExport(ctx, a, "expenses_xlsx", f.ProjectID, len(rows)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// FormatFCFA écrit un montant avec séparateur de milliers (« 1 155 000 »).
func FormatFCFA(amount int64) string {
	negative := amount < 0
	if negative {
		amount = -amount
	}
	digits := fmt.Sprintf("%d", amount)
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	if negative {
		return "-" + b.String()
	}
	return b.String()
}

// ProjectPDF produit le rapport financier d'un chantier : synthèse, détail par catégorie et dépenses.
func (s *Service) ProjectPDF(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]byte, error) {
	if err := rbac.Require(a, rbac.DataExport); err != nil {
		return nil, err
	}
	detail, err := s.finance.Project(ctx, a, projectID)
	if err != nil {
		return nil, err
	}
	rows, err := s.loadExpenses(ctx, a, expense.ListFilter{ProjectID: &projectID})
	if err != nil {
		return nil, err
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("") // polices de base en cp1252 : accents français
	pdf.SetTitle(tr("Rapport financier - "+detail.Name), false)
	pdf.SetAutoPageBreak(true, 15)
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(0, 6, fmt.Sprintf("Kodi - page %d", pdf.PageNo()), "", 0, "C", false, 0, "")
	})
	pdf.AddPage()

	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 9, tr(detail.Name), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, tr(fmt.Sprintf("Référence %s - rapport du %s - montants en FCFA", detail.Reference, time.Now().Format("02/01/2006"))), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	section := func(title string) {
		pdf.SetFont("Helvetica", "B", 12)
		pdf.CellFormat(0, 8, tr(title), "", 1, "L", false, 0, "")
	}
	section("Synthèse")
	pdf.SetFont("Helvetica", "", 10)
	for _, line := range []struct {
		label string
		value int64
	}{
		{"Budget publié", detail.Budget}, {"Dépenses approuvées", detail.Approved}, {"Dépenses en attente", detail.Pending},
		{"Paiements", detail.Paid}, {"Reste budgétaire", detail.Remains},
	} {
		pdf.CellFormat(70, 6, tr(line.label), "", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, FormatFCFA(line.value), "", 1, "R", false, 0, "")
	}
	pdf.Ln(4)

	section("Par catégorie")
	header := func(cols []string, widths []float64) {
		pdf.SetFont("Helvetica", "B", 9)
		pdf.SetFillColor(240, 240, 243)
		for i, c := range cols {
			align := "R"
			if i == 0 || (len(cols) == 5 && i < 4) {
				align = "L"
			}
			pdf.CellFormat(widths[i], 7, tr(c), "1", 0, align, true, 0, "")
		}
		pdf.Ln(-1)
		pdf.SetFont("Helvetica", "", 9)
	}
	catWidths := []float64{62, 32, 32, 32, 32}
	header([]string{"Catégorie", "Budget", "Approuvé", "En attente", "Reste"}, catWidths)
	for _, c := range detail.Categories {
		pdf.CellFormat(catWidths[0], 6, tr(c.Name), "1", 0, "L", false, 0, "")
		pdf.CellFormat(catWidths[1], 6, FormatFCFA(c.Budget), "1", 0, "R", false, 0, "")
		pdf.CellFormat(catWidths[2], 6, FormatFCFA(c.Approved), "1", 0, "R", false, 0, "")
		pdf.CellFormat(catWidths[3], 6, FormatFCFA(c.Pending), "1", 0, "R", false, 0, "")
		pdf.CellFormat(catWidths[4], 6, FormatFCFA(c.Remains), "1", 1, "R", false, 0, "")
	}
	pdf.Ln(4)

	section(fmt.Sprintf("Dépenses (%d)", len(rows)))
	expWidths := []float64{22, 80, 34, 24, 30}
	header([]string{"Date", "Description", "Catégorie", "Statut", "Montant"}, expWidths)
	var total int64
	for _, e := range rows {
		description := e.Description
		if len(description) > 48 {
			description = description[:48] + "..."
		}
		pdf.CellFormat(expWidths[0], 6, e.ExpenseDate.Format("02/01/2006"), "1", 0, "L", false, 0, "")
		pdf.CellFormat(expWidths[1], 6, tr(description), "1", 0, "L", false, 0, "")
		pdf.CellFormat(expWidths[2], 6, tr(e.CategoryName), "1", 0, "L", false, 0, "")
		pdf.CellFormat(expWidths[3], 6, tr(StatusLabel[e.Status]), "1", 0, "L", false, 0, "")
		pdf.CellFormat(expWidths[4], 6, FormatFCFA(e.Amount), "1", 1, "R", false, 0, "")
		total += e.Amount
	}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.CellFormat(expWidths[0]+expWidths[1]+expWidths[2]+expWidths[3], 7, tr("Total, tous statuts confondus"), "1", 0, "R", false, 0, "")
	pdf.CellFormat(expWidths[4], 7, FormatFCFA(total), "1", 1, "R", false, 0, "")

	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	if err := s.auditExport(ctx, a, "project_pdf", &projectID, len(rows)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PortfolioPDF produit le comparatif de tous les chantiers visibles.
func (s *Service) PortfolioPDF(ctx context.Context, a auth.Actor) ([]byte, error) {
	if err := rbac.Require(a, rbac.DataExport); err != nil {
		return nil, err
	}
	overview, err := s.finance.Overview(ctx, a, nil)
	if err != nil {
		return nil, err
	}
	pdf := fpdf.New("L", "mm", "A4", "")
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	pdf.SetTitle(tr("Comparatif des chantiers"), false)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "B", 16)
	pdf.CellFormat(0, 9, tr("Comparatif par chantier"), "", 1, "L", false, 0, "")
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(0, 6, tr(fmt.Sprintf("Rapport du %s - montants en FCFA", time.Now().Format("02/01/2006"))), "", 1, "L", false, 0, "")
	pdf.Ln(4)
	widths := []float64{77, 40, 40, 40, 40, 40}
	pdf.SetFont("Helvetica", "B", 9)
	pdf.SetFillColor(240, 240, 243)
	for i, h := range []string{"Chantier", "Budget publié", "Approuvé", "En attente", "Payé", "Reste"} {
		align := "R"
		if i == 0 {
			align = "L"
		}
		pdf.CellFormat(widths[i], 7, tr(h), "1", 0, align, true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 9)
	row := func(name string, f finance.Figures) {
		pdf.CellFormat(widths[0], 6, tr(name), "1", 0, "L", false, 0, "")
		for i, v := range []int64{f.Budget, f.Approved, f.Pending, f.Paid, f.Remains} {
			pdf.CellFormat(widths[i+1], 6, FormatFCFA(v), "1", 0, "R", false, 0, "")
		}
		pdf.Ln(-1)
	}
	for _, p := range overview.Projects {
		row(p.Name, p.Figures)
	}
	pdf.SetFont("Helvetica", "B", 9)
	row("Total", overview.Totals)
	if err := pdf.Error(); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	if err := s.auditExport(ctx, a, "portfolio_pdf", nil, len(overview.Projects)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
