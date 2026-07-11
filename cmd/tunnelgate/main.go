package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/signal"
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

    // Initialize database
    db, err := user.InitDB(cfg.Database)
    if err != nil {
        return fmt.Errorf("database init: %w", err)
    }
    defer db.Close()

    // Ensure certificate exists (if TLS ports)
    if len(cfg.Nginx.TLSPorts) > 0 {
        if err := cert.EnsureCertificate(cfg); err != nil {
            logger.Warn("Certificate setup failed", "error", err)
        }
    }

    // Generate Nginx config
    if err := nginx.Configure(cfg); err != nil {
        logger.Warn("Nginx config generation failed", "error", err)
    }

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    g, ctx := errgroup.WithContext(ctx)

    // Proxy core
    g.Go(func() error {
        return runWithRecovery(ctx, "proxy", func() error {
            return proxy.Start(ctx, cfg)
        })
    })

    // API server
    g.Go(func() error {
        return runWithRecovery(ctx, "api", func() error {
            return api.Start(ctx, cfg)
        })
    })

    // Metrics server
    g.Go(func() error {
        return runWithRecovery(ctx, "metrics", func() error {
            return metrics.Start(ctx, cfg)
        })
    })

    // Signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    select {
    case <-sigChan:
        logger.Info("Received shutdown signal, stopping services...")
        cancel()
    case <-ctx.Done():
        // context cancelled by error in one of the goroutines
    }

    // Give services a chance to clean up
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

// subcommand dispatcher (init, start, stop, status, user, cert, upgrade)
func runCommand(args []string) {
    if len(args) == 0 {
        return
    }
    cmd := args[0]
    switch cmd {
    case "init":
        // stub – would call interactive setup
        fmt.Println("init command not yet implemented")
    case "start":
        if err := run(); err != nil {
            logger.Error("Start failed", "error", err)
            os.Exit(1)
        }
    case "stop":
        // stop systemd services
        fmt.Println("stopping services...")
    case "status":
        fmt.Println("status command")
    case "user":
        // user management
        fmt.Println("user command")
    case "cert":
        // certificate renewal
        fmt.Println("cert command")
    case "upgrade":
        fmt.Println("upgrade command")
    default:
        fmt.Printf("Unknown command: %s\n", cmd)
        os.Exit(1)
    }
}
