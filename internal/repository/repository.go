package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hulipa487/catgirl/internal/database"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct {
	db *database.DB
}

func New(db *database.DB) *Repository {
	return &Repository{db: db}
}

// Session Repository

func (r *Repository) CreateSession(ctx context.Context, s *models.Session) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO sessions (id, telegram_user_id, name, status, created_at, updated_at, orchestrator_state, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, s.ID, s.TelegramUserID, s.Name, s.Status, s.CreatedAt, s.UpdatedAt, s.OrchestratorState, s.Metadata)
	return err
}

func (r *Repository) GetSession(ctx context.Context, id uuid.UUID) (*models.Session, error) {
	var s models.Session
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, telegram_user_id, name, status, created_at, updated_at, orchestrator_state, metadata
		FROM sessions WHERE id = $1
	`, id).Scan(&s.ID, &s.TelegramUserID, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt, &s.OrchestratorState, &s.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (r *Repository) GetSessionByTelegramUser(ctx context.Context, telegramUserID int64) (*models.Session, error) {
	var s models.Session
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, telegram_user_id, name, status, created_at, updated_at, orchestrator_state, metadata
		FROM sessions WHERE telegram_user_id = $1 AND status = 'active'
	`, telegramUserID).Scan(&s.ID, &s.TelegramUserID, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt, &s.OrchestratorState, &s.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (r *Repository) UpdateSession(ctx context.Context, s *models.Session) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE sessions SET name = $2, status = $3, updated_at = $4, orchestrator_state = $5, metadata = $6
		WHERE id = $1
	`, s.ID, s.Name, s.Status, time.Now(), s.OrchestratorState, s.Metadata)
	return err
}

func (r *Repository) ListSessions(ctx context.Context, limit, offset int) ([]*models.Session, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, telegram_user_id, name, status, created_at, updated_at, orchestrator_state, metadata
		FROM sessions ORDER BY created_at DESC LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*models.Session
	for rows.Next() {
		var s models.Session
		if err := rows.Scan(&s.ID, &s.TelegramUserID, &s.Name, &s.Status, &s.CreatedAt, &s.UpdatedAt, &s.OrchestratorState, &s.Metadata); err != nil {
			return nil, err
		}
		sessions = append(sessions, &s)
	}
	return sessions, nil
}

// Task Family Repository

func (r *Repository) CreateTaskFamily(ctx context.Context, tf *models.TaskFamily) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO task_families (task_id, session_id, container_id, root_description, status, max_depth_reached, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, tf.TaskID, tf.SessionID, tf.ContainerID, tf.RootDescription, tf.Status, tf.MaxDepthReached, tf.CreatedAt)
	return err
}

func (r *Repository) GetTaskFamily(ctx context.Context, taskID uuid.UUID) (*models.TaskFamily, error) {
	var tf models.TaskFamily
	err := r.db.Pool.QueryRow(ctx, `
		SELECT task_id, session_id, container_id, root_description, status, max_depth_reached, created_at, completed_at
		FROM task_families WHERE task_id = $1
	`, taskID).Scan(&tf.TaskID, &tf.SessionID, &tf.ContainerID, &tf.RootDescription, &tf.Status, &tf.MaxDepthReached, &tf.CreatedAt, &tf.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &tf, err
}

func (r *Repository) UpdateTaskFamily(ctx context.Context, tf *models.TaskFamily) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_families SET container_id = $2, status = $3, max_depth_reached = $4, completed_at = $5
		WHERE task_id = $1
	`, tf.TaskID, tf.ContainerID, tf.Status, tf.MaxDepthReached, tf.CompletedAt)
	return err
}

// Task Instance Repository

