package config

import (
    "encoding/json"
    "fmt"
    "os"
)

type Config struct {
    Domain      string `json:"domain"`
    Email       string `json:"email"`
    BackendHost string `json:"backend_host"`
    BackendPort int    `json:"backend_port"`

    Proxy struct {
        ListenHost  string `json:"listen_host"`
        ListenPort  int    `json:"listen_port"`
        IdleTimeout int    `json:"idle_timeout_seconds"`
        SharedPass  string `json:"shared_pass"`
    } `json:"proxy"`

    API struct {
        ListenHost string `json:"listen_host"`
        ListenPort int    `json:"listen_port"`
        Token      string `json:"token"`
    } `json:"api"`

    Nginx struct {
        HTTPPorts []int `json:"http_ports"`
        TLSPorts  []int `json:"tls_ports"`
    } `json:"nginx"`

    CertMethod       string `json:"cert_method"`
    CFAPIToken       string `json:"cf_api_token"`
    CFEmail          string `json:"cf_email"`
    CFGlobalAPIKey   string `json:"cf_global_api_key"`

    Database   string `json:"database"`
    LogLevel   string `json:"log_level"`
    LogFormat  string `json:"log_format"`
}

func Load(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("read config: %w", err)
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parse config: %w", err)
    }
    return &cfg, nil
}

func (c *Config) Save(path string) error {
    data, err := json.MarshalIndent(c, "", "  ")
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
    if len(c.Nginx.HTTPPorts) == 0 && len(c.Nginx.TLSPorts) == 0 {
        return fmt.Errorf("at least one HTTP or TLS port must be configured")
    }
    return nil
}
