package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	cfgpkg "github.com/hulipa487/catgirl/internal/config"
	dockersvc "github.com/hulipa487/catgirl/internal/services/docker"
	"github.com/rs/zerolog"
)

func main() {
	cfgPath := "catgirl.conf"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	cfg, err := cfgpkg.Load(cfgPath)
	if err != nil {
		fmt.Printf("LOAD_CONFIG_ERROR: %v\n", err)
		os.Exit(1)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	svc, err := dockersvc.NewDockerService(logger, cfg.RuntimeSeed.Global.DockerRegistry, cfg.RuntimeSeed.Global.DockerImage)
	if err != nil {
		fmt.Printf("NEW_DOCKER_SERVICE_ERROR: %v\n", err)
		os.Exit(1)
	}

	mgr := dockersvc.NewContainerManager(svc)
	workerInstanceID := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	info, err := mgr.GetOrCreateContainer(ctx, workerInstanceID, "")
	if err != nil {
		fmt.Printf("CREATE_ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("CREATED worker_instance_id=%s container_id=%s\n", workerInstanceID.String(), info.ContainerID)
	time.Sleep(2 * time.Second)

	if err := mgr.ReleaseContainer(ctx, workerInstanceID); err != nil {
		fmt.Printf("RELEASE_ERROR: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("RELEASED worker_instance_id=%s container_id=%s\n", workerInstanceID.String(), info.ContainerID)
}
