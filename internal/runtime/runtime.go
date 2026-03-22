package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/database"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/agent"
	"github.com/hulipa487/catgirl/internal/services/auth"
	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/hulipa487/catgirl/internal/services/rag"
	"github.com/hulipa487/catgirl/internal/services/snapshot"
	"github.com/hulipa487/catgirl/internal/services/task"
	"github.com/hulipa487/catgirl/internal/services/telegram"
	"github.com/rs/zerolog"
)

type RuntimeCoordinator struct {
	config   *config.Config
	logger   zerolog.Logger
	db       *database.DB
	repo     *repository.Repository

	llmSvc      *llm.LLMService
	taskQueue   *task.GlobalTaskQueue
	taskService *task.TaskService
	agentPool   *agent.AgentPool
	sessionSvc  *SessionService
	authSvc     *auth.AuthService
	snapshotSvc *snapshot.SnapshotService
	ragSvc      *rag.RAGService
	telegramSvc *telegram.TelegramService

	workerWg   sync.WaitGroup
	stopCh     chan struct{}

	// Track blocked agents waiting for async results
	blockedAgents    map[string]*blockedAgentInfo
	blockedAgentsMu  sync.RWMutex
}

type blockedAgentInfo struct {
	TaskInstance      *models.TaskInstance
	Session          *Session
	ConversationHistory []llm.ChatMessage
	Tools           []llm.Tool
}

