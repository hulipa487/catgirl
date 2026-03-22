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
	"github.com/hulipa487/catgirl/internal/services/task"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type SessionService struct {
	repo           *repository.Repository
	config         *config.RuntimeConfig
	logger         zerolog.Logger
	llmSvc         *llm.LLMService
	taskService    *task.TaskService
	toolLoader     *ToolLoader
	contextBuilder *ContextBuilder
	sessions       map[uuid.UUID]*Session
	mu             sync.RWMutex
	OnReply        func(telegramUserID int64, message string)
}

type OrchestratorState struct {
	CurrentTask  string `json:"current_task,omitempty"`
	Progress     int    `json:"progress,omitempty"`
	PendingTasks int    `json:"pending_tasks,omitempty"`
}

type Session struct {
	ID             uuid.UUID
	TelegramUserID int64
	BotToken       string
	State          *OrchestratorState
	LTM            *LongTermMemoryManager
	History        *ConversationHistoryManager
	Context        *OrchestratorContextManager // Persistent context with token tracking
	InputQueue     chan string // Queue for async messages from telegram/subtasks
	CreatedAt      time.Time
	LastActive     time.Time
}

type LongTermMemoryManager struct {
	sessionID uuid.UUID
	repo     *repository.Repository
	cfg      *config.RuntimeConfig
	logger   zerolog.Logger
}

type ConversationHistoryManager struct {
	sessionID    uuid.UUID
	repo         *repository.Repository
	turns        []*models.ConversationTurn
	currentTurnID int
	cfg          *config.ContextConfig
}

func NewSessionService(repo *repository.Repository, cfg *config.RuntimeConfig, logger zerolog.Logger, llmSvc *llm.LLMService, taskSvc *task.TaskService, toolLoader *ToolLoader) *SessionService {
	return &SessionService{
		repo:        repo,
		config:      cfg,
		logger:      logger,
		llmSvc:      llmSvc,
		taskService: taskSvc,
		toolLoader:  toolLoader,
		sessions:    make(map[uuid.UUID]*Session),
	}
}

// SetContextBuilder sets the context builder (called after RAG service is initialized)
func (s *SessionService) SetContextBuilder(cb *ContextBuilder) {
	s.contextBuilder = cb
}

