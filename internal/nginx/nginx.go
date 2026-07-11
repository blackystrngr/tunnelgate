package nginx

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"

    "github.com/tunnelgate/tunnelgate/internal/config"
)

const nginxConfDir = "/etc/nginx/sites-available"

func Configure(cfg *config.Config) error {
    domain := cfg.Domain
    certPath := "/etc/tunnelgate/certs/fullchain.pem"
    keyPath := "/etc/tunnelgate/certs/key.pem"

    // Build server blocks
    httpBlock := ""
    tlsBlock := ""

    // Port 80 (plain HTTP)
    if len(cfg.Nginx.HTTPPorts) > 0 {
        httpBlock = fmt.Sprintf(`
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
}`, cfg.Nginx.HTTPPorts[0], domain, cfg.Proxy.ListenHost, cfg.Proxy.ListenPort)
    }

    // Port 443 (TLS)
    if len(cfg.Nginx.TLSPorts) > 0 {
        tlsBlock = fmt.Sprintf(`
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
}`, cfg.Nginx.TLSPorts[0], domain, certPath, keyPath,
    cfg.Proxy.ListenHost, cfg.Proxy.ListenPort)
    }

    fullConfig := httpBlock + "\n" + tlsBlock
    confPath := filepath.Join(nginxConfDir, "tunnelgate.conf")
    if err := os.WriteFile(confPath, []byte(fullConfig), 0644); err != nil {
        return err
    }

    // Enable site
    exec.Command("ln", "-sf", confPath, "/etc/nginx/sites-enabled/").Run()
    // Test and reload
    exec.Command("nginx", "-t").Run()
    exec.Command("systemctl", "reload", "nginx").Run()
    return nil
}

func Reload() {
    exec.Command("nginx", "-t").Run()
    exec.Command("systemctl", "reload", "nginx").Run()
}
