package metrics

import (
    "net/http"

    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "github.com/tunnelgate/tunnelgate/internal/config"
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

func Start(cfg *config.Config) {
    http.Handle("/metrics", promhttp.Handler())
    // In reality we'd update metrics in proxy handlers.
    // For simplicity, we just expose the endpoint.
    http.ListenAndServe(":9100", nil)
}
