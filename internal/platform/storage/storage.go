// Package storage abstrait l'object storage S3-compatible. Le bucket reste privé :
// le navigateur ne lit et n'écrit que via des URL pré-signées de courte durée.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/cors"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrNotFound : l'objet n'a pas (encore) été déposé.
var ErrNotFound = errors.New("objet introuvable")

// ErrNotConfigured : aucun stockage configuré (environnement local sans MinIO).
var ErrNotConfigured = errors.New("stockage non configuré")

// ObjectInfo décrit un objet déposé.
type ObjectInfo struct{ Size int64 }

// Storage est le contrat utilisé par le module files.
type Storage interface {
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
	Stat(ctx context.Context, key string) (ObjectInfo, error)
	// ReadHead lit les n premiers octets, pour vérifier le type réel du fichier déposé.
	ReadHead(ctx context.Context, key string, n int64) ([]byte, error)
}

// S3 implémente Storage avec un serveur S3-compatible.
type S3 struct {
	client *minio.Client
	bucket string
}

// NewS3 construit le client. La région est fixée pour que la signature ne nécessite aucun appel réseau.
func NewS3(endpoint, accessKey, secretKey, bucket, region string) (*S3, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("S3_ENDPOINT invalide : %q", endpoint)
	}
	client, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: strings.EqualFold(u.Scheme, "https"),
		Region: region,
	})
	if err != nil {
		return nil, err
	}
	return &S3{client: client, bucket: bucket}, nil
}

// Setup crée le bucket s'il manque et autorise le dépôt direct depuis la PWA, et elle seule.
// Sans cette règle CORS, le navigateur refuse tout envoi vers le stockage.
func (s *S3) Setup(ctx context.Context, appOrigin string) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !exists {
		if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	return s.client.SetBucketCors(ctx, s.bucket, cors.NewConfig([]cors.Rule{{
		AllowedOrigin: []string{appOrigin},
		AllowedMethod: []string{http.MethodPut, http.MethodGet},
		AllowedHeader: []string{"*"},
		MaxAgeSeconds: 3600,
	}}))
}

// PresignPut implémente Storage.
func (s *S3) PresignPut(ctx context.Context, key, _ string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedPutObject(ctx, s.bucket, key, ttl)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// PresignGet implémente Storage.
func (s *S3) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, ttl, nil)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// Stat implémente Storage.
func (s *S3) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return ObjectInfo{}, ErrNotFound
		}
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: info.Size}, nil
}

// ReadHead implémente Storage.
func (s *S3) ReadHead(ctx context.Context, key string, n int64) ([]byte, error) {
	opts := minio.GetObjectOptions{}
	if err := opts.SetRange(0, n-1); err != nil {
		return nil, err
	}
	obj, err := s.client.GetObject(ctx, s.bucket, key, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()
	return io.ReadAll(io.LimitReader(obj, n))
}

// Disabled est utilisé sans configuration S3 : les pièces jointes répondent alors une erreur claire.
type Disabled struct{}

// PresignPut implémente Storage.
func (Disabled) PresignPut(context.Context, string, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// PresignGet implémente Storage.
func (Disabled) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotConfigured
}

// Stat implémente Storage.
func (Disabled) Stat(context.Context, string) (ObjectInfo, error) {
	return ObjectInfo{}, ErrNotConfigured
}

// ReadHead implémente Storage.
func (Disabled) ReadHead(context.Context, string, int64) ([]byte, error) {
	return nil, ErrNotConfigured
}
