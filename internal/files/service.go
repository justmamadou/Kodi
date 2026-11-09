// Package files gère les pièces jointes : les octets vont directement à l'object storage privé,
// l'API ne traite que les métadonnées et le contrôle d'accès.
package files

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/audit"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/storage"
	"github.com/justmamadou/Kodi/internal/rbac"
)

// Limites et durées de vie (architecture §6.4).
const (
	MaxSizeBytes = 25 * 1024 * 1024
	putTTL       = 5 * time.Minute
	getTTL       = 10 * time.Minute
	sniffBytes   = 512
)

// allowedTypes associe chaque type accepté à son extension.
var allowedTypes = map[string]string{
	"image/jpeg":      "jpg",
	"image/png":       "png",
	"image/webp":      "webp",
	"application/pdf": "pdf",
}

// Service porte les cas d'usage des pièces jointes.
type Service struct {
	tx    *db.TxManager
	store storage.Storage
}

// NewService construit le service.
func NewService(tx *db.TxManager, store storage.Storage) *Service {
	return &Service{tx: tx, store: store}
}

// PresignInput décrit un fichier à déposer. Il est rattaché à une dépense, à un rapport ou au chantier.
type PresignInput struct {
	ID            uuid.UUID  `json:"id"`
	ProjectID     *uuid.UUID `json:"project_id"`
	ExpenseID     *uuid.UUID `json:"expense_id"`
	DailyReportID *uuid.UUID `json:"daily_report_id"`
	Kind          string     `json:"kind"`
	MimeType      string     `json:"mime_type"`
	SizeBytes     int64      `json:"size_bytes"`
	OriginalName  *string    `json:"original_name"`
}

// Validate contrôle le type et la taille déclarés, avant toute écriture.
func Validate(in PresignInput) error {
	var v apperr.Validator
	v.Check(in.ID != uuid.Nil, "id", "L'identifiant est obligatoire.")
	_, allowed := allowedTypes[in.MimeType]
	v.Check(allowed, "mime_type", "Type de fichier non autorisé : JPEG, PNG, WebP ou PDF.")
	v.Check(in.SizeBytes > 0, "size_bytes", "Le fichier est vide.")
	v.Check(in.SizeBytes <= MaxSizeBytes, "size_bytes", "Le fichier dépasse la taille maximale de 25 Mo.")
	v.Check(in.Kind == "receipt" || in.Kind == "site_photo" || in.Kind == "document", "kind", "Nature de fichier inconnue.")
	v.Check(in.ExpenseID == nil || in.DailyReportID == nil, "expense_id", "Un fichier n'a qu'un seul propriétaire.")
	v.Check(in.ProjectID != nil || in.ExpenseID != nil || in.DailyReportID != nil, "project_id", "Indiquez le chantier, la dépense ou le rapport.")
	if in.OriginalName != nil {
		v.Check(len(*in.OriginalName) <= 255, "original_name", "Nom de fichier trop long.")
	}
	return v.Err()
}

// Presigned est la réponse à une demande de dépôt.
type Presigned struct {
	Attachment db.Attachment `json:"attachment"`
	UploadURL  string        `json:"upload_url"`
	ExpiresIn  int           `json:"expires_in"`
}

// StorageKey construit la clé objet. Le tenant et le chantier en font partie : aucune collision
// entre entreprises n'est possible, et le nom fourni par le client n'y entre jamais.
func StorageKey(orgID, projectID, id uuid.UUID, ext string, at time.Time) string {
	return fmt.Sprintf("org/%s/proj/%s/%04d/%02d/%s.%s", orgID, projectID, at.Year(), int(at.Month()), id, ext)
}

