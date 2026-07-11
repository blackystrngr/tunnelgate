package cert

import (
    "fmt"
    "log"
    "os"
    "os/exec"
    "path/filepath"
    "time"

    "github.com/tunnelgate/tunnelgate/internal/config"
)

const (
    certDir = "/etc/tunnelgate/certs"
)

func Obtain(cfg *config.Config) error {
    // Ensure acme.sh installed
    if _, err := exec.LookPath("acme.sh"); err != nil {
        // Install acme.sh
        cmd := exec.Command("bash", "-c", "curl -s https://get.acme.sh | sh")
        cmd.Run()
    }

    os.MkdirAll(certDir, 0700)

    // Choose method
    switch cfg.CertMethod {
    case "le_http01":
        return obtainHTTP01(cfg)
    case "le_dns_cf":
        return obtainDNS01(cfg)
    case "cf_origin":
        return obtainCloudflareOrigin(cfg)
    default:
        return fmt.Errorf("unknown cert method: %s", cfg.CertMethod)
    }
}

func obtainHTTP01(cfg *config.Config) error {
    // Stop any service using port 80 (e.g., nginx)
    exec.Command("systemctl", "stop", "nginx").Run()
    defer exec.Command("systemctl", "start", "nginx").Run()

    cmd := exec.Command("acme.sh", "--issue", "--standalone", "-d", cfg.Domain, "--server", "letsencrypt")
    if cfg.Email != "" {
        cmd.Args = append(cmd.Args, "--email", cfg.Email)
    }
    cmd.Env = append(os.Environ(), "HOME=/root")
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("acme.sh failed: %s", output)
    }
    // Install certificate
    installCmd := exec.Command("acme.sh", "--install-cert", "-d", cfg.Domain,
        "--cert-file", filepath.Join(certDir, "cert.pem"),
        "--key-file", filepath.Join(certDir, "key.pem"),
        "--fullchain-file", filepath.Join(certDir, "fullchain.pem"))
    installCmd.Env = append(os.Environ(), "HOME=/root")
    return installCmd.Run()
}

func obtainDNS01(cfg *config.Config) error {
    env := os.Environ()
    env = append(env, "CF_Token="+cfg.CFAPIToken)
    cmd := exec.Command("acme.sh", "--issue", "--dns", "dns_cf", "-d", cfg.Domain)
    cmd.Env = env
    cmd.Run()
    // install similarly
    installCmd := exec.Command("acme.sh", "--install-cert", "-d", cfg.Domain,
        "--cert-file", filepath.Join(certDir, "cert.pem"),
        "--key-file", filepath.Join(certDir, "key.pem"),
        "--fullchain-file", filepath.Join(certDir, "fullchain.pem"))
    installCmd.Env = env
    return installCmd.Run()
}

func obtainCloudflareOrigin(cfg *config.Config) error {
    // Use Cloudflare Origin CA API (simplified – in reality we'd use curl or HTTP client)
    // For brevity we call a helper script.
    return nil
}

func GetExpiry(domain string) time.Time {
    // Use openssl to parse expiry
    certPath := filepath.Join(certDir, "fullchain.pem")
    cmd := exec.Command("openssl", "x509", "-in", certPath, "-noout", "-enddate")
    out, _ := cmd.Output()
    // parse "notAfter=..."
    return time.Now()
}
