package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/hulipa487/catgirl/internal/services/task"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type AgentPool struct {
	repo     *repository.Repository
	taskSvc  *task.TaskService
	config   *config.AgentPoolConfig
	logger   zerolog.Logger
	mu       sync.RWMutex
	agents   map[string]*WorkerAgent
	stopCh   chan struct{}
}

type WorkerAgent struct {
	ID          string
	Type        models.AgentType
	Status      models.AgentStatus
	CurrentTask *models.TaskInstance
	LastActive  *time.Time
	mu          sync.Mutex

	// Input queue for async callbacks and communications
	InputQueue chan *AgentInput

	// Model output history (conversation history)
	OutputHistory []AgentMessage

	// Pending async tool calls waiting for callback
	PendingToolCalls map[string]*PendingToolCall

	// Agent state: "free" or "blocking"
	State string

	repo    *repository.Repository
	taskSvc *task.TaskService
	config  *config.Config
	logger  zerolog.Logger
	memory  *WorkingMemoryService
	ltm     *LongTermMemoryService
	billing *BillingService

	stopCh chan struct{}
}

// AgentInput represents an input to the worker agent
type AgentInput struct {
	Type    string      // "tool_result", "message", "signal"
	Content string      // content or result
	ToolID  string      // for tool_result type
	ToolName string     // name of the tool
}

// AgentMessage represents a message in the output history
type AgentMessage struct {
	Role       string   // "user", "assistant", "tool"
	Content    string   // message content
	ToolCalls  []ToolCallInfo
	ToolCallID string   // for tool messages
	Timestamp  time.Time
}

// ToolCallInfo represents a tool call from the model
type ToolCallInfo struct {
	ID       string
	Name     string
	Arguments string
}

const (
	AgentStateBlocking = "BLOCKING"
	AgentStateFree     = "FREE"
	AgentStateCompleted = "COMPLETED"
	AgentStateFailed   = "FAILED"
)

// PendingToolCall represents a tool call awaiting callback
type PendingToolCall struct {
	ID        string
	Name      string
	Arguments string
	Timestamp time.Time
}

func NewAgentPool(repo *repository.Repository, taskSvc *task.TaskService, cfg *config.Config, logger zerolog.Logger) *AgentPool {
	ap := &AgentPool{
		repo:    repo,
		taskSvc: taskSvc,
		config:  &cfg.AgentPool,
		logger:  logger,
		agents:  make(map[string]*WorkerAgent),
		stopCh:  make(chan struct{}),
	}

	ap.startIdleCheckLoop()

	return ap
}

func (ap *AgentPool) startIdleCheckLoop() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ap.cleanupIdleAgents()
			case <-ap.stopCh:
				return
			}
		}
	}()
}

func (ap *AgentPool) cleanupIdleAgents() {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	for _, agent := range ap.agents {
		if agent.Status == models.AgentStatusIdle {
			agent.mu.Lock()
			if agent.LastActive != nil && time.Since(*agent.LastActive) > time.Duration(ap.config.IdleTimeoutSecs)*time.Second {
				agent.stopCh <- struct{}{}
			}
			agent.mu.Unlock()
		}
	}
}

func (ap *AgentPool) SpawnAgent(ctx context.Context, agentType models.AgentType) (*WorkerAgent, error) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if len(ap.agents) >= ap.config.MaxAgents {
		return nil, fmt.Errorf("agent pool at maximum capacity")
	}

	agentID := fmt.Sprintf("%s_%s", agentType, uuid.New().String()[:8])
	now := time.Now()

	agent := &WorkerAgent{
		ID:                agentID,
		Type:              agentType,
		Status:            models.AgentStatusIdle,
		LastActive:        &now,
		stopCh:            make(chan struct{}),
		InputQueue:        make(chan *AgentInput, 100), // buffered channel for async inputs
		OutputHistory:     make([]AgentMessage, 0),
		PendingToolCalls:  make(map[string]*PendingToolCall),
		State:             AgentStateFree,
	}

	if err := ap.repo.CreateAgent(ctx, &models.Agent{
		ID:           agent.ID,
		Type:         agent.Type,
		Status:       agent.Status,
		CreatedAt:    time.Now(),
		LastActiveAt: &now,
	}); err != nil {
		return nil, fmt.Errorf("failed to persist agent: %w", err)
	}

	ap.agents[agentID] = agent

	ap.logger.Info().
		Str("agent_id", agent.ID).
		Str("type", string(agent.Type)).
		Msg("agent spawned")

	return agent, nil
}

