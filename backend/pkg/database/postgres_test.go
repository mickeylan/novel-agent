package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"

	"gorm.io/gorm"

	"novel-agent/internal/config"
)

type testClosablePool struct {
	closed bool
}

func (p *testClosablePool) PrepareContext(context.Context, string) (*sql.Stmt, error) {
	return nil, errors.New("not implemented")
}
func (p *testClosablePool) ExecContext(context.Context, string, ...interface{}) (sql.Result, error) {
	return nil, errors.New("not implemented")
}
func (p *testClosablePool) QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}
func (p *testClosablePool) QueryRowContext(context.Context, string, ...interface{}) *sql.Row {
	return &sql.Row{}
}
func (p *testClosablePool) Close() error {
	p.closed = true
	return errors.New("secret close detail")
}

func TestCloseCandidateReleasesPoolAndIgnoresSensitiveCloseError(t *testing.T) {
	pool := &testClosablePool{}
	closeCandidate(&gorm.DB{Config: &gorm.Config{ConnPool: pool}})
	if !pool.closed {
		t.Fatal("failed database candidate pool was not closed")
	}
}

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

func TestInitUsesNormalizedDatabaseCopyWithoutMutatingCaller(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})
	var capturedDSN string
	openDatabase = func(dsn string) (*gorm.DB, error) {
		capturedDSN = dsn
		return nil, errors.New("stop after capturing normalized DSN")
	}
	DB = nil
	cfg := config.Default().Database
	cfg.Host = " 127.0.0.1 "
	cfg.User = " novel_agent "
	cfg.DBName = " novel_agent "
	cfg.SSLMode = " DISABLE "
	cfg.SearchPath = " novel_agent , public "
	cfg.Password = " password-with-spaces "
	original := cfg
	if err := Init(&cfg); err == nil || err.Error() != "database driver initialization failed" {
		t.Fatalf("unexpected initialization result: %v", err)
	}
	parsed, err := url.Parse(capturedDSN)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Hostname() != "127.0.0.1" || parsed.User.Username() != "novel_agent" || parsed.Path != "/novel_agent" || parsed.Query().Get("sslmode") != "disable" || parsed.Query().Get("search_path") != "novel_agent,public" {
		t.Fatalf("database normalization was not applied to DSN: %q", capturedDSN)
	}
	if password != " password-with-spaces " {
		t.Fatalf("database password was normalized unexpectedly: %q", password)
	}
	if cfg != original {
		t.Fatalf("Init mutated caller config: got=%#v want=%#v", cfg, original)
	}
}

func TestInitRejectsInvalidConfigurationBeforeReplacingExistingPool(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})
	pool := &testClosablePool{}
	existing := &gorm.DB{Config: &gorm.Config{ConnPool: pool}}
	DB = existing
	called := false
	openDatabase = func(string) (*gorm.DB, error) {
		called = true
		return nil, errors.New("must not be called")
	}
	cfg := config.Default().Database
	cfg.Password = "test-password"
	cfg.MaxOpenConns = 0
	if err := Init(&cfg); err == nil || err.Error() != "database configuration is invalid" {
		t.Fatalf("unexpected invalid config error: %v", err)
	}
	if called || pool.closed || DB != existing {
		t.Fatalf("invalid replacement changed existing pool: called=%v closed=%v DB=%#v", called, pool.closed, DB)
	}
}

func TestInitRejectsNilConfigurationBeforeOpeningDatabase(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})

	called := false
	openDatabase = func(string) (*gorm.DB, error) {
		called = true
		return &gorm.DB{}, nil
	}
	DB = nil

	err := Init(nil)
	if err == nil || err.Error() != "database configuration is required" {
		t.Fatalf("unexpected nil configuration error: %v", err)
	}
	if called {
		t.Fatal("database opener was called for nil configuration")
	}
	if DB != nil {
		t.Fatal("nil configuration retained a database handle")
	}
}

func TestInitClosesCandidateReturnedWithDriverError(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})
	pool := &testClosablePool{}
	openDatabase = func(string) (*gorm.DB, error) {
		return &gorm.DB{Config: &gorm.Config{ConnPool: pool}}, errors.New("secret driver detail")
	}
	DB = nil
	cfg := config.Default().Database
	cfg.Password = "test-password"
	if err := Init(&cfg); err == nil || err.Error() != "database driver initialization failed" {
		t.Fatalf("unexpected driver error: %v", err)
	}
	if !pool.closed || DB != nil {
		t.Fatalf("failed candidate cleanup: closed=%v DB=%#v", pool.closed, DB)
	}
}

