package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Test d'intégration contre un vrai serveur S3 (RustFS du docker-compose).
// Activé par TEST_S3_ENDPOINT ; exécuté en CI.
func TestS3PresignedRoundTrip(t *testing.T) {
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT non défini")
	}
	const access, secret, bucket = "kodi", "kodi-dev", "kodi-test"
	ctx := context.Background()

	admin, err := minio.New(strings.TrimPrefix(endpoint, "http://"), &minio.Options{Creds: credentials.NewStaticV4(access, secret, "")})
	if err != nil {
		t.Fatal(err)
	}
	if exists, err := admin.BucketExists(ctx, bucket); err != nil {
		t.Fatal(err)
	} else if !exists {
		if err := admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	store, err := NewS3(endpoint, access, secret, bucket, "us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	// Le dépôt direct depuis le navigateur exige une règle CORS limitée à l'origine de la PWA.
	const origin = "http://app.test"
	if err := store.Setup(ctx, origin); err != nil {
		t.Fatalf("Setup : %v", err)
	}
	preflight := func(from string) string {
		req, err := http.NewRequest(http.MethodOptions, endpoint+"/"+bucket+"/x.jpg", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", from)
		req.Header.Set("Access-Control-Request-Method", http.MethodPut)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.Header.Get("Access-Control-Allow-Origin")
	}
	if got := preflight(origin); got != origin && got != "*" {
		t.Fatalf("préflight refusé pour la PWA : %q", got)
	}
	if got := preflight("http://evil.test"); got != "" {
		t.Fatalf("une autre origine ne doit pas être autorisée : %q", got)
	}

	key := "org/" + uuid.NewString() + "/proj/x/2026/11/" + uuid.NewString() + ".jpg"
	body := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte("x"), 1000)...)

	if _, err := store.Stat(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Stat d'un objet absent : %v", err)
	}

	// Le navigateur dépose directement via l'URL pré-signée, sans identifiants.
	putURL, err := store.PresignPut(ctx, key, "image/jpeg", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPut, putURL, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT pré-signé : %d", resp.StatusCode)
	}

	info, err := store.Stat(ctx, key)
	if err != nil || info.Size != int64(len(body)) {
		t.Fatalf("Stat : %+v, %v", info, err)
	}
	head, err := store.ReadHead(ctx, key, 4)
	if err != nil || !bytes.Equal(head, body[:4]) {
		t.Fatalf("ReadHead : %x, %v", head, err)
	}

	getURL, err := store.PresignGet(ctx, key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	got, err := http.Get(getURL) //nolint:gosec // URL générée par le test
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = got.Body.Close() }()
	data, err := io.ReadAll(got.Body)
	if err != nil || !bytes.Equal(data, body) {
		t.Fatalf("GET pré-signé : %d octets, %v", len(data), err)
	}

	// Le bucket est privé : sans signature, aucun accès.
	anonymous, err := http.Get(strings.SplitN(getURL, "?", 2)[0]) //nolint:gosec // URL générée par le test
	if err != nil {
		t.Fatal(err)
	}
	_ = anonymous.Body.Close()
	if anonymous.StatusCode != http.StatusForbidden {
		t.Fatalf("accès anonyme : %d, 403 attendu", anonymous.StatusCode)
	}
}
