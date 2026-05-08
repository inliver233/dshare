package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                    string
	DatabasePath            string
	AppBaseURL              string
	DiscordRedirectURL      string
	CookieSecure            bool
	DiscordClientID         string
	DiscordClientSecret     string
	NewAPIBaseURL           string
	NewAPIKey               string
	DS2APIBaseURL           string
	DS2APIAdminKey          string
	DS2APIAutoProxy         DS2APIAutoProxyConfig
	DS2APIValidateWorkers   int
	AdminUsername           string
	AdminPassword           string
	AdminDiscordIDs         map[string]bool
	SessionDays             int
	StaticDir               string
	DefaultRequestsPerMin   int
	DefaultRequestsPerDay   int
	DefaultMaxConcurrent    int
	HTTPClientTimeout       time.Duration
	StreamKeepAliveAfter    time.Duration
	StreamKeepAliveInterval time.Duration
	TrustedProxyHeaderName  string
}

type DS2APIAutoProxyConfig struct {
	Enabled          bool
	Type             string
	Host             string
	Port             int
	UsernameTemplate string
	Password         string
	NameTemplate     string
}

func Load() Config {
	cfg := Config{
		Addr:                env("ADDR", ":8080"),
		DatabasePath:        env("DATABASE_PATH", "/data/dshare.db"),
		AppBaseURL:          strings.TrimRight(env("APP_BASE_URL", "http://localhost:8080"), "/"),
		DiscordRedirectURL:  env("DISCORD_REDIRECT_URL", ""),
		CookieSecure:        envBool("COOKIE_SECURE", false),
		DiscordClientID:     env("DISCORD_CLIENT_ID", ""),
		DiscordClientSecret: env("DISCORD_CLIENT_SECRET", ""),
		NewAPIBaseURL:       strings.TrimRight(env("NEW_API_BASE_URL", ""), "/"),
		NewAPIKey:           env("NEW_API_KEY", ""),
		DS2APIBaseURL:       strings.TrimRight(env("DS2API_BASE_URL", ""), "/"),
		DS2APIAdminKey:      env("DS2API_ADMIN_KEY", ""),
		DS2APIAutoProxy: DS2APIAutoProxyConfig{
			Enabled:          envBool("DS2API_AUTO_PROXY_ENABLED", true),
			Type:             env("DS2API_AUTO_PROXY_TYPE", "socks5"),
			Host:             env("DS2API_AUTO_PROXY_HOST", "172.20.0.1"),
			Port:             envInt("DS2API_AUTO_PROXY_PORT", 21345),
			UsernameTemplate: env("DS2API_AUTO_PROXY_USERNAME_TEMPLATE", "Default.{local}"),
			Password:         env("DS2API_AUTO_PROXY_PASSWORD", ""),
			NameTemplate:     env("DS2API_AUTO_PROXY_NAME_TEMPLATE", "resin-{local}"),
		},
		DS2APIValidateWorkers:   envInt("DS2API_VALIDATE_WORKERS", 3),
		AdminUsername:           env("ADMIN_USERNAME", "admin"),
		AdminPassword:           env("ADMIN_PASSWORD", "admin"),
		AdminDiscordIDs:         envSet("ADMIN_DISCORD_IDS"),
		SessionDays:             envInt("SESSION_DAYS", 30),
		StaticDir:               env("STATIC_DIR", "./web/dist"),
		DefaultRequestsPerMin:   envInt("DEFAULT_REQUESTS_PER_MINUTE", 5),
		DefaultRequestsPerDay:   envInt("DEFAULT_REQUESTS_PER_DAY", 2500),
		DefaultMaxConcurrent:    envInt("DEFAULT_MAX_CONCURRENT", 4),
		HTTPClientTimeout:       time.Duration(envInt("UPSTREAM_TIMEOUT_SECONDS", 300)) * time.Second,
		StreamKeepAliveAfter:    time.Duration(envInt("STREAM_KEEPALIVE_AFTER_SECONDS", 20)) * time.Second,
		StreamKeepAliveInterval: time.Duration(envInt("STREAM_KEEPALIVE_INTERVAL_SECONDS", 10)) * time.Second,
		TrustedProxyHeaderName: env(
			"TRUSTED_PROXY_HEADER",
			"X-Forwarded-For",
		),
	}
	cfg.DS2APIAutoProxy = NormalizeDS2APIAutoProxy(cfg.DS2APIAutoProxy)
	if cfg.DS2APIValidateWorkers < 1 {
		cfg.DS2APIValidateWorkers = 1
	}
	if cfg.DS2APIValidateWorkers > 20 {
		cfg.DS2APIValidateWorkers = 20
	}
	if cfg.AdminUsername == "admin" || cfg.AdminPassword == "admin" {
		log.Println("warning: ADMIN_USERNAME or ADMIN_PASSWORD is using the default value; change it before production use")
	}
	if cfg.DiscordRedirectURL == "" && cfg.AppBaseURL != "" {
		cfg.DiscordRedirectURL = cfg.AppBaseURL + "/api/auth/discord/callback"
	}
	return cfg
}

func NormalizeDS2APIAutoProxy(in DS2APIAutoProxyConfig) DS2APIAutoProxyConfig {
	out := DS2APIAutoProxyConfig{
		Enabled:          in.Enabled,
		Type:             strings.ToLower(strings.TrimSpace(in.Type)),
		Host:             strings.TrimSpace(in.Host),
		Port:             in.Port,
		UsernameTemplate: strings.TrimSpace(in.UsernameTemplate),
		Password:         strings.TrimSpace(in.Password),
		NameTemplate:     strings.TrimSpace(in.NameTemplate),
	}
	if out.Type == "" {
		out.Type = "socks5"
	}
	if out.UsernameTemplate == "" {
		out.UsernameTemplate = "Default.{local}"
	}
	if out.NameTemplate == "" {
		out.NameTemplate = "resin-{local}"
	}
	return out
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return fallback
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envSet(key string) map[string]bool {
	out := map[string]bool{}
	for _, raw := range strings.Split(os.Getenv(key), ",") {
		item := strings.TrimSpace(raw)
		if item != "" {
			out[item] = true
		}
	}
	return out
}