func (r *Repository) CreateTaskInstance(ctx context.Context, ti *models.TaskInstance) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO task_instances (instance_id, task_id, session_id, owner_id, depth, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, constraints)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`, ti.InstanceID, ti.TaskID, ti.SessionID, ti.OwnerID, ti.Depth, ti.Description, ti.AgentType, ti.Status, ti.Priority, ti.PriorityScore, ti.AssignedAgentID, ti.ParentInstanceID, ti.CreatedAt, ti.Constraints)
	return err
}

func (r *Repository) GetTaskInstance(ctx context.Context, instanceID uuid.UUID) (*models.TaskInstance, error) {
	var ti models.TaskInstance
	err := r.db.Pool.QueryRow(ctx, `
		SELECT instance_id, task_id, session_id, owner_id, depth, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, started_at, completed_at, result, error, constraints, container_snapshot_id
		FROM task_instances WHERE instance_id = $1
	`, instanceID).Scan(&ti.InstanceID, &ti.TaskID, &ti.SessionID, &ti.OwnerID, &ti.Depth, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints, &ti.ContainerSnapshotID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &ti, err
}

func (r *Repository) UpdateTaskInstance(ctx context.Context, ti *models.TaskInstance) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_instances SET status = $2, priority_score = $3, assigned_agent_id = $4, started_at = $5, completed_at = $6, result = $7, error = $8, container_snapshot_id = $9
		WHERE instance_id = $1
	`, ti.InstanceID, ti.Status, ti.PriorityScore, ti.AssignedAgentID, ti.StartedAt, ti.CompletedAt, ti.Result, ti.Error, ti.ContainerSnapshotID)
	return err
}

func (r *Repository) ListTaskInstancesBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.TaskInstance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT instance_id, task_id, session_id, owner_id, depth, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, started_at, completed_at, result, error, constraints, container_snapshot_id
		FROM task_instances WHERE session_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*models.TaskInstance
	for rows.Next() {
		var ti models.TaskInstance
		if err := rows.Scan(&ti.InstanceID, &ti.TaskID, &ti.SessionID, &ti.OwnerID, &ti.Depth, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints, &ti.ContainerSnapshotID); err != nil {
			return nil, err
		}
		instances = append(instances, &ti)
	}
	return instances, nil
}

func (r *Repository) ListTaskInstancesByStatus(ctx context.Context, status models.TaskStatus, limit int) ([]*models.TaskInstance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT instance_id, task_id, session_id, owner_id, depth, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, started_at, completed_at, result, error, constraints, container_snapshot_id
		FROM task_instances WHERE status = $1 ORDER BY priority_score DESC, created_at ASC LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*models.TaskInstance
	for rows.Next() {
		var ti models.TaskInstance
		if err := rows.Scan(&ti.InstanceID, &ti.TaskID, &ti.SessionID, &ti.OwnerID, &ti.Depth, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints, &ti.ContainerSnapshotID); err != nil {
			return nil, err
		}
		instances = append(instances, &ti)
	}
	return instances, nil
}

// Container Snapshot Repository

func (r *Repository) CreateContainerSnapshot(ctx context.Context, cs *models.ContainerSnapshot) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO container_snapshots (snapshot_id, task_id, instance_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, cs.SnapshotID, cs.TaskID, cs.InstanceID, cs.SessionID, cs.ContainerID, cs.ImageID, cs.ImageName, cs.Reason, cs.Volumes, cs.Environment, cs.Metadata, cs.CreatedAt, cs.ExpiresAt)
	return err
}

func (r *Repository) GetContainerSnapshot(ctx context.Context, snapshotID uuid.UUID) (*models.ContainerSnapshot, error) {
	var cs models.ContainerSnapshot
	err := r.db.Pool.QueryRow(ctx, `
		SELECT snapshot_id, task_id, instance_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
		FROM container_snapshots WHERE snapshot_id = $1 AND deleted_at IS NULL
	`, snapshotID).Scan(&cs.SnapshotID, &cs.TaskID, &cs.InstanceID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &cs, err
}

func (r *Repository) ListContainerSnapshotsBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.ContainerSnapshot, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT snapshot_id, task_id, instance_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
		FROM container_snapshots WHERE session_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC LIMIT $2 OFFSET $3
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []*models.ContainerSnapshot
	for rows.Next() {
		var cs models.ContainerSnapshot
		if err := rows.Scan(&cs.SnapshotID, &cs.TaskID, &cs.InstanceID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, &cs)
	}
	return snapshots, nil
}

