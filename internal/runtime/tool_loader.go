package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hulipa487/catgirl/internal/services/llm"
	"github.com/rs/zerolog"
)

// ToolLoader watches a directory for tool JSON files and loads them
type ToolLoader struct {
	toolsDir string
	logger   zerolog.Logger
	tools    map[string]llm.Tool
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewToolLoader creates a new tool loader that watches the specified directory
func NewToolLoader(toolsDir string, logger zerolog.Logger) *ToolLoader {
	return &ToolLoader{
		toolsDir: toolsDir,
		logger:   logger,
		tools:    make(map[string]llm.Tool),
		stopCh:   make(chan struct{}),
	}
}

// Start begins watching the tools directory for changes
func (tl *ToolLoader) Start(ctx context.Context) error {
	// Initial load
	if err := tl.loadTools(); err != nil {
		tl.logger.Warn().Err(err).Msg("failed to load tools on startup")
	}

	// Start background watcher
	go tl.watchLoop(ctx)

	return nil
}

// Stop stops the tool loader
func (tl *ToolLoader) Stop() {
	close(tl.stopCh)
}

// GetTools returns a copy of the current tools
func (tl *ToolLoader) GetTools() []llm.Tool {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	tools := make([]llm.Tool, 0, len(tl.tools))
	for _, t := range tl.tools {
		tools = append(tools, t)
	}
	return tools
}

// GetToolsByName returns tools filtered by allowed names
func (tl *ToolLoader) GetToolsByName(allowedNames []string) []llm.Tool {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	if len(allowedNames) == 0 {
		// Return all tools if no filter specified
		tools := make([]llm.Tool, 0, len(tl.tools))
		for _, t := range tl.tools {
			tools = append(tools, t)
		}
		return tools
	}

	// Create set of allowed names
	allowedSet := make(map[string]bool)
	for _, name := range allowedNames {
		allowedSet[name] = true
	}

	tools := make([]llm.Tool, 0)
	for _, t := range tl.tools {
		if allowedSet[t.Function.Name] {
			tools = append(tools, t)
		}
	}
	return tools
}

// watchLoop periodically checks for changes in the tools directory
func (tl *ToolLoader) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tl.stopCh:
			return
		case <-ticker.C:
			if err := tl.loadTools(); err != nil {
				tl.logger.Warn().Err(err).Msg("failed to reload tools")
			}
		}
	}
}

// loadTools scans the tools directory and loads all JSON files
func (tl *ToolLoader) loadTools() error {
	entries, err := os.ReadDir(tl.toolsDir)
	if err != nil {
		if os.IsNotExist(err) {
			tl.logger.Info().Str("dir", tl.toolsDir).Msg("tools directory does not exist, creating")
			if mkErr := os.MkdirAll(tl.toolsDir, 0755); mkErr != nil {
				return fmt.Errorf("failed to create tools directory: %w", mkErr)
			}
			return nil
		}
		return fmt.Errorf("failed to read tools directory: %w", err)
	}

	newTools := make(map[string]llm.Tool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		tool, err := tl.loadToolFile(filepath.Join(tl.toolsDir, entry.Name()))
		if err != nil {
			tl.logger.Warn().Err(err).Str("file", entry.Name()).Msg("failed to load tool file")
			continue
		}

		newTools[tool.Function.Name] = tool
	}

	tl.mu.Lock()
	tl.tools = newTools
	tl.mu.Unlock()

	tl.logger.Info().Int("count", len(newTools)).Msg("tools loaded")

	return nil
}

// loadToolFile loads a single tool from a JSON file
func (tl *ToolLoader) loadToolFile(path string) (llm.Tool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return llm.Tool{}, fmt.Errorf("failed to read file: %w", err)
	}

	// Try to parse as OpenAI function format first
	var tool llm.Tool
	if err := json.Unmarshal(data, &tool); err == nil && tool.Type == "function" {
		return tool, nil
	}

	// Try to parse as just the function definition
	var funcDef struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	}
	if err := json.Unmarshal(data, &funcDef); err != nil {
		return llm.Tool{}, fmt.Errorf("failed to parse tool JSON: %w", err)
	}

	// Convert parameters to proper format
	params, err := json.Marshal(funcDef.Parameters)
	if err != nil {
		return llm.Tool{}, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	var paramsObj map[string]interface{}
	json.Unmarshal(params, &paramsObj)

	return llm.Tool{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        funcDef.Name,
			Description: funcDef.Description,
			Parameters:  paramsObj,
		},
	}, nil
}
