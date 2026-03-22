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

	rc := &RuntimeCoordinator{
		config:  cfg,
		logger:  logger,
		db:      db,
		repo:    repo,
		stopCh:  make(chan struct{}),
		blockedAgents: make(map[string]*blockedAgentInfo),
	}

	// Load or seed config
	if err := rc.loadOrSeedRuntimeConfig(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load/seed runtime config: %w", err)
	}

	llmSvc := llm.NewLLMService(&cfg.RuntimeSeed.LLM, logger)

	rc.llmSvc = llmSvc

	if err := rc.initializeServices(); err != nil {
		return nil, fmt.Errorf("failed to initialize services: %w", err)
	}

	return rc, nil
}

func (rc *RuntimeCoordinator) loadOrSeedRuntimeConfig(ctx context.Context) error {
	// The user request:
	// "runtime config should be loaded from the config file if it exists, failing back to looking up database"
	// "you should also init/update the configuration options of the database from the config file if it exists."

	// Because viper has parsed the local catgirl.conf file into `rc.config.RuntimeSeed` with defaults,
	// and we want it to be the primary source of truth over the DB *if* it exists,
	// we will always update the database with what was read from the config file on boot.

	rc.logger.Info().Msg("synchronizing catgirl.conf settings with database configuration")
	if err := rc.repo.UpdateRuntimeConfig(ctx, &rc.config.RuntimeSeed, "system_sync"); err != nil {
		return fmt.Errorf("failed to sync config to db: %w", err)
	}

	return nil
}

