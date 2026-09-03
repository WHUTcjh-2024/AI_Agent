package console

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr         string
	APIBaseURL   string
	APIToken     string
	Password     string
	SecureCookie bool
}

func LoadConfig() (Config, error) {
	cfg := Config{Addr: "127.0.0.1:18090", APIBaseURL: "http://127.0.0.1:18080", SecureCookie: true,
		APIToken: os.Getenv("ASKU_ADMIN_API_TOKEN"), Password: os.Getenv("ASKU_ADMIN_PASSWORD")}
	if value := os.Getenv("ASKU_ADMIN_ADDR"); value != "" {
		cfg.Addr = value
	}
	if value := os.Getenv("ASKU_ADMIN_API_BASE_URL"); value != "" {
		cfg.APIBaseURL = value
	}
	if value := os.Getenv("ASKU_ADMIN_SECURE_COOKIE"); value != "" {
		secure, err := strconv.ParseBool(value)
		if err != nil {
			return cfg, fmt.Errorf("ASKU_ADMIN_SECURE_COOKIE must be a boolean")
		}
		cfg.SecureCookie = secure
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	u, err := url.Parse(c.APIBaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("ASKU_ADMIN_API_BASE_URL must be an HTTP(S) origin without credentials, path, query or fragment")
	}
	if strings.TrimSpace(c.APIToken) == "" || strings.ContainsAny(c.APIToken, "\r\n") {
		return fmt.Errorf("ASKU_ADMIN_API_TOKEN is required")
	}
	if len(c.Password) < 12 {
		return fmt.Errorf("ASKU_ADMIN_PASSWORD must contain at least 12 bytes")
	}
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return fmt.Errorf("ASKU_ADMIN_ADDR must be host:port")
	}
	if !c.SecureCookie {
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("insecure cookies are only allowed on a loopback listener")
		}
	}
	return nil
}
