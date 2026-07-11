package cert

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "time"

    "github.com/tunnelgate/tunnelgate/internal/config"
    "github.com/tunnelgate/tunnelgate/internal/logger"
)

const certDir = "/etc/tunnelgate/certs"

// EnsureCertificate checks if a valid certificate exists; if not, obtains one.
func EnsureCertificate(cfg *config.Config) error {
    if valid, err := isCertValid(cfg.Domain); err == nil && valid {
        logger.Info("Certificate already valid", "domain", cfg.Domain)
        return nil
    }
    logger.Info("Obtaining certificate", "method", cfg.CertMethod, "domain", cfg.Domain)
    var lastErr error
    for attempt := 1; attempt <= 3; attempt++ {
        err := Obtain(cfg)
        if err == nil {
            return nil
        }
        lastErr = err
        logger.Warn("Certificate issuance attempt failed",
            "attempt", attempt, "error", err)
        time.Sleep(time.Duration(attempt*5) * time.Second)
    }
    return fmt.Errorf("certificate issuance failed after 3 attempts: %w", lastErr)
}

func isCertValid(domain string) (bool, error) {
    certPath := filepath.Join(certDir, "fullchain.pem")
    if _, err := os.Stat(certPath); err != nil {
        return false, err
    }
    // Check if expiry is more than 24h away
    cmd := exec.Command("openssl", "x509", "-in", certPath, "-noout", "-checkend", "86400")
    if err := cmd.Run(); err != nil {
        return false, err
    }
    return true, nil
}

// Obtain requests a new certificate (does NOT check for existing).
// It's called by EnsureCertificate only when needed.
func Obtain(cfg *config.Config) error {
    os.MkdirAll(certDir, 0700)

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
    exec.Command("systemctl", "stop", "nginx").Run()
    defer exec.Command("systemctl", "start", "nginx").Run()

    cmd := exec.Command("acme.sh", "--issue", "--standalone", "-d", cfg.Domain, "--server", "letsencrypt")
    if cfg.Email != "" {
        cmd.Args = append(cmd.Args, "--email", cfg.Email)
    }
    cmd.Env = append(os.Environ(), "HOME=/root")
    output, err := cmd.CombinedOutput()
    if err != nil {
        return fmt.Errorf("acme.sh failed: %w\n%s", err, output)
    }
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
    if err := cmd.Run(); err != nil {
        return err
    }
    installCmd := exec.Command("acme.sh", "--install-cert", "-d", cfg.Domain,
        "--cert-file", filepath.Join(certDir, "cert.pem"),
        "--key-file", filepath.Join(certDir, "key.pem"),
        "--fullchain-file", filepath.Join(certDir, "fullchain.pem"))
    installCmd.Env = env
    return installCmd.Run()
}

func obtainCloudflareOrigin(cfg *config.Config) error {
    // Stub – implement later
    return fmt.Errorf("Cloudflare Origin CA not implemented")
}
