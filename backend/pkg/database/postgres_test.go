package database

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"novel-agent/internal/config"
)

func TestBuildPostgresDSNEncodesCredentialsAndParameters(t *testing.T) {
	cfg := config.Default().Database
	cfg.Host = "2001:db8::1"
	cfg.Port = 5432
	cfg.User = "user name"
	cfg.Password = `p@ss word'\\secret`
	cfg.DBName = "novel/name"
	cfg.SSLMode = "require"
	cfg.SearchPath = "novel_agent,public"
	dsn := buildPostgresDSN(&cfg)
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	password, ok := parsed.User.Password()
	if !ok || parsed.User.Username() != cfg.User || password != cfg.Password {
		t.Fatalf("credentials did not round trip safely: %q", dsn)
	}
	if parsed.Hostname() != cfg.Host || parsed.Port() != "5432" || parsed.Path != "/"+cfg.DBName {
		t.Fatalf("endpoint did not round trip safely: %q", dsn)
	}
	query := parsed.Query()
	if query.Get("sslmode") != cfg.SSLMode || query.Get("search_path") != cfg.SearchPath || query.Get("statement_timeout") != "15000" || query.Get("connect_timeout") != "10" {
		t.Fatalf("unexpected DSN query parameters: %#v", query)
	}
	if dsn == "" || dsn == cfg.Password {
		t.Fatalf("unexpected DSN: %q", dsn)
	}
}

func TestPingRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ping(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping returned %v, want context.Canceled", err)
	}
}
