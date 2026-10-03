package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(
	logger *slog.Logger,
	apiKey string,
	healthHandler *HealthHandler,
	accountHandler *AccountHandler,
	paymentHandler *PaymentHandler,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(StructuredLogger(logger))

	// Liveness and Readiness probes
	r.Get("/healthz", healthHandler.Healthz)
	r.Get("/readyz", healthHandler.Readyz)

	// Prometheus Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// API Routes
	r.Route("/accounts", func(r chi.Router) {
		r.Get("/{id}", accountHandler.GetAccount)
		r.Post("/", accountHandler.CreateAccount)
	})

	r.Route("/payments", func(r chi.Router) {
		r.Get("/{id}", paymentHandler.GetPayment)
		r.Post("/", paymentHandler.CreatePayment)
	})

	return r
}

func StructuredLogger(logger *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				logger.Info("http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration_ms", time.Since(start).Milliseconds(),
					"req_id", middleware.GetReqID(r.Context()),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
