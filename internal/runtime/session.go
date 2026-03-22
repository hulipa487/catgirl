package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/agent"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type SessionService struct {
	repo   *repository.Repository
	config *config.Config
	logger zerolog.Logger
	llmSvc *llm.LLMService
	sessions map[uuid.UUID]*Session
	mu     sync.RWMutex
	OnReply func(telegramUserID int64, message string)
}

type Session struct {
	ID         uuid.UUID
	TelegramUserID int64
	State      *OrchestratorState
	LTM        *LongTermMemoryManager
	History    *ConversationHistoryManager
	CreatedAt  time.Time
	LastActive time.Time
}

type OrchestratorState struct {
	CurrentTask string `json:"current_task,omitempty"`
	Progress    int    `json:"progress,omitempty"`
	PendingTasks int   `json:"pending_tasks,omitempty"`
}

type LongTermMemoryManager struct {
	sessionID uuid.UUID
	repo     *repository.Repository
	cfg      *config.Config
	logger   zerolog.Logger
}

type ConversationHistoryManager struct {
	sessionID    uuid.UUID
	repo         *repository.Repository
	turns        []*models.ConversationTurn
	currentTurnID int
	cfg          *config.ContextConfig
}

func NewSessionService(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger, llmSvc *llm.LLMService) *SessionService {
	return &SessionService{
		repo:     repo,
		config:   cfg,
		logger:   logger,
		llmSvc:   llmSvc,
		sessions: make(map[uuid.UUID]*Session),
	}
}

