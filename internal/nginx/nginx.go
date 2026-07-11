package nginx

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"

    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
)

const nginxConfDir = "/etc/nginx/sites-available"

func Configure(cfg *config.Config) error {
    domain := cfg.Domain
    certPath := "/etc/tunnelgate/certs/fullchain.pem"
    keyPath := "/etc/tunnelgate/certs/key.pem"

    var httpBlocks string
    for _, port := range cfg.Nginx.HTTPPorts {
        httpBlocks += fmt.Sprintf(`
server {
    listen %d;
    server_name %s;
    location /ssh-ws {
        proxy_pass http://%s:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_buffering off;
    }
}`, port, domain, cfg.Proxy.ListenHost, port)
    }

    var tlsBlocks string
    for _, port := range cfg.Nginx.TLSPorts {
        // Check if certificate exists
        if _, err := os.Stat(certPath); err != nil {
            logger.Warn("Certificate file missing, skipping TLS port", "port", port)
            continue
        }
        tlsBlocks += fmt.Sprintf(`
server {
    listen %d ssl http2;
    server_name %s;
    ssl_certificate %s;
    ssl_certificate_key %s;
    ssl_protocols TLSv1.2 TLSv1.3;
    location /ssh-ws {
        proxy_pass http://%s:%d;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
        proxy_buffering off;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}`, port, domain, certPath, keyPath,
            cfg.Proxy.ListenHost, port)
    }

    fullConfig := httpBlocks + "\n" + tlsBlocks
    confPath := filepath.Join(nginxConfDir, "tunnelgate.conf")
    if err := os.WriteFile(confPath, []byte(fullConfig), 0644); err != nil {
        return fmt.Errorf("write config: %w", err)
    }

    // Symlink
    symlinkPath := "/etc/nginx/sites-enabled/tunnelgate.conf"
    if err := os.Symlink(confPath, symlinkPath); err != nil && !os.IsExist(err) {
        return fmt.Errorf("symlink: %w", err)
    }

    // Test and reload
    if err := exec.Command("nginx", "-t").Run(); err != nil {
        return fmt.Errorf("nginx config test failed: %w", err)
    }
    if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
        return fmt.Errorf("nginx reload: %w", err)
    }
    return nil
}

func Reload() error {
    if err := exec.Command("nginx", "-t").Run(); err != nil {
        return fmt.Errorf("nginx test failed: %w", err)
    }
    if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
        return fmt.Errorf("nginx reload: %w", err)
    }
    return nil
}
