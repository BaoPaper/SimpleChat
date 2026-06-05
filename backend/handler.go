package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Handler 处理器
type Handler struct {
	DB           *DB
	Settings     *Settings
	ModelsConfig *ModelsConfig
	OpenAI       *OpenAIService
}

// getUser 从上下文获取当前用户名
func getUser(c *gin.Context) string {
	return c.GetString("username")
}

// ==================== 认证 ====================

func (h *Handler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "用户名和密码不能为空"})
		return
	}

	if !h.Settings.FindUser(req.Username, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "用户名或密码错误"})
		return
	}

	token, err := GenerateToken(req.Username, h.Settings.JWTSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "生成令牌失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":    token,
		"username": req.Username,
	})
}

func (h *Handler) Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "username": getUser(c)})
}

// ==================== 模型 ====================

func (h *Handler) GetModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"default_model": h.ModelsConfig.DefaultModel,
		"models":        h.ModelsConfig.Models,
	})
}

// ==================== 会话 ====================

func (h *Handler) ListSessions(c *gin.Context) {
	username := getUser(c)
	sessions, err := h.DB.ListSessions(username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话列表失败"})
		return
	}
	if sessions == nil {
		sessions = []Session{}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *Handler) CreateSession(c *gin.Context) {
	username := getUser(c)
	var req struct {
		Title string `json:"title"`
	}
	c.ShouldBindJSON(&req)

	session, err := h.DB.CreateSession(username, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"session": session})
}

func (h *Handler) GetSession(c *gin.Context) {
	username := getUser(c)
	id := c.Param("id")
	session, messages, err := h.DB.GetSession(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话失败"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}
	if session.UserID != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权访问此会话"})
		return
	}
	if messages == nil {
		messages = []Message{}
	}

	c.JSON(http.StatusOK, gin.H{
		"session":  session,
		"messages": messages,
	})
}

func (h *Handler) UpdateSession(c *gin.Context) {
	username := getUser(c)
	id := c.Param("id")

	// 校验所有权
	session, _, err := h.DB.GetSession(id)
	if err != nil || session == nil || session.UserID != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求格式错误"})
		return
	}

	if err := h.DB.UpdateSession(id, req.Title); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新会话失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *Handler) DeleteSession(c *gin.Context) {
	username := getUser(c)
	id := c.Param("id")

	session, _, err := h.DB.GetSession(id)
	if err != nil || session == nil || session.UserID != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	if err := h.DB.DeleteSession(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// ==================== 聊天（流式 SSE） ====================

func (h *Handler) Chat(c *gin.Context) {
	username := getUser(c)

	var req struct {
		SessionID string `json:"session_id"`
		Model     string `json:"model"`
		Message   string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "消息不能为空"})
		return
	}

	if req.Model == "" {
		req.Model = h.ModelsConfig.DefaultModel
	}

	// 验证模型
	modelValid := req.Model == h.ModelsConfig.DefaultModel
	for _, m := range h.ModelsConfig.Models {
		if m.ID == req.Model {
			modelValid = true
			break
		}
	}
	if !modelValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的模型: " + req.Model})
		return
	}

	// 检查或创建会话
	var sessionID string
	if req.SessionID != "" {
		s, _, err := h.DB.GetSession(req.SessionID)
		if err != nil || s == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "会话不存在"})
			return
		}
		if s.UserID != username {
			c.JSON(http.StatusForbidden, gin.H{"error": "无权访问此会话"})
			return
		}
		sessionID = req.SessionID
	} else {
		s, err := h.DB.CreateSession(username, "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "创建会话失败"})
			return
		}
		sessionID = s.ID
	}

	// 保存用户消息
	userMsg, err := h.DB.AddMessage(sessionID, "user", req.Message)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存消息失败"})
		return
	}

	// 构建消息列表
	messages, err := h.DB.GetMessages(sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取消息历史失败"})
		return
	}

	chatMessages := make([]ChatMessage, 0, len(messages)+1)
	if h.Settings.SystemPrompt != "" {
		chatMessages = append(chatMessages, ChatMessage{Role: "system", Content: h.Settings.SystemPrompt})
	}
	for _, m := range messages {
		chatMessages = append(chatMessages, ChatMessage{Role: m.Role, Content: m.Content})
	}

	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "不支持流式传输"})
		return
	}

	sendSSE(c.Writer, flusher, "meta", map[string]interface{}{
		"session_id": sessionID,
		"message_id": userMsg.ID,
	})

	contentCh, errCh := h.OpenAI.StreamChat(req.Model, chatMessages)

	var fullContent strings.Builder

loop:
	for {
		select {
		case content, ok := <-contentCh:
			if !ok {
				break loop
			}
			fullContent.WriteString(content)
			sendSSE(c.Writer, flusher, "content", content)
		case err := <-errCh:
			if err != nil {
				sendSSE(c.Writer, flusher, "error", err.Error())
				return
			}
			break loop
		case <-c.Request.Context().Done():
			break loop
		}
	}

	select {
	case err := <-errCh:
		if err != nil {
			sendSSE(c.Writer, flusher, "error", err.Error())
			return
		}
	default:
	}

	assistantContent := fullContent.String()
	if assistantContent != "" {
		assistantMsg, err := h.DB.AddMessage(sessionID, "assistant", assistantContent)
		if err == nil {
			sendSSE(c.Writer, flusher, "done", map[string]interface{}{
				"session_id": sessionID,
				"message_id": assistantMsg.ID,
			})
		} else {
			sendSSE(c.Writer, flusher, "error", "保存回复失败")
		}
	} else {
		sendSSE(c.Writer, flusher, "error", "AI 未返回内容")
	}
}

func sendSSE(w io.Writer, flusher http.Flusher, eventType string, data interface{}) {
	payload := map[string]interface{}{
		"type": eventType,
	}
	switch v := data.(type) {
	case string:
		payload["content"] = v
	case map[string]interface{}:
		for k, val := range v {
			payload[k] = val
		}
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes))
	flusher.Flush()
}
