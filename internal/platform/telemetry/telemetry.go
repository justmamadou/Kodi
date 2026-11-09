// Package telemetry configure les logs structurés et les traces OpenTelemetry.
package telemetry

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// SetupLogging active les logs JSON sur la sortie standard. Le middleware y ajoute request_id,
// org_id et user_id ; ni mot de passe, ni jeton, ni contenu de document n'y entrent.
func SetupLogging(env string) {
	level := slog.LevelInfo
	if env == "development" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})))
}

// SetupTracing exporte les traces en OTLP/HTTP (endpoint lu dans OTEL_EXPORTER_OTLP_ENDPOINT).
// Sans endpoint, le traceur reste inactif et ne coûte rien.
func SetupTracing(ctx context.Context, endpoint string) (shutdown func(context.Context) error, err error) {
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown, nil
}
