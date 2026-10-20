package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// Addr is the listen address of the API server.
	Addr string
	// CORSOrigins lists origins allowed to call the API from a browser
	// (needed when the dashboard is served from a different origin).
	CORSOrigins []string
	// MongoURI is the MongoDB connection string. When empty the server falls back
	// to an in-memory store (development only, data is lost on restart).
	MongoURI string
	// MongoDB is the database name inside MongoDB.
	MongoDB string
	// JWTSecret signs access tokens. When empty a random one is generated at startup,
	// which invalidates all access tokens on every restart (development only).
	JWTSecret string
	// AdminUsername and AdminPassword seed the first administrator when no user exists.
	// An empty password makes the server generate one and print it once.
	AdminUsername string
	AdminPassword string
	// CookieSecure marks the refresh cookie as HTTPS-only; enable it behind HTTPS.
	CookieSecure bool
	// TrustedProxies lists reverse proxies (CIDR blocks or addresses) whose X-Forwarded-For header is
	// believed when working out the client address. Leave empty if clients connect directly.
	TrustedProxies []string
	// LoginPerMinute and RefreshPerMinute limit sign-in and token-refresh attempts per client address.
	// Zero turns the limit off.
	LoginPerMinute, RefreshPerMinute int
}

const (
	defaultLoginPerMinute   = 10
	defaultRefreshPerMinute = 60
)

// Load reads configuration from FLUXOR_* environment variables.
func Load() Config {
	secure, _ := strconv.ParseBool(os.Getenv("FLUXOR_COOKIE_SECURE"))
	return Config{
		Addr:             getenv("FLUXOR_ADDR", ":8080"),
		CORSOrigins:      splitList(os.Getenv("FLUXOR_CORS_ORIGINS")),
		MongoURI:         os.Getenv("FLUXOR_MONGO_URI"),
		MongoDB:          getenv("FLUXOR_MONGO_DB", "fluxor"),
		JWTSecret:        os.Getenv("FLUXOR_JWT_SECRET"),
		AdminUsername:    os.Getenv("FLUXOR_ADMIN_USERNAME"),
		AdminPassword:    os.Getenv("FLUXOR_ADMIN_PASSWORD"),
		CookieSecure:     secure,
		TrustedProxies:   splitList(os.Getenv("FLUXOR_TRUSTED_PROXIES")),
		LoginPerMinute:   getint("FLUXOR_RATE_LOGIN_PER_MIN", defaultLoginPerMinute),
		RefreshPerMinute: getint("FLUXOR_RATE_REFRESH_PER_MIN", defaultRefreshPerMinute),
	}
}

// getint reads a non-negative integer; anything else (unset, empty, malformed, negative) gives def.
func getint(key string, def int) int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && n >= 0 {
		return n
	}
	return def
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
