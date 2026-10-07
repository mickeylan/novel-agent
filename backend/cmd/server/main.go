package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"novel-agent/internal/config"
	"novel-agent/internal/handlers"
	"novel-agent/internal/service"
	"novel-agent/pkg/database"
)

func main() {
	// 加载 .env 文件（如果存在）
	// 支持从当前目录或上级目录加载
	envPaths := []string{
		".env",
		filepath.Join("..", ".env"),
		filepath.Join("..", "..", ".env"),
	}

	for _, envPath := range envPaths {
		if data, err := os.ReadFile(envPath); err == nil {
			// 简单解析 .env 文件
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if idx := strings.Index(line, "="); idx > 0 {
					key := strings.TrimSpace(line[:idx])
					value := strings.TrimSpace(line[idx+1:])
					if _, exists := os.LookupEnv(key); !exists {
						os.Setenv(key, value)
					}
				}
			}
			log.Printf("[config] Loaded .env from: %s", envPath)
			break
		}
	}

	// 加载配置
	cfgPath := "config.yaml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		cfgPath = envPath
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid configuration: %v", err)
	}

	// 初始化数据库
	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// 初始化服务
	svc := service.NewService(cfg)

	// 设置路由
	router := handlers.SetupRouter(svc)

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	log.Printf("🚀 Novel Agent API starting...")
	log.Printf("📍 Server: http://%s", addr)
	log.Printf("📖 Docs: http://%s/docs", addr)
	log.Printf("❤️  Health: http://%s/api/v1/health", addr)
	log.Printf("🤖 LLM Provider: %s / %s", cfg.LLM.Provider, cfg.LLM.Model)
	log.Printf("💾 Database: %s@%s:%d/%s", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.DBName)

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// A workflow can perform several sequential 300-second-bounded LLM calls.
		WriteTimeout: 20 * time.Minute,
		IdleTimeout:  120 * time.Second,
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