func (r *Repository) DeleteContainerSnapshot(ctx context.Context, snapshotID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE container_snapshots SET deleted_at = $2 WHERE snapshot_id = $1
	`, snapshotID, time.Now())
	return err
}

func (r *Repository) ListExpiredSnapshots(ctx context.Context) ([]*models.ContainerSnapshot, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT snapshot_id, task_id, instance_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
		FROM container_snapshots WHERE expires_at < $1 AND deleted_at IS NULL
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []*models.ContainerSnapshot
	for rows.Next() {
		var cs models.ContainerSnapshot
		if err := rows.Scan(&cs.SnapshotID, &cs.TaskID, &cs.InstanceID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, &cs)
	}
	return snapshots, nil
}

// Agent Repository

func (r *Repository) CreateAgent(ctx context.Context, a *models.Agent) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO agents (id, type, status, current_instance_id, created_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, a.ID, a.Type, a.Status, a.CurrentInstanceID, a.CreatedAt, a.Metadata)
	return err
}

func (r *Repository) GetAgent(ctx context.Context, id string) (*models.Agent, error) {
	var a models.Agent
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, type, status, current_instance_id, created_at, last_active_at, tasks_completed, metadata
		FROM agents WHERE id = $1
	`, id).Scan(&a.ID, &a.Type, &a.Status, &a.CurrentInstanceID, &a.CreatedAt, &a.LastActiveAt, &a.TasksCompleted, &a.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &a, err
}

func (r *Repository) UpdateAgent(ctx context.Context, a *models.Agent) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE agents SET status = $2, current_instance_id = $3, last_active_at = $4, tasks_completed = $5, metadata = $6
		WHERE id = $1
	`, a.ID, a.Status, a.CurrentInstanceID, a.LastActiveAt, a.TasksCompleted, a.Metadata)
	return err
}

func (r *Repository) ListAgentsByStatus(ctx context.Context, status models.AgentStatus) ([]*models.Agent, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, type, status, current_instance_id, created_at, last_active_at, tasks_completed, metadata
		FROM agents WHERE status = $1
	`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.Type, &a.Status, &a.CurrentInstanceID, &a.CreatedAt, &a.LastActiveAt, &a.TasksCompleted, &a.Metadata); err != nil {
			return nil, err
		}
		agents = append(agents, &a)
	}
	return agents, nil
}

func (r *Repository) ListAllAgents(ctx context.Context) ([]*models.Agent, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, type, status, current_instance_id, created_at, last_active_at, tasks_completed, metadata
		FROM agents WHERE status != 'removed'
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var a models.Agent
		if err := rows.Scan(&a.ID, &a.Type, &a.Status, &a.CurrentInstanceID, &a.CreatedAt, &a.LastActiveAt, &a.TasksCompleted, &a.Metadata); err != nil {
			return nil, err
		}
		agents = append(agents, &a)
	}
	return agents, nil
}

func (r *Repository) DeleteAgent(ctx context.Context, id string) error {
	_, err := r.db.Pool.Exec(ctx, `UPDATE agents SET status = 'removed' WHERE id = $1`, id)
	return err
}

// Working Memory Repository

