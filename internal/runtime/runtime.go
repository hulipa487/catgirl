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
	"github.com/hulipa487/catgirl/internal/services/mcp"
	"github.com/hulipa487/catgirl/internal/services/rag"
	"github.com/hulipa487/catgirl/internal/services/skill"
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
	mcpSvc      *mcp.MCPService
	skillSvc    *skill.SkillService
	snapshotSvc *snapshot.SnapshotService
	ragSvc      *rag.RAGService
	telegramSvc *telegram.TelegramService

	workerWg   sync.WaitGroup
	stopCh     chan struct{}
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

	sessionSvc := NewSessionService(rc.repo, rc.config, rc.logger)
	rc.sessionSvc = sessionSvc

	authSvc := auth.NewAuthService(&rc.config.Auth, rc.logger)
	rc.authSvc = authSvc

	mcpSvc := mcp.NewMCPService(rc.repo, rc.config, rc.logger)
	rc.mcpSvc = mcpSvc

	skillSvc := skill.NewSkillService(rc.repo, rc.config, rc.logger)
	rc.skillSvc = skillSvc

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
			agent := rc.agentPool.GetIdleAgent("")
			if agent == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			taskInstance := rc.taskQueue.Dequeue("")
			if taskInstance == nil {
				time.Sleep(1 * time.Second)
				continue
			}

			logger.Debug().
				Str("instance_id", taskInstance.InstanceID.String()).
				Str("agent_id", agent.ID).
				Msg("worker picked up task")

			if err := rc.executeTask(agent, taskInstance); err != nil {
				logger.Error().Err(err).Str("instance_id", taskInstance.InstanceID.String()).Msg("task execution failed")
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

	result := map[string]interface{}{
		"status":  "completed",
		"message": "Task executed successfully",
	}

	taskInstance.Status = models.TaskStatusCompleted
	taskInstance.CompletedAt = &now
	taskInstance.Result, _ = json.Marshal(result)

	rc.repo.UpdateTaskInstance(ctx, taskInstance)

	if rc.config.Snapshot.Enabled {
		_, err := rc.snapshotSvc.CreateSnapshot(ctx, taskInstance, models.SnapshotReasonCompleted)
		if err != nil {
			logger.Warn().Err(err).Msg("failed to create snapshot")
		}
	}

	logger.Info().Msg("task completed")
	return nil
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
