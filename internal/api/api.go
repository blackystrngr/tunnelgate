package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
    "github.com/tunnelgate/tunnelgate/internal/user"
)

func Start(ctx context.Context, cfg *config.Config) error {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", usersHandler(cfg))
    mux.HandleFunc("/api/status", statusHandler(cfg))
    mux.HandleFunc("/api/health", healthHandler)

    addr := fmt.Sprintf("%s:%d", cfg.API.ListenHost, cfg.API.ListenPort)
    srv := &http.Server{
        Addr:         addr,
        Handler:      mux,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
    }

    logger.Info("API listening", "addr", addr)

    errChan := make(chan error, 1)
    go func() {
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            errChan <- err
        }
    }()

    select {
    case <-ctx.Done():
        logger.Info("API shutting down...")
        shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        return srv.Shutdown(shutdownCtx)
    case err := <-errChan:
        return err
    }
}

func usersHandler(cfg *config.Config) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "GET" {
            http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
            return
        }
        db := user.OpenDB(cfg.Database)
        users, err := user.ListUsers(db)
        if err != nil {
            logger.Error("Failed to list users", "error", err)
            http.Error(w, "Internal error", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(users)
    }
}

func statusHandler(cfg *config.Config) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    }
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
