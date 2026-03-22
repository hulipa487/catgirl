package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type PriorityQueue struct {
	mu    sync.RWMutex
	items []*models.TaskInstance
	maxSize int
}

func NewPriorityQueue(maxSize int) *PriorityQueue {
	return &PriorityQueue{
		items:    make([]*models.TaskInstance, 0, maxSize),
		maxSize:  maxSize,
	}
}

func (pq *PriorityQueue) Enqueue(task *models.TaskInstance) error {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if pq.len() >= pq.maxSize {
		return fmt.Errorf("queue is full")
	}

	task.PriorityScore = CalculatePriorityScore(task, 0)
	pq.items = append(pq.items, task)
	pq.sortDesc()
	return nil
}

func (pq *PriorityQueue) Dequeue(agentType models.AgentType) *models.TaskInstance {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, task := range pq.items {
		if task.AgentType == agentType || agentType == "" {
			pq.items = append(pq.items[:i], pq.items[i+1:]...)
			return task
		}
	}

	if len(pq.items) > 0 {
		task := pq.items[0]
		pq.items = pq.items[1:]
		return task
	}

	return nil
}

func (pq *PriorityQueue) Peek() *models.TaskInstance {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	if len(pq.items) == 0 {
		return nil
	}
	return pq.items[0]
}

func (pq *PriorityQueue) Remove(instanceID uuid.UUID) bool {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	for i, task := range pq.items {
		if task.InstanceID == instanceID {
			pq.items = append(pq.items[:i], pq.items[i+1:]...)
			return true
		}
	}
	return false
}

func (pq *PriorityQueue) Size() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return pq.len()
}

func (pq *PriorityQueue) len() int {
	return len(pq.items)
}

func (pq *PriorityQueue) sortDesc() {
	sort.Slice(pq.items, func(i, j int) bool {
		if pq.items[i].PriorityScore != pq.items[j].PriorityScore {
			return pq.items[i].PriorityScore > pq.items[j].PriorityScore
		}
		return pq.items[i].CreatedAt.Before(pq.items[j].CreatedAt)
	})
}

func (pq *PriorityQueue) GetAll() []*models.TaskInstance {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	result := make([]*models.TaskInstance, len(pq.items))
	copy(result, pq.items)
	return result
}

func (pq *PriorityQueue) GetBySession(sessionID uuid.UUID) []*models.TaskInstance {
	pq.mu.RLock()
	defer pq.mu.RUnlock()

	var result []*models.TaskInstance
	for _, task := range pq.items {
		if task.SessionID == sessionID {
			result = append(result, task)
		}
	}
	return result
}

type GlobalTaskQueue struct {
	pq         *PriorityQueue
	repo       *repository.Repository
	config     *config.Config
	logger     zerolog.Logger
	mu         sync.RWMutex
	enqueueCh  chan *models.TaskInstance
	dequeueCh  chan *DequeueRequest
}

type DequeueRequest struct {
	AgentType models.AgentType
	ResultCh  chan *models.TaskInstance
}