func (ap *AgentPool) AssignTask(ctx context.Context, agentID string, taskInstance *models.TaskInstance) error {
	ap.mu.RLock()
	agent, ok := ap.agents[agentID]
	ap.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	agent.CurrentTask = taskInstance
	agent.Status = models.AgentStatusBusy

	now := time.Now()
	agent.LastActive = &now
	if err := ap.repo.UpdateAgent(ctx, &models.Agent{
		ID:                agent.ID,
		Status:            agent.Status,
		CurrentInstanceID: &taskInstance.InstanceID,
		LastActiveAt:      &now,
	}); err != nil {
		return fmt.Errorf("failed to update agent: %w", err)
	}

	return nil
}

func (ap *AgentPool) RemoveAgent(ctx context.Context, agentID string) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	agent, ok := ap.agents[agentID]
	if !ok {
		return fmt.Errorf("agent not found: %s", agentID)
	}

	agent.Status = models.AgentStatusRemoved
	close(agent.stopCh)

	delete(ap.agents, agentID)

	if err := ap.repo.DeleteAgent(ctx, agentID); err != nil {
		ap.logger.Error().Err(err).Str("agent_id", agentID).Msg("failed to delete agent from db")
	}

	ap.logger.Info().Str("agent_id", agentID).Msg("agent removed")
	return nil
}

func (ap *AgentPool) GetAgent(agentID string) (*WorkerAgent, bool) {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	agent, ok := ap.agents[agentID]
	return agent, ok
}

func (ap *AgentPool) ListAgents() []*WorkerAgent {
	ap.mu.RLock()
	defer ap.mu.RUnlock()

	agents := make([]*WorkerAgent, 0, len(ap.agents))
	for _, agent := range ap.agents {
		agents = append(agents, agent)
	}
	return agents
}

func (ap *AgentPool) GetIdleAgent(agentType models.AgentType) *WorkerAgent {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	for _, agent := range ap.agents {
		if agent.Status == models.AgentStatusIdle && (agentType == "" || agent.Type == agentType) {
			agent.Status = models.AgentStatusBusy
			return agent
		}
	}
	return nil
}

func (ap *AgentPool) GetPoolStatus() map[string]interface{} {
	ap.mu.RLock()
	defer ap.mu.RUnlock()

	status := map[string]interface{}{
		"total":      len(ap.agents),
		"idle":       0,
		"busy":       0,
		"destroying": 0,
		"by_type":    map[string]int{},
	}

	for _, agent := range ap.agents {
		switch agent.Status {
		case models.AgentStatusIdle:
			status["idle"] = status["idle"].(int) + 1
		case models.AgentStatusBusy:
			status["busy"] = status["busy"].(int) + 1
		case models.AgentStatusDestroying:
			status["destroying"] = status["destroying"].(int) + 1
		}
		typeKey := string(agent.Type)
		status["by_type"].(map[string]int)[typeKey]++
	}

	return status
}

func (ap *AgentPool) Stop() {
	close(ap.stopCh)
}

func (ap *AgentPool) UpdateAgentLastActive(agentID string) {
	ap.mu.RLock()
	agent, ok := ap.agents[agentID]
	ap.mu.RUnlock()

	if ok {
		now := time.Now()
		agent.LastActive = &now
	}
}

func (ap *AgentPool) SetAgentBlocking(agentID string, blocking bool) {
	ap.mu.RLock()
	agent, ok := ap.agents[agentID]
	ap.mu.RUnlock()

	if ok {
		agent.mu.Lock()
		if blocking {
			agent.State = AgentStateBlocking
		} else {
			agent.State = AgentStateFree
		}
		agent.mu.Unlock()
	}
}

// ResetForNewTask resets the agent's mutex for a new task (caller should hold lock or be single-threaded)
func (agent *WorkerAgent) ResetForNewTask() {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	agent.CurrentTask = nil
	agent.State = AgentStateFree
	agent.OutputHistory = make([]AgentMessage, 0)
	agent.PendingToolCalls = make(map[string]*PendingToolCall)
}

func (agent *WorkerAgent) IsBlocking() bool {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return agent.State == AgentStateBlocking
}

func (agent *WorkerAgent) SetBlocking(blocking bool) {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	if blocking {
		agent.State = AgentStateBlocking
	} else {
		agent.State = AgentStateFree
	}
}

func (agent *WorkerAgent) IsFree() bool {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	return agent.State == AgentStateFree
}

func (agent *WorkerAgent) SendInput(input *AgentInput) bool {
	agent.mu.Lock()
	defer agent.mu.Unlock()
	select {
	case agent.InputQueue <- input:
		return true
	default:
		return false // queue full
	}
}

