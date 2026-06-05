package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// User 用户
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Settings 应用设置
type Settings struct {
	Port         int    `json:"port"`
	Users        []User `json:"users"`
	JWTSecret    string `json:"jwt_secret"`
	DatabasePath string `json:"database_path"`
	SystemPrompt string `json:"system_prompt"`
}

// ModelsConfig 模型配置
type ModelsConfig struct {
	DefaultModel string      `json:"default_model"`
	APIBase      string      `json:"api_base"`
	APIKey       string      `json:"api_key"`
	Models       []ModelInfo `json:"models"`
}

// ModelInfo 单个模型信息
type ModelInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// LoadSettings 加载 settings.json
func LoadSettings(path string) (*Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	s := &Settings{
		Port:         8080,
		JWTSecret:    "simplechat-secret",
		DatabasePath: "./data/simplechat.db",
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}

// LoadModelsConfig 加载 models.json
func LoadModelsConfig(path string) (*ModelsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	mc := &ModelsConfig{}
	if err := json.Unmarshal(data, mc); err != nil {
		return nil, err
	}
	return mc, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// EnsureConfigsExist 确保配置文件存在，不存在则自动生成
func EnsureConfigsExist() error {
	if err := os.MkdirAll("config", 0755); err != nil {
		return fmt.Errorf("创建 config 目录失败: %w", err)
	}

	settingsPath := filepath.Join("config", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		defaultSettings := Settings{
			Port: 8080,
			Users: []User{
				{Username: "admin", Password: "admin"},
			},
			JWTSecret:    randomHex(32),
			DatabasePath: "./data/simplechat.db",
			SystemPrompt: "你是一个有用的 AI 助手。",
		}
		data, _ := json.MarshalIndent(defaultSettings, "", "  ")
		if err := os.WriteFile(settingsPath, data, 0644); err != nil {
			return fmt.Errorf("创建 settings.json 失败: %w", err)
		}
		fmt.Println("✅ 已生成默认配置文件: config/settings.json")
		fmt.Println("   （默认用户 admin / admin，可在 users 数组中添加更多用户）")
	}

	modelsPath := filepath.Join("config", "models.json")
	if _, err := os.Stat(modelsPath); os.IsNotExist(err) {
		defaultModels := ModelsConfig{
			DefaultModel: "gpt-4o-mini",
			APIBase:      "https://api.openai.com/v1",
			APIKey:       "sk-your-api-key-here",
			Models: []ModelInfo{
				{ID: "gpt-4o-mini", Name: "GPT-4o Mini"},
				{ID: "gpt-4o", Name: "GPT-4o"},
				{ID: "gpt-4-turbo", Name: "GPT-4 Turbo"},
				{ID: "gpt-3.5-turbo", Name: "GPT-3.5 Turbo"},
			},
		}
		data, _ := json.MarshalIndent(defaultModels, "", "  ")
		if err := os.WriteFile(modelsPath, data, 0644); err != nil {
			return fmt.Errorf("创建 models.json 失败: %w", err)
		}
		fmt.Println("✅ 已生成默认模型配置: config/models.json")
		fmt.Println("⚠️  请编辑 config/models.json 填入你的 API Key 后重启服务")
	}

	return nil
}

// FindUser 查找用户，返回是否找到
func (s *Settings) FindUser(username, password string) bool {
	for _, u := range s.Users {
		if u.Username == username && u.Password == password {
			return true
		}
	}
	return false
}
