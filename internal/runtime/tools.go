package runtime

import (
	"context"
	"encoding/json"

	"github.com/hulipa487/catgirl/internal/repository"
	"github.com/hulipa487/catgirl/internal/services/llm"
)

// LoadToolsFromDB loads active tools from the database and converts them to llm.Tool format
func LoadToolsFromDB(ctx context.Context, repo *repository.Repository) ([]llm.Tool, error) {
	dbTools, err := repo.ListActiveTools(ctx)
	if err != nil {
		return nil, err
	}

	var tools []llm.Tool
	for _, t := range dbTools {
		var params map[string]interface{}
		if err := json.Unmarshal(t.Parameters, &params); err != nil {
			continue
		}

		tools = append(tools, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  params,
			},
		})
	}

	return tools, nil
}
