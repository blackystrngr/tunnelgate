package metrics

import (
    "context"

    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
)

// Start is a no‑op stub – metrics disabled.
func Start(ctx context.Context, cfg *config.Config) error {
    logger.Info("Metrics disabled (Prometheus not available)")
    <-ctx.Done()
    return nil
}
