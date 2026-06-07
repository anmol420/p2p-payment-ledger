package observability

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func StartMetricsServer(addr string, logger *slog.Logger) func(context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			return
		}
	})
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	go func() {
		logger.Info("starting metrics server", "addr", addr)
		if err := srv.ListenAndServe(); err != nil {
			logger.Error("failed to start metrics server", "err", err)
		}
	}()
	return func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	}
}
