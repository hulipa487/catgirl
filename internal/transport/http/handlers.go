package http

import (
	"net/http"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/runtime"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Handlers struct {
	runtime *runtime.RuntimeCoordinator
	config  *config.Config
	logger  zerolog.Logger
}

func NewHandlers(rt *runtime.RuntimeCoordinator, cfg *config.Config, logger zerolog.Logger) *Handlers {
	return &Handlers{
		runtime: rt,
		config:  cfg,
		logger:  logger,
	}
}

func (h *Handlers) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "catgirl-runtime",
	})
}

func (h *Handlers) GetHealth(c *gin.Context) {
	ctx := c.Request.Context()
	dbHealth := h.runtime.GetRepository().Ping(ctx)

	c.JSON(http.StatusOK, gin.H{
		"status":   "ok",
		"database": dbHealth,
		"config": gin.H{
			"server_port": h.config.Server.Port,
		},
	})
}

func (h *Handlers) ListSessions(c *gin.Context) {
	ctx := c.Request.Context()
	repo := h.runtime.GetRepository()

	sessions, err := repo.ListSessions(ctx, 100, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *Handlers) GetSession(c *gin.Context) {
	sessionIDStr := c.Param("session_id")
	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	ctx := c.Request.Context()
	repo := h.runtime.GetRepository()

	session, err := repo.GetSession(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"session": session})
}

func (h *Handlers) ListTasks(c *gin.Context) {
	ctx := c.Request.Context()
	taskSvc := h.runtime.GetTaskService()

	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	tasks, err := taskSvc.ListTasksBySession(ctx, sessionID, 100, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *Handlers) GetTask(c *gin.Context) {
	instanceIDStr := c.Param("instance_id")
	instanceID, err := uuid.Parse(instanceIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instance_id"})
		return
	}

	ctx := c.Request.Context()
	taskSvc := h.runtime.GetTaskService()

	task, err := taskSvc.GetTask(ctx, instanceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"task": task})
}

func (h *Handlers) GetQueueStatus(c *gin.Context) {
	taskSvc := h.runtime.GetTaskService()
	status := taskSvc.GetQueueStatus()
	c.JSON(http.StatusOK, status)
}

func (h *Handlers) GetAgentPoolStatus(c *gin.Context) {
	agentPool := h.runtime.GetAgentPool()
	status := agentPool.GetPoolStatus()
	c.JSON(http.StatusOK, status)
}

func (h *Handlers) ListAgents(c *gin.Context) {
	agentPool := h.runtime.GetAgentPool()
	agents := agentPool.ListAgents()
	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (h *Handlers) ListSnapshots(c *gin.Context) {
	ctx := c.Request.Context()
	repo := h.runtime.GetRepository()

	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	snapshots, err := repo.ListContainerSnapshotsBySession(ctx, sessionID, 100, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"snapshots": snapshots})
}


func (h *Handlers) GetUsageSummary(c *gin.Context) {
	ctx := c.Request.Context()
	repo := h.runtime.GetRepository()

	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	summary, err := repo.GetUsageSummaryBySession(ctx, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"usage": summary})
}

func (h *Handlers) SearchMemory(c *gin.Context) {
	ctx := c.Request.Context()
	ragSvc := h.runtime.GetRAGService()

	sessionIDStr := c.Query("session_id")
	if sessionIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	sessionID, err := uuid.Parse(sessionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session_id"})
		return
	}

	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query required (q)"})
		return
	}

	topK := 5
	if topKStr := c.Query("top_k"); topKStr != "" {
		// parse top_k
	}

	memories, err := ragSvc.RetrieveMemories(ctx, sessionID, query, topK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"memories": memories})
}

func (h *Handlers) GetSystemMetrics(c *gin.Context) {
	taskSvc := h.runtime.GetTaskService()
	agentPool := h.runtime.GetAgentPool()
	repo := h.runtime.GetRepository()

	ctx := c.Request.Context()

	c.JSON(http.StatusOK, gin.H{
		"queue": taskSvc.GetQueueStatus(),
		"agents": agentPool.GetPoolStatus(),
		"database": repo.Ping(ctx),
	})
}