func (rc *RuntimeCoordinator) initializeServices() error {
	taskQueue := task.NewGlobalTaskQueue(rc.repo, &rc.config.RuntimeSeed, rc.logger)
	rc.taskQueue = taskQueue
	rc.taskService = task.NewTaskService(rc.repo, taskQueue, &rc.config.RuntimeSeed, rc.logger)

	agentPool := agent.NewAgentPool(rc.repo, rc.taskService, &rc.config.RuntimeSeed, rc.logger)
	rc.agentPool = agentPool

	sessionSvc := NewSessionService(rc.repo, &rc.config.RuntimeSeed, rc.logger, rc.llmSvc, rc.taskService)
	rc.sessionSvc = sessionSvc

	authSvc := auth.NewAuthService(&rc.config.RuntimeSeed.Auth, rc.logger)
	rc.authSvc = authSvc

	snapshotSvc := snapshot.NewSnapshotService(rc.repo, &rc.config.RuntimeSeed.Snapshot, rc.logger)
	rc.snapshotSvc = snapshotSvc

	ragSvc := rag.NewRAGService(rc.repo, rc.llmSvc, &rc.config.RuntimeSeed.RAG, rc.logger)
	rc.ragSvc = ragSvc

	telegramSvc, err := telegram.NewTelegramService(&rc.config.RuntimeSeed.Telegram, rc.repo, rc.sessionSvc, rc.logger)
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
	workerCount := rc.config.RuntimeSeed.AgentPool.MinAgents

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

	tf, err := rc.repo.GetTaskFamily(ctx, taskInstance.TaskID)
	if err != nil || tf == nil {
		logger.Error().Err(err).Msg("Failed to get task family")
		return fmt.Errorf("failed to get task family")
	}

	agent.SetAgentServices(workerAgent, rc.repo, tf.SessionID, rc.llmSvc, &rc.config.RuntimeSeed)

	// Get session context
	session, err := rc.sessionSvc.GetSession(ctx, tf.SessionID)
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
			agentPrompt := session.Settings.AgentSystemPrompt
			if agentPrompt == "" {
				agentPrompt = rc.config.RuntimeSeed.LLM.DefaultAgentSystemPrompt
			}
			msg.Content = fmt.Sprintf(agentPrompt, input.Content)

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
		allTools, err := LoadToolsFromDB(ctx, rc.repo)
		tools := []llm.Tool{}
		if err == nil {
			allowedTools := session.Settings.AllowedAgentTools
			if len(allowedTools) == 0 {
				allowedTools = rc.config.RuntimeSeed.LLM.DefaultAgentTools
			}

			// Filter based on allowed tools
			for _, t := range allTools {
				for _, allowed := range allowedTools {
					if t.Function.Name == allowed {
						tools = append(tools, t)
						break
					}
				}
			}
		}

		if len(tools) == 0 {
			logger.Warn().Msg("No tools available for agent")
		}

		// Call LLM
		model := rc.llmSvc.GetRandomGPModel(session.Settings.GPModel)
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

				case "SEND_PARENT":
					msgText, _ := args["message"].(string)
					if msgText != "" && taskInstance.ParentInstanceID != nil {
						// Need to find the agent handling the parent instance
						// For this, we'll iterate through our blocked agents
						rc.blockedAgentsMu.RLock()
						var parentAgentID string
						for agentID, info := range rc.blockedAgents {
							if info.TaskInstance.InstanceID == *taskInstance.ParentInstanceID {
								parentAgentID = agentID
								break
							}
						}
						rc.blockedAgentsMu.RUnlock()

						if parentAgentID != "" {
							// Formulate a tool result for the parent that looks like it came from the child
							toolResultStr := fmt.Sprintf(`Child Task ID: %s
Task Description: %s
Message: %s`, taskInstance.InstanceID.String(), taskInstance.Description, msgText)
							// The child sends this as an async input to the parent
							err := rc.SendToolResultToAgent(parentAgentID, "", "ASYNC_CHILD_MESSAGE", toolResultStr)
							if err == nil {
								toolResult = `{"success": true}`
							} else {
								toolResult = fmt.Sprintf(`{"success": false, "error": "failed to send to parent: %s"}`, err.Error())
							}
						} else {
							// If parent is not blocked/active, it might be the Orchestrator itself
							// or an agent that has died.
							// Assuming orchestrator for now if parent agent not found
							rc.sessionSvc.HandleUserMessage(ctx, session.ID, session.TelegramUserID, fmt.Sprintf("Child Task ID: %s\nTask Description: %s\nMessage: %s", taskInstance.InstanceID.String(), taskInstance.Description, msgText))
							toolResult = `{"success": true}`
						}
					} else if msgText != "" {
						// Send to orchestrator if no parent
						rc.sessionSvc.HandleUserMessage(ctx, session.ID, session.TelegramUserID, fmt.Sprintf("Root Task ID: %s\nTask Description: %s\nMessage: %s", taskInstance.InstanceID.String(), taskInstance.Description, msgText))
						toolResult = `{"success": true}`
					} else {
						toolResult = `{"success": false, "error": "message is required"}`
					}

				case "SPAWN_TASK":
					desc, _ := args["description"].(string)
					if desc != "" {
						// Note: We need to figure out the depth of the parent task here
						// Assuming we can derive it from the context or we might need to fetch the parent task
						// For now, let's query the DB for the parent task depth
						parentDepth := 0
						if tf, _ := rc.repo.GetTaskFamily(ctx, taskInstance.TaskID); tf != nil {
							// For simplicity, we just use the MaxDepthReached as an approximation,
							// but properly we should fetch the parent TaskInstance itself
							// Actually, `taskInstance` IS the parent in this context
							parentTask, err := rc.repo.GetTaskInstance(ctx, taskInstance.InstanceID)
							if err == nil && parentTask != nil {
								// We'll need to count parents manually if depth isn't cached
								// Let's implement a quick depth counter here
								current := parentTask
								for current.ParentInstanceID != nil {
									parentDepth++
									parent, _ := rc.repo.GetTaskInstance(ctx, *current.ParentInstanceID)
									if parent == nil {
										break
									}
									current = parent
								}
							}
						}

						subTask, err := rc.taskService.SpawnSubTask(ctx, taskInstance, desc, models.AgentTypeGeneralPurpose, models.PriorityNormal, parentDepth)
						if err != nil {
							toolResult = fmt.Sprintf(`{"success": false, "error": "failed to spawn task: %s"}`, err.Error())
						} else {
							toolResult = fmt.Sprintf(`{"success": true, "task_id": "%s"}`, subTask.InstanceID.String())
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
		ltm := NewLongTermMemoryManager(session.ID, rc.repo, &rc.config.RuntimeSeed, rc.logger)
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
