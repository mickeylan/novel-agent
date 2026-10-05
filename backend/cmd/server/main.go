package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

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
		log.Printf("[config] Warning: %v", err)
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

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := router.Run(addr); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	<-quit
	log.Println("👋 Shutting down server...")
}
