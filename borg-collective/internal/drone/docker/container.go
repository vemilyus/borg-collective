package docker

import (
	"context"
	"github.com/docker/docker/api/types/container"
)

func (d *Docker) ensureContainerStopped(ctx context.Context, id string) error {
	var inspect container.InspectResponse
	var err error
	if inspect, err = d.dc.ContainerInspect(ctx, id); err == nil {
		if !inspect.State.Dead {
			err = d.dc.ContainerStop(ctx, id, container.StopOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		return err
	}

	return nil
}

func (d *Docker) ensureContainerRunning(ctx context.Context, id string) error {
	var inspect container.InspectResponse
	var err error
	if inspect, err = d.dc.ContainerInspect(ctx, id); err == nil {
		if !inspect.State.Running {
			err = d.dc.ContainerStart(ctx, id, container.StartOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		return err
	}

	return nil
}
