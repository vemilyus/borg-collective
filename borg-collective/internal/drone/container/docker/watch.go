// Copyright (C) 2025 Alex Katlein
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package docker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
)

type Watch struct {
	updates chan model.ContainerBackupProject
	err     chan error
}

func (w *Watch) Close() error {
	close(w.updates)
	close(w.err)

	return nil
}

func (w *Watch) Updates() <-chan model.ContainerBackupProject {
	return w.updates
}

func (w *Watch) Errors() <-chan error {
	return w.err
}

func (c *Client) Watch(ctx context.Context) (*Watch, error) {
	dockerEvents, errChan := c.dc.Events(
		ctx,
		events.ListOptions{
			Filters: filters.NewArgs(
				filters.Arg("event", (string)(events.ActionCreate)),
				filters.Arg("event", (string)(events.ActionUpdate)),
				filters.Arg("event", (string)(events.ActionDestroy)),
			),
		},
	)

	watch := &Watch{
		updates: make(chan model.ContainerBackupProject),
		err:     make(chan error),
	}

	go func() {
		for {
			select {
			case event, ok := <-dockerEvents:
				if !ok {
					watch.err <- errors.New("Docker events channel closed")
					_ = watch.Close()
					return
				}

				project, err := c.handleEvent(ctx, event)
				if err != nil {
					watch.err <- err
				} else if project != nil {
					watch.updates <- *project
				}
			case eventsErr, ok := <-errChan:
				if !ok {
					return
				}

				watch.err <- eventsErr
			case <-ctx.Done():
				watch.err <- errors.New("watch ctx is done")
				_ = watch.Close()
				return
			}
		}
	}()

	log.Info().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Msg("watching for container changes")

	return watch, nil
}

func (c *Client) handleEvent(ctx context.Context, event events.Message) (*model.ContainerBackupProject, error) {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	eventHandled := false
	var project *model.ContainerBackupProject
	var err error

	if event.Type == events.ContainerEventType {
		if event.Action == events.ActionCreate || event.Action == events.ActionUpdate {
			eventHandled = true
			project, err = c.handleContainerUpdated(ctx, event.Actor.ID)
		} else if event.Action == events.ActionDestroy {
			eventHandled = true
			project = c.handleContainerDestroyed(event.Actor.ID)
		}
	} else {
		// we only care about events concerning containers
		eventHandled = true
	}

	if !eventHandled && log.Debug().Enabled() {
		evtJson, _ := json.Marshal(event)
		log.Debug().
			Ctx(ctx).
			RawJSON("event", evtJson).
			Msg("received unrecognized event from Docker daemon")
	}

	return project, err
}

func (c *Client) handleContainerUpdated(ctx context.Context, containerID string) (*model.ContainerBackupProject, error) {
	inspect, err := c.dc.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to inspect container %s", containerID))
	}

	if !isBorgdEnabled(inspect) {
		log.Info().
			Ctx(ctx).
			Str("engine", (string)(model.ContainerEngineDocker)).
			Str("container", containerID).
			Msg("Container not enabled for borgd")

		return nil, nil
	}

	project, err := findOrCreateProject(c.cache, inspect)
	if err != nil {
		log.Warn().
			Ctx(ctx).
			Err(err).
			Str("engine", (string)(model.ContainerEngineDocker)).
			Str("container", inspect.ID).
			Msg("failed to find or create project")

		return nil, nil
	}

	backup, err := mapInspectToContainerBackup(inspect)
	if err != nil {
		log.Warn().
			Ctx(ctx).
			Err(err).
			Str("engine", (string)(model.ContainerEngineDocker)).
			Str("container", inspect.ID).
			Msg("failed to map inspect to container backup")

		return nil, nil
	}

	if log.Debug().Enabled() {
		if _, found := c.cache[project.ProjectName]; !found {
			projectJson, _ := json.Marshal(project)
			log.Debug().
				Ctx(ctx).
				Str("engine", (string)(model.ContainerEngineDocker)).
				RawJSON("project", projectJson).
				Str("projectName", project.ProjectName).
				Msg("detected new container backup project")
		}

		backupJson, _ := json.Marshal(backup)
		log.Debug().
			Ctx(ctx).
			Str("engine", (string)(model.ContainerEngineDocker)).
			RawJSON("backup", backupJson).
			Str("container", inspect.ID).
			Msg("detected new or updated container backup")
	}

	project.Containers[backup.ServiceName] = *backup
	c.cache[project.ProjectName] = project

	return &project, nil
}

func (c *Client) handleContainerDestroyed(containerID string) *model.ContainerBackupProject {
	var project model.ContainerBackupProject
	var backup model.ContainerBackup
	var found bool
	for _, p := range c.cache {
		for _, container := range p.Containers {
			if container.ID == containerID {
				project = p
				backup = container
				found = true
				break
			}
		}
	}

	if !found {
		return nil
	}

	delete(project.Containers, backup.ServiceName)

	backupJson, _ := json.Marshal(backup)
	log.Info().
		Str("engine", (string)(model.ContainerEngineDocker)).
		RawJSON("backup", backupJson).
		Str("container", backup.ID).
		Msg("discarding container backup")

	if len(project.Containers) == 0 {
		delete(c.cache, project.ProjectName)

		projectJson, _ := json.Marshal(project)
		log.Info().
			Str("engine", (string)(model.ContainerEngineDocker)).
			RawJSON("project", projectJson).
			Str("projectName", project.ProjectName).
			Msg("discarding container backup project")
	} else {
		c.cache[project.ProjectName] = project
	}

	return &project
}
