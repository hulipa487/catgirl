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

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type DockerService struct {
	logger       zerolog.Logger
	cli          *client.Client
	registry     string
	defaultImage string
}

type ContainerInfo struct {
	ContainerID string
	TaskID     uuid.UUID
	CreatedAt  time.Time
}

// NewDockerService creates a new Docker service
func NewDockerService(logger zerolog.Logger, registry string, defaultImage string) (*DockerService, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &DockerService{
		logger:       logger,
		cli:          cli,
		registry:     registry,
		defaultImage: defaultImage,
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
		// Use the configured default image
		if s.registry != "" {
			image = s.registry + "/" + s.defaultImage
		} else {
			image = s.defaultImage
		}
	}

	// Pull image if needed
	if err := s.PullImage(ctx, image); err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	// Create container
	resp, err := s.cli.ContainerCreate(ctx, client.ContainerCreateOptions{
		Image: image,
		Config: &container.Config{
			Env: []string{
				fmt.Sprintf("TASK_ID=%s", taskID.String()),
			},
			Tty:       true,
			OpenStdin: true,
		},
		HostConfig: &container.HostConfig{
			AutoRemove: true,
			Resources: container.Resources{
				Memory: 512 * 1024 * 1024, // 512MB limit
			},
		},
		Name: "catgirl-" + taskID.String()[:8],
	})
	if err != nil {
		return "", fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	_, err = s.cli.ContainerStart(ctx, resp.ID, client.ContainerStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start container: %w", err)
	}

	return resp.ID, nil
}

// PullImage pulls a Docker image
func (s *DockerService) PullImage(ctx context.Context, image string) error {
	resp, err := s.cli.ImagePull(ctx, image, client.ImagePullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer resp.Close()

	// Wait for pull to complete
	buf := make([]byte, 1024)
	for {
		_, err := resp.Read(buf)
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
	timeout := 10
	_, err := s.cli.ContainerStop(ctx, containerID, client.ContainerStopOptions{Timeout: &timeout})
	return err
}

// ExecuteCode executes code in a container and returns the output
func (s *DockerService) ExecuteCode(ctx context.Context, containerID string, code string, language string) (string, error) {
	// First try via HTTP API if container has it
	output, err := s.ExecuteCodeViaAPI(ctx, containerID, code, language)
	if err == nil {
		return output, nil
	}

	// Fallback to direct exec using moby client
	s.logger.Debug().Msg("HTTP API not available, using container exec")

	execResp, err := s.cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		AttachStdout: true,
		AttachStderr: true,
		TTY:          true,
		Cmd:          []string{language, "-c", code},
	})
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	// Start exec
	_, err = s.cli.ExecStart(ctx, execResp.ID, client.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to start exec: %w", err)
	}

	// Attach to get output
	attachResp, err := s.cli.ExecAttach(ctx, execResp.ID, client.ExecAttachOptions{
		TTY: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer attachResp.Close()

	// Read output using the hijacked connection
	var outputBuf bytes.Buffer
	buf := make([]byte, 1024)
	for {
		n, err := attachResp.Conn.Read(buf)
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
	inspectResp, err := s.cli.ExecInspect(ctx, execResp.ID, client.ExecInspectOptions{})
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
	// Get container IP - try to get from default network
	inspectResult, err := s.cli.ContainerInspect(ctx, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	var ip string
	if inspectResult.Container.NetworkSettings != nil {
		// Try to get IP from default network
		for name, network := range inspectResult.Container.NetworkSettings.Networks {
			_ = name // unused
			if network.IPAddress.String() != "" {
				ip = network.IPAddress.String()
				break
			}
		}
	}
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
	execResp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute code: %w", err)
	}
	defer execResp.Body.Close()

	body, _ := io.ReadAll(execResp.Body)
	if execResp.StatusCode != 200 {
		return "", fmt.Errorf("execution failed: %s", string(body))
	}

	var response map[string]string
	json.Unmarshal(body, &response)
	return response["output"], nil
}

// Close closes the Docker client
func (s *DockerService) Close() {
	if s.cli != nil {
		s.cli.Close()
	}
}
