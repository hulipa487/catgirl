package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// OrchestratorContextManager manages persistent context for the orchestrator
type OrchestratorContextManager struct {
	sessionID     uuid.UUID
	repo          *repository.Repository
	logger        zerolog.Logger
	turns         []*models.SessionTurn
	currentTurnID int
	mu            sync.RWMutex
}

// NewOrchestratorContextManager creates a new context manager for an orchestrator session
func NewOrchestratorContextManager(sessionID uuid.UUID, repo *repository.Repository, logger zerolog.Logger) *OrchestratorContextManager {
	return &OrchestratorContextManager{
		sessionID:     sessionID,
		repo:          repo,
		logger:        logger,
		turns:         make([]*models.SessionTurn, 0),
		currentTurnID: 0,
	}
}

// LoadHistory loads existing turns from the database
func (o *OrchestratorContextManager) LoadHistory(ctx context.Context) error {
	turns, err := o.repo.GetSessionTurns(ctx, o.sessionID)
	if err != nil {
		return err
	}

	o.mu.Lock()
	o.turns = turns
	if len(turns) > 0 {
		o.currentTurnID = turns[len(turns)-1].TurnID
	}
	o.mu.Unlock()

	o.logger.Debug().Int("turns_loaded", len(turns)).Msg("Loaded orchestrator context history")
	return nil
}

// AddUserTurn records a user message
func (o *OrchestratorContextManager) AddUserTurn(ctx context.Context, content string) error {
	o.mu.Lock()
	o.currentTurnID++
	turn := &models.SessionTurn{
		TurnID:    o.currentTurnID,
		SessionID: o.sessionID,
		Role:      "user",
		Content:   content,
		Timestamp: time.Now(),
	}
	o.turns = append(o.turns, turn)
	o.mu.Unlock()

	return o.repo.AddSessionTurn(ctx, turn)
}

// AddAssistantTurn records an assistant message with optional tool calls
func (o *OrchestratorContextManager) AddAssistantTurn(ctx context.Context, content string, toolCalls []llm.ToolCall, inputTokens, outputTokens int) error {
	o.mu.Lock()
	o.currentTurnID++

	// Store first tool call info for quick lookup
	var toolArgsJSON json.RawMessage
	var toolName *string
	if len(toolCalls) > 0 {
		tc := toolCalls[0]
		args, _ := json.Marshal(struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		}{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
		toolArgsJSON = args
		toolName = &tc.Function.Name
	}

	turn := &models.SessionTurn{
		TurnID:        o.currentTurnID,
		SessionID:     o.sessionID,
		Role:          "assistant",
		Content:       content,
		ToolName:      toolName,
		ToolArguments: toolArgsJSON,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		Timestamp:     time.Now(),
	}
	o.turns = append(o.turns, turn)
	o.mu.Unlock()

	return o.repo.AddSessionTurn(ctx, turn)
}

// AddToolResultTurn records a tool result message
func (o *OrchestratorContextManager) AddToolResultTurn(ctx context.Context, toolCallID, toolName, result string) error {
	o.mu.Lock()
	o.currentTurnID++
	turn := &models.SessionTurn{
		TurnID:     o.currentTurnID,
		SessionID:  o.sessionID,
		Role:       "tool",
		Content:    result,
		ToolCallID: &toolCallID,
		ToolName:   &toolName,
		ToolResult: json.RawMessage(result),
		Timestamp:  time.Now(),
	}
	o.turns = append(o.turns, turn)
	o.mu.Unlock()

	return o.repo.AddSessionTurn(ctx, turn)
}

// BuildMessages reconstructs the OpenAI-compliant message array from stored turns
// If additionalContext is provided, it's injected as a user message after the system prompt
// TODO: Implement context compaction when token count exceeds CompactionThreshold
func (o *OrchestratorContextManager) BuildMessages(systemPrompt string, additionalContext string) []llm.ChatMessage {
	o.mu.RLock()
	defer o.mu.RUnlock()

	builder := llm.NewMessageBuilder(systemPrompt)

	// Inject additional context as a user message after system prompt
	if additionalContext != "" {
		builder.AddUserMessage(additionalContext + "\n\n---\n\nProceed with the conversation, using the context provided if relevant.")
	}

	for _, turn := range o.turns {
		switch turn.Role {
		case "user":
			builder.AddUserMessage(turn.Content)
		case "assistant":
			var toolCalls []llm.ToolCall
			if len(turn.ToolArguments) > 0 {
				var tcInfo struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}
				if err := json.Unmarshal(turn.ToolArguments, &tcInfo); err == nil && tcInfo.ID != "" {
					toolCalls = append(toolCalls, llm.ToolCall{
						ID:   tcInfo.ID,
						Type: "function",
						Function: llm.ToolCallFunction{
							Name:      tcInfo.Name,
							Arguments: tcInfo.Arguments,
						},
					})
				}
			}
			builder.AddAssistantMessage(turn.Content, toolCalls)
		case "tool":
			builder.AddToolResult(*turn.ToolCallID, turn.Content)
		}
	}

	return builder.Build()
}

// GetRecentTurns returns the last N turns
func (o *OrchestratorContextManager) GetRecentTurns(count int) []*models.SessionTurn {
	o.mu.RLock()
	defer o.mu.RUnlock()

	start := len(o.turns) - count
	if start < 0 {
		start = 0
	}
	return o.turns[start:]
}

// GetTotalTokens returns the sum of all input and output tokens
func (o *OrchestratorContextManager) GetTotalTokens() (int, int) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	var inputTotal, outputTotal int
	for _, turn := range o.turns {
		inputTotal += turn.InputTokens
		outputTotal += turn.OutputTokens
	}
	return inputTotal, outputTotal
}

// Clear removes all turns from memory (keeps DB records)
func (o *OrchestratorContextManager) Clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.turns = make([]*models.SessionTurn, 0)
	o.currentTurnID = 0
}

// GetSessionID returns the session ID
func (o *OrchestratorContextManager) GetSessionID() uuid.UUID {
	return o.sessionID
}