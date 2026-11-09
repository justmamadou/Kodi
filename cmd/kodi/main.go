// Command kodi lance l'API ou les migrations : kodi api | migrate <up|down|version> | storage-setup.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/justmamadou/Kodi/internal/app"
	"github.com/justmamadou/Kodi/internal/platform/config"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/justmamadou/Kodi/internal/platform/mailer"
	"github.com/justmamadou/Kodi/internal/platform/migrate"
	"github.com/justmamadou/Kodi/internal/platform/storage"
	"github.com/justmamadou/Kodi/internal/platform/telemetry"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage : kodi api | migrate <up|down|version> | storage-setup")
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("configuration invalide :\n%w", err)
	}
	telemetry.SetupLogging(cfg.Env)
	switch args[0] {
	case "api":
		return serve(cfg)
	case "migrate":
		return runMigrate(cfg, args[1:])
	case "storage-setup":
		s3, err := storage.NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Region)
		if err != nil {
			return err
		}
		return s3.Setup(context.Background(), cfg.AppBaseURL)
	default:
		return fmt.Errorf("commande inconnue : %s", args[0])
	}
}

func runMigrate(cfg config.Config, args []string) error {
	if cfg.MigrateURL == "" {
		return errors.New("MIGRATE_DATABASE_URL est obligatoire pour migrer")
	}
	if len(args) != 1 {
		return errors.New("usage : kodi migrate <up|down|version>")
	}
	switch args[0] {
	case "up":
		return migrate.Up(cfg.MigrateURL)
	case "down":
		return migrate.Down(cfg.MigrateURL)
	case "version":
		v, err := migrate.Version(cfg.MigrateURL)
		if err != nil {
			return err
		}
		fmt.Println(v)
		return nil
	default:
		return fmt.Errorf("sous-commande inconnue : %s", args[0])
	}
}

func purgeLoop(ctx context.Context, tx *db.TxManager) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		keys, sessions, err := tx.PurgeExpired(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("purge en échec", "error", err)
		} else if keys+sessions > 0 {
			slog.Info("purge effectuée", "idempotency_keys", keys, "sessions", sessions)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func serve(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := telemetry.SetupTracing(ctx, cfg.OTLPEndpoint)
	if err != nil {
		return fmt.Errorf("traces : %w", err)
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	var store storage.Storage = storage.Disabled{}
	if cfg.S3Endpoint != "" {
		s3, err := storage.NewS3(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Region)
		if err != nil {
			return err
		}
		// En production, la configuration du bucket est une étape de déploiement : kodi storage-setup.
		if !cfg.IsProduction() {
			if err := s3.Setup(ctx, cfg.AppBaseURL); err != nil {
				return fmt.Errorf("configuration du stockage S3 : %w", err)
			}
		}
		store = s3
	}
	var mail mailer.Mailer = mailer.Noop{}
	if cfg.SMTPAddr != "" {
		mail = mailer.SMTP{Addr: cfg.SMTPAddr, From: cfg.MailFrom}
	}

	registry := prometheus.NewRegistry()
	tx := db.NewTxManager(pool)
	router, err := app.NewRouter(app.Deps{
		Config: cfg, Tx: tx, Storage: store, Mailer: mail, Registry: registry,
	})
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           otelhttp.NewHandler(router, "kodi.api"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	var metricsServer *http.Server
	if cfg.MetricsAddr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		metricsServer = &http.Server{Addr: cfg.MetricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("serveur de métriques arrêté", "error", err)
			}
		}()
	}

	// Maintenance légère : une instance de plus ne fait qu'une purge vide de plus.
	go purgeLoop(ctx, tx)

	errCh := make(chan error, 1)
	go func() {
		slog.Info("API démarrée", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("arrêt demandé")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	err = server.Shutdown(shutdownCtx)
	if metricsServer != nil {
		err = errors.Join(err, metricsServer.Shutdown(shutdownCtx))
	}
	return errors.Join(err, shutdownTracing(shutdownCtx))
}
