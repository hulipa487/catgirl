package runtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/agent"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// WorkerContextManager manages persistent context for worker agents
type WorkerContextManager struct {
	instanceID uuid.UUID
	repo       *repository.Repository
	logger     zerolog.Logger
	turns      []*models.TaskInstanceTurn
	currentTurnID int
	mu         sync.RWMutex
}

// NewWorkerContextManager creates a new context manager for a task instance
func NewWorkerContextManager(instanceID uuid.UUID, repo *repository.Repository, logger zerolog.Logger) *WorkerContextManager {
	return &WorkerContextManager{
		instanceID:    instanceID,
		repo:          repo,
		logger:        logger,
		turns:         make([]*models.TaskInstanceTurn, 0),
		currentTurnID: 0,
	}
}

// LoadHistory loads existing turns from the database
func (w *WorkerContextManager) LoadHistory(ctx context.Context) error {
	turns, err := w.repo.GetTaskInstanceTurns(ctx, w.instanceID)
	if err != nil {
		return err
	}

	w.mu.Lock()
	w.turns = turns
	if len(turns) > 0 {
		w.currentTurnID = turns[len(turns)-1].TurnID
	}
	w.mu.Unlock()

	w.logger.Debug().Int("turns_loaded", len(turns)).Msg("Loaded worker context history")
	return nil
}

// AddUserTurn records a user message
func (w *WorkerContextManager) AddUserTurn(ctx context.Context, content string) error {
	w.mu.Lock()
	w.currentTurnID++
	turn := &models.TaskInstanceTurn{
		TurnID:     w.currentTurnID,
		InstanceID: w.instanceID,
		Role:       "user",
		Content:    content,
		Timestamp:  time.Now(),
	}
	w.turns = append(w.turns, turn)
	w.mu.Unlock()

	return w.repo.AddTaskInstanceTurn(ctx, turn)
}

// AddAssistantTurn records an assistant message with optional tool calls
func (w *WorkerContextManager) AddAssistantTurn(ctx context.Context, content string, toolCalls []agent.ToolCallInfo, inputTokens, outputTokens int) error {
	w.mu.Lock()
	w.currentTurnID++

	var toolArgsJSON json.RawMessage
	if len(toolCalls) == 1 {
		args, _ := json.Marshal(toolCalls[0])
		toolArgsJSON = args
	}

	turn := &models.TaskInstanceTurn{
		TurnID:        w.currentTurnID,
		InstanceID:    w.instanceID,
		Role:          "assistant",
		Content:       content,
		ToolArguments: toolArgsJSON,
		InputTokens:   inputTokens,
		OutputTokens:  outputTokens,
		Timestamp:     time.Now(),
	}
	w.turns = append(w.turns, turn)
	w.mu.Unlock()

	return w.repo.AddTaskInstanceTurn(ctx, turn)
}

// AddToolResultTurn records a tool result message
func (w *WorkerContextManager) AddToolResultTurn(ctx context.Context, toolCallID, toolName, result string) error {
	w.mu.Lock()
	w.currentTurnID++
	turn := &models.TaskInstanceTurn{
		TurnID:     w.currentTurnID,
		InstanceID: w.instanceID,
		Role:       "tool",
		Content:    result,
		ToolCallID: &toolCallID,
		ToolName:   &toolName,
		ToolResult: json.RawMessage(result),
		Timestamp:  time.Now(),
	}
	w.turns = append(w.turns, turn)
	w.mu.Unlock()

	return w.repo.AddTaskInstanceTurn(ctx, turn)
}

// BuildMessages reconstructs the OpenAI-compliant message array from stored turns
// If additionalContext is provided, it's injected as a user message after the system prompt
// TODO: Implement context compaction when token count exceeds CompactionThreshold
// - Summarize old turns using a reasoner model
// - Preserve recent N turns (PreserveRecentTurns config)
// - Keep tool call/result pairs together
func (w *WorkerContextManager) BuildMessages(systemPrompt string, additionalContext string) []llm.ChatMessage {
	w.mu.RLock()
	defer w.mu.RUnlock()

	builder := llm.NewMessageBuilder(systemPrompt)

	// Inject additional context as a system-like user message
	if additionalContext != "" {
		builder.AddUserMessage(additionalContext + "\n\n---\n\nProceed with the task above, using the context provided if relevant.")
	}

	for _, turn := range w.turns {
		switch turn.Role {
		case "user":
			builder.AddUserMessage(turn.Content)
		case "assistant":
			var toolCalls []llm.ToolCall
			if len(turn.ToolArguments) > 0 {
				// Try to unmarshal as ToolCallInfo
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
func (w *WorkerContextManager) GetRecentTurns(count int) []*models.TaskInstanceTurn {
	w.mu.RLock()
	defer w.mu.RUnlock()

	start := len(w.turns) - count
	if start < 0 {
		start = 0
	}
	return w.turns[start:]
}

// GetTotalTokens returns the sum of all input and output tokens
func (w *WorkerContextManager) GetTotalTokens() (int, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	var inputTotal, outputTotal int
	for _, turn := range w.turns {
		inputTotal += turn.InputTokens
		outputTotal += turn.OutputTokens
	}
	return inputTotal, outputTotal
}

// Clear removes all turns from memory (keeps DB records)
func (w *WorkerContextManager) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.turns = make([]*models.TaskInstanceTurn, 0)
	w.currentTurnID = 0
}

// GetInstanceID returns the instance ID
func (w *WorkerContextManager) GetInstanceID() uuid.UUID {
	return w.instanceID
}