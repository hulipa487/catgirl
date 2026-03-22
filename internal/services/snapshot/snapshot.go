package snapshot

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

type SnapshotService struct {
	repo   *repository.Repository
	config *config.SnapshotConfig
	logger zerolog.Logger
}

func NewSnapshotService(repo *repository.Repository, cfg *config.SnapshotConfig, logger zerolog.Logger) *SnapshotService {
	return &SnapshotService{
		repo:   repo,
		config: cfg,
		logger: logger,
	}
}

func (s *SnapshotService) CreateSnapshot(ctx context.Context, tf *models.TaskFamily, reason models.SnapshotReason) (*models.ContainerSnapshot, error) {
	if !s.config.Enabled {
		return nil, nil
	}

	snapshotID := uuid.New()
	imageName := fmt.Sprintf("snapshot_%s_%d", tf.TaskID.String()[:8], time.Now().Unix())

	envJSON, _ := json.Marshal(map[string]string{
		"CATGIRL_TASK_ID":   tf.TaskID.String(),
		"CATGIRL_SESSION_ID": tf.SessionID.String(),
	})

	metadata := map[string]interface{}{
		"execution_time_seconds": 0,
		"memory_used_bytes":    0,
		"cpu_time_seconds":     0,
	}
	metadataJSON, _ := json.Marshal(metadata)

	expiresAt := s.calculateExpiresAt(reason)

	snapshot := &models.ContainerSnapshot{
		SnapshotID:  snapshotID,
		TaskID:      tf.TaskID,
		SessionID:   tf.SessionID,
		ContainerID: tf.ContainerID,
		ImageID:     "",
		ImageName:   imageName,
		Reason:      reason,
		Volumes:     json.RawMessage(`{}`),
		Environment: envJSON,
		Metadata:    metadataJSON,
		CreatedAt:   time.Now(),
		ExpiresAt:   expiresAt,
	}

	if err := s.repo.CreateContainerSnapshot(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	tf.ContainerSnapshotID = &snapshotID
	s.logger.Info().
		Str("snapshot_id", snapshotID.String()).
		Str("task_id", tf.TaskID.String()).
		Str("reason", string(reason)).
		Msg("snapshot created")

	return snapshot, nil
}

func (s *SnapshotService) GetSnapshot(ctx context.Context, snapshotID uuid.UUID) (*models.ContainerSnapshot, error) {
	return s.repo.GetContainerSnapshot(ctx, snapshotID)
}

func (s *SnapshotService) ListSnapshotsBySession(ctx context.Context, sessionID uuid.UUID, limit, offset int) ([]*models.ContainerSnapshot, error) {
	return s.repo.ListContainerSnapshotsBySession(ctx, sessionID, limit, offset)
}

func (s *SnapshotService) RecallSnapshot(ctx context.Context, snapshotID uuid.UUID) (string, error) {
	snapshot, err := s.repo.GetContainerSnapshot(ctx, snapshotID)
	if err != nil {
		return "", err
	}
	if snapshot == nil {
		return "", fmt.Errorf("snapshot not found: %s", snapshotID)
	}

	newContainerID := fmt.Sprintf("catgirl_recalled_%s", snapshotID.String()[:8])

	s.logger.Info().
		Str("snapshot_id", snapshotID.String()).
		Str("new_container_id", newContainerID).
		Msg("snapshot recalled")

	return newContainerID, nil
}

func (s *SnapshotService) DeleteSnapshot(ctx context.Context, snapshotID uuid.UUID) error {
	return s.repo.DeleteContainerSnapshot(ctx, snapshotID)
}

func (s *SnapshotService) CleanupExpiredSnapshots(ctx context.Context) error {
	expired, err := s.repo.ListExpiredSnapshots(ctx)
	if err != nil {
		return fmt.Errorf("failed to list expired snapshots: %w", err)
	}

	for _, snapshot := range expired {
		if err := s.DeleteSnapshot(ctx, snapshot.SnapshotID); err != nil {
			s.logger.Error().
				Err(err).
				Str("snapshot_id", snapshot.SnapshotID.String()).
				Msg("failed to delete expired snapshot")
		} else {
			s.logger.Info().
				Str("snapshot_id", snapshot.SnapshotID.String()).
				Msg("deleted expired snapshot")
		}
	}

	return nil
}

func (s *SnapshotService) calculateExpiresAt(reason models.SnapshotReason) *time.Time {
	var duration time.Duration
	switch reason {
	case models.SnapshotReasonCompleted:
		duration = parseDuration(s.config.Retention.Completed)
	case models.SnapshotReasonFailed:
		duration = parseDuration(s.config.Retention.Failed)
	case models.SnapshotReasonExited:
		duration = parseDuration(s.config.Retention.Exited)
	case models.SnapshotReasonInterrupted:
		duration = parseDuration(s.config.Retention.Interrupted)
	default:
		duration = 7 * 24 * time.Hour
	}

	expiresAt := time.Now().Add(duration)
	return &expiresAt
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 7 * 24 * time.Hour
	}
	return d
}