func NewRuntimeCoordinator(cfg *config.Config, logger zerolog.Logger) (*RuntimeCoordinator, error) {
	db, err := database.New(&cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	repo := repository.New(db)

	llmSvc := llm.NewLLMService(&cfg.LLM, logger)

	rc := &RuntimeCoordinator{
		config:  cfg,
		logger:  logger,
		db:      db,
		repo:    repo,
		llmSvc:  llmSvc,
		stopCh:  make(chan struct{}),
		blockedAgents: make(map[string]*blockedAgentInfo),
	}

	if err := rc.initializeServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	return rc, nil
}

func (rc *RuntimeCoordinator) initializeServices() error {
	taskQueue := task.NewGlobalTaskQueue(rc.repo, rc.config, rc.logger)
	rc.taskQueue = taskQueue
	rc.taskService = task.NewTaskService(rc.repo, taskQueue, rc.config, rc.logger)

	agentPool := agent.NewAgentPool(rc.repo, rc.taskService, rc.config, rc.logger)
	rc.agentPool = agentPool

	sessionSvc := NewSessionService(rc.repo, rc.config, rc.logger, rc.llmSvc, rc.taskService)
	rc.sessionSvc = sessionSvc

	authSvc := auth.NewAuthService(&rc.config.Auth, rc.logger)
	rc.authSvc = authSvc

	snapshotSvc := snapshot.NewSnapshotService(rc.repo, &rc.config.Snapshot, rc.logger)
	rc.snapshotSvc = snapshotSvc

	ragSvc := rag.NewRAGService(rc.repo, rc.llmSvc, &rc.config.RAG, rc.logger)
	rc.ragSvc = ragSvc

	telegramSvc, err := telegram.NewTelegramService(&rc.config.Telegram, rc.repo, rc.sessionSvc, rc.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize telegram service: %w", err)
	}
	rc.telegramSvc = telegramSvc

	rc.sessionSvc.OnReply = func(telegramUserID int64, message string) {
		rc.telegramSvc.SendMessage(telegramUserID, message)
	}

	return nil
}

func (rc *RuntimeCoordinator) Start(ctx context.Context) error {
	rc.logger.Info().Msg("starting runtime coordinator")

	if err := rc.startBackgroundWorkers(); err != nil {
		return fmt.Errorf("failed to start background workers: %w", err)
	}

	if err := rc.startWorkerLoop(); err != nil {
		return fmt.Errorf("failed to start worker loop: %w", err)
	}

	if err := rc.telegramSvc.SetWebhook(ctx); err != nil {
		rc.logger.Warn().Err(err).Msg("failed to set telegram webhook (continuing anyway)")
	}

	rc.logger.Info().Msg("runtime coordinator started")
	return nil
}

func (rc *RuntimeCoordinator) startBackgroundWorkers() error {
	go rc.snapshotCleanupWorker()
	go rc.memoryConsolidationWorker()
	go rc.healthMonitorWorker()
	return nil
}

func (rc *RuntimeCoordinator) startWorkerLoop() error {
	workerCount := rc.config.AgentPool.MinAgents

	for i := 0; i < workerCount; i++ {
		rc.workerWg.Add(1)
		go rc.workerLoop(i)
	}

	return nil
}

func (rc *RuntimeCoordinator) workerLoop(workerID int) {
	defer rc.workerWg.Done()

	logger := rc.logger.With().Int("worker_id", workerID).Logger()

	logger.Debug().Msg("worker started")

	for {
		select {
		case <-rc.stopCh:
			logger.Debug().Msg("worker stopping")
			return

		default:
			// Check if there's a task available
			taskInstance := rc.taskQueue.Dequeue("")
			if taskInstance == nil {
				// No tasks available, wait before trying again
				time.Sleep(1 * time.Second)
				continue
			}

			// Spawn a new agent (agents are removed after task completion)
			agent, err := rc.agentPool.SpawnAgent(context.Background(), models.AgentTypeGeneralPurpose)
			if err != nil {
				// At max capacity - re-enqueue the task and wait
				rc.taskQueue.Enqueue(taskInstance)
				logger.Debug().Err(err).Msg("could not spawn agent, re-enqueueing task and waiting...")
				time.Sleep(1 * time.Second)
				continue
			}
			logger.Info().Str("agent_id", agent.ID).Str("instance_id", taskInstance.InstanceID.String()).Msg("spawned new agent for task")

			// Execute task - blocks until agent finishes (COMPLETED/FAILED) or SendInput fails
			if err := rc.executeTask(agent, taskInstance); err != nil {
				logger.Error().Err(err).Str("instance_id", taskInstance.InstanceID.String()).Msg("task execution failed")
			}

			// Remove the agent after task completion since LLMs are stateless
			if rmErr := rc.agentPool.RemoveAgent(context.Background(), agent.ID); rmErr != nil {
				logger.Warn().Err(rmErr).Str("agent_id", agent.ID).Msg("failed to remove agent")
			}
		}
	}
}

func (rc *RuntimeCoordinator) executeTask(workerAgent *agent.WorkerAgent, taskInstance *models.TaskInstance) error {
	ctx := context.Background()

	now := time.Now()
	taskInstance.Status = models.TaskStatusInProgress
	taskInstance.AssignedAgentID = &workerAgent.ID
	taskInstance.StartedAt = &now
	rc.repo.UpdateTaskInstance(ctx, taskInstance)

	logger := rc.logger.With().
		Str("instance_id", taskInstance.InstanceID.String()).
		Str("agent_id", workerAgent.ID).
		Logger()

	logger.Info().Msg("executing task")

	agent.SetAgentServices(workerAgent, rc.repo, taskInstance.SessionID, rc.llmSvc, rc.config)

	// Get session context
	session, err := rc.sessionSvc.GetSession(ctx, taskInstance.SessionID)
	if err != nil || session == nil {
		logger.Error().Err(err).Msg("Failed to get session context for task")
		return fmt.Errorf("failed to get session context")
	}

	// Reset agent state for new task
	workerAgent.ResetForNewTask()
	workerAgent.CurrentTask = taskInstance
	workerAgent.State = agent.AgentStateFree
	workerAgent.OutputHistory = make([]agent.AgentMessage, 0)

	// Load tools from database
	tools, err := LoadToolsFromDB(ctx, rc.repo)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load tools from database")
		return err
	}

	// Register this agent as available for async callbacks
	rc.registerAgentForCallback(workerAgent.ID, taskInstance, session, tools)

	// Send initial task input to the agent
	initialInput := &agent.AgentInput{
		Type:    "task_start",
		Content: taskInstance.Description,
	}
	if !workerAgent.SendInput(initialInput) {
		logger.Error().Msg("Failed to send initial input to agent (queue full)")
		return fmt.Errorf("agent queue full")
	}

	// Run the agent's main loop - this blocks until agent signals "free" state
	return rc.runAgentLoop(workerAgent, taskInstance, session, logger)
}

