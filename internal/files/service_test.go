package files

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidate(t *testing.T) {
	project := uuid.New()
	base := func() PresignInput {
		return PresignInput{ID: uuid.New(), ProjectID: &project, Kind: "receipt", MimeType: "image/jpeg", SizeBytes: 1000}
	}
	tests := []struct {
		name   string
		mutate func(*PresignInput)
		ok     bool
	}{
		{"valide", func(*PresignInput) {}, true},
		{"taille maximale", func(in *PresignInput) { in.SizeBytes = MaxSizeBytes }, true},
		{"trop gros", func(in *PresignInput) { in.SizeBytes = MaxSizeBytes + 1 }, false},
		{"vide", func(in *PresignInput) { in.SizeBytes = 0 }, false},
		{"exécutable", func(in *PresignInput) { in.MimeType = "application/x-msdownload" }, false},
		{"svg (scriptable)", func(in *PresignInput) { in.MimeType = "image/svg+xml" }, false},
		{"sans propriétaire", func(in *PresignInput) { in.ProjectID = nil }, false},
		{"deux propriétaires", func(in *PresignInput) {
			a, b := uuid.New(), uuid.New()
			in.ExpenseID, in.DailyReportID = &a, &b
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := base()
			tt.mutate(&in)
			if err := Validate(in); (err == nil) != tt.ok {
				t.Fatalf("erreur = %v, valide attendu = %v", err, tt.ok)
			}
		})
	}
}

func TestMatchesContent(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	pdf := []byte("%PDF-1.7")
	png := []byte("\x89PNG\r\n\x1a\nxxxx")
	webp := []byte("RIFFxxxxWEBPVP8 ")
	if !MatchesContent("image/jpeg", jpeg) || !MatchesContent("application/pdf", pdf) ||
		!MatchesContent("image/png", png) || !MatchesContent("image/webp", webp) {
		t.Error("une signature valide est refusée")
	}
	if MatchesContent("image/jpeg", pdf) {
		t.Error("un PDF ne doit pas passer pour un JPEG")
	}
	if MatchesContent("application/pdf", []byte("<script>alert(1)</script>")) {
		t.Error("du HTML ne doit pas passer pour un PDF")
	}
}

func TestStorageKeyIsTenantScoped(t *testing.T) {
	org, proj, id := uuid.New(), uuid.New(), uuid.New()
	key := StorageKey(org, proj, id, "jpg", time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC))
	want := "org/" + org.String() + "/proj/" + proj.String() + "/2026/11/" + id.String() + ".jpg"
	if key != want {
		t.Fatalf("clé = %s", key)
	}
	if strings.Contains(key, "..") {
		t.Fatal("la clé ne doit pas permettre de remonter l'arborescence")
	}
}

func TestCleanNameDropsPath(t *testing.T) {
	name := `..\..\etc/passwd`
	if got := cleanName(&name); *got != "passwd" {
		t.Fatalf("nom = %q", *got)
	}
}
