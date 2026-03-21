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
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type SessionService struct {
	repo   *repository.Repository
	config *config.Config
	logger zerolog.Logger
	sessions map[uuid.UUID]*Session
	mu     sync.RWMutex
}

type Session struct {
	ID         uuid.UUID
	TelegramUserID int64
	Orchestrator *MainOrchestrator
	State      *OrchestratorState
	LTM        *LongTermMemoryManager
	History    *ConversationHistoryManager
	MCP        *MCPSessionClient
	Skills     *SkillSessionClient
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
}

type ConversationHistoryManager struct {
	sessionID    uuid.UUID
	repo         *repository.Repository
	turns        []*models.ConversationTurn
	currentTurnID int
	cfg          *config.ContextConfig
}

type MCPSessionClient struct {
	sessionID uuid.UUID
	servers   map[string]*models.MCPServer
}

type SkillSessionClient struct {
	sessionID uuid.UUID
	skills    map[string]*models.Skill
}

func NewSessionService(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *SessionService {
	return &SessionService{
		repo:     repo,
		config:   cfg,
		logger:   logger,
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
		CreatedAt:       sessionModel.CreatedAt,
		LastActive:      sessionModel.UpdatedAt,
	}

	s.mu.Lock()
	s.sessions[sessionID] = sess
	s.mu.Unlock()

	return sess, nil
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

func NewLongTermMemoryManager(sessionID uuid.UUID, repo *repository.Repository, cfg *config.Config) *LongTermMemoryManager {
	return &LongTermMemoryManager{
		sessionID: sessionID,
		repo:      repo,
		cfg:       cfg,
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
	return make([]float32, m.cfg.RAG.EmbeddingDims), nil
}

func filterFrequentEntries(entries []*models.WorkingMemoryEntry, minAccess int) []*models.WorkingMemoryEntry {
	return entries
}

type ConversationHistoryManager struct {
	sessionID    uuid.UUID
	repo         *repository.Repository
	turns        []*models.ConversationTurn
	currentTurnID int
	cfg          *config.ContextConfig
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
