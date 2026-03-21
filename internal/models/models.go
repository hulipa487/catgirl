package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusPaused    SessionStatus = "paused"
	SessionStatusTerminated SessionStatus = "terminated"
)

type Session struct {
	ID                uuid.UUID       `json:"id" db:"id"`
	TelegramUserID    int64           `json:"telegram_user_id" db:"telegram_user_id"`
	Name              string          `json:"name" db:"name"`
	Status            SessionStatus   `json:"status" db:"status"`
	CreatedAt         time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at" db:"updated_at"`
	OrchestratorState json.RawMessage `json:"orchestrator_state" db:"orchestrator_state"`
	Metadata          json.RawMessage `json:"metadata" db:"metadata"`
}

type TaskFamily struct {
	TaskID            uuid.UUID  `json:"task_id" db:"task_id"`
	SessionID         uuid.UUID  `json:"session_id" db:"session_id"`
	ContainerID      string     `json:"container_id" db:"container_id"`
	RootDescription  string     `json:"root_description" db:"root_description"`
	Status            string     `json:"status" db:"status"`
	MaxDepthReached   int        `json:"max_depth_reached" db:"max_depth_reached"`
	CreatedAt         time.Time  `json:"created_at" db:"created_at"`
	CompletedAt       *time.Time `json:"completed_at" db:"completed_at"`
}

type TaskStatus string

const (
	TaskStatusPending     TaskStatus = "pending"
	TaskStatusAssigned    TaskStatus = "assigned"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusFailed     TaskStatus = "failed"
	TaskStatusExited     TaskStatus = "exited"
)

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
	PriorityCritical Priority = "critical"
)

type AgentType string

const (
	AgentTypeGeneralPurpose AgentType = "general_purpose"
	AgentTypeReasoner       AgentType = "reasoner"
)

type TaskInstance struct {
	InstanceID          uuid.UUID       `json:"instance_id" db:"instance_id"`
	TaskID              uuid.UUID       `json:"task_id" db:"task_id"`
	SessionID           uuid.UUID       `json:"session_id" db:"session_id"`
	OwnerID             string          `json:"owner_id" db:"owner_id"`
	Depth               int             `json:"depth" db:"depth"`
	Description         string          `json:"description" db:"description"`
	AgentType           AgentType       `json:"agent_type" db:"agent_type"`
	Status              TaskStatus      `json:"status" db:"status"`
	Priority            Priority        `json:"priority" db:"priority"`
	PriorityScore       float64         `json:"priority_score" db:"priority_score"`
	AssignedAgentID     *string         `json:"assigned_agent_id" db:"assigned_agent_id"`
	ParentInstanceID    *uuid.UUID      `json:"parent_instance_id" db:"parent_instance_id"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	StartedAt           *time.Time      `json:"started_at" db:"started_at"`
	CompletedAt         *time.Time      `json:"completed_at" db:"completed_at"`
	Result              json.RawMessage `json:"result" db:"result"`
	Error               *string         `json:"error" db:"error"`
	Constraints         json.RawMessage `json:"constraints" db:"constraints"`
	ContainerSnapshotID *uuid.UUID      `json:"container_snapshot_id" db:"container_snapshot_id"`
}

type SnapshotReason string

const (
	SnapshotReasonCompleted   SnapshotReason = "COMPLETED"
	SnapshotReasonFailed      SnapshotReason = "FAILED"
	SnapshotReasonExited      SnapshotReason = "EXITED"
	SnapshotReasonInterrupted SnapshotReason = "INTERRUPTED"
)

type ContainerSnapshot struct {
	SnapshotID   uuid.UUID       `json:"snapshot_id" db:"snapshot_id"`
	TaskID       uuid.UUID       `json:"task_id" db:"task_id"`
	InstanceID   uuid.UUID       `json:"instance_id" db:"instance_id"`
	SessionID    uuid.UUID       `json:"session_id" db:"session_id"`
	ContainerID  string          `json:"container_id" db:"container_id"`
	ImageID      string          `json:"image_id" db:"image_id"`
	ImageName    string          `json:"image_name" db:"image_name"`
	Reason       SnapshotReason  `json:"reason" db:"reason"`
	Volumes      json.RawMessage `json:"volumes" db:"volumes"`
	Environment  json.RawMessage `json:"environment" db:"environment"`
	Metadata     json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	ExpiresAt    *time.Time      `json:"expires_at" db:"expires_at"`
	DeletedAt    *time.Time      `json:"deleted_at" db:"deleted_at"`
}

type AgentStatus string

const (
	AgentStatusIdle     AgentStatus = "idle"
	AgentStatusBusy     AgentStatus = "busy"
	AgentStatusDestroying AgentStatus = "destroying"
	AgentStatusRemoved   AgentStatus = "removed"
)

type Agent struct {
	ID                 string           `json:"id" db:"id"`
	Type               AgentType        `json:"type" db:"type"`
	Status             AgentStatus      `json:"status" db:"status"`
	CurrentInstanceID  *uuid.UUID       `json:"current_instance_id" db:"current_instance_id"`
	CreatedAt          time.Time        `json:"created_at" db:"created_at"`
	LastActiveAt       *time.Time       `json:"last_active_at" db:"last_active_at"`
	TasksCompleted     int              `json:"tasks_completed" db:"tasks_completed"`
	Metadata           json.RawMessage  `json:"metadata" db:"metadata"`
}

type WorkingMemoryEntry struct {
	AgentID    string          `json:"agent_id" db:"agent_id"`
	Key        string          `json:"key" db:"key"`
	Value      json.RawMessage `json:"value" db:"value"`
	CreatedAt  time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at" db:"updated_at"`
}