func (r *Repository) SetWorkingMemory(ctx context.Context, agentID, key string, value interface{}) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO working_memory (agent_id, key, value, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (agent_id, key) DO UPDATE SET value = $3, updated_at = NOW()
	`, agentID, key, jsonValue)
	return err
}

func (r *Repository) GetWorkingMemory(ctx context.Context, agentID, key string) (interface{}, error) {
	var value json.RawMessage
	err := r.db.Pool.QueryRow(ctx, `
		SELECT value FROM working_memory WHERE agent_id = $1 AND key = $2
	`, agentID, key).Scan(&value)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result interface{}
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *Repository) GetAllWorkingMemory(ctx context.Context, agentID string) ([]*models.WorkingMemoryEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT agent_id, key, value, created_at, updated_at
		FROM working_memory WHERE agent_id = $1 ORDER BY key
	`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.WorkingMemoryEntry
	for rows.Next() {
		var e models.WorkingMemoryEntry
		if err := rows.Scan(&e.AgentID, &e.Key, &e.Value, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

func (r *Repository) DeleteWorkingMemory(ctx context.Context, agentID, key string) error {
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM working_memory WHERE agent_id = $1 AND key = $2
	`, agentID, key)
	return err
}

func (r *Repository) ScanWorkingMemoryBySession(ctx context.Context, sessionID uuid.UUID) ([]*models.WorkingMemoryEntry, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT wm.agent_id, wm.key, wm.value, wm.created_at, wm.updated_at
		FROM working_memory wm
		JOIN task_instances ti ON wm.agent_id = ti.assigned_agent_id
		WHERE ti.session_id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*models.WorkingMemoryEntry
	for rows.Next() {
		var e models.WorkingMemoryEntry
		if err := rows.Scan(&e.AgentID, &e.Key, &e.Value, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, &e)
	}
	return entries, nil
}

// Long-Term Memory Repository

func (r *Repository) CreateLongTermMemory(ctx context.Context, m *models.LongTermMemory) error {
	embeddingJSON, err := json.Marshal(m.Embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}
	sourceAgentsJSON, err := json.Marshal(m.SourceAgentIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal source agents: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO long_term_memories (id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, m.ID, m.SessionID, m.Tier, m.Content, embeddingJSON, m.Metadata, m.AccessCount, m.LastAccessedAt, sourceAgentsJSON, m.CreatedAt, m.ExpiresAt)
	return err
}

func (r *Repository) SearchLongTermMemory(ctx context.Context, sessionID uuid.UUID, queryEmbedding []float32, topK int) ([]*models.LongTermMemory, error) {
	embeddingJSON, err := json.Marshal(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal embedding: %w", err)
	}

	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at,
			   1 - (embedding <=> $1) AS similarity
		FROM long_term_memories
		WHERE session_id = $2
		ORDER BY embedding <=> $1
		LIMIT $3
	`, embeddingJSON, sessionID, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*models.LongTermMemory
	for rows.Next() {
		var m models.LongTermMemory
		var embeddingJSON, sourceAgentsJSON json.RawMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Tier, &m.Content, &embeddingJSON, &m.Metadata, &m.AccessCount, &m.LastAccessedAt, &sourceAgentsJSON, &m.CreatedAt, &m.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(embeddingJSON, &m.Embedding); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sourceAgentsJSON, &m.SourceAgentIDs); err != nil {
			return nil, err
		}
		memories = append(memories, &m)
	}
	return memories, nil
}

func (r *Repository) IncrementMemoryAccessCount(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE long_term_memories SET access_count = access_count + 1, last_accessed_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) ListLongTermMemoriesBySession(ctx context.Context, sessionID uuid.UUID, tier string, limit, offset int) ([]*models.LongTermMemory, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at
		FROM long_term_memories WHERE session_id = $1 AND tier = $2
		ORDER BY created_at DESC LIMIT $3 OFFSET $4
	`, sessionID, tier, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*models.LongTermMemory
	for rows.Next() {
		var m models.LongTermMemory
		var embeddingJSON, sourceAgentsJSON json.RawMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Tier, &m.Content, &embeddingJSON, &m.Metadata, &m.AccessCount, &m.LastAccessedAt, &sourceAgentsJSON, &m.CreatedAt, &m.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(embeddingJSON, &m.Embedding); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sourceAgentsJSON, &m.SourceAgentIDs); err != nil {
			return nil, err
		}
		memories = append(memories, &m)
	}
	return memories, nil
}

func (r *Repository) DeleteLongTermMemory(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM long_term_memories WHERE id = $1`, id)
	return err
}

func (r *Repository) ListExpiredLongTermMemories(ctx context.Context) ([]*models.LongTermMemory, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at
		FROM long_term_memories WHERE expires_at IS NOT NULL AND expires_at < NOW()
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*models.LongTermMemory
	for rows.Next() {
		var m models.LongTermMemory
		var embeddingJSON, sourceAgentsJSON json.RawMessage
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Tier, &m.Content, &embeddingJSON, &m.Metadata, &m.AccessCount, &m.LastAccessedAt, &sourceAgentsJSON, &m.CreatedAt, &m.ExpiresAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(embeddingJSON, &m.Embedding); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(sourceAgentsJSON, &m.SourceAgentIDs); err != nil {
			return nil, err
		}
		memories = append(memories, &m)
	}
	return memories, nil
}

// Skill Repository

func (r *Repository) CreateSkill(ctx context.Context, s *models.Skill) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO skills (id, session_id, name, description, version, definition, code, created_by_agent_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, s.ID, s.SessionID, s.Name, s.Description, s.Version, s.Definition, s.Code, s.CreatedByAgentID, s.CreatedAt)
	return err
}

