package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"novel-agent/internal/config"
	"novel-agent/internal/handlers"
	"novel-agent/internal/service"
	"novel-agent/pkg/database"
)

const maxDotEnvBytes int64 = 64 << 10

var (
	setEnvironmentVariable   = os.Setenv
	unsetEnvironmentVariable = os.Unsetenv
)

func validEnvName(name string) bool {
	if name == "" || !(name[0] == '_' || name[0] >= 'A' && name[0] <= 'Z' || name[0] >= 'a' && name[0] <= 'z') {
		return false
	}
	for index := 1; index < len(name); index++ {
		value := name[index]
		if !(value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9') {
			return false
		}
	}
	return true
}

func containsUnsafeEnvRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || (unicode.IsSpace(r) && r != ' ') {
			return true
		}
	}
	return false
}

func validConfigPath(path string) bool {
	return path != "" && len(path) <= 4096 && utf8.ValidString(path) && !containsUnsafeEnvRune(path)
}

func resolveConfigPath() (string, error) {
	path, exists := os.LookupEnv("CONFIG_PATH")
	if !exists {
		return "config.yaml", nil
	}
	if !validConfigPath(path) {
		return "", fmt.Errorf("CONFIG_PATH is invalid")
	}
	return path, nil
}

func readEnvironmentFile(file io.ReadCloser) ([]byte, error) {
	data, readErr := io.ReadAll(io.LimitReader(file, maxDotEnvBytes+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return nil, fmt.Errorf("failed to read environment file")
	}
	if int64(len(data)) > maxDotEnvBytes {
		return nil, fmt.Errorf("environment file exceeds 65536 byte limit")
	}
	return data, nil
}

func loadDotEnv(path string) (bool, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to open environment file")
	}
	data, err := readEnvironmentFile(file)
	if err != nil {
		return false, err
	}
	if !utf8.Valid(data) {
		return false, fmt.Errorf("environment file must contain valid UTF-8")
	}
	type assignment struct {
		key   string
		value string
	}
	assignments := make([]assignment, 0)
	seen := make(map[string]struct{})
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(rawLine, "\r")
		trimmedLine := strings.TrimSpace(line)
		if trimmedLine == "" || strings.HasPrefix(trimmedLine, "#") {
			continue
		}
		index := strings.IndexByte(line, '=')
		if index < 1 {
			return false, fmt.Errorf("environment file contains an invalid assignment")
		}
		key := strings.TrimSpace(line[:index])
		value := line[index+1:]
		if key != "DB_PASSWORD" {
			value = strings.TrimSpace(value)
		}
		if !validEnvName(key) || containsUnsafeEnvRune(value) {
			return false, fmt.Errorf("environment file contains an invalid assignment")
		}
		if _, duplicate := seen[key]; duplicate {
			return false, fmt.Errorf("environment file contains a duplicate variable")
		}
		seen[key] = struct{}{}
		assignments = append(assignments, assignment{key: key, value: value})
	}

	applied := make([]string, 0, len(assignments))
	for _, item := range assignments {
		if _, exists := os.LookupEnv(item.key); exists {
			continue
		}
		if err := setEnvironmentVariable(item.key, item.value); err != nil {
			rollbackFailed := false
			for index := len(applied) - 1; index >= 0; index-- {
				if unsetEnvironmentVariable(applied[index]) != nil {
					rollbackFailed = true
				}
			}
			if rollbackFailed {
				return false, fmt.Errorf("failed to apply and roll back environment file")
			}
			return false, fmt.Errorf("failed to apply environment file")
		}
		applied = append(applied, item.key)
	}
	return true, nil
}

func configureGinMode(mode string) error {
	switch mode {
	case gin.DebugMode, gin.ReleaseMode, gin.TestMode:
		gin.SetMode(mode)
		return nil
	default:
		return fmt.Errorf("unsupported server mode")
	}
}

func main() {
	// 加载 .env 文件（如果存在）
	// 支持从当前目录或上级目录加载
	envPaths := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "..", ".env"),
	}

	for _, envPath := range envPaths {
		loaded, err := loadDotEnv(envPath)
		if err != nil {
			log.Fatalf("Failed to load environment overrides: %v", err)
		}
		if loaded {
			log.Printf("[config] Loaded local environment overrides")
			break
		}
	}

	// 加载配置
	cfgPath, err := resolveConfigPath()
	if err != nil {
		log.Fatalf("Failed to resolve config path")
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	if err := configureGinMode(cfg.Server.Mode); err != nil {
		log.Fatalf("Failed to configure HTTP framework")
	}

	// 初始化数据库
	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Printf("Database pool close failed")
		}
	}()

	// 初始化服务
	svc, err := service.NewService(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize services")
	}

	// 设置路由
	router := handlers.SetupRouter(svc)

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("🚀 Novel Agent API starting...")
	log.Printf("📍 Server: http://%s", addr)
	log.Printf("📖 Docs: http://%s/docs", addr)
	log.Printf("❤️  Health: http://%s/api/v1/health", addr)
	log.Printf("🤖 AI services configured")
	log.Printf("💾 Database pool configured; connectivity is reported by /api/v1/health")

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// A workflow can perform several sequential 300-second-bounded LLM calls.
		WriteTimeout:   20 * time.Minute,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
	}

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
		close(serverErrors)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		log.Printf("👋 Shutting down server after %s...", sig)
	case err := <-serverErrors:
		if err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server graceful shutdown timed out: %v", err)
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("Server forced close failed: %v", closeErr)
		}
	}
	log.Println("Server stopped")
}
