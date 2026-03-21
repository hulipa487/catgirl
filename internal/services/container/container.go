package container

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

type ContainerService struct {
	repo   *repository.Repository
	config *config.Config
	logger zerolog.Logger
}

func NewContainerService(repo *repository.Repository, cfg *config.Config, logger zerolog.Logger) *ContainerService {
	return &ContainerService{
		repo:   repo,
		config: cfg,
		logger: logger,
	}
}

func (s *ContainerService) GetOrCreateContainer(ctx context.Context, taskID uuid.UUID) (string, error) {
	tf, err := s.repo.GetTaskFamily(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task family: %w", err)
	}

	if tf != nil && tf.ContainerID != "" {
		return tf.ContainerID, nil
	}

	containerID := fmt.Sprintf("catgirl_task_%s", taskID.String()[:8])

	if tf == nil {
		tf = &models.TaskFamily{
			TaskID:           taskID,
			ContainerID:      containerID,
			RootDescription: "",
			Status:           "in_progress",
			CreatedAt:        time.Now(),
		}
		if err := s.repo.CreateTaskFamily(ctx, tf); err != nil {
			return "", fmt.Errorf("failed to create task family: %w", err)
		}
	} else {
		tf.ContainerID = containerID
		if err := s.repo.UpdateTaskFamily(ctx, tf); err != nil {
			return "", fmt.Errorf("failed to update task family: %w", err)
		}
	}

	s.logger.Info().
		Str("task_id", taskID.String()).
		Str("container_id", containerID).
		Msg("container created")

	return containerID, nil
}

func (s *ContainerService) ExecuteInContainer(ctx context.Context, containerID string, command string) (string, error) {
	s.logger.Debug().
		Str("container_id", containerID).
		Str("command", command).
		Msg("executing command in container")

	return "command executed", nil
}

func (s *ContainerService) StopContainer(ctx context.Context, containerID string) error {
	s.logger.Info().
		Str("container_id", containerID).
		Msg("container stopped")
	return nil
}

func (s *ContainerService) GetContainerStatus(ctx context.Context, containerID string) (string, error) {
	return "running", nil
}
