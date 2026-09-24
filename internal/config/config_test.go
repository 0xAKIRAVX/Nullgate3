package config

import (
	"net/url"
	"os"
	"testing"
)

// TestDatabaseURL_PGFallback verifies the zero-config DSN assembly from
// libpq-style PG* variables (providers that inject only those).
func TestDatabaseURL_PGFallback(t *testing.T) {
	env := map[string]string{
		"PGHOST":     "postgres.railway.internal",
		"PGPORT":     "6543",
		"PGUSER":     "ng",
		"PGPASSWORD": "p@ss word:x",
		"PGDATABASE": "nullgate",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	t.Setenv("DATABASE_URL", "")

	got := databaseURL()
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if u.Scheme != "postgres" {
		t.Fatalf("scheme = %q", u.Scheme)
	}
	if u.Host != "postgres.railway.internal:6543" {
		t.Fatalf("host = %q", u.Host)
	}
	if u.Path != "/nullgate" {
		t.Fatalf("path = %q", u.Path)
	}
	if pw, _ := u.User.Password(); pw != "p@ss word:x" {
		t.Fatalf("password not round-tripped: %q", pw)
	}
	if u.Query().Get("sslmode") != "disable" {
		t.Fatalf("sslmode = %q", u.Query().Get("sslmode"))
	}
}

// TestDatabaseURL_ExplicitWins: DATABASE_URL always takes priority over PG*.
func TestDatabaseURL_ExplicitWins(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://a:b@h:1/db")
	t.Setenv("PGHOST", "ignored")
	if got := databaseURL(); got != "postgres://a:b@h:1/db" {
		t.Fatalf("got %q", got)
	}
}

// TestDatabaseURL_Empty: no DATABASE_URL and no PGHOST → empty string
// (main.go turns that into the actionable "Connect the postgres service" hint).
func TestDatabaseURL_Empty(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGHOST", "")
	if got := databaseURL(); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

// TestLoad_NoSecret: SECRET was removed (dead config); Load must not panic
// and defaults must stay sane when nothing is set.
func TestLoad_NoSecret(t *testing.T) {
	for _, k := range []string{"PORT", "DATABASE_URL", "REDIS_URL", "SECRET",
		"PGHOST", "PGPORT", "PGUSER", "PGPASSWORD", "PGDATABASE", "PGSSLMODE"} {
		os.Unsetenv(k)
	}
	c := Load()
	if c.HTTPPort != "8080" || c.RealitySNI != "www.samsung.com" || c.AppPort != 9000 {
		t.Fatalf("defaults changed: %+v", c)
	}
}
