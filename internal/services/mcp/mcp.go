package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hulipa487/catgirl/internal/config"
	"github.com/hulipa487/catgirl/internal/models"
	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type MCPService struct {
	repo   *repository.Repository
	config *config.Config
	logger zerolog.Logger
	servers map[uuid.UUID]*MCPServerClient
}

type MCPServerClient struct {
	ServerID   uuid.UUID
	SessionID  uuid.UUID
	Name       string
	Status     models.MCPStatus
	Tools      []models.ToolDefinition
}

func NewMCPService(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *MCPService {
	return &MCPService{
		repo:    repo,
		config:  cfg,
		logger:  logger,
		servers: make(map[uuid.UUID]*MCPServerClient),
	}
}

func (s *MCPService) AddMCPServer(ctx context.Context, sessionID uuid.UUID, serverConfig *MCPServerConfig) (*models.MCPServer, error) {
	server := &models.MCPServer{
		ID:               uuid.New(),
		SessionID:        sessionID,
		Name:             serverConfig.Name,
		ConnectionType:   serverConfig.ConnectionType,
		ConnectionString: serverConfig.ConnectionString,
		Command:          serverConfig.Command,
		Status:           models.MCPStatusDisconnected,
		CreatedAt:        time.Now(),
	}

	if err := s.repo.CreateMCPServer(ctx, server); err != nil {
		return nil, fmt.Errorf("failed to create MCP server: %w", err)
	}

	s.logger.Info().
		Str("server_id", server.ID.String()).
		Str("session_id", sessionID.String()).
		Str("name", server.Name).
		Msg("MCP server added")

	return server, nil
}

func (s *MCPService) GetMCPServer(ctx context.Context, serverID uuid.UUID) (*models.MCPServer, error) {
	return s.repo.GetMCPServer(ctx, serverID)
}

func (s *MCPService) ListSessionMCPServers(ctx context.Context, sessionID uuid.UUID) ([]*models.MCPServer, error) {
	return s.repo.ListMCPServersBySession(ctx, sessionID)
}

func (s *MCPService) CallTool(ctx context.Context, sessionID uuid.UUID, toolName string, arguments map[string]interface{}) (interface{}, error) {
	servers, err := s.repo.ListMCPServersBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	for _, server := range servers {
		for _, tool := range parseTools(server.Tools) {
			if tool.Name == toolName {
				s.logger.Debug().
					Str("server", server.Name).
					Str("tool", toolName).
					Msg("calling MCP tool")

				return map[string]interface{}{
					"tool":   toolName,
					"result": "success",
					"output": "tool executed",
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("tool not found: %s", toolName)
}

func (s *MCPService) ListTools(ctx context.Context, sessionID uuid.UUID) ([]models.ToolDefinition, error) {
	servers, err := s.repo.ListMCPServersBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var allTools []models.ToolDefinition
	for _, server := range servers {
		if server.Status == models.MCPStatusConnected {
			allTools = append(allTools, parseTools(server.Tools)...)
		}
	}

	return allTools, nil
}

func (s *MCPService) SeedDefaultTools(ctx context.Context, sessionID uuid.UUID) error {
	// check if built-in server already exists
	servers, err := s.repo.ListMCPServersBySession(ctx, sessionID)
	if err != nil {
		return err
	}

	for _, srv := range servers {
		if srv.Name == "system_tools" {
			return nil
		}
	}

	tools := []models.ToolDefinition{
		{
			Name:        "SPAWN_TASK",
			Description: "Spawn a sub-task",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"description": map[string]interface{}{"type": "string"},
					"priority":    map[string]interface{}{"type": "string", "enum": []string{"low", "normal", "high", "critical"}},
				},
				"required": []string{"description", "priority"},
			},
		},
		{
			Name:        "COMPLETE_TASK",
			Description: "Mark the current task as completed",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"result_summary": map[string]interface{}{"type": "string"},
				},
				"required": []string{"result_summary"},
			},
		},
		{
			Name:        "FAIL_TASK",
			Description: "Mark the current task as failed",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reason": map[string]interface{}{"type": "string"},
				},
				"required": []string{"reason"},
			},
		},
		{
			Name:        "SEND_MESSAGE",
			Description: "Send a message to the user/orchestrator",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{"type": "string"},
				},
				"required": []string{"message"},
			},
		},
		{
			Name:        "SET_STATE",
			Description: "Signal the runtime what state you are in. IDLE = waiting for user. WAIT = waiting for a task. CONTINUE = loop again immediately.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"state": map[string]interface{}{"type": "string", "enum": []string{"IDLE", "WAIT", "CONTINUE"}},
				},
				"required": []string{"state"},
			},
		},
	}

	toolsJSON, _ := json.Marshal(tools)

	server := &models.MCPServer{
		ID:               uuid.New(),
		SessionID:        sessionID,
		Name:             "system_tools",
		ConnectionType:   "internal",
		Status:           models.MCPStatusConnected,
		Tools:            toolsJSON,
		CreatedAt:        time.Now(),
	}

	return s.repo.CreateMCPServer(ctx, server)
}

func parseTools(toolsJSON json.RawMessage) []models.ToolDefinition {
	if len(toolsJSON) == 0 {
		return nil
	}

	var tools []models.ToolDefinition
	if err := json.Unmarshal(toolsJSON, &tools); err != nil {
		return nil
	}
	return tools
}

type MCPServerConfig struct {
	Name             string                  `json:"name"`
	ConnectionType   models.MCPConnectionType `json:"connection_type"`
	ConnectionString string                  `json:"connection_string"`
	Command          string                  `json:"command"`
}
