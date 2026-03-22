package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
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

func (r *Repository) Ping(ctx context.Context) map[string]interface{} {
	return r.db.Health(ctx)
}

// Session Repository

func (r *Repository) GetRuntimeConfig(ctx context.Context) (*config.RuntimeConfig, error) {
	// First, fetch the singleton system_config row
	var maxTaskDepth, maxQueueSize, embeddingDims, maxTokens, timeoutSecs int
	var defaultSystemPrompt, defaultAgentSystemPrompt string
	var defaultOrchestratorToolsJSON, defaultAgentToolsJSON json.RawMessage
	var minAgents, maxAgents, idleTimeoutSecs int
	var snapshotEnabled bool
	var snapshotStoragePath, snapshotRetCompleted, snapshotRetFailed, snapshotRetExited, snapshotRetInterrupted string
	var telegramBotsJSON json.RawMessage
	var telegramListenAddr string
	var authJwtSecret, authJwtIssuer string
	var authAllowedMemberships json.RawMessage
	var contextMaxTokens, contextPreserveRecentTurns int
	var contextCompactionThreshold float64
	var contextCompactionAgentType string
	var ragEnabled, ragAutoRetrieveEnabled, ragAutoRetrieveOnLlmCall bool
	var ragDefaultTopK, ragAutoRetrieveTopK, ragAutoRetrieveMaxResults int
	var ragMinSimilarity float64
	var snapshotMaxStorageBytes int64

	err := r.db.Pool.QueryRow(ctx, `
		SELECT
			max_task_depth, max_queue_size,
			embedding_dims, max_tokens, timeout_seconds, system_prompt, agent_system_prompt, default_orchestrator_tools, default_agent_tools,
			min_agents, max_agents, idle_timeout_seconds,
			snapshot_enabled, snapshot_storage_path, snapshot_max_storage_bytes,
			snapshot_retention_completed, snapshot_retention_failed, snapshot_retention_exited, snapshot_retention_interrupted,
			telegram_bots, telegram_listen_addr,
			auth_jwt_secret, auth_jwt_issuer, auth_allowed_memberships,
			context_max_tokens, context_compaction_threshold, context_preserve_recent_turns, context_compaction_agent_type,
			rag_enabled, rag_default_top_k, rag_auto_retrieve_enabled, rag_auto_retrieve_on_llm_call, rag_auto_retrieve_top_k, rag_auto_retrieve_max_results, rag_min_similarity
		FROM system_config WHERE id = 1
	`).Scan(
		&maxTaskDepth, &maxQueueSize,
		&embeddingDims, &maxTokens, &timeoutSecs, &defaultSystemPrompt, &defaultAgentSystemPrompt, &defaultOrchestratorToolsJSON, &defaultAgentToolsJSON,
		&minAgents, &maxAgents, &idleTimeoutSecs,
		&snapshotEnabled, &snapshotStoragePath, &snapshotMaxStorageBytes,
		&snapshotRetCompleted, &snapshotRetFailed, &snapshotRetExited, &snapshotRetInterrupted,
		&telegramBotsJSON, &telegramListenAddr,
		&authJwtSecret, &authJwtIssuer, &authAllowedMemberships,
		&contextMaxTokens, &contextCompactionThreshold, &contextPreserveRecentTurns, &contextCompactionAgentType,
		&ragEnabled, &ragDefaultTopK, &ragAutoRetrieveEnabled, &ragAutoRetrieveOnLlmCall, &ragAutoRetrieveTopK, &ragAutoRetrieveMaxResults, &ragMinSimilarity,
	)

	if err == pgx.ErrNoRows {
		return nil, nil // No config yet
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch system_config: %w", err)
	}

	// Fetch Providers
	rows, err := r.db.Pool.Query(ctx, `SELECT provider_type, base_url, api_key, models FROM llm_providers`)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch llm_providers: %w", err)
	}
	defer rows.Close()

	var gpProviders, reasonerProviders, embeddingProviders []config.ModelProviderConfig
	for rows.Next() {
		var pType, baseURL, apiKey string
		var modelsJSON json.RawMessage
		if err := rows.Scan(&pType, &baseURL, &apiKey, &modelsJSON); err != nil {
			return nil, err
		}
		var modelsList []string
		_ = json.Unmarshal(modelsJSON, &modelsList)
		provider := config.ModelProviderConfig{BaseURL: baseURL, APIKey: apiKey, Models: modelsList}

		switch pType {
		case "gp":
			gpProviders = append(gpProviders, provider)
		case "reasoner":
			reasonerProviders = append(reasonerProviders, provider)
		case "embedding":
			embeddingProviders = append(embeddingProviders, provider)
		}
	}

	var allowedMemberships []string
	_ = json.Unmarshal(authAllowedMemberships, &allowedMemberships)

	var defaultOrchestratorTools, defaultAgentTools []string
	_ = json.Unmarshal(defaultOrchestratorToolsJSON, &defaultOrchestratorTools)
	_ = json.Unmarshal(defaultAgentToolsJSON, &defaultAgentTools)

	var telegramBots []config.TelegramBotConfig
	_ = json.Unmarshal(telegramBotsJSON, &telegramBots)

	return &config.RuntimeConfig{
		Global: config.GlobalConfig{MaxTaskDepth: maxTaskDepth, MaxQueueSize: maxQueueSize},
		LLM: config.LLMConfig{
			Providers:                gpProviders,
			ReasonerProviders:        reasonerProviders,
			EmbeddingProviders:       embeddingProviders,
			EmbeddingDims:            embeddingDims,
			MaxTokens:                maxTokens,
			TimeoutSecs:              timeoutSecs,
			DefaultSystemPrompt:      defaultSystemPrompt,
			DefaultAgentSystemPrompt: defaultAgentSystemPrompt,
			DefaultOrchestratorTools: defaultOrchestratorTools,
			DefaultAgentTools:        defaultAgentTools,
		},
		AgentPool: config.AgentPoolConfig{MinAgents: minAgents, MaxAgents: maxAgents, IdleTimeoutSecs: idleTimeoutSecs},
		Snapshot: config.SnapshotConfig{
			Enabled:         snapshotEnabled,
			StoragePath:     snapshotStoragePath,
			MaxStorageBytes: snapshotMaxStorageBytes,
			Retention: config.RetentionConfig{
				Completed:   snapshotRetCompleted,
				Failed:      snapshotRetFailed,
				Exited:      snapshotRetExited,
				Interrupted: snapshotRetInterrupted,
			},
		},
		Telegram: config.TelegramConfig{Bots: telegramBots, ListenAddr: telegramListenAddr},
		Auth:     config.AuthConfig{JWTSecret: authJwtSecret, JWTIssuer: authJwtIssuer, AllowedMemberships: allowedMemberships},
		Context: config.ContextConfig{
			MaxTokens:           contextMaxTokens,
			CompactionThreshold: contextCompactionThreshold,
			PreserveRecentTurns: contextPreserveRecentTurns,
			CompactionAgentType: contextCompactionAgentType,
		},
		RAG: config.RAGConfig{
			Enabled:       ragEnabled,
			DefaultTopK:   ragDefaultTopK,
			MinSimilarity: ragMinSimilarity,
			AutoRetrieve: config.AutoRetrieveConfig{
				Enabled:    ragAutoRetrieveEnabled,
				OnLLMCall:  ragAutoRetrieveOnLlmCall,
				TopK:       ragAutoRetrieveTopK,
				MaxResults: ragAutoRetrieveMaxResults,
			},
		},
	}, nil
}

