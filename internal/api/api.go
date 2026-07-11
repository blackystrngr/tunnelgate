package api

import (
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "strconv"

    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/user"
)

func Start(cfg *config.Config) {
    mux := http.NewServeMux()
    mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
        if r.Method != "GET" {
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
        db := user.OpenDB(cfg.Database)
        users, err := user.ListUsers(db)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        json.NewEncoder(w).Encode(users)
    })

    mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
        // return simple status
        w.Write([]byte(`{"status":"ok"}`))
    })

    addr := fmt.Sprintf("%s:%d", cfg.API.ListenHost, cfg.API.ListenPort)
    log.Printf("API listening on %s", addr)
    log.Fatal(http.ListenAndServe(addr, mux))
}