func (r *Repository) GetSkill(ctx context.Context, id uuid.UUID) (*models.Skill, error) {
	var s models.Skill
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, session_id, name, description, version, definition, code, created_by_agent_id, created_at
		FROM skills WHERE id = $1
	`, id).Scan(&s.ID, &s.SessionID, &s.Name, &s.Description, &s.Version, &s.Definition, &s.Code, &s.CreatedByAgentID, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (r *Repository) GetSkillByName(ctx context.Context, sessionID uuid.UUID, name string) (*models.Skill, error) {
	var s models.Skill
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, session_id, name, description, version, definition, code, created_by_agent_id, created_at
		FROM skills WHERE session_id = $1 AND name = $2
	`, sessionID, name).Scan(&s.ID, &s.SessionID, &s.Name, &s.Description, &s.Version, &s.Definition, &s.Code, &s.CreatedByAgentID, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (r *Repository) ListSkillsBySession(ctx context.Context, sessionID uuid.UUID) ([]*models.Skill, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, name, description, version, definition, code, created_by_agent_id, created_at
		FROM skills WHERE session_id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var skills []*models.Skill
	for rows.Next() {
		var s models.Skill
		if err := rows.Scan(&s.ID, &s.SessionID, &s.Name, &s.Description, &s.Version, &s.Definition, &s.Code, &s.CreatedByAgentID, &s.CreatedAt); err != nil {
			return nil, err
		}
		skills = append(skills, &s)
	}
	return skills, nil
}

func (r *Repository) DeleteSkill(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM skills WHERE id = $1`, id)
	return err
}

// MCP Server Repository

func (r *Repository) CreateMCPServer(ctx context.Context, s *models.MCPServer) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO mcp_servers (id, session_id, name, connection_type, connection_string, command, status, tools, created_by_agent_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, s.ID, s.SessionID, s.Name, s.ConnectionType, s.ConnectionString, s.Command, s.Status, s.Tools, s.CreatedByAgentID, s.CreatedAt)
	return err
}

func (r *Repository) GetMCPServer(ctx context.Context, id uuid.UUID) (*models.MCPServer, error) {
	var s models.MCPServer
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, session_id, name, connection_type, connection_string, command, status, tools, created_by_agent_id, created_at
		FROM mcp_servers WHERE id = $1
	`, id).Scan(&s.ID, &s.SessionID, &s.Name, &s.ConnectionType, &s.ConnectionString, &s.Command, &s.Status, &s.Tools, &s.CreatedByAgentID, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &s, err
}

func (r *Repository) ListMCPServersBySession(ctx context.Context, sessionID uuid.UUID) ([]*models.MCPServer, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, name, connection_type, connection_string, command, status, tools, created_by_agent_id, created_at
		FROM mcp_servers WHERE session_id = $1
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []*models.MCPServer
	for rows.Next() {
		var s models.MCPServer
		if err := rows.Scan(&s.ID, &s.SessionID, &s.Name, &s.ConnectionType, &s.ConnectionString, &s.Command, &s.Status, &s.Tools, &s.CreatedByAgentID, &s.CreatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, &s)
	}
	return servers, nil
}

func (r *Repository) UpdateMCPServer(ctx context.Context, s *models.MCPServer) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE mcp_servers SET status = $2, tools = $3 WHERE id = $1
	`, s.ID, s.Status, s.Tools)
	return err
}

func (r *Repository) DeleteMCPServer(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM mcp_servers WHERE id = $1`, id)
	return err
}

// Usage Record Repository

func (r *Repository) CreateUsageRecord(ctx context.Context, u *models.UsageRecord) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO usage_records (usage_id, task_id, session_id, user_id, operation_type, operation_name, input_tokens, output_tokens, total_tokens, membership_level, cost_multiplier, effective_tokens, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, u.UsageID, u.TaskID, u.SessionID, u.UserID, u.OperationType, u.OperationName, u.InputTokens, u.OutputTokens, u.TotalTokens, u.MembershipLevel, u.CostMultiplier, u.EffectiveTokens, u.Timestamp)
	return err
}