func (s *SessionService) CreateSession(ctx context.Context, telegramUserID int64, botToken string, username, firstName, lastName string) (*Session, error) {
	sessionID := uuid.New()
	now := time.Now()

	session := &models.Session{
		ID:             sessionID,
		TelegramUserID: telegramUserID,
		BotToken:       botToken,
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
		ID:             sessionID,
		TelegramUserID: telegramUserID,
		BotToken:       botToken,
		State:          &OrchestratorState{},
		History:        NewConversationHistoryManager(sessionID, s.repo, &s.config.Context),
		Context:        NewOrchestratorContextManager(sessionID, s.repo, s.logger),
		InputQueue:     make(chan string, 100),
		CreatedAt:      now,
		LastActive:     now,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	// Start the background orchestrator loop for this new session
	go s.orchestratorLoop(sess)

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
		ID:             sessionModel.ID,
		TelegramUserID: sessionModel.TelegramUserID,
		BotToken:       sessionModel.BotToken,
		State:          &state,
		History:        NewConversationHistoryManager(sessionModel.ID, s.repo, &s.config.Context),
		Context:        NewOrchestratorContextManager(sessionModel.ID, s.repo, s.logger),
		InputQueue:     make(chan string, 100), // Input queue for the session
		CreatedAt:      sessionModel.CreatedAt,
		LastActive:     sessionModel.UpdatedAt,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	// Start the background loop for the session since we just loaded it
	go s.orchestratorLoop(sess)

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

func (s *SessionService) CreateSessionForTelegramUser(ctx context.Context, telegramUserID int64, botToken string, username, firstName, lastName string) (interface{}, error) {
	session, err := s.CreateSession(ctx, telegramUserID, botToken, username, firstName, lastName)
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

	// Send message to the session's input queue
	select {
	case session.InputQueue <- message:
	default:
		s.logger.Warn().Str("session_id", sessionID.String()).Msg("Session input queue full, dropping message")
	}

	return nil
}

// orchestratorLoop is the background loop running for a session
func (s *SessionService) orchestratorLoop(session *Session) {
	ctx := context.Background()

	// Load existing context history
	if err := session.Context.LoadHistory(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("Failed to load orchestrator context history, starting fresh")
	}

	for {
		message := <-session.InputQueue

		// Wait a brief moment to ensure DB transaction finishes
		time.Sleep(100 * time.Millisecond)

		var botConfig *config.TelegramBotConfig
		for _, b := range s.config.Telegram.Bots {
			if b.BotToken == session.BotToken {
				bCopy := b
				botConfig = &bCopy
				break
			}
		}

		if botConfig == nil {
			s.logger.Error().Str("bot_token", session.BotToken).Msg("Bot config not found for session")
			// Fallback to avoid panic
			botConfig = &config.TelegramBotConfig{}
		}

		// Build context from RAG for orchestrator
		var contextStr string
		if s.contextBuilder != nil {
			ctxContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			contextStr, _ = s.contextBuilder.BuildOrchestratorContext(ctxContext, session.ID, message)
			cancel()
			if contextStr != "" {
				s.logger.Debug().Int("context_len", len(contextStr)).Msg("Injected context for orchestrator")
			}
		}

		// Store user message in persistent context
		if err := session.Context.AddUserTurn(ctx, message); err != nil {
			s.logger.Warn().Err(err).Msg("Failed to store user turn in context")
		}

		// Build messages using persistent context
		llmMessages := session.Context.BuildMessages(botConfig.OrchestratorSystemPrompt, contextStr)

		// Load tools from file-based tool loader
		tools := s.toolLoader.GetToolsByName(botConfig.AllowedOrchestratorTools)

		if len(tools) == 0 {
			s.logger.Warn().Msg("No tools loaded for orchestrator")
		}

		// Call LLM
		resp, err := s.llmSvc.ChatWithTools(ctx, s.llmSvc.GetRandomGPModel(botConfig.GPModel, s.config.LLM.Providers), llmMessages, tools, 0)

		if err != nil || len(resp.Choices) == 0 {
			s.logger.Error().Err(err).Msg("Failed to call LLM for reply")
			return
		}

		llmMsg := resp.Choices[0].Message

		// Record usage
		billingSvc := agent.NewBillingService(s.repo, session.ID, fmt.Sprintf("%d", session.TelegramUserID))
		_ = billingSvc.RecordUsage(ctx, nil, models.UsageOperationLLMCall, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, string(models.MembershipFree))

		// Process Tool Calls (or text response)
		if len(llmMsg.ToolCalls) > 0 {
			// Store assistant turn with tool calls
			if err := session.Context.AddAssistantTurn(ctx, llmMsg.Content, llmMsg.ToolCalls, resp.Usage.PromptTokens, resp.Usage.CompletionTokens); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to store assistant turn in context")
			}

			// Process each tool call
			for _, tc := range llmMsg.ToolCalls {
				s.logger.Info().Str("tool", tc.Function.Name).Str("args", tc.Function.Arguments).Msg("Main Orchestrator called tool")

				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

				var toolResult string

				switch tc.Function.Name {
				case "SEND_MESSAGE":
					if text, ok := args["message"].(string); ok {
						if s.OnReply != nil {
							s.OnReply(session.TelegramUserID, text)
						}
						toolResult = fmt.Sprintf(`{"status": "sent", "message": %s}`, tc.Function.Arguments)
					}
				case "SPAWN_TASK":
					description, _ := args["description"].(string)
					if description != "" {
						taskInstance, err := s.taskService.SpawnRootTask(
							ctx,
							session.ID,
							"orchestrator",
							description,
							models.AgentTypeGeneralPurpose,
							models.PriorityNormal,
						)
						if err != nil {
							s.logger.Error().Err(err).Msg("Failed to spawn task")
							toolResult = fmt.Sprintf(`{"success": false, "error": "failed to spawn task: %s"}`, err.Error())
						} else {
							s.logger.Info().
								Str("instance_id", taskInstance.InstanceID.String()).
								Str("task_id", taskInstance.TaskID.String()).
								Msg("Task spawned by orchestrator")
							toolResult = fmt.Sprintf(`{"success": true, "task_id": "%s"}`, taskInstance.InstanceID.String())
						}
					}
				case "SET_STATE":
					// Orchestrator doesn't use SET_STATE, just acknowledge
					toolResult = `{"status": "acknowledged"}`
				default:
					toolResult = `{"error": "unknown tool"}`
				}

				// Store tool result in persistent context
				if err := session.Context.AddToolResultTurn(ctx, tc.ID, tc.Function.Name, toolResult); err != nil {
					s.logger.Warn().Err(err).Msg("Failed to store tool result in context")
				}
			}

			// Rebuild messages with tool results for follow-up call
			llmMessages = session.Context.BuildMessages(botConfig.OrchestratorSystemPrompt, "")

			// Follow-up LLM call with tool results
			resp, err = s.llmSvc.ChatWithTools(ctx, s.llmSvc.GetRandomGPModel(botConfig.GPModel, s.config.LLM.Providers), llmMessages, tools, 0)
			if err != nil || len(resp.Choices) == 0 {
				s.logger.Error().Err(err).Msg("Failed to call LLM for follow-up")
				continue
			}

			// Record usage for follow-up call
			_ = billingSvc.RecordUsage(ctx, nil, models.UsageOperationLLMCall, resp.Model, resp.Usage.PromptTokens, resp.Usage.CompletionTokens, string(models.MembershipFree))

			llmMsg = resp.Choices[0].Message

			// Store follow-up assistant turn (internal reasoning)
			if llmMsg.Content != "" || len(llmMsg.ToolCalls) > 0 {
				if err := session.Context.AddAssistantTurn(ctx, llmMsg.Content, llmMsg.ToolCalls, resp.Usage.PromptTokens, resp.Usage.CompletionTokens); err != nil {
					s.logger.Warn().Err(err).Msg("Failed to store follow-up assistant turn in context")
				}
			}

			// Any text content after tool calls is internal reasoning only
			if llmMsg.Content != "" {
				s.logger.Info().Str("content", llmMsg.Content).Msg("Orchestrator internal reasoning after tool call")
			}
		} else {
			// Store assistant turn (internal reasoning)
			if err := session.Context.AddAssistantTurn(ctx, llmMsg.Content, nil, resp.Usage.PromptTokens, resp.Usage.CompletionTokens); err != nil {
				s.logger.Warn().Err(err).Msg("Failed to store assistant turn in context")
			}

			// Text content is internal reasoning, DO NOT send to user
			// The only way to communicate with the user is via SEND_MESSAGE tool
			if llmMsg.Content != "" {
				s.logger.Info().Str("content", llmMsg.Content).Msg("Main Orchestrator reasoned (no tool called)")
			}
		}
	}
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

func NewLongTermMemoryManager(sessionID uuid.UUID, repo *repository.Repository, cfg *config.RuntimeConfig, logger zerolog.Logger) *LongTermMemoryManager {
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