func (r *Repository) UpdateRuntimeConfig(ctx context.Context, cfg *config.RuntimeConfig, updatedBy string) error {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	orchestratorToolsJSON, _ := json.Marshal(cfg.LLM.DefaultOrchestratorTools)
	agentToolsJSON, _ := json.Marshal(cfg.LLM.DefaultAgentTools)
	membershipsJSON, _ := json.Marshal(cfg.Auth.AllowedMemberships)

	telegramBotsJSONBytes, _ := json.Marshal(cfg.Telegram.Bots)

	// Upsert System Config
	_, err = tx.Exec(ctx, `
		INSERT INTO system_config (
			id, max_task_depth, max_queue_size,
			embedding_dims, max_tokens, timeout_seconds, system_prompt, agent_system_prompt, default_orchestrator_tools, default_agent_tools,
			min_agents, max_agents, idle_timeout_seconds,
			snapshot_enabled, snapshot_storage_path, snapshot_max_storage_bytes, snapshot_retention_completed, snapshot_retention_failed, snapshot_retention_exited, snapshot_retention_interrupted,
			telegram_bots, telegram_listen_addr,
			auth_jwt_secret, auth_jwt_issuer, auth_allowed_memberships,
			context_max_tokens, context_compaction_threshold, context_preserve_recent_turns, context_compaction_agent_type,
			rag_enabled, rag_default_top_k, rag_auto_retrieve_enabled, rag_auto_retrieve_on_llm_call, rag_auto_retrieve_top_k, rag_auto_retrieve_max_results, rag_min_similarity,
			updated_by, updated_at
		) VALUES (
			1, $1, $2,
			$3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12,
			$13, $14, $15, $16, $17, $18, $19,
			$20, $21,
			$22, $23, $24,
			$25, $26, $27, $28,
			$29, $30, $31, $32, $33, $34, $35,
			$36, NOW()
		)
		ON CONFLICT (id) DO UPDATE SET
			max_task_depth = EXCLUDED.max_task_depth, max_queue_size = EXCLUDED.max_queue_size,
			embedding_dims = EXCLUDED.embedding_dims, max_tokens = EXCLUDED.max_tokens, timeout_seconds = EXCLUDED.timeout_seconds, system_prompt = EXCLUDED.system_prompt, agent_system_prompt = EXCLUDED.agent_system_prompt, default_orchestrator_tools = EXCLUDED.default_orchestrator_tools, default_agent_tools = EXCLUDED.default_agent_tools,
			min_agents = EXCLUDED.min_agents, max_agents = EXCLUDED.max_agents, idle_timeout_seconds = EXCLUDED.idle_timeout_seconds,
			snapshot_enabled = EXCLUDED.snapshot_enabled, snapshot_storage_path = EXCLUDED.snapshot_storage_path, snapshot_max_storage_bytes = EXCLUDED.snapshot_max_storage_bytes, snapshot_retention_completed = EXCLUDED.snapshot_retention_completed, snapshot_retention_failed = EXCLUDED.snapshot_retention_failed, snapshot_retention_exited = EXCLUDED.snapshot_retention_exited, snapshot_retention_interrupted = EXCLUDED.snapshot_retention_interrupted,
			telegram_bots = EXCLUDED.telegram_bots, telegram_listen_addr = EXCLUDED.telegram_listen_addr,
			auth_jwt_secret = EXCLUDED.auth_jwt_secret, auth_jwt_issuer = EXCLUDED.auth_jwt_issuer, auth_allowed_memberships = EXCLUDED.auth_allowed_memberships,
			context_max_tokens = EXCLUDED.context_max_tokens, context_compaction_threshold = EXCLUDED.context_compaction_threshold, context_preserve_recent_turns = EXCLUDED.context_preserve_recent_turns, context_compaction_agent_type = EXCLUDED.context_compaction_agent_type,
			rag_enabled = EXCLUDED.rag_enabled, rag_default_top_k = EXCLUDED.rag_default_top_k, rag_auto_retrieve_enabled = EXCLUDED.rag_auto_retrieve_enabled, rag_auto_retrieve_on_llm_call = EXCLUDED.rag_auto_retrieve_on_llm_call, rag_auto_retrieve_top_k = EXCLUDED.rag_auto_retrieve_top_k, rag_auto_retrieve_max_results = EXCLUDED.rag_auto_retrieve_max_results, rag_min_similarity = EXCLUDED.rag_min_similarity,
			updated_by = EXCLUDED.updated_by, updated_at = NOW()
	`,
		cfg.Global.MaxTaskDepth, cfg.Global.MaxQueueSize,
		cfg.LLM.EmbeddingDims, cfg.LLM.MaxTokens, cfg.LLM.TimeoutSecs, cfg.LLM.DefaultSystemPrompt, cfg.LLM.DefaultAgentSystemPrompt, orchestratorToolsJSON, agentToolsJSON,
		cfg.AgentPool.MinAgents, cfg.AgentPool.MaxAgents, cfg.AgentPool.IdleTimeoutSecs,
		cfg.Snapshot.Enabled, cfg.Snapshot.StoragePath, cfg.Snapshot.MaxStorageBytes, cfg.Snapshot.Retention.Completed, cfg.Snapshot.Retention.Failed, cfg.Snapshot.Retention.Exited, cfg.Snapshot.Retention.Interrupted,
		telegramBotsJSONBytes, cfg.Telegram.ListenAddr,
		cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer, membershipsJSON,
		cfg.Context.MaxTokens, cfg.Context.CompactionThreshold, cfg.Context.PreserveRecentTurns, cfg.Context.CompactionAgentType,
		cfg.RAG.Enabled, cfg.RAG.DefaultTopK, cfg.RAG.AutoRetrieve.Enabled, cfg.RAG.AutoRetrieve.OnLLMCall, cfg.RAG.AutoRetrieve.TopK, cfg.RAG.AutoRetrieve.MaxResults, cfg.RAG.MinSimilarity,
		updatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert system config: %w", err)
	}

	// Sync Providers (Replace all strategy for simplicity)
	_, err = tx.Exec(ctx, `DELETE FROM llm_providers`)
	if err != nil {
		return fmt.Errorf("failed to clear llm_providers: %w", err)
	}

	insertProvider := func(pType string, p config.ModelProviderConfig) error {
		modelsJSON, _ := json.Marshal(p.Models)
		_, err := tx.Exec(ctx, `
			INSERT INTO llm_providers (provider_type, base_url, api_key, models)
			VALUES ($1, $2, $3, $4)
		`, pType, p.BaseURL, p.APIKey, modelsJSON)
		return err
	}

	for _, p := range cfg.LLM.Providers {
		if err := insertProvider("gp", p); err != nil {
			return err
		}
	}
	for _, p := range cfg.LLM.ReasonerProviders {
		if err := insertProvider("reasoner", p); err != nil {
			return err
		}
	}
	for _, p := range cfg.LLM.EmbeddingProviders {
		if err := insertProvider("embedding", p); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
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
		INSERT INTO task_families (task_id, session_id, container_id, root_description, status, max_depth_reached, created_at, container_snapshot_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, tf.TaskID, tf.SessionID, tf.ContainerID, tf.RootDescription, tf.Status, tf.MaxDepthReached, tf.CreatedAt, tf.ContainerSnapshotID)
	return err
}

func (r *Repository) GetTaskFamily(ctx context.Context, taskID uuid.UUID) (*models.TaskFamily, error) {
	var tf models.TaskFamily
	err := r.db.Pool.QueryRow(ctx, `
		SELECT task_id, session_id, container_id, root_description, status, max_depth_reached, created_at, completed_at, container_snapshot_id
		FROM task_families WHERE task_id = $1
	`, taskID).Scan(&tf.TaskID, &tf.SessionID, &tf.ContainerID, &tf.RootDescription, &tf.Status, &tf.MaxDepthReached, &tf.CreatedAt, &tf.CompletedAt, &tf.ContainerSnapshotID)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &tf, err
}

func (r *Repository) UpdateTaskFamily(ctx context.Context, tf *models.TaskFamily) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_families SET container_id = $2, status = $3, max_depth_reached = $4, completed_at = $5, container_snapshot_id = $6
		WHERE task_id = $1
	`, tf.TaskID, tf.ContainerID, tf.Status, tf.MaxDepthReached, tf.CompletedAt, tf.ContainerSnapshotID)
	return err
}

// Task Instance Repository

func (r *Repository) CreateTaskInstance(ctx context.Context, ti *models.TaskInstance) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO task_instances (instance_id, task_id, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, constraints)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, ti.InstanceID, ti.TaskID, ti.Description, ti.AgentType, ti.Status, ti.Priority, ti.PriorityScore, ti.AssignedAgentID, ti.ParentInstanceID, ti.CreatedAt, ti.Constraints)
	return err
}

func (r *Repository) GetTaskInstance(ctx context.Context, instanceID uuid.UUID) (*models.TaskInstance, error) {
	var ti models.TaskInstance
	err := r.db.Pool.QueryRow(ctx, `
		SELECT instance_id, task_id, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, started_at, completed_at, result, error, constraints
		FROM task_instances WHERE instance_id = $1
	`, instanceID).Scan(&ti.InstanceID, &ti.TaskID, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &ti, err
}

func (r *Repository) UpdateTaskInstance(ctx context.Context, ti *models.TaskInstance) error {
	_, err := r.db.Pool.Exec(ctx, `
		UPDATE task_instances SET status = $2, priority_score = $3, assigned_agent_id = $4, started_at = $5, completed_at = $6, result = $7, error = $8
		WHERE instance_id = $1
	`, ti.InstanceID, ti.Status, ti.PriorityScore, ti.AssignedAgentID, ti.StartedAt, ti.CompletedAt, ti.Result, ti.Error)
	return err
}

func (r *Repository) ListTaskInstancesBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.TaskInstance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT ti.instance_id, ti.task_id, ti.description, ti.agent_type, ti.status, ti.priority, ti.priority_score, ti.assigned_agent_id, ti.parent_instance_id, ti.created_at, ti.started_at, ti.completed_at, ti.result, ti.error, ti.constraints
		FROM task_instances ti
		JOIN task_families tf ON ti.task_id = tf.task_id
		WHERE tf.session_id = $1 ORDER BY ti.created_at DESC LIMIT $2 OFFSET $3
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*models.TaskInstance
	for rows.Next() {
		var ti models.TaskInstance
		if err := rows.Scan(&ti.InstanceID, &ti.TaskID, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints); err != nil {
			return nil, err
		}
		instances = append(instances, &ti)
	}
	return instances, nil
}

func (r *Repository) ListTaskInstancesByStatus(ctx context.Context, status models.TaskStatus, limit int) ([]*models.TaskInstance, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT instance_id, task_id, description, agent_type, status, priority, priority_score, assigned_agent_id, parent_instance_id, created_at, started_at, completed_at, result, error, constraints
		FROM task_instances WHERE status = $1 ORDER BY priority_score DESC, created_at ASC LIMIT $2
	`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instances []*models.TaskInstance
	for rows.Next() {
		var ti models.TaskInstance
		if err := rows.Scan(&ti.InstanceID, &ti.TaskID, &ti.Description, &ti.AgentType, &ti.Status, &ti.Priority, &ti.PriorityScore, &ti.AssignedAgentID, &ti.ParentInstanceID, &ti.CreatedAt, &ti.StartedAt, &ti.CompletedAt, &ti.Result, &ti.Error, &ti.Constraints); err != nil {
			return nil, err
		}
		instances = append(instances, &ti)
	}
	return instances, nil
}