func (r *Repository) GetUsageSummaryBySession(ctx context.Context, sessionID uuid.UUID) (map[string]interface{}, error) {
	row := r.db.Pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(input_tokens), 0) as total_input,
			COALESCE(SUM(output_tokens), 0) as total_output,
			COALESCE(SUM(total_tokens), 0) as total_tokens,
			COALESCE(SUM(effective_tokens), 0) as total_effective,
			COUNT(*) as record_count
		FROM usage_records WHERE session_id = $1
	`, sessionID)

	var totalInput, totalOutput, totalTokens, totalEffective int
	var recordCount int
	if err := row.Scan(&totalInput, &totalOutput, &totalTokens, &totalEffective, &recordCount); err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_input_tokens":     totalInput,
		"total_output_tokens":    totalOutput,
		"total_tokens":           totalTokens,
		"total_effective_tokens": totalEffective,
		"record_count":           recordCount,
	}, nil
}

// Task Owner Channel Repository

func (r *Repository) CreateTaskOwnerChannel(ctx context.Context, c *models.TaskOwnerChannel) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO task_owner_channels (channel_id, task_instance_id, owner_id, created_at, last_activity)
		VALUES ($1, $2, $3, $4, $5)
	`, c.ChannelID, c.TaskInstanceID, c.OwnerID, c.CreatedAt, c.LastActivity)
	return err
}

func (r *Repository) GetTaskOwnerChannel(ctx context.Context, channelID uuid.UUID) (*models.TaskOwnerChannel, error) {
	var c models.TaskOwnerChannel
	err := r.db.Pool.QueryRow(ctx, `
		SELECT channel_id, task_instance_id, owner_id, created_at, last_activity
		FROM task_owner_channels WHERE channel_id = $1
	`, channelID).Scan(&c.ChannelID, &c.TaskInstanceID, &c.OwnerID, &c.CreatedAt, &c.LastActivity)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (r *Repository) GetTaskOwnerChannelByTask(ctx context.Context, taskInstanceID uuid.UUID) (*models.TaskOwnerChannel, error) {
	var c models.TaskOwnerChannel
	err := r.db.Pool.QueryRow(ctx, `
		SELECT channel_id, task_instance_id, owner_id, created_at, last_activity
		FROM task_owner_channels WHERE task_instance_id = $1
	`, taskInstanceID).Scan(&c.ChannelID, &c.TaskInstanceID, &c.OwnerID, &c.CreatedAt, &c.LastActivity)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (r *Repository) UpdateChannelLastActivity(ctx context.Context, channelID uuid.UUID) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_owner_channels SET last_activity = NOW() WHERE channel_id = $1
	`, channelID)
	return err
}

// Task Message Repository

func (r *Repository) CreateTaskMessage(ctx context.Context, m *models.TextMessage) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO task_messages (message_id, channel_id, from_id, to_id, content, timestamp, read)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, m.MessageID, m.ChannelID, m.FromID, m.ToID, m.Content, m.Timestamp, m.Read)
	return err
}

func (r *Repository) GetUnreadMessages(ctx context.Context, channelID uuid.UUID, receiverID string) ([]*models.TextMessage, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT message_id, channel_id, from_id, to_id, content, timestamp, read
		FROM task_messages WHERE channel_id = $1 AND to_id = $2 AND read = FALSE
		ORDER BY timestamp ASC
	`, channelID, receiverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.TextMessage
	for rows.Next() {
		var m models.TextMessage
		if err := rows.Scan(&m.MessageID, &m.ChannelID, &m.FromID, &m.ToID, &m.Content, &m.Timestamp, &m.Read); err != nil {
			return nil, err
		}
		messages = append(messages, &m)
	}
	return messages, nil
}

func (r *Repository) MarkMessagesAsRead(ctx context.Context, channelID uuid.UUID, receiverID string) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_messages SET read = TRUE
		WHERE channel_id = $1 AND to_id = $2 AND read = FALSE
	`, channelID, receiverID)
	return err
}

func (r *Repository) ListMessagesByChannel(ctx context.Context, channelID uuid.UUID, limit, offset int) ([]*models.TextMessage, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT message_id, channel_id, from_id, to_id, content, timestamp, read
		FROM task_messages WHERE channel_id = $1
		ORDER BY timestamp ASC LIMIT $2 OFFSET $3
	`, channelID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*models.TextMessage
	for rows.Next() {
		var m models.TextMessage
		if err := rows.Scan(&m.MessageID, &m.ChannelID, &m.FromID, &m.ToID, &m.Content, &m.Timestamp, &m.Read); err != nil {
			return nil, err
		}
		messages = append(messages, &m)
	}
	return messages, nil
}

