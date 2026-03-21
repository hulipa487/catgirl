package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/rs/zerolog"
)

type LLMService struct {
	config *config.LLMConfig
	logger zerolog.Logger
	client *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	MaxTokens int          `json:"max_tokens,omitempty"`
	Temperature float64    `json:"temperature,omitempty"`
}

type ChatResponse struct {
	ID      string   `json:"id"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

type Choice struct {
	Message ChatMessage `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type EmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type EmbeddingResponse struct {
	Data []EmbeddingData `json:"data"`
	Usage Usage          `json:"usage"`
}

type EmbeddingData struct {
	Embedding []float32 `json:"embedding"`
}

func NewLLMService(cfg *config.LLMConfig, logger zerolog.Logger) *LLMService {
	return &LLMService{
		config: cfg,
		logger: logger,
		client: &http.Client{
			Timeout: time.Duration(cfg.TimeoutSecs) * time.Second,
		},
	}
}

func (s *LLMService) Chat(ctx context.Context, model string, messages []ChatMessage, maxTokens int) (*ChatResponse, error) {
	reqBody := ChatRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	if maxTokens == 0 {
		reqBody.MaxTokens = s.config.MaxTokens
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", s.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.APIKey))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("llm request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &chatResp, nil
}

func (s *LLMService) ChatSimple(ctx context.Context, model string, systemPrompt, userMessage string) (string, *Usage, error) {
	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	resp, err := s.Chat(ctx, model, messages, 0)
	if err != nil {
		return "", nil, err
	}

	if len(resp.Choices) == 0 {
		return "", &resp.Usage, fmt.Errorf("no choices in response")
	}

	return resp.Choices[0].Message.Content, &resp.Usage, nil
}

func (s *LLMService) Embed(ctx context.Context, texts []string) ([][]float32, *Usage, error) {
	reqBody := EmbeddingRequest{
		Model: s.config.EmbeddingModel,
		Input: texts,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", s.config.BaseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.config.APIKey))

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("embedding request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var embResp EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embResp); err != nil {
		return nil, nil, fmt.Errorf("failed to decode response: %w", err)
	}

	embeddings := make([][]float32, len(embResp.Data))
	for i, data := range embResp.Data {
		embeddings[i] = data.Embedding
	}

	return embeddings, &embResp.Usage, nil
}

func (s *LLMService) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	embeddings, _, err := s.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

func (s *LLMService) CountTokens(text string) int {
	return len(text) / 4
}