// Presign contrôle l'accès à la ressource propriétaire et prépare le dépôt.
func (s *Service) Presign(ctx context.Context, a auth.Actor, in PresignInput) (Presigned, error) {
	if err := rbac.Require(a, rbac.FileWrite); err != nil {
		return Presigned{}, err
	}
	if err := Validate(in); err != nil {
		return Presigned{}, err
	}
	var out Presigned
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		projectID, err := resolveProject(ctx, q, a, in)
		if err != nil {
			return err
		}
		if _, err := rbac.ProjectWritable(ctx, q, a, projectID); err != nil {
			return err
		}
		key := StorageKey(a.OrgID, projectID, in.ID, allowedTypes[in.MimeType], time.Now().UTC())
		att, err := q.CreateAttachment(ctx, db.CreateAttachmentParams{
			ID: in.ID, OrgID: a.OrgID, ProjectID: projectID, ExpenseID: in.ExpenseID, DailyReportID: in.DailyReportID,
			StorageKey: key, Kind: in.Kind, MimeType: in.MimeType, SizeBytes: in.SizeBytes,
			OriginalName: cleanName(in.OriginalName), UploadedBy: a.UserID,
		})
		if db.IsNoRows(err) {
			// Renvoi hors connexion : même id, on redonne une URL pour le même objet.
			att, err = q.GetAttachment(ctx, db.GetAttachmentParams{OrgID: a.OrgID, ID: in.ID})
			if db.IsNoRows(err) || (err == nil && att.Status != "pending") {
				return apperr.Conflict("attachment.id_conflict", "Cet identifiant de fichier n'est pas utilisable.")
			}
		}
		if err != nil {
			return err
		}
		url, err := s.store.PresignPut(ctx, att.StorageKey, att.MimeType, putTTL)
		if err != nil {
			return storageErr(err)
		}
		out = Presigned{Attachment: att, UploadURL: url, ExpiresIn: int(putTTL.Seconds())}
		return nil
	})
	return out, err
}

