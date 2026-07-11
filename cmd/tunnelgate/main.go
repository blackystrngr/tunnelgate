package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/exec"
    "os/signal"
    "strconv"
    "syscall"
    "time"

    "golang.org/x/sync/errgroup"

    "github.com/tunnelgate/tunnelgate/internal/api"
    "github.com/tunnelgate/tunnelgate/internal/cert"
    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
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
    if len(args) > 0 {
        runCommand(args)
        return
    }

    if err := run(); err != nil {
        logger.Error("Fatal error", "error", err)
        os.Exit(1)
    }
}

func run() error {
    cfg, err := config.Load(cfgPath)
    if err != nil {
        return fmt.Errorf("load config: %w", err)
    }
    if err := cfg.Validate(); err != nil {
        return fmt.Errorf("invalid config: %w", err)
    }

    logger.Init(cfg.LogLevel, cfg.LogFormat)
    logger.Info("Starting TunnelGate", "domain", cfg.Domain)

    db, err := user.InitDB(cfg.Database)
    if err != nil {
        return fmt.Errorf("database init: %w", err)
    }
    defer db.Close()

    if len(cfg.Nginx.TLSPorts) > 0 {
        if err := cert.EnsureCertificate(cfg); err != nil {
            logger.Warn("Certificate setup failed", "error", err)
        }
    }

    if err := nginx.Configure(cfg); err != nil {
        logger.Warn("Nginx config generation failed", "error", err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g, ctx := errgroup.WithContext(ctx)

    // Start proxy (will listen on all ports)
    g.Go(func() error {
        return runWithRecovery(ctx, "proxy", func() error {
            return proxy.Start(ctx, cfg)
        })
    })

    g.Go(func() error {
        return runWithRecovery(ctx, "api", func() error {
            return api.Start(ctx, cfg)
        })
    })

    g.Go(func() error {
        return runWithRecovery(ctx, "metrics", func() error {
            return metrics.Start(ctx, cfg)
        })
    })

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    select {
    case <-sigChan:
        logger.Info("Received shutdown signal, stopping services...")
        cancel()
    case <-ctx.Done():
    }

    shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer shutdownCancel()

    if err := g.Wait(); err != nil && err != context.Canceled {
        logger.Error("Service exited with error", "error", err)
        return err
    }

    logger.Info("All services stopped gracefully")
    return nil
}

func runWithRecovery(ctx context.Context, name string, fn func() error) func() error {
    return func() error {
        defer func() {
            if r := recover(); r != nil {
                logger.Error("Panic recovered", "service", name, "panic", r)
            }
        }()
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            return fn()
        }
    }
}

// ---------------------------------------------------------------------
// subcommands
// ---------------------------------------------------------------------
func runCommand(args []string) {
    if len(args) == 0 {
        return
    }
    cmd := args[0]
    switch cmd {
    case "init":
        // interactive setup – stub (call external script)
        fmt.Println("Run 'sudo ./install.sh' for interactive setup.")
    case "start":
        if err := run(); err != nil {
            logger.Error("Start failed", "error", err)
            os.Exit(1)
        }
    case "stop":
        for _, svc := range []string{"proxy", "api"} {
            exec.Command("systemctl", "stop", "tunnelgate-"+svc+".service").Run()
        }
        fmt.Println("Services stopped.")
    case "status":
        printStatus()
    case "user":
        userCommand(args[1:])
    case "cert":
        certCommand(args[1:])
    case "port":
        portCommand(args[1:])
    case "upgrade":
        fmt.Println("Upgrade not implemented yet.")
    default:
        fmt.Printf("Unknown command: %s\n", cmd)
        os.Exit(1)
    }
}

func printStatus() {
    cfg, err := config.Load(cfgPath)
    if err != nil {
        logger.Error("Load config", "error", err)
        return
    }
    fmt.Printf("Domain:          %s\n", cfg.Domain)
    fmt.Printf("HTTP ports:      %v\n", cfg.Nginx.HTTPPorts)
    fmt.Printf("TLS ports:       %v\n", cfg.Nginx.TLSPorts)
    fmt.Printf("Backend:         %s:%d\n", cfg.BackendHost, cfg.BackendPort)
    db := user.OpenDB(cfg.Database)
    count, _ := user.CountUsers(db)
    fmt.Printf("Users:           %d\n", count)
}

// ---------------------------------------------------------------------
// user subcommands
// ---------------------------------------------------------------------
func userCommand(args []string) {
    if len(args) < 1 {
        fmt.Println("Usage: tunnelgate user <add|list|extend|lock|delete> ...")
        return
    }
    cfg, err := config.Load(cfgPath)
    if err != nil {
        logger.Error("Load config", "error", err)
        return
    }
    db := user.OpenDB(cfg.Database)

    sub := args[0]
    switch sub {
    case "add":
        if len(args) < 2 {
            fmt.Println("Usage: tunnelgate user add <username> [--days N] [--password P]")
            return
        }
        username := args[1]
        days := 30
        password := ""
        for i := 2; i < len(args); i++ {
            if args[i] == "--days" && i+1 < len(args) {
                days, _ = strconv.Atoi(args[i+1])
                i++
            }
            if args[i] == "--password" && i+1 < len(args) {
                password = args[i+1]
                i++
            }
        }
        u, err := user.AddUser(db, username, password, days)
        if err != nil {
            logger.Error("Add user failed", "error", err)
            return
        }
        fmt.Printf("User %s added, expires %s\n", u.Username, u.Expiry)
    case "list":
        users, err := user.ListUsers(db)
        if err != nil {
            logger.Error("List users failed", "error", err)
            return
        }
        fmt.Printf("%-20s %-15s %-6s\n", "Username", "Expiry", "Locked")
        for _, u := range users {
            fmt.Printf("%-20s %-15s %-6v\n", u.Username, u.Expiry.Format("2006-01-02"), u.Locked)
        }
    default:
        fmt.Printf("Unknown user subcommand: %s\n", sub)
    }
}

// ---------------------------------------------------------------------
// cert subcommands
// ---------------------------------------------------------------------
func certCommand(args []string) {
    if len(args) < 1 {
        fmt.Println("Usage: tunnelgate cert renew")
        return
    }
    if args[0] == "renew" {
        cfg, err := config.Load(cfgPath)
        if err != nil {
            logger.Error("Load config", "error", err)
            return
        }
        if err := cert.EnsureCertificate(cfg); err != nil {
            logger.Error("Cert renewal failed", "error", err)
        } else {
            nginx.Reload()
            fmt.Println("Certificate renewed.")
        }
    }
}

// ---------------------------------------------------------------------
// port subcommands
// ---------------------------------------------------------------------
func portCommand(args []string) {
    if len(args) < 2 {
        fmt.Println("Usage: tunnelgate port <add|remove> <port> [--tls]")
        return
    }
    action := args[1]
    cfg, err := config.Load(cfgPath)
    if err != nil {
        logger.Error("Load config", "error", err)
        return
    }

    switch action {
    case "add":
        if len(args) < 3 {
            fmt.Println("Usage: tunnelgate port add <port> [--tls]")
            return
        }
        port, _ := strconv.Atoi(args[2])
        if port == 0 {
            logger.Error("Invalid port number")
            return
        }
        tls := false
        for _, a := range args {
            if a == "--tls" {
                tls = true
                break
            }
        }
        if tls {
            cfg.Nginx.TLSPorts = append(cfg.Nginx.TLSPorts, port)
        } else {
            cfg.Nginx.HTTPPorts = append(cfg.Nginx.HTTPPorts, port)
        }
        if err := cfg.Save(cfgPath); err != nil {
            logger.Error("Save config", "error", err)
            return
        }
        // regenerate nginx
        if err := nginx.Configure(cfg); err != nil {
            logger.Error("Nginx config update failed", "error", err)
            return
        }
        // restart proxy service
        exec.Command("systemctl", "restart", "tunnelgate-proxy.service").Run()
        // open firewall
        exec.Command("iptables", "-A", "INPUT", "-p", "tcp", "--dport", strconv.Itoa(port), "-j", "ACCEPT").Run()
        exec.Command("netfilter-persistent", "save").Run()
        fmt.Printf("Port %d added (%s)\n", port, map[bool]string{true: "TLS", false: "HTTP"}[tls])

    case "remove":
        if len(args) < 3 {
            fmt.Println("Usage: tunnelgate port remove <port>")
            return
        }
        port, _ := strconv.Atoi(args[2])
        if port == 0 {
            logger.Error("Invalid port number")
            return
        }
        // remove from slices
        newHTTP := []int{}
        for _, p := range cfg.Nginx.HTTPPorts {
            if p != port {
                newHTTP = append(newHTTP, p)
            }
        }
        cfg.Nginx.HTTPPorts = newHTTP
        newTLS := []int{}
        for _, p := range cfg.Nginx.TLSPorts {
            if p != port {
                newTLS = append(newTLS, p)
            }
        }
        cfg.Nginx.TLSPorts = newTLS
        if err := cfg.Save(cfgPath); err != nil {
            logger.Error("Save config", "error", err)
            return
        }
        nginx.Configure(cfg)
        exec.Command("systemctl", "restart", "tunnelgate-proxy.service").Run()
        fmt.Printf("Port %d removed.\n", port)

    default:
        fmt.Printf("Unknown port action: %s\n", action)
    }
}
