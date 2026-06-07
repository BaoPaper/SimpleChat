package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

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
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话失败"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}
	if session.UserID != username {
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
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话失败"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}
	if session.UserID != username {
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

	model, err := h.normalizeModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Model = model

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

		// 检查是否存在正在生成中的 assistant 占位消息
		pending, err := h.DB.HasPendingAssistant(req.SessionID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "检查会话状态失败"})
			return
		}
		if pending {
			c.JSON(http.StatusConflict, gin.H{"error": "当前会话已有回复正在生成，请稍后再试"})
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

	chatMessages := buildChatMessages(h.Settings.SystemPrompt, messages)

	// 流式响应（普通 Chat，user 消息是刚创建的，占位失败时可以删除）
	h.streamChatToSSE(c, sessionID, req.Model, chatMessages, userMsg.ID, true)
}

// ==================== 编辑消息（重新发送）====================

func (h *Handler) EditChat(c *gin.Context) {
	username := getUser(c)

	messageIDStr := c.Param("message_id")
	var messageID int64
	if _, err := fmt.Sscanf(messageIDStr, "%d", &messageID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的消息 ID"})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Content   string `json:"content"`
		Model     string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "内容不能为空"})
		return
	}

	// 校验会话所有权
	session, _, err := h.DB.GetSession(req.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话失败"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}
	if session.UserID != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	// 校验消息归属和角色
	msg, err := h.DB.GetMessageByID(messageID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取消息失败"})
		return
	}
	if msg == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "消息不存在"})
		return
	}
	if msg.SessionID != req.SessionID {
		c.JSON(http.StatusForbidden, gin.H{"error": "消息不属于当前会话"})
		return
	}
	if msg.Role != "user" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能编辑用户消息"})
		return
	}

	// 安全更新用户消息内容（带 session 和 role 校验）
	updated, err := h.DB.UpdateUserMessageContent(req.SessionID, messageID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新消息失败"})
		return
	}
	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "消息不存在或不可编辑"})
		return
	}

	// 删除该消息之后的所有消息
	if err := h.DB.DeleteMessagesAfter(req.SessionID, messageID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "截断会话失败"})
		return
	}

	// 选择模型
	model, err := h.normalizeModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 构建消息上下文
	messages, err := h.DB.GetMessages(req.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取消息历史失败"})
		return
	}

	chatMessages := buildChatMessages(h.Settings.SystemPrompt, messages)

	// 流式响应（EditChat，user 消息是已存在的，绝不能因占位失败删除）
	h.streamChatToSSE(c, req.SessionID, model, chatMessages, messageID, false)
}

// ==================== 重新生成 ====================

func (h *Handler) RegenerateChat(c *gin.Context) {
	username := getUser(c)

	messageIDStr := c.Param("message_id")
	var messageID int64
	if _, err := fmt.Sscanf(messageIDStr, "%d", &messageID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的消息 ID"})
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Model     string `json:"model"`
	}
	_ = c.ShouldBindJSON(&req)

	// 获取要删除的消息
	msg, err := h.DB.GetMessageByID(messageID)
	if err != nil || msg == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "消息不存在"})
		return
	}
	if msg.Role != "assistant" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "只能重新生成 AI 回复"})
		return
	}

	// 如果传了 session_id，校验与消息所属会话一致
	if req.SessionID != "" && req.SessionID != msg.SessionID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "消息不属于当前会话"})
		return
	}

	// 校验会话所有权
	session, _, err := h.DB.GetSession(msg.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取会话失败"})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "会话不存在"})
		return
	}
	if session.UserID != username {
		c.JSON(http.StatusForbidden, gin.H{"error": "无权操作此会话"})
		return
	}

	// 删除该消息及之后的所有消息
	if err := h.DB.DeleteMessagesFrom(msg.SessionID, messageID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "截断会话失败"})
		return
	}

	// 获取剩余消息作为上下文
	messages, err := h.DB.GetMessages(msg.SessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取消息历史失败"})
		return
	}

	chatMessages := buildChatMessages(h.Settings.SystemPrompt, messages)

	// 使用指定模型或默认模型
	model, err := h.normalizeModel(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 流式响应（RegenerateChat，userMsgID=0，不需要清理）
	h.streamChatToSSE(c, msg.SessionID, model, chatMessages, 0, false)
}

// normalizeModel 统一模型校验
func (h *Handler) normalizeModel(model string) (string, error) {
	if model == "" {
		model = h.ModelsConfig.DefaultModel
	}
	for _, m := range h.ModelsConfig.Models {
		if m.ID == model {
			return model, nil
		}
	}
	return "", fmt.Errorf("不支持的模型: %s", model)
}