func cleanName(name *string) *string {
	if name == nil {
		return nil
	}
	base := filepath.Base(strings.ReplaceAll(*name, `\`, "/"))
	return &base
}

func storageErr(err error) error {
	if errors.Is(err, storage.ErrNotConfigured) {
		return apperr.New(http.StatusServiceUnavailable, "attachment.storage_unavailable", "Le stockage de fichiers n'est pas configuré.")
	}
	return err
}

// resolveProject déduit le chantier du propriétaire : un client ne peut pas rattacher un fichier
// à une dépense d'un autre chantier en annonçant un project_id qui l'arrange.
func resolveProject(ctx context.Context, q *db.Queries, a auth.Actor, in PresignInput) (uuid.UUID, error) {
	var projectID uuid.UUID
	switch {
	case in.ExpenseID != nil:
		e, err := q.GetExpense(ctx, db.GetExpenseParams{OrgID: a.OrgID, ID: *in.ExpenseID})
		if err != nil {
			if db.IsNoRows(err) {
				return uuid.Nil, apperr.NotFound("expense")
			}
			return uuid.Nil, err
		}
		projectID = e.ProjectID
	case in.DailyReportID != nil:
		r, err := q.GetDailyReport(ctx, db.GetDailyReportParams{OrgID: a.OrgID, ID: *in.DailyReportID})
		if err != nil {
			if db.IsNoRows(err) {
				return uuid.Nil, apperr.NotFound("report")
			}
			return uuid.Nil, err
		}
		projectID = r.ProjectID
	default:
		return *in.ProjectID, nil
	}
	if in.ProjectID != nil && *in.ProjectID != projectID {
		var v apperr.Validator
		v.Check(false, "project_id", "Ce chantier ne correspond pas à la ressource indiquée.")
		return uuid.Nil, v.Err()
	}
	return projectID, nil
}

// Complete confirme le dépôt : vérifie les octets réellement reçus (taille et type), pas seulement
// ce que le client a annoncé, puis marque le justificatif de la dépense.
func (s *Service) Complete(ctx context.Context, a auth.Actor, id uuid.UUID) (db.Attachment, error) {
	if err := rbac.Require(a, rbac.FileWrite); err != nil {
		return db.Attachment{}, err
	}
	var out db.Attachment
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		att, err := q.GetAttachment(ctx, db.GetAttachmentParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("attachment")
			}
			return err
		}
		if _, err := rbac.Project(ctx, q, a, att.ProjectID); err != nil {
			return err
		}
		if att.Status == "uploaded" {
			out = att
			return nil
		}
		info, err := s.store.Stat(ctx, att.StorageKey)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return apperr.Conflict("attachment.not_uploaded", "Le fichier n'a pas encore été reçu.")
			}
			return storageErr(err)
		}
		if info.Size <= 0 || info.Size > MaxSizeBytes {
			return apperr.New(http.StatusUnprocessableEntity, "attachment.invalid_size", "La taille du fichier reçu n'est pas acceptée.")
		}
		head, err := s.store.ReadHead(ctx, att.StorageKey, sniffBytes)
		if err != nil {
			return storageErr(err)
		}
		if !MatchesContent(att.MimeType, head) {
			return apperr.New(http.StatusUnprocessableEntity, "attachment.type_mismatch", "Le contenu du fichier ne correspond pas à son type déclaré.")
		}
		out, err = q.CompleteAttachment(ctx, db.CompleteAttachmentParams{OrgID: a.OrgID, ID: id, SizeBytes: info.Size})
		if err != nil {
			return err
		}
		if att.ExpenseID != nil && att.Kind == "receipt" {
			if err := q.SetExpenseHasReceipt(ctx, db.SetExpenseHasReceiptParams{OrgID: a.OrgID, ID: *att.ExpenseID}); err != nil {
				return err
			}
		}
		return audit.Record(ctx, q, a, audit.Entry{Action: "attachment.uploaded", EntityType: "attachment", EntityID: &id, ProjectID: &att.ProjectID})
	})
	return out, err
}

// MatchesContent vérifie la signature du fichier contre le type déclaré.
func MatchesContent(mime string, head []byte) bool {
	switch mime {
	case "image/jpeg":
		return len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF
	case "image/png":
		return len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1a\n"
	case "image/webp":
		return len(head) >= 12 && string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP"
	case "application/pdf":
		return len(head) >= 5 && string(head[:5]) == "%PDF-"
	}
	return false
}

// ListProject liste les documents d'un chantier (onglet Documents).
func (s *Service) ListProject(ctx context.Context, a auth.Actor, projectID uuid.UUID) ([]db.ListProjectAttachmentsRow, error) {
	if err := rbac.Require(a, rbac.FileRead); err != nil {
		return nil, err
	}
	var out []db.ListProjectAttachmentsRow
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		if _, err := rbac.Project(ctx, q, a, projectID); err != nil {
			return err
		}
		var err error
		out, err = q.ListProjectAttachments(ctx, db.ListProjectAttachmentsParams{OrgID: a.OrgID, ProjectID: projectID})
		return err
	})
	return out, err
}

// DownloadURL émet un lien de lecture de courte durée, après contrôle du tenant et du chantier.
func (s *Service) DownloadURL(ctx context.Context, a auth.Actor, id uuid.UUID) (string, int, error) {
	if err := rbac.Require(a, rbac.FileRead); err != nil {
		return "", 0, err
	}
	var key string
	err := s.tx.InTenantTx(ctx, a.OrgID, func(q *db.Queries) error {
		att, err := q.GetAttachment(ctx, db.GetAttachmentParams{OrgID: a.OrgID, ID: id})
		if err != nil {
			if db.IsNoRows(err) {
				return apperr.NotFound("attachment")
			}
			return err
		}
		if _, err := rbac.Project(ctx, q, a, att.ProjectID); err != nil {
			return err
		}
		if att.Status != "uploaded" {
			return apperr.Conflict("attachment.not_uploaded", "Le fichier n'a pas encore été reçu.")
		}
		key = att.StorageKey
		return nil
	})
	if err != nil {
		return "", 0, err
	}
	url, err := s.store.PresignGet(ctx, key, getTTL)
	if err != nil {
		return "", 0, storageErr(err)
	}
	return url, int(getTTL.Seconds()), nil
}