// runAgentLoop processes inputs from the agent's queue and calls the LLM
func (rc *RuntimeCoordinator) runAgentLoop(workerAgent *agent.WorkerAgent, taskInstance *models.TaskInstance, session *Session, logger zerolog.Logger) error {
	ctx := context.Background()
	now := time.Now()

	// Main agent loop - processes inputs until state is "free"
	for {
		// Wait for input from the queue
		input := <-workerAgent.InputQueue
		logger.Debug().Str("input_type", input.Type).Msg("Agent received input")

		// Build message from input
		var msg agent.AgentMessage
		msg.Timestamp = time.Now()

		switch input.Type {
		case "task_start":
			// Initial task description
			msg.Role = "user"
			msg.Content = fmt.Sprintf("You are an autonomous worker agent. Your task is: %s\nUse tools to accomplish this task. Use SET_STATE with BLOCKING when waiting for async results. When done, use SET_STATE with COMPLETED or FAILED.", input.Content)

		case "tool_result":
			// Async tool callback result
			msg.Role = "tool"
			msg.ToolCallID = input.ToolID
			msg.Content = input.Content

		case "message":
			// Direct message input
			msg.Role = "user"
			msg.Content = input.Content

		default:
			logger.Warn().Str("input_type", input.Type).Msg("Unknown input type")
			continue
		}

		// Add to output history
		workerAgent.OutputHistory = append(workerAgent.OutputHistory, msg)

		// Convert output history to LLM messages
		llmMessages := rc.convertToLLMMessages(workerAgent.OutputHistory)

		// Load tools
		tools, err := LoadToolsFromDB(ctx, rc.repo)
		if err != nil || len(tools) == 0 {
			logger.Warn().Msg("No tools available")
			tools = []llm.Tool{}
		}

		// Call LLM
		model := rc.llmSvc.GetRandomGPModel()
		resp, err := rc.llmSvc.ChatWithTools(ctx, model, llmMessages, tools, 0)
		if err != nil || len(resp.Choices) == 0 {
			logger.Error().Err(err).Msg("LLM call failed")
			return err
		}

		llmMsg := resp.Choices[0].Message

		// Add LLM response to output history
		assistantMsg := agent.AgentMessage{
			Role:     "assistant",
			Content:  llmMsg.Content,
			Timestamp: time.Now(),
		}
		for _, tc := range llmMsg.ToolCalls {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, agent.ToolCallInfo{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}
		workerAgent.OutputHistory = append(workerAgent.OutputHistory, assistantMsg)

		// Process tool calls
		if len(llmMsg.ToolCalls) > 0 {
			stateChange := "" // "", "BLOCKING", "COMPLETED", or "FAILED"

			for _, tc := range llmMsg.ToolCalls {
				logger.Info().Str("tool", tc.Function.Name).Str("args", tc.Function.Arguments).Msg("Agent called tool")

				var args map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)

				var toolResult string

				switch tc.Function.Name {
				case "SET_STATE":
					state, _ := args["state"].(string)
					result, _ := args["result"].(string)
					stateChange = state

					switch state {
					case "BLOCKING":
						workerAgent.SetBlocking(true)
						toolResult = `{"status": "ok", "state": "BLOCKING"}`
						logger.Info().Str("agent_id", workerAgent.ID).Msg("Agent state: BLOCKING")

					case "COMPLETED":
						workerAgent.SetBlocking(false)
						taskInstance.Status = models.TaskStatusCompleted
						taskInstance.CompletedAt = &now
						taskInstance.Result, _ = json.Marshal(map[string]string{"summary": result})
						rc.repo.UpdateTaskInstance(ctx, taskInstance)
						toolResult = `{"status": "ok", "state": "COMPLETED"}`
						logger.Info().Str("agent_id", workerAgent.ID).Msg("Agent state: COMPLETED")

					case "FAILED":
						workerAgent.SetBlocking(false)
						taskInstance.Status = models.TaskStatusFailed
						taskInstance.CompletedAt = &now
						if result != "" {
							taskInstance.Error = &result
						}
						rc.repo.UpdateTaskInstance(ctx, taskInstance)
						toolResult = `{"status": "ok", "state": "FAILED"}`
						logger.Info().Str("agent_id", workerAgent.ID).Msg("Agent state: FAILED")

					default:
						toolResult = fmt.Sprintf(`{"error": "unknown state: %s"}`, state)
					}

				case "SEND_MESSAGE":
					if text, ok := args["message"].(string); ok {
						rc.sessionSvc.OnReply(session.TelegramUserID, text)
						toolResult = `{"status": "sent"}`
					}

				case "SPAWN_TASK":
					desc, _ := args["description"].(string)
					if desc != "" {
						subTask, err := rc.taskService.SpawnSubTask(ctx, taskInstance, desc, models.AgentTypeGeneralPurpose, models.PriorityNormal)
						if err != nil {
							toolResult = fmt.Sprintf(`{"error": "failed to spawn task: %s"}`, err.Error())
						} else {
							toolResult = fmt.Sprintf(`{"status": "spawned", "instance_id": "%s", "task_id": "%s"}`,
								subTask.InstanceID.String(), subTask.TaskID.String())
						}
					}

				default:
					toolResult = `{"error": "unknown tool"}`
				}

				// Add tool result to output history
				toolMsg := agent.AgentMessage{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:     toolResult,
					Timestamp:  time.Now(),
				}
				workerAgent.OutputHistory = append(workerAgent.OutputHistory, toolMsg)
			}

			// If state is BLOCKING, wait for more input
			if stateChange == "BLOCKING" {
				logger.Info().Str("agent_id", workerAgent.ID).Msg("Agent blocking, waiting for async input")
				continue
			}

			// If state is COMPLETED or FAILED, exit the loop
			if stateChange == "COMPLETED" || stateChange == "FAILED" {
				rc.unregisterAgentForCallback(workerAgent.ID)
				return nil
			}
		}

		// No tool calls or no state change to BLOCKING - exit loop
		logger.Info().Str("agent_id", workerAgent.ID).Msg("Agent exiting loop (no state change)")
		break
	}

	return nil
}