func TestInitRejectsUnusableDriverHandlesWithoutPublishingThem(t *testing.T) {
	for name, result := range map[string]*gorm.DB{
		"nil":       nil,
		"malformed": {},
	} {
		t.Run(name, func(t *testing.T) {
			originalOpen := openDatabase
			originalDB := DB
			t.Cleanup(func() {
				openDatabase = originalOpen
				DB = originalDB
			})
			openDatabase = func(string) (*gorm.DB, error) { return result, nil }
			DB = nil
			cfg := config.Default().Database
			cfg.Password = "test-password"
			if err := Init(&cfg); err == nil || err.Error() != "database driver initialization failed" {
				t.Fatalf("unexpected unusable handle error: %v", err)
			}
			if DB != nil {
				t.Fatal("unusable driver handle was published globally")
			}
		})
	}
}

func TestInitRedactsDriverErrorAndClearsGlobalHandle(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})

	const secret = "postgres://user:super-secret@example.invalid/database"
	openDatabase = func(string) (*gorm.DB, error) {
		return &gorm.DB{}, errors.New("driver rejected " + secret)
	}
	DB = nil

	cfg := config.Default().Database
	cfg.Password = "super-secret"
	err := Init(&cfg)
	if err == nil || err.Error() != "database driver initialization failed" {
		t.Fatalf("unexpected initialization error: %v", err)
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), cfg.Password) {
		t.Fatalf("database secret leaked through initialization error: %v", err)
	}
	if DB != nil {
		t.Fatal("failed initialization retained a database handle")
	}
	if errors.Unwrap(err) != nil {
		t.Fatal("sanitized initialization error retained an underlying error chain")
	}
}

func TestCloseClearsGlobalHandleEvenWhenPoolLookupFails(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	DB = &gorm.DB{}

	err := Close()
	if err == nil || err.Error() != "database pool close failed" {
		t.Fatalf("unexpected close error: %v", err)
	}
	if DB != nil {
		t.Fatal("Close retained a globally usable database handle after failure")
	}
}

func TestCloseRedactsPoolFailureAndClearsHandle(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	pool := &testClosablePool{}
	DB = &gorm.DB{Config: &gorm.Config{ConnPool: pool}}

	err := Close()
	if err == nil || err.Error() != "database pool close failed" {
		t.Fatalf("unexpected close error: %v", err)
	}
	if !pool.closed || DB != nil {
		t.Fatalf("close failure state: closed=%v DB=%#v", pool.closed, DB)
	}
	if strings.Contains(err.Error(), "secret") || errors.Unwrap(err) != nil {
		t.Fatalf("database close error leaked details: %v", err)
	}
}

func TestCloseRejectsMissingConnectionPoolWithoutPanic(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	DB = &gorm.DB{Config: &gorm.Config{}}
	if err := Close(); err == nil || err.Error() != "database pool close failed" {
		t.Fatalf("unexpected missing pool close error: %v", err)
	}
	if DB != nil {
		t.Fatal("missing connection pool retained global handle")
	}
}

func TestCloseIsIdempotentWithoutDatabase(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	DB = nil
	if err := Close(); err != nil {
		t.Fatalf("Close(nil) returned error: %v", err)
	}
}

func TestDatabaseAvailabilityRejectsMalformedHandleWithoutPanic(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() { DB = originalDB })
	DB = &gorm.DB{}
	if IsInitialized() {
		t.Fatal("malformed GORM handle reported as initialized")
	}
	if err := Ping(context.Background()); err == nil || err.Error() != "database not initialized" {
		t.Fatalf("unexpected malformed handle ping result: %v", err)
	}
}

func TestInitRejectsMalformedExistingPoolBeforeOpeningReplacement(t *testing.T) {
	originalOpen := openDatabase
	originalDB := DB
	t.Cleanup(func() {
		openDatabase = originalOpen
		DB = originalDB
	})
	called := false
	openDatabase = func(string) (*gorm.DB, error) {
		called = true
		return &gorm.DB{}, nil
	}
	DB = &gorm.DB{}
	cfg := config.Default().Database
	cfg.Password = "test-password"
	if err := Init(&cfg); err == nil || err.Error() != "database pool reset failed" {
		t.Fatalf("unexpected reset error: %v", err)
	}
	if called {
		t.Fatal("replacement database was opened after old pool reset failure")
	}
	if DB != nil {
		t.Fatal("failed pool reset retained malformed global handle")
	}
}

func TestPingRejectsCanceledContextBeforeDatabaseAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Ping(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Ping returned %v, want context.Canceled", err)
	}
}