func (s *SessionService) CreateSession(ctx context.Context, telegramUserID int64, username, firstName, lastName string) (*Session, error) {
	sessionID := uuid.New()
	now := time.Now()

	session := &models.Session{
		ID:             sessionID,
		TelegramUserID: telegramUserID,
		Name:           fmt.Sprintf("session_%s", sessionID.String()[:8]),
		Status:         models.SessionStatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repo.CreateSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	if err := s.repo.CreateTelegramUser(ctx, telegramUserID, &sessionID, username, firstName, lastName); err != nil {
		s.logger.Error().Err(err).Int64("telegram_user_id", telegramUserID).Msg("failed to create telegram user")
	}

	sess := &Session{
		ID:              sessionID,
		TelegramUserID:  telegramUserID,
		State:           &OrchestratorState{},
		History:         NewConversationHistoryManager(sessionID, s.repo, &s.config.Context),
		CreatedAt:       now,
		LastActive:      now,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	s.logger.Info().
		Str("session_id", sessionID.String()).
		Int64("telegram_user_id", telegramUserID).
		Msg("session created")

	return sess, nil
}

func (s *SessionService) GetSession(ctx context.Context, sessionID uuid.UUID) (*Session, error) {
	s.mu.RLock()
	if session, ok := s.sessions[sessionID]; ok {
		s.mu.RUnlock()
		return session, nil
	}
	s.mu.RUnlock()

	sessionModel, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if sessionModel == nil {
		return nil, nil
	}

	var state OrchestratorState
	if sessionModel.OrchestratorState != nil {
		json.Unmarshal(sessionModel.OrchestratorState, &state)
	}

	sess := &Session{
		ID:              sessionModel.ID,
		TelegramUserID:  sessionModel.TelegramUserID,
		State:           &state,
		History:         NewConversationHistoryManager(sessionModel.ID, s.repo, &s.config.Context),
		CreatedAt:       sessionModel.CreatedAt,
		LastActive:      sessionModel.UpdatedAt,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	return sess, nil
}

func (s *SessionService) GetSessionIDByTelegramUser(ctx context.Context, telegramUserID int64) (interface{}, error) {
	session, err := s.GetSessionByTelegramUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, nil
	}
	return session.ID, nil
}

func (s *SessionService) CreateSessionForTelegramUser(ctx context.Context, telegramUserID int64, username, firstName, lastName string) (interface{}, error) {
	session, err := s.CreateSession(ctx, telegramUserID, username, firstName, lastName)
	if err != nil {
		return nil, err
	}
	return session.ID, nil
}

func (s *SessionService) HandleUserMessage(ctx context.Context, sessionIDInterface interface{}, telegramUserID int64, message string) error {
	sessionID, ok := sessionIDInterface.(uuid.UUID)
	if !ok {
		return fmt.Errorf("invalid session ID format")
	}

	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil {
		return fmt.Errorf("session not found")
	}

	// Basic implementation: we log the message and let the orchestrator pick it up.
	// In the full architecture, we would either enqueue a root task or feed it
	// to the Main Orchestrator's thought-action loop directly.
	s.logger.Info().
		Str("session_id", sessionID.String()).
		Str("message", message).
		Msg("Message fed into session orchestrator")

	// Store it in conversation history for the orchestrator to see
	resultMap := map[string]interface{}{"text": message}
	resultBytes, _ := json.Marshal(resultMap)

	turn := &models.ConversationTurn{
		Thought:   "",
		Action:    "USER_MESSAGE",
		Result:    resultBytes,
		Tokens:    0,
		Timestamp: time.Now(),
	}
	if err := s.AddConversationTurn(ctx, sessionID, turn); err != nil {
		return err
	}

	// Real implementation of orchestrator thought-action loop picking it up and replying
	go func() {
		// Wait a brief moment to ensure DB transaction finishes
		time.Sleep(100 * time.Millisecond)

		// Get recent context
		recentTurns := session.History.GetRecentTurns(session.History.cfg.PreserveRecentTurns)

		sysPrompt := s.config.LLM.SystemPrompt
		if sysPrompt == "" {
			sysPrompt = "You are an autonomous agent. You MUST use the SEND_MESSAGE tool to communicate with the user. Any raw text you output will be treated as internal thoughts and the user will not see it."
		}

		messages := []llm.ChatMessage{
			{Role: "system", Content: sysPrompt},
		}

		for _, t := range recentTurns {
			if t.Action == "USER_MESSAGE" {
				var msgData map[string]interface{}
				if err := json.Unmarshal(t.Result, &msgData); err == nil {
					if text, ok := msgData["text"].(string); ok {
						if text != "" {
							messages = append(messages, llm.ChatMessage{Role: "user", Content: text})
						}
					}
				}
			} else if t.Action == "SEND_MESSAGE" {
				var msgData map[string]interface{}
				if err := json.Unmarshal(t.Result, &msgData); err == nil {
					if text, ok := msgData["text"].(string); ok {
						if text != "" {
							messages = append(messages, llm.ChatMessage{Role: "assistant", Content: text})
						}
					}
				}
			} else if t.Action == "THINK" {
				// We can optionally pass previous thoughts as assistant messages
				messages = append(messages, llm.ChatMessage{Role: "assistant", Content: string(t.Result)})
			} else if t.Action == "TOOL_CALL" || t.Action == "TOOL_RESULT" {
				// In a full implementation, we'd reconstruct the exact tool call history here.
				// We will handle the current loop history inside the loop below.
			}
		}

		// If recentTurns didn't include the current message we just pushed
		// (e.g. async timing), add it
		if len(messages) == 1 || messages[len(messages)-1].Content != message {
			messages = append(messages, llm.ChatMessage{Role: "user", Content: message})
		}

		tools := []llm.Tool{
			{
				Type: "function",
				Function: llm.ToolFunction{
					Name:        "SPAWN_TASK",
					Description: "Spawn a sub-task",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"description": map[string]interface{}{"type": "string"},
							"priority":    map[string]interface{}{"type": "string", "enum": []string{"low", "normal", "high", "critical"}},
						},
						"required": []string{"description", "priority"},
					},
				},
			},
			{
				Type: "function",
				Function: llm.ToolFunction{
					Name:        "COMPLETE_TASK",
					Description: "Mark the current task as completed",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"result_summary": map[string]interface{}{"type": "string"},
						},
						"required": []string{"result_summary"},
					},
				},
			},
			{
				Type: "function",
				Function: llm.ToolFunction{
					Name:        "FAIL_TASK",
					Description: "Mark the current task as failed",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"reason": map[string]interface{}{"type": "string"},
						},
						"required": []string{"reason"},
					},
				},
			},
			{
				Type: "function",
				Function: llm.ToolFunction{
					Name:        "SEND_MESSAGE",
					Description: "Send a message to the user/orchestrator",
					Parameters: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{"type": "string"},
						},
						"required": []string{"message"},
					},
				},
			},
		}

		resp, err := s.llmSvc.ChatWithTools(context.Background(), s.config.LLM.GPModel, messages, tools, 0)

		if err != nil || len(resp.Choices) == 0 {
			s.logger.Error().Err(err).Msg("Failed to call LLM for reply")
			return
		}

		msg := resp.Choices[0].Message

		// Record usage
		billingSvc := agent.NewBillingService(s.repo, sessionID, fmt.Sprintf("%d", telegramUserID))
		_ = billingSvc.RecordUsage(context.Background(), nil, models.UsageOperationLLMCall, s.config.LLM.GPModel, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, string(models.MembershipFree))

		// Process Tool Calls (or text response)
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				s.logger.Info().Str("tool", tc.Function.Name).Str("args", tc.Function.Arguments).Msg("Main Orchestrator called tool")

				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

				switch tc.Function.Name {
				case "SEND_MESSAGE":
					if text, ok := args["message"].(string); ok {
						if s.OnReply != nil {
							s.OnReply(telegramUserID, text)
						}

						replyResultMap := map[string]interface{}{"text": text}
						replyResultBytes, _ := json.Marshal(replyResultMap)

						agentTurn := &models.ConversationTurn{
							Thought:   "I am sending a message.",
							Action:    "SEND_MESSAGE",
							Result:    replyResultBytes,
							Tokens:    0,
							Timestamp: time.Now(),
						}
						_ = s.AddConversationTurn(context.Background(), sessionID, agentTurn)
					}
				case "SPAWN_TASK":
					// Here you would enqueue the task
					s.logger.Info().Msg("SPAWN_TASK tool called by orchestrator")
				case "COMPLETE_TASK":
					s.logger.Info().Msg("COMPLETE_TASK tool called by orchestrator")
				case "FAIL_TASK":
					s.logger.Info().Msg("FAIL_TASK tool called by orchestrator")
				}
			}
		} else {
			// Log the text as internal thought/reasoning, but DO NOT send it to the user
			s.logger.Info().Str("content", msg.Content).Msg("Main Orchestrator reasoned (no tool called)")

			agentTurn := &models.ConversationTurn{
				Thought:   msg.Content,
				Action:    "THINK",
				Result:    []byte(`{}`), // No tool result to capture
				Tokens:    0,
				Timestamp: time.Now(),
			}
			_ = s.AddConversationTurn(context.Background(), sessionID, agentTurn)
		}
	}()

	return nil
}