// convertToLLMMessages converts agent output history to LLM chat messages
func (rc *RuntimeCoordinator) convertToLLMMessages(history []agent.AgentMessage) []llm.ChatMessage {
	messages := []llm.ChatMessage{}

	for _, msg := range history {
		llmMsg := llm.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}

		if msg.Role == "tool" {
			llmMsg.ToolCallID = msg.ToolCallID
		}

		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				llmMsg.ToolCalls = append(llmMsg.ToolCalls, llm.ToolCall{
					ID: tc.ID,
					Function: llm.ToolCallFunction{
						Name:      tc.Name,
						Arguments: tc.Arguments,
					},
				})
			}
		}

		messages = append(messages, llmMsg)
	}

	return messages
}

func (rc *RuntimeCoordinator) snapshotCleanupWorker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			if err := rc.snapshotSvc.CleanupExpiredSnapshots(ctx); err != nil {
				rc.logger.Error().Err(err).Msg("snapshot cleanup failed")
			}
		case <-rc.stopCh:
			return
		}
	}
}

func (rc *RuntimeCoordinator) memoryConsolidationWorker() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rc.runMemoryConsolidation()
		case <-rc.stopCh:
			return
		}
	}
}

func (rc *RuntimeCoordinator) runMemoryConsolidation() {
	ctx := context.Background()
	sessions, err := rc.repo.ListSessions(ctx, 100, 0)
	if err != nil {
		rc.logger.Error().Err(err).Msg("failed to list sessions for consolidation")
		return
	}

	for _, session := range sessions {
		ltm := NewLongTermMemoryManager(session.ID, rc.repo, rc.config, rc.logger)
		if err := ltm.ConsolidateMemories(ctx); err != nil {
			rc.logger.Error().Err(err).Str("session_id", session.ID.String()).Msg("memory consolidation failed")
		}
	}
}

