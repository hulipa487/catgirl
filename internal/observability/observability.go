package observability

import (
	"time"

	"github.com/rs/zerolog"
)

type MetricsCollector struct {
	logger zerolog.Logger
}

func NewMetricsCollector(logger zerolog.Logger) *MetricsCollector {
	return &MetricsCollector{
		logger: logger,
	}
}

type Metrics struct {
	Timestamp   time.Time `json:"timestamp"`
	QueueSize   int       `json:"queue_size"`
	ActiveAgents int      `json:"active_agents"`
	IdleAgents  int       `json:"idle_agents"`
	TotalTasks  int       `json:"total_tasks"`
	CompletedTasks int    `json:"completed_tasks"`
	FailedTasks int       `json:"failed_tasks"`
}

func (m *MetricsCollector) RecordTaskCompletion(duration time.Duration, success bool) {
	m.logger.Info().
		Dur("duration", duration).
		Bool("success", success).
		Msg("task_completed")
}

func (m *MetricsCollector) RecordAgentSpawn(agentType string) {
	m.logger.Info().
		Str("agent_type", agentType).
		Msg("agent_spawned")
}

func (m *MetricsCollector) RecordQueueDepth(depth int) {
	m.logger.Debug().
		Int("depth", depth).
		Msg("queue_depth")
}

func (m *MetricsCollector) RecordTokenUsage(tokens int, operation string) {
	m.logger.Debug().
		Int("tokens", tokens).
		Str("operation", operation).
		Msg("token_usage")
}
