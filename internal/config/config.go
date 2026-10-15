// Package config собирает настройки из окружения.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — настройки приложения.
type Config struct {
	Addr          string
	DBPath        string
	Admins        []string
	SessionTTL    time.Duration
	SecureCookies bool
	BaseURL       string
	MaxPlans      int
}

// Load читает настройки из переменных окружения с разумными значениями по умолчанию.
func Load() Config {
	c := Config{
		Addr:          env("KULTYASKA_ADDR", "127.0.0.1:8080"),
		DBPath:        env("KULTYASKA_DB", "data/kultyaska.db"),
		Admins:        splitList(env("KULTYASKA_ADMINS", "")),
		SessionTTL:    time.Duration(envInt("KULTYASKA_SESSION_DAYS", 30)) * 24 * time.Hour,
		SecureCookies: envBool("KULTYASKA_SECURE_COOKIES", false),
		BaseURL:       env("KULTYASKA_BASE_URL", ""),
		MaxPlans:      envInt("KULTYASKA_MAX_PLANS", 200),
	}
	return c
}

// IsAdmin сообщает, входит ли ник в список администраторов.
func (c Config) IsAdmin(nick string) bool {
	for _, a := range c.Admins {
		if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(nick)) {
			return true
		}
	}
	return false
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(env(k, "")); err == nil {
		return v
	}
	return def
}

func envBool(k string, def bool) bool {
	v := strings.ToLower(env(k, ""))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
