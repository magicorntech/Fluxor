package config

import (
	"reflect"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	for _, k := range []string{
		"FLUXOR_ADDR", "FLUXOR_CORS_ORIGINS", "FLUXOR_MONGO_URI", "FLUXOR_MONGO_DB",
		"FLUXOR_JWT_SECRET", "FLUXOR_ADMIN_USERNAME", "FLUXOR_ADMIN_PASSWORD", "FLUXOR_COOKIE_SECURE",
		"FLUXOR_TRUSTED_PROXIES", "FLUXOR_RATE_LOGIN_PER_MIN", "FLUXOR_RATE_REFRESH_PER_MIN",
	} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.Addr != ":8080" || c.MongoDB != "fluxor" || c.MongoURI != "" || len(c.CORSOrigins) != 0 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.JWTSecret != "" || c.AdminUsername != "" || c.AdminPassword != "" || c.CookieSecure {
		t.Fatalf("auth settings must default to empty/off: %+v", c)
	}
	if c.LoginPerMinute != 10 || c.RefreshPerMinute != 60 || len(c.TrustedProxies) != 0 {
		t.Fatalf("rate limits default to 10 and 60 per minute, no trusted proxies: %+v", c)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("FLUXOR_ADDR", ":9000")
	t.Setenv("FLUXOR_CORS_ORIGINS", " http://a , http://b,, ")
	t.Setenv("FLUXOR_MONGO_URI", "mongodb://db:27017")
	t.Setenv("FLUXOR_MONGO_DB", "custom")
	t.Setenv("FLUXOR_JWT_SECRET", "s3cret")
	t.Setenv("FLUXOR_ADMIN_USERNAME", "root")
	t.Setenv("FLUXOR_ADMIN_PASSWORD", "pw")
	t.Setenv("FLUXOR_COOKIE_SECURE", "true")
	t.Setenv("FLUXOR_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.1.5")
	t.Setenv("FLUXOR_RATE_LOGIN_PER_MIN", "25")
	t.Setenv("FLUXOR_RATE_REFRESH_PER_MIN", "0")
	c := Load()
	if c.Addr != ":9000" || c.MongoURI != "mongodb://db:27017" || c.MongoDB != "custom" {
		t.Fatalf("unexpected config: %+v", c)
	}
	if c.JWTSecret != "s3cret" || c.AdminUsername != "root" || c.AdminPassword != "pw" || !c.CookieSecure {
		t.Fatalf("unexpected auth config: %+v", c)
	}
	if want := []string{"http://a", "http://b"}; !reflect.DeepEqual(c.CORSOrigins, want) {
		t.Fatalf("CORSOrigins = %v, want %v", c.CORSOrigins, want)
	}
	if want := []string{"10.0.0.0/8", "192.168.1.5"}; !reflect.DeepEqual(c.TrustedProxies, want) {
		t.Fatalf("TrustedProxies = %v, want %v", c.TrustedProxies, want)
	}
	if c.LoginPerMinute != 25 || c.RefreshPerMinute != 0 {
		t.Fatalf("rate limits: %d and %d; an explicit 0 turns a limit off", c.LoginPerMinute, c.RefreshPerMinute)
	}
}

func TestRateLimitSettingsIgnoreGarbage(t *testing.T) {
	for _, v := range []string{"abc", "-3", "", " 7x"} {
		t.Setenv("FLUXOR_RATE_LOGIN_PER_MIN", v)
		if got := Load().LoginPerMinute; got != 10 {
			t.Errorf("%q: got %d, want the default 10", v, got)
		}
	}
	t.Setenv("FLUXOR_RATE_LOGIN_PER_MIN", " 7 ")
	if got := Load().LoginPerMinute; got != 7 {
		t.Errorf("surrounding spaces are fine: got %d", got)
	}
}
