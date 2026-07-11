package proxy

import (
    "context"
    "fmt"
    "net/http"
    "sync"
    "time"

    "github.com/gorilla/websocket"
    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
    ReadBufferSize:  4096,
    WriteBufferSize: 4096,
}

func Start(ctx context.Context, cfg *config.Config) error {
    var wg sync.WaitGroup
    errChan := make(chan error, len(cfg.Nginx.HTTPPorts)+len(cfg.Nginx.TLSPorts))

    // Gather all ports to listen on (HTTP and TLS)
    allPorts := append(cfg.Nginx.HTTPPorts, cfg.Nginx.TLSPorts...)
    if len(allPorts) == 0 {
        return fmt.Errorf("no ports configured")
    }

    // We listen on the same 127.0.0.1 for both HTTP and TLS
    // because Nginx forwards TLS to 127.0.0.1 as well.
    listenHost := cfg.Proxy.ListenHost

    for _, port := range allPorts {
        wg.Add(1)
        go func(p int) {
            defer wg.Done()
            addr := fmt.Sprintf("%s:%d", listenHost, p)
            mux := http.NewServeMux()
            mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
                handleProxy(w, r, cfg)
            })

            srv := &http.Server{
                Addr:         addr,
                Handler:      mux,
                ReadTimeout:  10 * time.Second,
                WriteTimeout: 10 * time.Second,
                IdleTimeout:  time.Duration(cfg.Proxy.IdleTimeout) * time.Second,
            }

            logger.Info("Proxy listening", "addr", addr)

            if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                errChan <- fmt.Errorf("port %d: %w", p, err)
            }
        }(port)
    }

    // Wait for error or context cancellation
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-ctx.Done():
        logger.Info("Proxy shutting down...")
        // We don't have a graceful shutdown here in this simple example
        return nil
    case err := <-errChan:
        return err
    case <-done:
        return nil
    }
}

func handleProxy(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
    // (same as before – handle WebSocket upgrade and relay)
    // For brevity, I omit the full handler code – it is unchanged from previous.
    // Please reuse the earlier handleProxy implementation.
}
