package proxy

import (
    "fmt"
    "io"
    "log"
    "net"
    "net/http"
    "time"

    "github.com/gorilla/websocket"
    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/user"
)

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

func Start(cfg *config.Config) {
    // Listen for WebSocket connections on backend port
    http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
        handleProxy(w, r, cfg)
    })

    addr := fmt.Sprintf("%s:%d", cfg.Proxy.ListenHost, cfg.Proxy.ListenPort)
    log.Printf("Proxy listening on %s", addr)
    log.Fatal(http.ListenAndServe(addr, nil))
}

func handleProxy(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Printf("Upgrade failed: %v", err)
        return
    }
    defer conn.Close()

    // Optional: authenticate user via Basic Auth or custom header
    username, password, ok := r.BasicAuth()
    if ok {
        db := user.OpenDB(cfg.Database)
        if !user.Authenticate(db, username, password) {
            conn.WriteMessage(websocket.CloseMessage, []byte("unauthorized"))
            return
        }
    }

    // Connect to backend SSH
    backendAddr := fmt.Sprintf("%s:%d", cfg.BackendHost, cfg.BackendPort)
    backend, err := net.DialTimeout("tcp", backendAddr, 5*time.Second)
    if err != nil {
        log.Printf("Backend connection failed: %v", err)
        conn.WriteMessage(websocket.CloseMessage, []byte("backend unavailable"))
        return
    }
    defer backend.Close()

    // Perform fake HTTP Upgrade response (optional: send 101)
    // Since we use WebSocket, no need to send a fake response – the WebSocket handshake is already 101.

    // Bidirectional copy
    errChan := make(chan error, 2)

    go func() {
        for {
            _, data, err := conn.ReadMessage()
            if err != nil {
                errChan <- err
                return
            }
            _, err = backend.Write(data)
            if err != nil {
                errChan <- err
                return
            }
        }
    }()

    go func() {
        _, err := io.Copy(conn.UnderlyingConn(), backend)
        errChan <- err
    }()

    <-errChan
    // Close gracefully
    conn.WriteMessage(websocket.CloseMessage, []byte{})
}
