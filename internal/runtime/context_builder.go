package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/rs/zerolog"
)

// ContextInjectionConfig controls what context is injected
type ContextInjectionConfig struct {
	EnableWorkingMemory bool
	EnableParentContext bool
	WorkingMemoryLimit  int
}

// ContextBuilder builds context messages from multiple sources
type ContextBuilder struct {
	repo     *repository.Repository
	config   *ContextInjectionConfig
	logger   zerolog.Logger
}

// NewContextBuilder creates a new context builder
func NewContextBuilder(repo *repository.Repository, ragConfig *config.RAGConfig, logger zerolog.Logger) *ContextBuilder {
	return &ContextBuilder{
		repo:   repo,
		config: &ContextInjectionConfig{
			EnableWorkingMemory: true,
			EnableParentContext: true,
			WorkingMemoryLimit:  10,
		},
		logger: logger,
	}
}

// BuildWorkerContext builds context for a worker agent
func (cb *ContextBuilder) BuildWorkerContext(ctx context.Context, sessionID uuid.UUID, agentID string, taskDescription string, parentInstanceID *uuid.UUID) (string, error) {
	var contextParts []string

	// 1. Working Memory - Agent's scratchpad
	if cb.config.EnableWorkingMemory {
		wmContext, err := cb.buildWorkingMemoryContext(ctx, agentID)
		if err != nil {
			cb.logger.Warn().Err(err).Msg("Failed to build working memory context")
		} else if wmContext != "" {
			contextParts = append(contextParts, wmContext)
		}
	}

	// 2. Parent Task Context - What the parent task knows
	if cb.config.EnableParentContext && parentInstanceID != nil {
		parentContext, err := cb.buildParentTaskContext(ctx, *parentInstanceID)
		if err != nil {
			cb.logger.Warn().Err(err).Msg("Failed to build parent task context")
		} else if parentContext != "" {
			contextParts = append(contextParts, parentContext)
		}
	}

	if len(contextParts) == 0 {
		return "", nil
	}

	return fmt.Sprintf("<context>\n%s\n</context>", strings.Join(contextParts, "\n\n")), nil
}

// BuildOrchestratorContext builds context for the orchestrator
func (cb *ContextBuilder) BuildOrchestratorContext(ctx context.Context, sessionID uuid.UUID, currentMessage string) (string, error) {
	// Currently no orchestrator-specific context without RAG
	return "", nil
}

func (cb *ContextBuilder) buildWorkingMemoryContext(ctx context.Context, agentID string) (string, error) {
	entries, err := cb.repo.GetAllWorkingMemory(ctx, agentID)
	if err != nil {
		return "", err
	}

	if len(entries) == 0 {
		return "", nil
	}

	// Limit entries
	if len(entries) > cb.config.WorkingMemoryLimit {
		entries = entries[:cb.config.WorkingMemoryLimit]
	}

	var sb strings.Builder
	sb.WriteString("<working_memory>\n")
	sb.WriteString("Your stored notes and data:\n\n")

	for _, entry := range entries {
		var value string
		if err := json.Unmarshal(entry.Value, &value); err == nil && value != "" {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", entry.Key, value))
		} else {
			// Try as map or other type
			sb.WriteString(fmt.Sprintf("- %s: %s\n", entry.Key, string(entry.Value)))
		}
	}

	sb.WriteString("</working_memory>")
	return sb.String(), nil
}

func (cb *ContextBuilder) buildParentTaskContext(ctx context.Context, parentInstanceID uuid.UUID) (string, error) {
	// Get parent task instance
	parent, err := cb.repo.GetTaskInstance(ctx, parentInstanceID)
	if err != nil || parent == nil {
		return "", err
	}

	// Get recent turns from parent's context
	turns, err := cb.repo.GetTaskInstanceTurns(ctx, parentInstanceID)
	if err != nil {
		return "", err
	}

	if len(turns) == 0 {
		// Just return parent description without recent activity
		return fmt.Sprintf("<parent_task>\nParent task: %s\n</parent_task>", parent.Description), nil
	}

	// Get last few turns for context
	recentCount := 3
	if len(turns) < recentCount {
		recentCount = len(turns)
	}
	recentTurns := turns[len(turns)-recentCount:]

	var sb strings.Builder
	sb.WriteString("<parent_task>\n")
	sb.WriteString(fmt.Sprintf("Parent task: %s\n\n", parent.Description))
	sb.WriteString("Recent parent activity:\n")

	for _, turn := range recentTurns {
		switch turn.Role {
		case "assistant":
			if turn.Content != "" {
				sb.WriteString(fmt.Sprintf("Parent thought: %s\n", truncate(turn.Content, 200)))
			}
			if turn.ToolName != nil && *turn.ToolName != "" {
				sb.WriteString(fmt.Sprintf("Parent used tool: %s\n", *turn.ToolName))
			}
		}
	}

	sb.WriteString("</parent_task>")
	return sb.String(), nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}