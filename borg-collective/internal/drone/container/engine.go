package container

import (
	"context"
	"io"
)

type Engine interface {
	EnsureContainerRunning(ctx context.Context, containerID string) error
	EnsureContainerStopped(ctx context.Context, containerID string) error

	Exec(ctx context.Context, containerID string, cmd []string) error
	ExecWithOutput(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error)
}
