package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/moby/moby/api/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type DockerService struct {
	logger   zerolog.Logger
	cli      *client.Client
	registry string
}

type ContainerInfo struct {
	ContainerID string
	TaskID     uuid.UUID
	CreatedAt  time.Time
}

// NewDockerService creates a new Docker service
func NewDockerService(logger zerolog.Logger, registry string) (*DockerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerService{
		logger:   logger,
		cli:      cli,
		registry: registry,
	}, nil
}

// ContainerManager manages containers per task
type ContainerManager struct {
	svc        *DockerService
	containers map[uuid.UUID]*ContainerInfo
	mu         sync.RWMutex
}

// NewContainerManager creates a new container manager
func NewContainerManager(svc *DockerService) *ContainerManager {
	return &ContainerManager{
		svc:        svc,
		containers: make(map[uuid.UUID]*ContainerInfo),
	}
}

// GetOrCreateContainer gets an existing container for a task or creates a new one
func (m *ContainerManager) GetOrCreateContainer(ctx context.Context, taskID uuid.UUID, image string) (*ContainerInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Return existing container if available
	if info, ok := m.containers[taskID]; ok {
		m.svc.logger.Info().Str("task_id", taskID.String()).Str("container_id", info.ContainerID).Msg("Reusing existing container")
		return info, nil
	}

	// Create new container
	containerID, err := m.svc.CreateContainer(ctx, taskID, image)
	if err != nil {
		return nil, err
	}

	info := &ContainerInfo{
		ContainerID: containerID,
		TaskID:      taskID,
		CreatedAt:   time.Now(),
	}
	m.containers[taskID] = info

	m.svc.logger.Info().Str("task_id", taskID.String()).Str("container_id", containerID).Msg("Created new container for task")
	return info, nil
}

// ReleaseContainer removes a container for a task
func (m *ContainerManager) ReleaseContainer(ctx context.Context, taskID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, ok := m.containers[taskID]
	if !ok {
		return nil
	}

	if err := m.svc.StopContainer(ctx, info.ContainerID); err != nil {
		m.svc.logger.Warn().Err(err).Str("container_id", info.ContainerID).Msg("Failed to stop container")
	}

	delete(m.containers, taskID)
	return nil
}

// CreateContainer creates a new Docker container
func (s *DockerService) CreateContainer(ctx context.Context, taskID uuid.UUID, image string) (string, error) {
	if image == "" {
		image = s.registry + "/catgirl-runtime:latest"
	}

	// Pull image if needed
	if err := s.PullImage(ctx, image); err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	resp, err := s.cli.ContainerCreate(ctx, &container.Config{
		Image: image,
		Env: []string{
			fmt.Sprintf("TASK_ID=%s", taskID.String()),
		},
		Tty:        true,
		OpenStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{
		AutoRemove: true,
		Memory:    512 * 1024 * 1024, // 512MB limit
		CpuPeriod: 100000,
		CpuQuota:   50000, // 50% CPU
	}, nil, nil, "catgirl-"+taskID.String()[:8])
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := s.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

// PullImage pulls a Docker image
func (s *DockerService) PullImage(ctx context.Context, image string) error {
	reader, err := s.cli.ImagePull(ctx, image, types.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Wait for pull to complete
	buf := make([]byte, 1024)
	for {
		_, err := reader.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// StopContainer stops and removes a container
func (s *DockerService) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10 * time.Second
	return s.cli.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}

// ExecuteCode executes code in a container and returns the output
func (s *DockerService) ExecuteCode(ctx context.Context, containerID string, code string, language string) (string, error) {
	// First try via HTTP API if container has it
	output, err := s.ExecuteCodeViaAPI(ctx, containerID, code, language)
	if err == nil {
		return output, nil
	}

	// Fallback to direct exec
	s.logger.Debug().Msg("HTTP API not available, using direct exec")

	execResp, err := s.cli.ContainerExecCreate(ctx, containerID, types.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          []string{language, "-c", code},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	attachResp, err := s.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read output
	var outputBuf bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := attachResp.Reader.Read(buf)
		if n > 0 {
			outputBuf.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}

	// Inspect exec to get exit code
	inspectResp, err := s.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect exec: %w", err)
	}

	result := outputBuf.String()
	if inspectResp.ExitCode != 0 {
		return "", fmt.Errorf("execution failed with exit code %d: %s", inspectResp.ExitCode, result)
	}

	return result, nil
}

// ExecuteCodeViaAPI executes code using an HTTP API inside the container
func (s *DockerService) ExecuteCodeViaAPI(ctx context.Context, containerID string, code string, language string) (string, error) {
	// Get container IP
	info, err := s.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	ip := info.NetworkSettings.IPAddress
	if ip == "" {
		return "", fmt.Errorf("container has no IP address")
	}

	// Call the code execution API inside the container
	url := fmt.Sprintf("http://%s:8080/execute", ip)

	payload := map[string]string{
		"code":     code,
		"language": language,
	}

	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("execution failed: %s", string(body))
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	return result["output"], nil
}

// Close closes the Docker client
func (s *DockerService) Close() {
	if s.cli != nil {
		s.cli.Close()
	}
}
