package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func main() {
	// 自动生成默认配置文件
	if err := EnsureConfigsExist(); err != nil {
		log.Fatalf("配置初始化失败: %v", err)
	}

	// 加载配置
	settings, err := LoadSettings(filepath.Join("config", "settings.json"))
	if err != nil {
		log.Fatalf("加载 settings.json 失败: %v", err)
	}

	modelsConfig, err := LoadModelsConfig(filepath.Join("config", "models.json"))
	if err != nil {
		log.Fatalf("加载 models.json 失败: %v", err)
	}

	// 配置校验
	if err := settings.Validate(); err != nil {
		log.Fatalf("settings.json 配置错误: %v", err)
	}

	if err := modelsConfig.Validate(); err != nil {
		log.Fatalf("models.json 配置错误: %v", err)
	}

	// 确保 data 目录存在
	dataDir := filepath.Dir(settings.DatabasePath)
	if dataDir != "." {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			log.Fatalf("创建数据目录失败: %v", err)
		}
	}

	// 初始化数据库
	db, err := InitDB(settings.DatabasePath)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 创建 OpenAI 服务
	openaiService := &OpenAIService{
		ModelsConfig: modelsConfig,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	// 创建处理器
	handler := &Handler{
		DB:           db,
		Settings:     settings,
		ModelsConfig: modelsConfig,
		OpenAI:       openaiService,
	}

	// 设置 Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	// ========== API 路由 ==========
	api := r.Group("/api")
	{
		api.POST("/login", handler.Login)

		auth := api.Group("")
		auth.Use(AuthMiddleware(settings.JWTSecret))
		{
			auth.GET("/check", handler.Check)
			auth.GET("/models", handler.GetModels)
			auth.GET("/sessions", handler.ListSessions)
			auth.POST("/sessions", handler.CreateSession)
			auth.GET("/sessions/:id", handler.GetSession)
			auth.PUT("/sessions/:id", handler.UpdateSession)
			auth.DELETE("/sessions/:id", handler.DeleteSession)
			auth.POST("/chat", handler.Chat)
			auth.PUT("/chat/edit/:message_id", handler.EditChat)
			auth.POST("/chat/regenerate/:message_id", handler.RegenerateChat)
		}
	}

	// ========== 前端内嵌文件服务 ==========
	// 直接读 embed 文件，避免 http.FS 路由问题
	r.GET("/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", mustReadEmbedded("frontend/index.html"))
	})
	r.GET("/styles.css", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css; charset=utf-8", mustReadEmbedded("frontend/styles.css"))
	})
	r.GET("/script.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", mustReadEmbedded("frontend/script.js"))
	})
	r.GET("/libs/marked.min.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", mustReadEmbedded("frontend/libs/marked.min.js"))
	})
	r.GET("/libs/highlight.min.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", mustReadEmbedded("frontend/libs/highlight.min.js"))
	})
	r.GET("/libs/github-dark.min.css", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css; charset=utf-8", mustReadEmbedded("frontend/libs/github-dark.min.css"))
	})
	r.GET("/libs/purify.min.js", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/javascript; charset=utf-8", mustReadEmbedded("frontend/libs/purify.min.js"))
	})

	// SPA 回退：其余非 API 路径返回 index.html
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", mustReadEmbedded("frontend/index.html"))
	})

	// 启动服务
	addr := fmt.Sprintf(":%d", settings.Port)
	fmt.Printf("\n🚀 SimpleChat 启动成功！\n")
	fmt.Printf("📋 访问地址: http://localhost%s\n", addr)
	fmt.Printf("👥 用户数量: %d\n", len(settings.Users))
	for _, u := range settings.Users {
		fmt.Printf("   - %s\n", u.Username)
	}
	fmt.Printf("⚙️  配置文件: config/settings.json, config/models.json\n\n")

	if err := r.Run(addr); err != nil {
		log.Fatalf("启动服务失败: %v", err)
	}
}

// mustReadEmbedded 从内嵌文件系统读取文件，失败则 panic
func mustReadEmbedded(path string) []byte {
	data, err := frontendFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("读取内嵌文件 %s 失败: %v", path, err))
	}
	return data
}
