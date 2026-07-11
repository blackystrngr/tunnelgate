package config

import (
    "os"
    "gopkg.in/yaml.v3"
)

type Config struct {
    Domain       string `yaml:"domain"`
    Email        string `yaml:"email"`
    BackendHost  string `yaml:"backend_host"`
    BackendPort  int    `yaml:"backend_port"`

    Proxy struct {
        ListenHost    string `yaml:"listen_host"`
        ListenPort    int    `yaml:"listen_port"`
        IdleTimeout   int    `yaml:"idle_timeout_seconds"`
        SharedPass    string `yaml:"shared_pass"`
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

    CertMethod      string `yaml:"cert_method"`
    CFAPIToken      string `yaml:"cf_api_token"`
    CFEmail         string `yaml:"cf_email"`
    CFGlobalAPIKey  string `yaml:"cf_global_api_key"`

    Database string `yaml:"database"`
}

func Load(path string) *Config {
    data, err := os.ReadFile(path)
    if err != nil {
        // fallback to default
        return DefaultConfig()
    }
    var cfg Config
    yaml.Unmarshal(data, &cfg)
    return &cfg
}

func (c *Config) Save(path string) error {
    data, err := yaml.Marshal(c)
    if err != nil {
        return err
    }
    return os.WriteFile(path, data, 0600)
}

func DefaultConfig() *Config {
    return &Config{
        Domain: "tunnel.example.com",
        BackendHost: "127.0.0.1",
        BackendPort: 109,
        Database: "/var/lib/tunnelgate/users.db",
        Proxy: struct {
            ListenHost    string `yaml:"listen_host"`
            ListenPort    int    `yaml:"listen_port"`
            IdleTimeout   int    `yaml:"idle_timeout_seconds"`
            SharedPass    string `yaml:"shared_pass"`
        }{
            ListenHost: "127.0.0.1",
            ListenPort: 8888,
            IdleTimeout: 180,
        },
        API: struct {
            ListenHost string `yaml:"listen_host"`
            ListenPort int    `yaml:"listen_port"`
            Token      string `yaml:"token"`
        }{
            ListenHost: "127.0.0.1",
            ListenPort: 8080,
            Token: "change-me",
        },
        Nginx: struct {
            HTTPPorts []int `yaml:"http_ports"`
            TLSPorts  []int `yaml:"tls_ports"`
        }{
            HTTPPorts: []int{80},
            TLSPorts:  []int{443},
        },
        CertMethod: "le_http01",
    }
}
