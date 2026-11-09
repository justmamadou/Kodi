package reporting

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/justmamadou/Kodi/internal/expense"
	"github.com/justmamadou/Kodi/internal/httpx"
)

// Handler expose les exports.
type Handler struct{ svc *Service }

// NewHandler construit le handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

// Routes monte les routes.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/exports/expenses.xlsx", httpx.H(h.expensesXLSX))
	r.Get("/exports/projects/{id}/report.pdf", httpx.H(h.projectPDF))
	r.Get("/exports/portfolio.pdf", httpx.H(h.portfolioPDF))
}

func (h *Handler) expensesXLSX(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.QueryUUID(r, "project_id")
	if err != nil {
		return err
	}
	categoryID, err := httpx.QueryUUID(r, "category_id")
	if err != nil {
		return err
	}
	from, err := httpx.QueryDate(r, "date_from")
	if err != nil {
		return err
	}
	to, err := httpx.QueryDate(r, "date_to")
	if err != nil {
		return err
	}
	body, err := h.svc.ExpensesXLSX(r.Context(), httpx.Actor(r), expense.ListFilter{
		ProjectID: projectID, Status: httpx.QueryString(r, "status"), CategoryID: categoryID,
		DateFrom: from, DateTo: to, Q: httpx.QueryString(r, "q"),
	})
	if err != nil {
		return err
	}
	return attachment(w, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		fmt.Sprintf("depenses-%s.xlsx", time.Now().Format("2006-01-02")), body)
}

func (h *Handler) projectPDF(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathUUID(r, "id")
	if err != nil {
		return err
	}
	body, err := h.svc.ProjectPDF(r.Context(), httpx.Actor(r), id)
	if err != nil {
		return err
	}
	return attachment(w, "application/pdf", fmt.Sprintf("rapport-chantier-%s.pdf", time.Now().Format("2006-01-02")), body)
}

func (h *Handler) portfolioPDF(w http.ResponseWriter, r *http.Request) error {
	body, err := h.svc.PortfolioPDF(r.Context(), httpx.Actor(r))
	if err != nil {
		return err
	}
	return attachment(w, "application/pdf", fmt.Sprintf("rapport-chantiers-%s.pdf", time.Now().Format("2006-01-02")), body)
}

func attachment(w http.ResponseWriter, contentType, name string, body []byte) error {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.WriteHeader(http.StatusOK)
	_, err := w.Write(body) //nolint:gosec // G705 : PDF ou classeur servi en pièce jointe, jamais du HTML
	return err
}