type LTMTier string

const (
	LTTier1Raw     LTMTier = "tier1_raw"
	LTTTier2Summary LTMTier = "tier2_summary"
	LTTTier3Brief   LTMTier = "tier3_brief"
)

type LongTermMemory struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	SessionID      uuid.UUID       `json:"session_id" db:"session_id"`
	Tier           LTMTier         `json:"tier" db:"tier"`
	Content        string          `json:"content" db:"content"`
	Embedding      []float32       `json:"embedding" db:"embedding"`
	Metadata       json.RawMessage `json:"metadata" db:"metadata"`
	AccessCount    int             `json:"access_count" db:"access_count"`
	LastAccessedAt *time.Time      `json:"last_accessed_at" db:"last_accessed_at"`
	SourceAgentIDs []string        `json:"source_agent_ids" db:"source_agent_ids"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	ExpiresAt      *time.Time      `json:"expires_at" db:"expires_at"`
}

type Skill struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	SessionID   uuid.UUID       `json:"session_id" db:"session_id"`
	Name        string          `json:"name" db:"name"`
	Description string          `json:"description" db:"description"`
	Version     string          `json:"version" db:"version"`
	Definition  json.RawMessage `json:"definition" db:"definition"`
	Code        *string         `json:"code" db:"code"`
	CreatedByAgentID *string    `json:"created_by_agent_id" db:"created_by_agent_id"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

type MCPConnectionType string

const (
	MCPConnectionTypeSTDIO      MCPConnectionType = "stdio"
	MCPConnectionTypeHTTP      MCPConnectionType = "http"
	MCPConnectionTypeWebSocket MCPConnectionType = "websocket"
)

type MCPStatus string

const (
	MCPStatusConnected    MCPStatus = "connected"
	MCPStatusDisconnected MCPStatus = "disconnected"
	MCPStatusError        MCPStatus = "error"
)

type MCPServer struct {
	ID              uuid.UUID           `json:"id" db:"id"`
	SessionID       uuid.UUID           `json:"session_id" db:"session_id"`
	Name            string              `json:"name" db:"name"`
	ConnectionType  MCPConnectionType   `json:"connection_type" db:"connection_type"`
	ConnectionString string             `json:"connection_string" db:"connection_string"`
	Command         string              `json:"command" db:"command"`
	Status          MCPStatus           `json:"status" db:"status"`
	Tools           json.RawMessage     `json:"tools" db:"tools"`
	CreatedByAgentID *string           `json:"created_by_agent_id" db:"created_by_agent_id"`
	CreatedAt      time.Time            `json:"created_at" db:"created_at"`
}

type UsageOperationType string

const (
	UsageOperationLLMCall    UsageOperationType = "llm_call"
	UsageOperationToolCall   UsageOperationType = "tool_call"
	UsageOperationEmbedding  UsageOperationType = "embedding"
)