func (rc *RuntimeCoordinator) healthMonitorWorker() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rc.logHealthStatus()
		case <-rc.stopCh:
			return
		}
	}
}

func (rc *RuntimeCoordinator) logHealthStatus() {
	ctx := context.Background()

	dbHealth := rc.db.Health(ctx)
	taskStatus := rc.taskService.GetQueueStatus()
	agentStatus := rc.agentPool.GetPoolStatus()

	rc.logger.Info().
		Interface("database", dbHealth).
		Interface("tasks", taskStatus).
		Interface("agents", agentStatus).
		Msg("health status")
}

// registerAgentForCallback registers an agent as available for async callbacks
func (rc *RuntimeCoordinator) registerAgentForCallback(agentID string, taskInstance *models.TaskInstance, session *Session, tools []llm.Tool) {
	rc.blockedAgentsMu.Lock()
	defer rc.blockedAgentsMu.Unlock()

	rc.blockedAgents[agentID] = &blockedAgentInfo{
		TaskInstance: taskInstance,
		Session:     session,
		Tools:       tools,
	}

	rc.logger.Debug().Str("agent_id", agentID).Str("instance_id", taskInstance.InstanceID.String()).Msg("Agent registered for callback")
}

// unregisterAgentForCallback removes an agent from the callback registry
func (rc *RuntimeCoordinator) unregisterAgentForCallback(agentID string) {
	rc.blockedAgentsMu.Lock()
	defer rc.blockedAgentsMu.Unlock()

	delete(rc.blockedAgents, agentID)

	rc.logger.Debug().Str("agent_id", agentID).Msg("Agent unregistered from callback")
}

// SendToolResultToAgent sends a tool result to a blocked agent's input queue
func (rc *RuntimeCoordinator) SendToolResultToAgent(agentID string, toolID string, toolName string, result string) error {
	rc.blockedAgentsMu.RLock()
	_, exists := rc.blockedAgents[agentID]
	rc.blockedAgentsMu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s is not registered for callback", agentID)
	}

	workerAgent, ok := rc.agentPool.GetAgent(agentID)
	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	// Send input to agent's queue
	input := &agent.AgentInput{
		Type:    "tool_result",
		ToolID:  toolID,
		ToolName: toolName,
		Content: result,
	}

	if !workerAgent.SendInput(input) {
		return fmt.Errorf("failed to send input to agent %s queue", agentID)
	}

	rc.logger.Info().Str("agent_id", agentID).Str("tool_id", toolID).Msg("Sent tool result to agent")
	return nil
}

func (rc *RuntimeCoordinator) Stop() error {
	rc.logger.Info().Msg("stopping runtime coordinator")

	close(rc.stopCh)

	rc.agentPool.Stop()

	rc.workerWg.Wait()

	rc.db.Close()

	rc.logger.Info().Msg("runtime coordinator stopped")
	return nil
}

func (rc *RuntimeCoordinator) GetTaskService() *task.TaskService {
	return rc.taskService
}

func (rc *RuntimeCoordinator) GetAgentPool() *agent.AgentPool {
	return rc.agentPool
}

func (rc *RuntimeCoordinator) GetSessionService() *SessionService {
	return rc.sessionSvc
}

func (rc *RuntimeCoordinator) GetAuthService() *auth.AuthService {
	return rc.authSvc
}

func (rc *RuntimeCoordinator) GetRAGService() *rag.RAGService {
	return rc.ragSvc
}

func (rc *RuntimeCoordinator) GetRepository() *repository.Repository {
	return rc.repo
}

func (rc *RuntimeCoordinator) GetTelegramService() *telegram.TelegramService {
	return rc.telegramSvc
}