// buildChatMessages 构建发送给 OpenAI 的消息列表
func buildChatMessages(systemPrompt string, messages []Message) []ChatMessage {
	chatMessages := make([]ChatMessage, 0, len(messages)+1)
	if systemPrompt != "" {
		chatMessages = append(chatMessages, ChatMessage{Role: "system", Content: systemPrompt})
	}
	for _, m := range messages {
		if m.Status == "error" {
			continue
		}
		chatMessages = append(chatMessages, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return chatMessages
}

// streamChatToSSE 向 OpenAI 发起流式请求，将结果以 SSE 写入响应
// cleanupUserMsgOnPlaceholderFail：创建 assistant 占位失败时是否删除 userMsgID 对应的消息
func (h *Handler) streamChatToSSE(
	c *gin.Context,
	sessionID string,
	model string,
	chatMessages []ChatMessage,
	userMsgID int64,
	cleanupUserMsgOnPlaceholderFail bool,
) {
	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}

	connected := true
	clientDone := c.Request.Context().Done()

	// 后台任务 context，客户端断开不影响 LLM 继续
	jobCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 先创建 assistant 占位消息，作为任务锚点
	assistantMsg, err := h.DB.AddAssistantPlaceholder(sessionID)
	if err != nil {
		// 数据库层面防止了并发占位（唯一索引），可能因冲突失败
		// 仅当调用来明确允许时，才删除刚创建的 user 消息
		if cleanupUserMsgOnPlaceholderFail && userMsgID > 0 {
			_ = h.DB.DeleteMessageInSession(sessionID, userMsgID)
		}
		if connected {
			_ = sendSSE(c.Writer, flusher, "error", "当前会话已有回复正在生成，请稍后再试")
		}
		return
	}
	assistantMsgID := assistantMsg.ID

	// 发送 meta 事件
	metaData := map[string]interface{}{
		"session_id":           sessionID,
		"assistant_message_id": assistantMsgID,
	}
	if userMsgID > 0 {
		metaData["user_message_id"] = userMsgID
	}
	if connected {
		if err := sendSSE(c.Writer, flusher, "meta", metaData); err != nil {
			connected = false
		}
	}

	contentCh, errCh := h.OpenAI.StreamChat(jobCtx, model, chatMessages)

	var fullContent strings.Builder
	var finalErr error

	lastPersist := time.Now()
	const persistInterval = 500 * time.Millisecond

	for contentCh != nil || errCh != nil {
		select {
		case <-clientDone:
			// 客户端断开，停止发送 SSE，继续读取 LLM
			connected = false
			clientDone = nil

		case content, ok := <-contentCh:
			if !ok {
				contentCh = nil
				continue
			}
			fullContent.WriteString(content)

			// 定期持久化生成进度
			if time.Since(lastPersist) >= persistInterval {
				updated, persistErr := h.DB.UpdateAssistantStreamingContent(sessionID, assistantMsgID, fullContent.String())
				if persistErr != nil {
					// 写进度失败，不要误判为占位被删除，记录后继续
					fmt.Printf("保存生成进度失败: %v\n", persistErr)
					lastPersist = time.Now()
				} else if !updated {
					// 只有没有 DB 错误且确实没更新到行，才认为占位被删除
					cancel()
					return
				} else {
					lastPersist = time.Now()
				}
			}

			if connected {
				if err := sendSSE(c.Writer, flusher, "content", content); err != nil {
					connected = false
				}
			}

		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if err != nil {
				finalErr = err
			}
		}
	}

	// 最后确保持久化最新内容
	assistantContent := fullContent.String()
	if assistantContent != "" {
		if updated, persistErr := h.DB.UpdateAssistantStreamingContent(sessionID, assistantMsgID, assistantContent); persistErr != nil {
			fmt.Printf("保存最终生成进度失败: %v\n", persistErr)
		} else if !updated {
			// 占位消息已被删除，任务失效
			return
		}
	}

	// LLM 完成后处理结果
	if finalErr != nil {
		errText := "错误: " + finalErr.Error()
		if assistantContent != "" {
			errText = assistantContent + "\n\n" + errText
		}

		if _, updateErr := h.DB.FailAssistantMessage(sessionID, assistantMsgID, errText); updateErr != nil {
			fmt.Printf("标记 assistant 消息失败: %v\n", updateErr)
		}
		if connected {
			_ = sendSSE(c.Writer, flusher, "error", finalErr.Error())
		}
		return
	}

	if assistantContent == "" {
		if _, updateErr := h.DB.FailAssistantMessage(sessionID, assistantMsgID, "错误: AI 未返回内容"); updateErr != nil {
			fmt.Printf("标记 assistant 空回复失败: %v\n", updateErr)
		}
		if connected {
			_ = sendSSE(c.Writer, flusher, "error", "AI 未返回内容")
		}
		return
	}

	// 完成 assistant 回复
	updated, err := h.DB.FinishAssistantMessage(sessionID, assistantMsgID, assistantContent)
	if err != nil {
		if connected {
			_ = sendSSE(c.Writer, flusher, "error", "保存回复失败")
		}
		return
	}
	if !updated {
		fmt.Printf("assistant 消息已失效，跳过完成: session=%s message=%d\n", sessionID, assistantMsgID)
		return
	}

	if connected {
		_ = sendSSE(c.Writer, flusher, "done", map[string]interface{}{
			"session_id": sessionID,
			"message_id": assistantMsgID,
		})
	}
}

func sendSSE(w io.Writer, flusher http.Flusher, eventType string, data interface{}) error {
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
		return fmt.Errorf("序列化 SSE 事件失败: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(jsonBytes)); err != nil {
		return fmt.Errorf("写入 SSE 失败: %w", err)
	}
	flusher.Flush()
	return nil
}
