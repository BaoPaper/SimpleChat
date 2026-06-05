package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIService OpenAI API 服务
type OpenAIService struct {
	ModelsConfig *ModelsConfig
	client       *http.Client
}

func (s *OpenAIService) getClient() *http.Client {
	if s.client == nil {
		s.client = &http.Client{
			Timeout: 5 * time.Minute,
		}
	}
	return s.client
}

// ChatMessage OpenAI 消息格式
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionRequest OpenAI 请求
type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// chatCompletionChunk 流式响应块
type chatCompletionChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// StreamChat 发送流式聊天请求
func (s *OpenAIService) StreamChat(model string, messages []ChatMessage) (<-chan string, <-chan error) {
	contentCh := make(chan string, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(contentCh)
		defer close(errCh)

		reqBody := chatCompletionRequest{
			Model:    model,
			Messages: messages,
			Stream:   true,
		}

		bodyJSON, err := json.Marshal(reqBody)
		if err != nil {
			errCh <- fmt.Errorf("序列化请求失败: %w", err)
			return
		}

		apiBase := strings.TrimRight(s.ModelsConfig.APIBase, "/")
		url := apiBase + "/chat/completions"

		req, err := http.NewRequest("POST", url, bytes.NewReader(bodyJSON))
		if err != nil {
			errCh <- fmt.Errorf("创建请求失败: %w", err)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.ModelsConfig.APIKey)
		req.Header.Set("Accept", "text/event-stream")

		resp, err := s.getClient().Do(req)
		if err != nil {
			errCh <- fmt.Errorf("请求 API 失败: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("API 返回错误 (%d): %s", resp.StatusCode, string(body))
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 4096), 1024*1024)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" || !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}

			var chunk chatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					select {
					case contentCh <- choice.Delta.Content:
					default:
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("读取流失败: %w", err)
		}
	}()

	return contentCh, errCh
}
