package database

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"novel-agent/internal/config"
)

var DB *gorm.DB

func IsInitialized() bool {
	return DB != nil && DB.Config != nil && DB.Config.ConnPool != nil
}

func closeCandidate(candidate *gorm.DB) {
	if candidate == nil || candidate.Config == nil || candidate.Config.ConnPool == nil {
		return
	}
	if closer, ok := candidate.Config.ConnPool.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
}

var openDatabase = func(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{
		// SQL values may contain unpublished novel text. Repository errors are logged
		// through request-correlated handlers instead of GORM's interpolated SQL logger.
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func buildPostgresDSN(cfg *config.DatabaseConfig) string {
	query := url.Values{}
	query.Set("sslmode", cfg.SSLMode)
	query.Set("search_path", cfg.SearchPath)
	query.Set("connect_timeout", strconv.Itoa(cfg.ConnectTimeoutSec))
	query.Set("statement_timeout", strconv.Itoa(cfg.StatementTimeoutMS))
	connectionURL := &url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:     "/" + cfg.DBName,
		RawPath:  "/" + url.PathEscape(cfg.DBName),
		RawQuery: query.Encode(),
	}
	return connectionURL.String()
}

func Init(cfg *config.DatabaseConfig) error {
	normalized, err := config.NormalizeDatabaseConfig(cfg)
	if err != nil {
		return err
	}
	cfg = normalized
	if DB != nil {
		if err := Close(); err != nil {
			return fmt.Errorf("database pool reset failed")
		}
	}
	dsn := buildPostgresDSN(cfg)

	candidate, err := openDatabase(dsn)
	if err != nil || candidate == nil || candidate.Config == nil || candidate.Config.ConnPool == nil {
		closeCandidate(candidate)
		// Driver errors may include the DSN. Never return them to startup logs because
		// the DSN contains the database password.
		return fmt.Errorf("database driver initialization failed")
	}

	sqlDB, err := candidate.DB()
	if err != nil {
		closeCandidate(candidate)
		return fmt.Errorf("database pool initialization failed")
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)
	sqlDB.SetConnMaxIdleTime(time.Duration(cfg.ConnMaxIdleTimeSec) * time.Second)
	DB = candidate

	// gorm.Open configures a lazy pool; connectivity is reported by /health.
	log.Println("[database] PostgreSQL connection pool configured")
	return nil
}

func Close() error {
	current := DB
	DB = nil
	if current == nil {
		return nil
	}
	if current.Config == nil || current.Config.ConnPool == nil {
		return fmt.Errorf("database pool close failed")
	}
	closer, ok := current.Config.ConnPool.(interface{ Close() error })
	if !ok {
		return fmt.Errorf("database pool close failed")
	}
	if err := closer.Close(); err != nil {
		return fmt.Errorf("database pool close failed")
	}
	return nil
}

func Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !IsInitialized() {
		return fmt.Errorf("database not initialized")
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