func NewGlobalTaskQueue(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *GlobalTaskQueue {
	gtq := &GlobalTaskQueue{
		pq:         NewPriorityQueue(cfg.Global.MaxQueueSize),
		repo:       repo,
		config:     cfg,
		logger:     logger,
		enqueueCh:  make(chan *models.TaskInstance, 100),
		dequeueCh:  make(chan *DequeueRequest, 100),
	}

	go gtq.processLoop()
	return gtq
}

func (gtq *GlobalTaskQueue) processLoop() {
	for {
		select {
		case task := <-gtq.enqueueCh:
			if err := gtq.pq.Enqueue(task); err != nil {
				gtq.logger.Error().Err(err).Str("instance_id", task.InstanceID.String()).Msg("failed to enqueue task")
			} else {
				gtq.logger.Debug().Str("instance_id", task.InstanceID.String()).Int("queue_size", gtq.pq.Size()).Msg("task enqueued")
			}

		case req := <-gtq.dequeueCh:
			task := gtq.pq.Dequeue(req.AgentType)
			req.ResultCh <- task

		case <-time.After(1 * time.Second):
			gtq.rebalancePriorities()
		}
	}
}

func (gtq *GlobalTaskQueue) rebalancePriorities() {
	gtq.mu.Lock()
	defer gtq.mu.Unlock()
}

func (gtq *GlobalTaskQueue) Enqueue(task *models.TaskInstance) {
	gtq.enqueueCh <- task
}

func (gtq *GlobalTaskQueue) Dequeue(agentType models.AgentType) *models.TaskInstance {
	req := &DequeueRequest{
		AgentType: agentType,
		ResultCh:  make(chan *models.TaskInstance, 1),
	}
	gtq.dequeueCh <- req
	return <-req.ResultCh
}

func (gtq *GlobalTaskQueue) Size() int {
	return gtq.pq.Size()
}

func (gtq *GlobalTaskQueue) GetAllTasks() []*models.TaskInstance {
	return gtq.pq.GetAll()
}

func (gtq *GlobalTaskQueue) GetTasksBySession(sessionID uuid.UUID) []*models.TaskInstance {
	return gtq.pq.GetBySession(sessionID)
}

func (gtq *GlobalTaskQueue) Remove(instanceID uuid.UUID) bool {
	return gtq.pq.Remove(instanceID)
}

func CalculatePriorityScore(task *models.TaskInstance, sessionBoost float64) float64 {
	basePriority := float64(0)
	switch task.Priority {
	case models.PriorityLow:
		basePriority = 0
	case models.PriorityNormal:
		basePriority = 1
	case models.PriorityHigh:
		basePriority = 2
	case models.PriorityCritical:
		basePriority = 3
	}

	ageMinutes := time.Since(task.CreatedAt).Minutes()
	ageBoost := min(ageMinutes/60, 1)

	return basePriority + sessionBoost + ageBoost
}

func CalculateMembershipBoost(membership models.MembershipLevel) float64 {
	switch membership {
	case models.MembershipFree:
		return 0
	case models.MembershipBasic:
		return 0.5
	case models.MembershipPro:
		return 1.0
	case models.MembershipEnterprise:
		return 2.0
	default:
		return 0
	}
}

func (gtq *GlobalTaskQueue) GetQueueStatus() map[string]interface{} {
	allTasks := gtq.pq.GetAll()

	byPriority := make(map[string]int)
	byAgentType := make(map[string]int)

	for _, task := range allTasks {
		byPriority[string(task.Priority)]++
		byAgentType[string(task.AgentType)]++
	}

	return map[string]interface{}{
		"total_tasks":   len(allTasks),
		"by_priority":   byPriority,
		"by_agent_type": byAgentType,
	}
}

type TaskService struct {
	repo       *repository.Repository
	queue      *GlobalTaskQueue
	config     *config.Config
	logger     zerolog.Logger
}

func NewTaskService(repo *repository.Repository, queue *GlobalTaskQueue, cfg *config.Config, logger zerolog.Logger) *TaskService {
	return &TaskService{
		repo:   repo,
		queue:  queue,
		config: cfg,
		logger: logger,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, task *models.TaskInstance, ownerID string, depth int) error {
	if task.InstanceID == uuid.Nil {
		task.InstanceID = uuid.New()
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	if task.Status == "" {
		task.Status = models.TaskStatusPending
	}
	if task.PriorityScore == 0 {
		task.PriorityScore = CalculatePriorityScore(task, 0)
	}

	if err := s.repo.CreateTaskInstance(ctx, task); err != nil {
		return fmt.Errorf("failed to create task instance: %w", err)
	}

	if depth > s.config.Global.MaxTaskDepth {
		return fmt.Errorf("task depth %d exceeds maximum %d", depth, s.config.Global.MaxTaskDepth)
	}

	s.queue.Enqueue(task)

	tf, err := s.repo.GetTaskFamily(ctx, task.TaskID)
	if err != nil {
		return fmt.Errorf("failed to get task family: %w", err)
	}
	if tf == nil {
		return fmt.Errorf("task family not found for task %s", task.TaskID)
	}

	channel := &models.TaskOwnerChannel{
		ChannelID:      task.InstanceID,
		TaskInstanceID: task.InstanceID,
		OwnerID:        ownerID,
		CreatedAt:      time.Now(),
		LastActivity:   time.Now(),
	}
	if err := s.repo.CreateTaskOwnerChannel(ctx, channel); err != nil {
		s.logger.Error().Err(err).Str("instance_id", task.InstanceID.String()).Msg("failed to create task channel")
	}

	s.logger.Info().
		Str("instance_id", task.InstanceID.String()).
		Str("task_id", task.TaskID.String()).
		Str("session_id", tf.SessionID.String()).
		Str("agent_type", string(task.AgentType)).
		Msg("task created and enqueued")

	return nil
}

func (s *TaskService) GetTask(ctx context.Context, instanceID uuid.UUID) (*models.TaskInstance, error) {
	return s.repo.GetTaskInstance(ctx, instanceID)
}

func (s *TaskService) UpdateTaskStatus(ctx context.Context, instanceID uuid.UUID, status models.TaskStatus, result interface{}, errMsg *string) error {
	task, err := s.repo.GetTaskInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", instanceID)
	}

	task.Status = status
	if status == models.TaskStatusCompleted || status == models.TaskStatusFailed {
		now := time.Now()
		task.CompletedAt = &now
		if result != nil {
			resultJSON, _ := toJSON(result)
			task.Result = resultJSON
		}
		if errMsg != nil {
			task.Error = errMsg
		}
	}
	if status == models.TaskStatusInProgress && task.StartedAt == nil {
		now := time.Now()
		task.StartedAt = &now
	}

	return s.repo.UpdateTaskInstance(ctx, task)
}

func (s *TaskService) AssignTask(ctx context.Context, instanceID uuid.UUID, agentID string) error {
	task, err := s.repo.GetTaskInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", instanceID)
	}

	agentIDStr := agentID
	task.AssignedAgentID = &agentIDStr
	task.Status = models.TaskStatusAssigned

	return s.repo.UpdateTaskInstance(ctx, task)
}

func (s *TaskService) SpawnSubTask(ctx context.Context, parent *models.TaskInstance, description string, agentType models.AgentType, priority models.Priority, depth int) (*models.TaskInstance, error) {
	newDepth := depth + 1
	if newDepth > s.config.Global.MaxTaskDepth {
		return nil, fmt.Errorf("sub-task depth %d exceeds maximum %d", newDepth, s.config.Global.MaxTaskDepth)
	}

	task := &models.TaskInstance{
		InstanceID:       uuid.New(),
		TaskID:           parent.TaskID,
		Description:      description,
		AgentType:        agentType,
		Status:           models.TaskStatusPending,
		Priority:         priority,
		ParentInstanceID: &parent.InstanceID,
		CreatedAt:        time.Now(),
	}

	if err := s.CreateTask(ctx, task, parent.InstanceID.String(), newDepth); err != nil {
		return nil, err
	}

	tf, _ := s.repo.GetTaskFamily(ctx, parent.TaskID)
	if tf != nil && newDepth > tf.MaxDepthReached {
		tf.MaxDepthReached = newDepth
		s.repo.UpdateTaskFamily(ctx, tf)
	}

	return task, nil
}

// SpawnRootTask creates a new task family and enqueues a root task instance
func (s *TaskService) SpawnRootTask(ctx context.Context, sessionID uuid.UUID, ownerID string, description string, agentType models.AgentType, priority models.Priority) (*models.TaskInstance, error) {
	taskID := uuid.New()
	now := time.Now()

	// Create task family
	tf := &models.TaskFamily{
		TaskID:           taskID,
		SessionID:        sessionID,
		RootDescription:  description,
		Status:           "in_progress",
		MaxDepthReached:  0,
		CreatedAt:        now,
	}

	if err := s.repo.CreateTaskFamily(ctx, tf); err != nil {
		return nil, fmt.Errorf("failed to create task family: %w", err)
	}

	// Create root task instance
	task := &models.TaskInstance{
		InstanceID:  uuid.New(),
		TaskID:      taskID,
		Description: description,
		AgentType:   agentType,
		Status:      models.TaskStatusPending,
		Priority:    priority,
		CreatedAt:   now,
	}

	if err := s.CreateTask(ctx, task, ownerID, 0); err != nil {
		return nil, fmt.Errorf("failed to create root task: %w", err)
	}

	return task, nil
}

func (s *TaskService) GetQueueStatus() map[string]interface{} {
	return s.queue.GetQueueStatus()
}

func (s *TaskService) ListTasksBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.TaskInstance, error) {
	return s.repo.ListTaskInstancesBySession(ctx, sessionID, limit, offset)
}

func toJSON(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
