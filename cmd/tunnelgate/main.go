package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "os"
    "os/exec"
    "os/signal"
    "syscall"
    "time"

    "github.com/tunnelgate/tunnelgate/internal/api"
    "github.com/tunnelgate/tunnelgate/internal/cert"
    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/metrics"
    "github.com/tunnelgate/tunnelgate/internal/nginx"
    "github.com/tunnelgate/tunnelgate/internal/proxy"
    "github.com/tunnelgate/tunnelgate/internal/user"
)

var cfgPath string

func init() {
    flag.StringVar(&cfgPath, "config", "/etc/tunnelgate/config.yaml", "path to config file")
}

func main() {
    flag.Parse()
    args := flag.Args()
    if len(args) < 1 {
        fmt.Println("Usage: tunnelgate <command> [options]")
        fmt.Println("Commands: init, start, stop, status, user, cert, upgrade")
        os.Exit(1)
    }

    cmd := args[0]
    switch cmd {
    case "init":
        runInit()
    case "start":
        runStart()
    case "stop":
        runStop()
    case "status":
        runStatus()
    case "user":
        userCLI(args[1:])
    case "cert":
        certCLI(args[1:])
    case "upgrade":
        runUpgrade()
    default:
        fmt.Printf("Unknown command: %s\n", cmd)
        os.Exit(1)
    }
}

func runInit() {
    fmt.Println("=== TunnelGate Interactive Setup ===")
    cfg := &config.Config{}
    // TODO: interactive prompts for domain, email, ports, cert method, etc.
    // For brevity, we'll hardcode some defaults or use the example config.
    cfg.Domain = ask("Domain (e.g. tunnel.example.com): ", "tunnel.example.com")
    cfg.Email = ask("Contact email: ", "")
    cfg.CertMethod = ask("Cert method (le_http01/le_dns_cf/cf_origin): ", "le_http01")
    // Save config
    cfg.Save(cfgPath)
    // Generate Nginx config
    nginx.Configure(cfg)
    // Obtain certificate if needed
    if len(cfg.TLSPorts) > 0 {
        cert.Obtain(cfg)
    }
    // Initialize database
    user.InitDB(cfg.Database)
    // Start services (or just enable them)
    runStart()
    fmt.Println("Setup complete. Run 'tunnelgate user add <name>' to create users.")
}

func runStart() {
    cfg := config.Load(cfgPath)
    // Start proxy core
    go proxy.Start(cfg)
    // Start API server
    go api.Start(cfg)
    // Start metrics exporter
    go metrics.Start(cfg)
    // Monitor signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    fmt.Println("Shutting down...")
}

func runStop() {
    // Stop systemd services
    exec.Command("systemctl", "stop", "tunnelgate-proxy.service").Run()
    exec.Command("systemctl", "stop", "tunnelgate-api.service").Run()
    fmt.Println("Services stopped.")
}

func runStatus() {
    // Print status: active connections, cert expiry, etc.
    fmt.Println("TunnelGate Status:")
    // Query systemd
    out, _ := exec.Command("systemctl", "is-active", "tunnelgate-proxy.service").Output()
    fmt.Printf("Proxy: %s", out)
    // Check certificate
    cfg := config.Load(cfgPath)
    expiry := cert.GetExpiry(cfg.Domain)
    fmt.Printf("Certificate expiry: %s\n", expiry)
    // User count
    db := user.OpenDB(cfg.Database)
    count, _ := user.CountUsers(db)
    fmt.Printf("Total users: %d\n", count)
}

func runUpgrade() {
    fmt.Println("Upgrading TunnelGate...")
    // Download latest binary, replace, restart
    // Implement later
}

func userCLI(args []string) {
    if len(args) < 1 {
        fmt.Println("Usage: tunnelgate user <subcommand> [options]")
        fmt.Println("Subcommands: add, list, extend, lock, delete")
        return
    }
    cfg := config.Load(cfgPath)
    db := user.OpenDB(cfg.Database)
    sub := args[0]
    switch sub {
    case "add":
        if len(args) < 2 {
            fmt.Println("Usage: tunnelgate user add <username> --days <days> [--password <pass>]")
            return
        }
        username := args[1]
        days := 30
        password := ""
        // parse flags
        for i := 2; i < len(args); i++ {
            if args[i] == "--days" && i+1 < len(args) {
                fmt.Sscan(args[i+1], &days)
                i++
            }
            if args[i] == "--password" && i+1 < len(args) {
                password = args[i+1]
                i++
            }
        }
        u, err := user.AddUser(db, username, password, days)
        if err != nil {
            log.Fatal(err)
        }
        fmt.Printf("User %s added, expires %s\n", u.Username, u.Expiry)
    case "list":
        users, _ := user.ListUsers(db)
        fmt.Printf("%-20s %-15s %-6s\n", "Username", "Expiry", "Locked")
        for _, u := range users {
            fmt.Printf("%-20s %-15s %-6v\n", u.Username, u.Expiry, u.Locked)
        }
    default:
        fmt.Printf("Unknown user subcommand: %s\n", sub)
    }
}

func certCLI(args []string) {
    if len(args) < 1 {
        fmt.Println("Usage: tunnelgate cert renew")
        return
    }
    if args[0] == "renew" {
        cfg := config.Load(cfgPath)
        cert.Obtain(cfg)
        // Reload Nginx
        nginx.Reload()
        fmt.Println("Certificate renewed.")
    }
}

func ask(prompt, defaultVal string) string {
    fmt.Print(prompt)
    var input string
    fmt.Scanln(&input)
    if input == "" {
        return defaultVal
    }
    return input
}