func (agent *WorkerAgent) GetInput() *AgentInput {
	return <-agent.InputQueue
}

type WorkingMemoryService struct {
	repo    *repository.Repository
	agentID string
}

func NewWorkingMemoryService(repo *repository.Repository, agentID string) *WorkingMemoryService {
	return &WorkingMemoryService{
		repo:    repo,
		agentID: agentID,
	}
}

func (s *WorkingMemoryService) Set(ctx context.Context, key string, value interface{}) error {
	return s.repo.SetWorkingMemory(ctx, s.agentID, key, value)
}

func (s *WorkingMemoryService) Get(ctx context.Context, key string) (interface{}, error) {
	return s.repo.GetWorkingMemory(ctx, s.agentID, key)
}

func (s *WorkingMemoryService) Delete(ctx context.Context, key string) error {
	return s.repo.DeleteWorkingMemory(ctx, s.agentID, key)
}

func (s *WorkingMemoryService) GetAll(ctx context.Context) ([]*models.WorkingMemoryEntry, error) {
	return s.repo.GetAllWorkingMemory(ctx, s.agentID)
}

type LongTermMemoryService struct {
	repo      *repository.Repository
	sessionID uuid.UUID
	llm       *llm.LLMService
	cfg       *config.Config
}

func NewLongTermMemoryService(repo *repository.Repository, sessionID uuid.UUID, llmSvc *llm.LLMService, cfg *config.Config) *LongTermMemoryService {
	return &LongTermMemoryService{
		repo:      repo,
		sessionID: sessionID,
		llm:       llmSvc,
		cfg:       cfg,
	}
}

func (s *LongTermMemoryService) Search(ctx context.Context, query string, topK int) ([]*models.LongTermMemory, error) {
	embedding, err := s.llm.EmbedOne(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	memories, err := s.repo.SearchLongTermMemory(ctx, s.sessionID, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search memory: %w", err)
	}

	for _, mem := range memories {
		s.repo.IncrementMemoryAccessCount(ctx, mem.ID)
	}

	return memories, nil
}

func (s *LongTermMemoryService) Store(ctx context.Context, content string, tier models.LTMTier, metadata map[string]interface{}) error {
	embedding, err := s.llm.EmbedOne(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to embed content: %w", err)
	}

	mem := &models.LongTermMemory{
		ID:        uuid.New(),
		SessionID: s.sessionID,
		Tier:      tier,
		Content:   content,
		Embedding: embedding,
	}

	if metadata != nil {
		metadataJSON, _ := json.Marshal(metadata)
		mem.Metadata = metadataJSON
	}

	return s.repo.CreateLongTermMemory(ctx, mem)
}

type BillingService struct {
	repo      *repository.Repository
	sessionID uuid.UUID
	userID    string
}

func NewBillingService(repo *repository.Repository, sessionID uuid.UUID, userID string) *BillingService {
	return &BillingService{
		repo:      repo,
		sessionID: sessionID,
		userID:    userID,
	}
}

func (s *BillingService) RecordUsage(ctx context.Context, taskID *uuid.UUID, operationType models.UsageOperationType, operationName string, inputTokens, outputTokens int, membershipLevel string) error {
	totalTokens := inputTokens + outputTokens
	costMultiplier := getCostMultiplier(membershipLevel)
	effectiveTokens := float64(totalTokens) * costMultiplier

	record := &models.UsageRecord{
		UsageID:          uuid.New(),
		TaskID:          taskID,
		SessionID:       s.sessionID,
		UserID:          s.userID,
		OperationType:   operationType,
		OperationName:   operationName,
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		TotalTokens:     totalTokens,
		MembershipLevel: membershipLevel,
		CostMultiplier:  costMultiplier,
		EffectiveTokens: effectiveTokens,
		Timestamp:       time.Now(),
	}

	return s.repo.CreateUsageRecord(ctx, record)
}

func getCostMultiplier(membership string) float64 {
	switch membership {
	case "free":
		return 1.0
	case "basic":
		return 0.9
	case "pro":
		return 0.7
	case "enterprise":
		return 0.5
	default:
		return 1.0
	}
}

func SetAgentServices(agent *WorkerAgent, repo *repository.Repository, sessionID uuid.UUID, llmSvc *llm.LLMService, cfg *config.Config) {
	agent.memory = NewWorkingMemoryService(repo, agent.ID)
	agent.ltm = NewLongTermMemoryService(repo, sessionID, llmSvc, cfg)
	agent.billing = NewBillingService(repo, sessionID, "")
}
