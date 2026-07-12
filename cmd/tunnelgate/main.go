package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "os/signal"
    "sync"
    "syscall"
    "time"

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
    flag.StringVar(&cfgPath, "config", "/etc/tunnelgate/config.json", "path to config file")
}

func main() {
    flag.Parse()
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

    var wg sync.WaitGroup
    errChan := make(chan error, 3)

    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := proxy.Start(ctx, cfg); err != nil {
            errChan <- err
        }
    }()

    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := api.Start(ctx, cfg); err != nil {
            errChan <- err
        }
    }()

    wg.Add(1)
    go func() {
        defer wg.Done()
        if err := metrics.Start(ctx, cfg); err != nil {
            errChan <- err
        }
    }()

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    select {
    case <-sigChan:
        logger.Info("Received shutdown signal")
        cancel()
    case err := <-errChan:
        logger.Error("Service error", "error", err)
        cancel()
    }

    // Wait for goroutines to finish
    done := make(chan struct{})
    go func() {
        wg.Wait()
        close(done)
    }()

    select {
    case <-done:
        logger.Info("All services stopped")
    case <-time.After(5 * time.Second):
        logger.Warn("Shutdown timeout, forcing exit")
    }

    return nil
}