// Conversation History Repository

func (r *Repository) AddConversationTurn(ctx context.Context, sessionID uuid.UUID, turn *models.ConversationTurn) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO conversation_history (id, session_id, turn_id, thought, action, result, tokens, timestamp)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (session_id, turn_id) DO UPDATE SET thought = $4, action = $5, result = $6, tokens = $7
	`, uuid.New(), sessionID, turn.TurnID, turn.Thought, turn.Action, turn.Result, turn.Tokens, turn.Timestamp)
	return err
}

func (r *Repository) GetConversationHistory(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.ConversationTurn, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT turn_id, thought, action, result, tokens, timestamp
		FROM conversation_history WHERE session_id = $1
		ORDER BY turn_id ASC LIMIT $2 OFFSET $3
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var turns []*models.ConversationTurn
	for rows.Next() {
		var t models.ConversationTurn
		if err := rows.Scan(&t.TurnID, &t.Thought, &t.Action, &t.Result, &t.Tokens, &t.Timestamp); err != nil {
			return nil, err
		}
		turns = append(turns, &t)
	}
	return turns, nil
}

func (r *Repository) GetTurnCount(ctx context.Context, sessionID uuid.UUID) (int, error) {
	var count int
	err := r.db.Pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM conversation_history WHERE session_id = $1
	`, sessionID).Scan(&count)
	return count, err
}

func (r *Repository) DeleteConversationTurns(ctx context.Context, sessionID uuid.UUID, upToTurnID int) error {
	_, err := r.db.Pool.Exec(ctx, `
		DELETE FROM conversation_history WHERE session_id = $1 AND turn_id <= $2
	`, sessionID, upToTurnID)
	return err
}

// Telegram User Repository

func (r *Repository) CreateTelegramUser(ctx context.Context, telegramUserID int64, sessionID *uuid.UUID, username, firstName, lastName string) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO telegram_users (telegram_user_id, session_id, username, first_name, last_name, created_at, last_activity)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (telegram_user_id) DO UPDATE SET session_id = $2, username = $3, first_name = $4, last_name = $5, last_activity = NOW()
	`, telegramUserID, sessionID, username, firstName, lastName)
	return err
}

func (r *Repository) GetTelegramUser(ctx context.Context, telegramUserID int64) (*models.TelegramUser, error) {
	type TelegramUser struct {
		TelegramUserID int64
		SessionID      *uuid.UUID
		Username       *string
		FirstName      *string
		LastName       *string
		IsBanned       bool
		CreatedAt      time.Time
		LastActivity   time.Time
	}

	var u TelegramUser
	err := r.db.Pool.QueryRow(ctx, `
		SELECT telegram_user_id, session_id, username, first_name, last_name, is_banned, created_at, last_activity
		FROM telegram_users WHERE telegram_user_id = $1
	`, telegramUserID).Scan(&u.TelegramUserID, &u.SessionID, &u.Username, &u.FirstName, &u.LastName, &u.IsBanned, &u.CreatedAt, &u.LastActivity)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return (*models.TelegramUser)(&u), err
}

func (r *Repository) IsTelegramUserBanned(ctx context.Context, telegramUserID int64) (bool, error) {
	var isBanned bool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT is_banned FROM telegram_users WHERE telegram_user_id = $1
	`, telegramUserID).Scan(&isBanned)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return isBanned, err
}

func (r *Repository) BanTelegramUser(ctx context.Context, telegramUserID int64, banned bool) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE telegram_users SET is_banned = $2 WHERE telegram_user_id = $1
	`, telegramUserID, banned)
	return err
}