// Container Snapshot Repository

func (r *Repository) CreateContainerSnapshot(ctx context.Context, cs *models.ContainerSnapshot) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO container_snapshots (snapshot_id, task_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, cs.SnapshotID, cs.TaskID, cs.SessionID, cs.ContainerID, cs.ImageID, cs.ImageName, cs.Reason, cs.Volumes, cs.Environment, cs.Metadata, cs.CreatedAt, cs.ExpiresAt)
	return err
}

func (r *Repository) GetContainerSnapshot(ctx context.Context, snapshotID uuid.UUID) (*models.ContainerSnapshot, error) {
	var cs models.ContainerSnapshot
	err := r.db.Pool.QueryRow(ctx, `
		SELECT snapshot_id, task_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
		FROM container_snapshots WHERE snapshot_id = $1 AND deleted_at IS NULL
	`, snapshotID).Scan(&cs.SnapshotID, &cs.TaskID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &cs, err
}

func (r *Repository) ListContainerSnapshotsBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.ContainerSnapshot, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT snapshot_id, task_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
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
		if err := rows.Scan(&cs.SnapshotID, &cs.TaskID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt); err != nil {
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
		SELECT snapshot_id, task_id, session_id, container_id, image_id, image_name, reason, volumes, environment, metadata, created_at, expires_at, deleted_at
		FROM container_snapshots WHERE expires_at < $1 AND deleted_at IS NULL
	`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []*models.ContainerSnapshot
	for rows.Next() {
		var cs models.ContainerSnapshot
		if err := rows.Scan(&cs.SnapshotID, &cs.TaskID, &cs.SessionID, &cs.ContainerID, &cs.ImageID, &cs.ImageName, &cs.Reason, &cs.Volumes, &cs.Environment, &cs.Metadata, &cs.CreatedAt, &cs.ExpiresAt, &cs.DeletedAt); err != nil {
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
		JOIN task_families tf ON ti.task_id = tf.task_id
		WHERE tf.session_id = $1
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
	embeddingStr := string(embeddingJSON)

	sourceAgentsJSON, err := json.Marshal(m.SourceAgentIDs)
	if err != nil {
		return fmt.Errorf("failed to marshal source agents: %w", err)
	}
	_, err = r.db.Pool.Exec(ctx, `
		INSERT INTO long_term_memories (id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5::vector, $6, $7, $8, $9, $10, $11)
	`, m.ID, m.SessionID, m.Tier, m.Content, embeddingStr, m.Metadata, m.AccessCount, m.LastAccessedAt, sourceAgentsJSON, m.CreatedAt, m.ExpiresAt)
	return err
}

func (r *Repository) SearchLongTermMemory(ctx context.Context, sessionID uuid.UUID, queryEmbedding []float32, topK int) ([]*models.LongTermMemory, error) {
	// Format array as Postgres vector string '[1.0, 2.0, ...]'
	embeddingStr := fmt.Sprintf("%v", queryEmbedding)
	// Replace spaces with commas to match vector format
	// Quick hack since %v gives "[1 2 3]" and vector needs "[1,2,3]"
	embeddingJSON, _ := json.Marshal(queryEmbedding)
	embeddingStr = string(embeddingJSON)

	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, session_id, tier, content, embedding, metadata, access_count, last_accessed_at, source_agent_ids, created_at, expires_at,
			   1 - (embedding <=> $1::vector) AS similarity
		FROM long_term_memories
		WHERE session_id = $2
		ORDER BY embedding <=> $1::vector
		LIMIT $3
	`, embeddingStr, sessionID, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*models.LongTermMemory
	for rows.Next() {
		var m models.LongTermMemory
		var embeddingJSON, sourceAgentsJSON json.RawMessage
		var similarity float64
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Tier, &m.Content, &embeddingJSON, &m.Metadata, &m.AccessCount, &m.LastAccessedAt, &sourceAgentsJSON, &m.CreatedAt, &m.ExpiresAt, &similarity); err != nil {
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

// Tool Repository

func (r *Repository) ListActiveTools(ctx context.Context) ([]*models.Tool, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, name, description, parameters, is_active, created_at, updated_at
		FROM tools WHERE is_active = TRUE ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tools []*models.Tool
	for rows.Next() {
		var t models.Tool
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Parameters, &t.IsActive, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		tools = append(tools, &t)
	}
	return tools, nil
}

func (r *Repository) GetToolByName(ctx context.Context, name string) (*models.Tool, error) {
	var t models.Tool
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, name, description, parameters, is_active, created_at, updated_at
		FROM tools WHERE name = $1 AND is_active = TRUE
	`, name).Scan(&t.ID, &t.Name, &t.Description, &t.Parameters, &t.IsActive, &t.CreatedAt, &t.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

func (r *Repository) CreateTool(ctx context.Context, t *models.Tool) error {
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO tools (id, name, description, parameters, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (name) DO UPDATE SET description = $3, parameters = $4, updated_at = NOW()
	`, t.ID, t.Name, t.Description, t.Parameters, t.IsActive, t.CreatedAt, t.UpdatedAt)
	return err
}
