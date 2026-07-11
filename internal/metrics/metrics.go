package metrics

import (
    "context"
    "fmt"
    "net/http"
    "time"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
)

var (
    activeConns = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "tunnelgate_active_connections",
        Help: "Current active WebSocket connections",
    })
    totalConns = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "tunnelgate_connections_total",
        Help: "Total connections handled",
    })
)

func init() {
    prometheus.MustRegister(activeConns, totalConns)
}

func Start(ctx context.Context, cfg *config.Config) error {
    mux := http.NewServeMux()
    mux.Handle("/metrics", promhttp.Handler())

    addr := fmt.Sprintf(":%d", 9100) // fixed for simplicity – could be configurable
    srv := &http.Server{
        Addr:         addr,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    logger.Info("Metrics listening", "addr", addr)

    errChan := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errChan <- err
        }
    }()

    select {
    case <-ctx.Done():
        logger.Info("Metrics shutting down...")
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        return srv.Shutdown(shutdownCtx)
    case err := <-errChan:
        return err
    }
}