func (s *SessionService) GetSessionByTelegramUser(ctx context.Context, telegramUserID int64) (*Session, error) {
	s.mu.RLock()
	for _, session := range s.sessions {
		if session.TelegramUserID == telegramUserID {
			s.mu.RUnlock()
			return session, nil
		}
	}
	s.mu.RUnlock()

	sessionModel, err := s.repo.GetSessionByTelegramUser(ctx, telegramUserID)
	if err != nil {
		return nil, err
	}
	if sessionModel == nil {
		return nil, nil
	}

	return s.GetSession(ctx, sessionModel.ID)
}

func (s *SessionService) UpdateSessionState(ctx context.Context, sessionID uuid.UUID, state *OrchestratorState) error {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session not found")
	}

	session.State = state
	session.LastActive = time.Now()

	stateJSON, _ := json.Marshal(state)

	sessionModel, err := s.repo.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sessionModel != nil {
		sessionModel.OrchestratorState = stateJSON
		return s.repo.UpdateSession(ctx, sessionModel)
	}

	return nil
}

func (s *SessionService) ListSessions(ctx context.Context, limit, offset int) ([]*models.Session, error) {
	return s.repo.ListSessions(ctx, limit, offset)
}

func (s *SessionService) AddConversationTurn(ctx context.Context, sessionID uuid.UUID, turn *models.ConversationTurn) error {
	s.mu.RLock()
	session, ok := s.sessions[sessionID]
	s.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session not found")
	}

	session.LastActive = time.Now()
	turn.TurnID = session.History.currentTurnID + 1

	if err := s.repo.AddConversationTurn(ctx, sessionID, turn); err != nil {
		return err
	}

	session.History.turns = append(session.History.turns, turn)
	session.History.currentTurnID++

	return nil
}

func (s *SessionService) GetConversationHistory(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.ConversationTurn, error) {
	return s.repo.GetConversationHistory(ctx, sessionID, limit, offset)
}

func NewLongTermMemoryManager(sessionID uuid.UUID, repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *LongTermMemoryManager {
	return &LongTermMemoryManager{
		sessionID: sessionID,
		repo:      repo,
		cfg:       cfg,
		logger:    logger,
	}
}

func (m *LongTermMemoryManager) ConsolidateMemories(ctx context.Context) error {
	entries, err := m.repo.ScanWorkingMemoryBySession(ctx, m.sessionID)
	if err != nil {
		return err
	}

	frequentEntries := filterFrequentEntries(entries, 3)

	for _, entry := range frequentEntries {
		var value string
		if err := json.Unmarshal(entry.Value, &value); err != nil {
			continue
		}

		embedding, err := m.generateEmbedding(ctx, value)
		if err != nil {
			continue
		}

		mem := &models.LongTermMemory{
			ID:        uuid.New(),
			SessionID: m.sessionID,
			Tier:      models.LTTier1Raw,
			Content:   value,
			Embedding: embedding,
		}

		if err := m.repo.CreateLongTermMemory(ctx, mem); err != nil {
			m.logger.Error().Err(err).Str("key", entry.Key).Msg("failed to store LTM")
		}
	}

	return nil
}

func (m *LongTermMemoryManager) generateEmbedding(ctx context.Context, text string) ([]float32, error) {
	return make([]float32, m.cfg.LLM.EmbeddingDims), nil
}

func filterFrequentEntries(entries []*models.WorkingMemoryEntry, minAccess int) []*models.WorkingMemoryEntry {
	return entries
}

func NewConversationHistoryManager(sessionID uuid.UUID, repo *repository.Repository, cfg *config.ContextConfig) *ConversationHistoryManager {
	return &ConversationHistoryManager{
		sessionID:    sessionID,
		repo:         repo,
		turns:        make([]*models.ConversationTurn, 0),
		currentTurnID: 0,
		cfg:          cfg,
	}
}

func (h *ConversationHistoryManager) ShouldCompact(totalTokens int) bool {
	threshold := int(float64(h.cfg.MaxTokens) * h.cfg.CompactionThreshold)
	return totalTokens > threshold
}

func (h *ConversationHistoryManager) GetRecentTurns(count int) []*models.ConversationTurn {
	start := len(h.turns) - count
	if start < 0 {
		start = 0
	}
	return h.turns[start:]
}
