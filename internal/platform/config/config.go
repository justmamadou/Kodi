// Package config charge et valide la configuration au démarrage : une configuration incomplète
// arrête le processus immédiatement, avec tous les problèmes listés d'un coup.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config regroupe les paramètres de l'application.
type Config struct {
	Env               string // development | test | staging | production
	HTTPAddr          string
	MetricsAddr       string // vide = métriques désactivées ; ne jamais exposer publiquement
	AppBaseURL        string // URL publique de la PWA (liens d'invitation)
	DatabaseURL       string // rôle cf_app, soumis à la RLS
	MigrateURL        string // rôle propriétaire, réservé aux migrations
	CookieSecure      bool
	TrustProxyHeaders bool // true uniquement derrière un reverse proxy maîtrisé (Caddy)
	AuthRatePerMinute int

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string

	SMTPAddr string
	MailFrom string

	OTLPEndpoint string
}

// IsProduction indique un environnement où les garde-fous de sécurité sont obligatoires.
func (c Config) IsProduction() bool { return c.Env == "production" || c.Env == "staging" }

// Load lit la configuration depuis l'environnement et la valide.
func Load() (Config, error) { return load(os.Getenv) }

func load(get func(string) string) (Config, error) {
	var errs []error
	c := Config{
		Env:          orDefault(get("APP_ENV"), "development"),
		HTTPAddr:     orDefault(get("HTTP_ADDR"), ":8080"),
		MetricsAddr:  get("METRICS_ADDR"),
		AppBaseURL:   orDefault(get("APP_BASE_URL"), "http://localhost:3000"),
		DatabaseURL:  get("DATABASE_URL"),
		MigrateURL:   get("MIGRATE_DATABASE_URL"),
		S3Endpoint:   get("S3_ENDPOINT"),
		S3AccessKey:  get("S3_ACCESS_KEY"),
		S3SecretKey:  get("S3_SECRET_KEY"),
		S3Bucket:     get("S3_BUCKET"),
		S3Region:     orDefault(get("S3_REGION"), "us-east-1"),
		SMTPAddr:     get("SMTP_ADDR"),
		MailFrom:     get("MAIL_FROM"),
		OTLPEndpoint: get("OTEL_EXPORTER_OTLP_ENDPOINT"),
	}

	switch c.Env {
	case "development", "test", "staging", "production":
	default:
		errs = append(errs, fmt.Errorf("APP_ENV invalide : %q", c.Env))
	}
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL est obligatoire"))
	}
	if u, err := url.Parse(c.AppBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		errs = append(errs, fmt.Errorf("APP_BASE_URL invalide : %q", c.AppBaseURL))
	}

	c.CookieSecure = parseBool(get("COOKIE_SECURE"), c.IsProduction(), "COOKIE_SECURE", &errs)
	c.TrustProxyHeaders = parseBool(get("TRUST_PROXY_HEADERS"), false, "TRUST_PROXY_HEADERS", &errs)
	c.AuthRatePerMinute = parseInt(get("AUTH_RATE_PER_MINUTE"), 20, "AUTH_RATE_PER_MINUTE", &errs)
	if c.AuthRatePerMinute < 1 {
		errs = append(errs, errors.New("AUTH_RATE_PER_MINUTE doit être positif"))
	}

	s3Set := []string{c.S3Endpoint, c.S3AccessKey, c.S3SecretKey, c.S3Bucket}
	s3Count := 0
	for _, v := range s3Set {
		if v != "" {
			s3Count++
		}
	}
	if s3Count != 0 && s3Count != len(s3Set) {
		errs = append(errs, errors.New("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY et S3_BUCKET vont ensemble"))
	}
	if c.SMTPAddr != "" && c.MailFrom == "" {
		errs = append(errs, errors.New("MAIL_FROM est obligatoire si SMTP_ADDR est défini"))
	}
	if c.IsProduction() {
		if !c.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE ne peut pas être désactivé en production"))
		}
		if s3Count == 0 {
			errs = append(errs, errors.New("le stockage S3 est obligatoire en production"))
		}
		if !strings.HasPrefix(c.AppBaseURL, "https://") {
			errs = append(errs, errors.New("APP_BASE_URL doit être en HTTPS en production"))
		}
	}
	return c, errors.Join(errs...)
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func parseBool(v string, def bool, name string, errs *[]error) bool {
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s invalide : %q", name, v))
		return def
	}
	return b
}

func parseInt(v string, def int, name string, errs *[]error) int {
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s invalide : %q", name, v))
		return def
	}
	return i
}
