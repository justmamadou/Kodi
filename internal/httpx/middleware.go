package httpx

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/justmamadou/Kodi/internal/apperr"
	"github.com/justmamadou/Kodi/internal/auth"
	"github.com/justmamadou/Kodi/internal/platform/db"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// CSRFCookie et CSRFHeader implémentent le double envoi : le cookie est lisible par la PWA,
// qui le recopie dans l'en-tête ; un site tiers ne peut pas lire le cookie.
const (
	CSRFCookie = "cf_csrf"
	CSRFHeader = "X-CSRF-Token"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += n
	return n, err
}

// Metrics regroupe les métriques HTTP exposées à Prometheus.
type Metrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

// NewMetrics enregistre les métriques HTTP.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "kodi_http_requests_total", Help: "Requêtes HTTP par route, méthode et statut.",
		}, []string{"route", "method", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "kodi_http_request_duration_seconds", Help: "Latence HTTP par route.",
			Buckets: []float64{.01, .025, .05, .1, .2, .35, .5, .7, 1, 2, 5},
		}, []string{"route", "method"}),
	}
	reg.MustRegister(m.requests, m.duration)
	return m
}

// RequestContext pose le request_id et les métadonnées, journalise chaque requête et rattrape les panics.
func RequestContext(trustProxy bool, metrics *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			requestID := r.Header.Get("X-Request-Id")
			if requestID == "" || len(requestID) > 64 {
				requestID = uuid.NewString()
			}
			meta := auth.RequestMeta{RequestID: requestID, IP: ClientIP(r, trustProxy), UserAgent: r.UserAgent()}
			ctx, actor := auth.WithActorSlot(auth.WithMeta(r.Context(), meta))
			r = r.WithContext(ctx)
			w.Header().Set("X-Request-Id", requestID)
			rec := &statusRecorder{ResponseWriter: w}

			defer func() {
				if p := recover(); p != nil {
					slog.ErrorContext(r.Context(), "panic", "panic", p, "stack", string(debug.Stack()), "request_id", requestID)
					if rec.status == 0 {
						WriteError(rec, r, errors.New("panic"))
					}
				}
				route := "unmatched"
				if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
					route = rc.RoutePattern()
				}
				status := rec.status
				if status == 0 {
					status = http.StatusOK
				}
				attrs := []any{
					"request_id", requestID, "method", r.Method, "route", route,
					"status", status, "duration_ms", time.Since(start).Milliseconds(),
				}
				if actor.OrgID != uuid.Nil {
					attrs = append(attrs, "org_id", actor.OrgID, "user_id", actor.UserID)
				}
				// Le motif de route n'est connu qu'après le routage : on nomme le span maintenant, pour
				// retrouver un parcours métier dans les traces sans multiplier les noms par identifiant.
				span := trace.SpanFromContext(r.Context())
				span.SetName(r.Method + " " + route)
				span.SetAttributes(attribute.String("http.route", route), attribute.String("request.id", requestID))
				slog.InfoContext(r.Context(), "http", attrs...)
				if metrics != nil {
					metrics.requests.WithLabelValues(route, r.Method, strconv.Itoa(status)).Inc()
					metrics.duration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// ClientIP retourne l'adresse du client. X-Forwarded-For n'est lu que derrière un proxy maîtrisé :
// sinon n'importe qui pourrait contourner la limitation de débit en le falsifiant.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// SecurityHeaders ajoute les en-têtes de sécurité de l'API.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// RateLimiter limite le débit par adresse IP. État en mémoire : suffisant pour une instance ;
// à déplacer vers un stockage partagé au passage à plusieurs instances (voir docs/DECISIONS.md).
type RateLimiter struct {
	mu       sync.Mutex
	limit    rate.Limit
	burst    int
	visitors map[string]*visitor
	now      func() time.Time
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter autorise perMinute requêtes par minute et par IP.
func NewRateLimiter(perMinute int) *RateLimiter {
	return &RateLimiter{
		limit: rate.Limit(float64(perMinute) / 60), burst: perMinute,
		visitors: map[string]*visitor{}, now: time.Now,
	}
}

func (l *RateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.visitors) > 10_000 { // borne la mémoire face à un balayage d'adresses
		for k, v := range l.visitors {
			if now.Sub(v.lastSeen) > time.Hour {
				delete(l.visitors, k)
			}
		}
	}
	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = now
	return v.limiter.AllowN(now, 1)
}

// Middleware refuse les requêtes au-delà du débit autorisé (429).
func (l *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(auth.MetaFrom(r.Context()).IP) {
			w.Header().Set("Retry-After", "60")
			WriteError(w, r, apperr.New(http.StatusTooManyRequests, "auth.rate_limited",
				"Trop de tentatives. Réessayez dans une minute."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// CSRF exige le double envoi du jeton sur les mutations authentifiées par cookie.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isSafeMethod(r.Method) {
			cookie, err := r.Cookie(CSRFCookie)
			header := r.Header.Get(CSRFHeader)
			if err != nil || header == "" || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				WriteError(w, r, apperr.New(http.StatusForbidden, "auth.csrf", "Jeton de sécurité manquant ou invalide. Rechargez la page."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

type capturingWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (c *capturingWriter) WriteHeader(code int) {
	c.status = code
	c.ResponseWriter.WriteHeader(code)
}

func (c *capturingWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	c.body.Write(b)
	return c.ResponseWriter.Write(b)
}

// Idempotency rejoue la réponse mémorisée si une mutation arrive deux fois avec la même clé.
// C'est ce qui rend sûr le rejeu de l'outbox hors ligne et le double clic. Seuls les succès sont
// mémorisés : après une erreur, le client peut corriger et réessayer avec la même clé.
func Idempotency(tx *db.TxManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Idempotency-Key")
			if raw == "" || isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			key, err := uuid.Parse(raw)
			if err != nil {
				WriteError(w, r, apperr.New(http.StatusBadRequest, "idempotency.invalid_key", "Idempotency-Key doit être un UUID."))
				return
			}
			actor, ok := auth.ActorFrom(r.Context())
			if !ok {
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
			if err != nil {
				WriteError(w, r, apperr.New(http.StatusRequestEntityTooLarge, "request.too_large", "Requête trop volumineuse."))
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			h := sha256.New()
			h.Write([]byte(r.Method + " " + r.URL.Path + "\n"))
			h.Write(body)
			hash := h.Sum(nil)

			var replay *db.IdempotencyKey
			err = tx.InTenantTx(r.Context(), actor.OrgID, func(q *db.Queries) error {
				_, err := q.ClaimIdempotencyKey(r.Context(), db.ClaimIdempotencyKeyParams{
					OrgID: actor.OrgID, Key: key, UserID: actor.UserID,
					Method: r.Method, Path: r.URL.Path, RequestHash: hash,
				})
				if err == nil {
					return nil
				}
				if !db.IsNoRows(err) {
					return err
				}
				existing, err := q.GetIdempotencyKey(r.Context(), db.GetIdempotencyKeyParams{OrgID: actor.OrgID, Key: key})
				if err != nil {
					return err
				}
				replay = &existing
				return nil
			})
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if replay != nil {
				if replay.UserID != actor.UserID || subtle.ConstantTimeCompare(replay.RequestHash, hash) != 1 {
					WriteError(w, r, apperr.New(http.StatusUnprocessableEntity, "idempotency.key_reused",
						"Cette clé d'idempotence a déjà servi pour une autre requête."))
					return
				}
				if replay.ResponseStatus == nil {
					WriteError(w, r, apperr.Conflict("idempotency.in_progress", "Cette requête est déjà en cours de traitement."))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Idempotency-Replayed", "true")
				w.WriteHeader(int(*replay.ResponseStatus))
				_, _ = w.Write(replay.ResponseBody)
				return
			}

			rec := &capturingWriter{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			// La requête d'origine est terminée : son contexte peut déjà être annulé côté client.
			ctx := withoutCancel(r)
			err = tx.InTenantTx(ctx, actor.OrgID, func(q *db.Queries) error {
				if rec.status >= 200 && rec.status < 300 {
					status := int16(rec.status)
					return q.SaveIdempotencyResponse(ctx, db.SaveIdempotencyResponseParams{
						OrgID: actor.OrgID, Key: key, ResponseStatus: &status, ResponseBody: rec.body.Bytes(),
					})
				}
				return q.DeleteIdempotencyKey(ctx, db.DeleteIdempotencyKeyParams{OrgID: actor.OrgID, Key: key})
			})
			if err != nil {
				slog.ErrorContext(ctx, "clé d'idempotence non enregistrée", "error", err)
			}
		})
	}
}