type UsageRecord struct {
	UsageID           uuid.UUID            `json:"usage_id" db:"usage_id"`
	TaskID           *uuid.UUID           `json:"task_id" db:"task_id"`
	SessionID        uuid.UUID            `json:"session_id" db:"session_id"`
	UserID           string               `json:"user_id" db:"user_id"`
	OperationType    UsageOperationType   `json:"operation_type" db:"operation_type"`
	OperationName    string               `json:"operation_name" db:"operation_name"`
	InputTokens      int                  `json:"input_tokens" db:"input_tokens"`
	OutputTokens     int                  `json:"output_tokens" db:"output_tokens"`
	TotalTokens      int                  `json:"total_tokens" db:"total_tokens"`
	MembershipLevel  string               `json:"membership_level" db:"membership_level"`
	CostMultiplier   float64              `json:"cost_multiplier" db:"cost_multiplier"`
	EffectiveTokens  float64             `json:"effective_tokens" db:"effective_tokens"`
	Timestamp        time.Time            `json:"timestamp" db:"timestamp"`
}

type MembershipLevel string

const (
	MembershipFree       MembershipLevel = "free"
	MembershipBasic      MembershipLevel = "basic"
	MembershipPro        MembershipLevel = "pro"
	MembershipEnterprise MembershipLevel = "enterprise"
)

type JWTClaims struct {
	UserID         string           `json:"sub"`
	Email         string           `json:"email"`
	Membership    MembershipLevel  `json:"membership"`
	SessionID     uuid.UUID        `json:"session_id"`
	Exp           int64            `json:"exp"`
	Iat           int64            `json:"iat"`
	Permissions   []string         `json:"permissions"`
}

type ConversationTurn struct {
	TurnID    int             `json:"turn_id"`
	Thought   string          `json:"thought"`
	Action    string          `json:"action"`
	Result    json.RawMessage `json:"result"`
	Tokens    int             `json:"tokens"`
	Timestamp time.Time       `json:"timestamp"`
}

type ConversationHistory struct {
	SessionID        uuid.UUID          `json:"session_id"`
	Turns            []ConversationTurn `json:"turns"`
	CompactedSummary *string            `json:"compacted_summary,omitempty"`
	TotalTokens      int                `json:"total_tokens"`
	MaxTokens        int                `json:"max_tokens"`
}

type TaskOwnerChannel struct {
	ChannelID      uuid.UUID  `json:"channel_id" db:"channel_id"`
	TaskInstanceID uuid.UUID  `json:"task_instance_id" db:"task_instance_id"`
	OwnerID        string     `json:"owner_id" db:"owner_id"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	LastActivity   time.Time  `json:"last_activity" db:"last_activity"`
}

type TextMessage struct {
	MessageID uuid.UUID `json:"message_id" db:"message_id"`
	ChannelID uuid.UUID `json:"channel_id" db:"channel_id"`
	FromID    string    `json:"from_id" db:"from_id"`
	ToID      string    `json:"to_id" db:"to_id"`
	Content   string    `json:"content" db:"content"`
	Timestamp time.Time `json:"timestamp" db:"timestamp"`
	Read      bool      `json:"read" db:"read"`
}

type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type ActionType string

const (
	ActionSpawnTask         ActionType = "SPAWN_TASK"
	ActionCompleteTask      ActionType = "COMPLETE_TASK"
	ActionFailTask          ActionType = "FAIL_TASK"
	ActionSetMemory         ActionType = "SET_MEMORY"
	ActionGetMemory         ActionType = "GET_MEMORY"
	ActionDeleteMemory      ActionType = "DELETE_MEMORY"
	ActionSearchMemory      ActionType = "SEARCH_LONG_TERM"
	ActionCallTool          ActionType = "CALL_TOOL"
	ActionListTools         ActionType = "LIST_TOOLS"
	ActionAddMCPServer      ActionType = "ADD_MCP_SERVER"
	ActionCreateSkill       ActionType = "CREATE_SKILL"
	ActionExecuteSkill      ActionType = "EXECUTE_SKILL"
	ActionRunCode           ActionType = "RUN_CODE"
	ActionSendMessage       ActionType = "SEND_MESSAGE"
	ActionReadMessages      ActionType = "READ_MESSAGES"
	ActionEmitResult        ActionType = "EMIT_RESULT"
	ActionNotify            ActionType = "NOTIFY"
	ActionGetContext        ActionType = "GET_CONTEXT"
)

type Action struct {
	Type    ActionType                `json:"action_type"`
	Payload map[string]interface{}     `json:"payload"`
}

type ActionResult struct {
	Success bool                   `json:"success"`
	Result  interface{}            `json:"result,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Logs    []string               `json:"logs,omitempty"`
}
