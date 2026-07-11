package config

import (
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

type Config struct {
    Domain      string `yaml:"domain"`
    Email       string `yaml:"email"`
    BackendHost string `yaml:"backend_host"`
    BackendPort int    `yaml:"backend_port"`

    Proxy struct {
        ListenHost  string `yaml:"listen_host"`
        ListenPort  int    `yaml:"listen_port"`
        IdleTimeout int    `yaml:"idle_timeout_seconds"`
        SharedPass  string `yaml:"shared_pass"`
    } `yaml:"proxy"`

    API struct {
        ListenHost string `yaml:"listen_host"`
        ListenPort int    `yaml:"listen_port"`
        Token      string `yaml:"token"`
    } `yaml:"api"`

    Nginx struct {
        HTTPPorts []int `yaml:"http_ports"`
        TLSPorts  []int `yaml:"tls_ports"`
    } `yaml:"nginx"`

    CertMethod       string `yaml:"cert_method"`
    CFAPIToken       string `yaml:"cf_api_token"`
    CFEmail          string `yaml:"cf_email"`
    CFGlobalAPIKey   string `yaml:"cf_global_api_key"`

    Database string `yaml:"database"`

    // Logging
    LogLevel  string `yaml:"log_level"`
    LogFormat string `yaml:"log_format"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    return &cfg, nil
}

func (c *Config) Save(path string) error {
    data, err := yaml.Marshal(c)
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0600)
}

func (c *Config) Validate() error {
    if c.Domain == "" {
        return fmt.Errorf("domain is required")
    }
    if c.BackendPort <= 0 || c.BackendPort > 65535 {
        return fmt.Errorf("invalid backend_port: %d", c.BackendPort)
    }
    if c.Proxy.ListenPort <= 0 || c.Proxy.ListenPort > 65535 {
        return fmt.Errorf("invalid proxy.listen_port: %d", c.Proxy.ListenPort)
    }
    if len(c.Nginx.HTTPPorts) == 0 && len(c.Nginx.TLSPorts) == 0 {
        return fmt.Errorf("at least one HTTP or TLS port must be configured")
    }
    validMethods := map[string]bool{
        "le_http01": true,
        "le_dns_cf": true,
        "cf_origin": true,
        "selfsigned": true,
    }
    if !validMethods[c.CertMethod] {
        return fmt.Errorf("invalid cert_method: %s", c.CertMethod)
    }
    if c.CertMethod == "le_dns_cf" && c.CFAPIToken == "" {
        return fmt.Errorf("cf_api_token required for le_dns_cf")
    }
    if c.CertMethod == "cf_origin" && (c.CFEmail == "" || c.CFGlobalAPIKey == "") {
        return fmt.Errorf("cf_email and cf_global_api_key required for cf_origin")
    }
    return nil
}
