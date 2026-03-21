package skill

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

type SkillService struct {
	repo   *repository.Repository
	config *config.Config
	logger zerolog.Logger
}

func NewSkillService(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *SkillService {
	return &SkillService{
		repo:    repo,
		config:  cfg,
		logger:  logger,
	}
}

type SkillDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Version     string                 `json:"version"`
	Prompt      string                 `json:"prompt"`
	Code        *string                `json:"code,omitempty"`
	Tools       []string               `json:"tools,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

func (s *SkillService) CreateSkill(ctx context.Context, sessionID uuid.UUID, def *SkillDefinition, createdByAgentID *string) (*models.Skill, error) {
	definitionJSON, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal skill definition: %w", err)
	}

	skill := &models.Skill{
		ID:              uuid.New(),
		SessionID:       sessionID,
		Name:            def.Name,
		Description:     def.Description,
		Version:         def.Version,
		Definition:      definitionJSON,
		Code:            def.Code,
		CreatedByAgentID: createdByAgentID,
		CreatedAt:       time.Now(),
	}

	if err := s.repo.CreateSkill(ctx, skill); err != nil {
		return nil, fmt.Errorf("failed to create skill: %w", err)
	}

	s.logger.Info().
		Str("skill_id", skill.ID.String()).
		Str("session_id", sessionID.String()).
		Str("name", skill.Name).
		Msg("skill created")

	return skill, nil
}

func (s *SkillService) GetSkill(ctx context.Context, skillID uuid.UUID) (*models.Skill, error) {
	return s.repo.GetSkill(ctx, skillID)
}

func (s *SkillService) GetSkillByName(ctx context.Context, sessionID uuid.UUID, name string) (*models.Skill, error) {
	return s.repo.GetSkillByName(ctx, sessionID, name)
}

func (s *SkillService) ListSkills(ctx context.Context, sessionID uuid.UUID) ([]*models.Skill, error) {
	return s.repo.ListSkillsBySession(ctx, sessionID)
}

func (s *SkillService) ExecuteSkill(ctx context.Context, skill *models.Skill, context map[string]interface{}) (interface{}, error) {
	var def SkillDefinition
	if err := json.Unmarshal(skill.Definition, &def); err != nil {
		return nil, fmt.Errorf("failed to parse skill definition: %w", err)
	}

	s.logger.Debug().
		Str("skill_id", skill.ID.String()).
		Str("name", skill.Name).
		Msg("executing skill")

	return map[string]interface{}{
		"skill_name": skill.Name,
		"result":    "skill executed",
		"context":    context,
	}, nil
}

func (s *SkillService) DeleteSkill(ctx context.Context, skillID uuid.UUID) error {
	return s.repo.DeleteSkill(ctx, skillID)
}
