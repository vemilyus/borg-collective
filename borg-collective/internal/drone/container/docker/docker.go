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
	"github.com/vemilyus/borg-collective/internal/utils"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
)

type Client struct {
	dc         *client.Client
	cacheMutex sync.Mutex
	cache      map[string]model.ContainerBackupProject
}

func NewClient(dc *client.Client) *Client {
	return &Client{
		dc:    dc,
		cache: make(map[string]model.ContainerBackupProject),
	}
}

func (c *Client) EnsureContainerRunning(ctx context.Context, containerID string) error {
	var inspect container.InspectResponse
	var err error

	log.Debug().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Str("container", containerID).
		Msg("checking if container is running:")

	if inspect, err = c.dc.ContainerInspect(ctx, containerID); err == nil {
		if !inspect.State.Running {
			log.Info().
				Ctx(ctx).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", containerID).
				Msg("starting container")

			err = c.dc.ContainerStart(ctx, containerID, container.StartOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		return err
	}

	return nil
}

func (c *Client) EnsureContainerStopped(ctx context.Context, containerID string) error {
	var inspect container.InspectResponse
	var err error

	log.Debug().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Str("container", containerID).
		Msg("checking if container is stopped")

	if inspect, err = c.dc.ContainerInspect(ctx, containerID); err == nil {
		if !inspect.State.Dead {
			log.Info().
				Ctx(ctx).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", containerID).
				Msg("stopping container")

			err = c.dc.ContainerStop(ctx, containerID, container.StopOptions{})
			if err != nil {
				return err
			}
		}
	} else {
		return err
	}

	return nil
}

func (c *Client) Exec(ctx context.Context, containerID string, cmd []string) error {
	log.Info().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Str("command", strings.Join(cmd, " ")).
		Str("container", containerID).
		Msg("executing command in container")

	exec, err := c.dc.ContainerExecCreate(
		ctx,
		containerID,
		container.ExecOptions{Cmd: cmd},
	)

	if err != nil {
		return err
	}

	err = c.dc.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return err
	}

	return c.waitForExec(ctx, exec.ID)
}

type execAttachWrapper struct {
	response    types.HijackedResponse
	err         chan error
	gotErrValue bool
	returnedErr error
}

func (e *execAttachWrapper) Close() error {
	return e.response.Conn.Close()
}

func (e *execAttachWrapper) Read(p []byte) (n int, err error) {
	return e.response.Conn.Read(p)
}

func (e *execAttachWrapper) Error() error {
	if !e.gotErrValue {
		retErr := <-e.err
		e.returnedErr = retErr
		e.gotErrValue = true
	}

	return e.returnedErr
}

func (c *Client) ExecWithOutput(ctx context.Context, containerID string, cmd []string) (utils.ErrorReadCloser, error) {
	log.Info().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Str("command", strings.Join(cmd, " ")).
		Str("container", containerID).
		Msg("executing command (for output) in container")

	exec, err := c.dc.ContainerExecCreate(
		ctx,
		containerID,
		container.ExecOptions{
			Cmd:          cmd,
			AttachStdout: true,
		},
	)

	if err != nil {
		return nil, err
	}

	attach, err := c.dc.ContainerExecAttach(ctx, containerID, container.ExecAttachOptions{})
	if err != nil {
		return nil, err
	}

	err = c.dc.ContainerExecStart(ctx, exec.ID, container.ExecStartOptions{})
	if err != nil {
		return nil, err
	}

	wrapper := &execAttachWrapper{response: attach}

	go func() {
		err = c.waitForExec(ctx, exec.ID)
		wrapper.err <- err
	}()

	return wrapper, nil
}

func (c *Client) waitForExec(ctx context.Context, execID string) error {
	for {
		execInspect, err := c.dc.ContainerExecInspect(ctx, execID)
		if err != nil {
			return err
		}

		if !execInspect.Running {
			if execInspect.ExitCode != 0 {
				newErr := fmt.Errorf("exec container exited with %d", execInspect.ExitCode)
				log.Error().
					Ctx(ctx).
					Err(newErr).
					Str("engine", (string)(model.ContainerEngineDocker)).
					AnErr("originalErr", err).
					Str("container", execInspect.ContainerID).
					Int("exitCode", execInspect.ExitCode).
					Msg("container exec failed")

				return newErr
			}

			break
		}
	}

	return nil
}

func (c *Client) ReadProjects(ctx context.Context) ([]model.ContainerBackupProject, error) {
	log.Info().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Msg("reading container backup projects")

	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	containerList, err := c.dc.ContainerList(
		ctx,
		container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("label", model.LabelBorgdEnabled)),
		},
	)

	if err != nil {
		return nil, err
	}

	projects := make(map[string]model.ContainerBackupProject)
	for _, ctnr := range containerList {
		inspect, err := c.dc.ContainerInspect(ctx, ctnr.ID)
		if err != nil {
			return nil, err
		}

		if !isBorgdEnabled(inspect) {
			log.Info().
				Ctx(ctx).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", ctnr.ID).
				Msg("container not enabled for borgd")

			continue
		}

		project, err := findOrCreateProject(projects, inspect)
		if err != nil {
			log.Warn().
				Ctx(ctx).
				Err(err).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", ctnr.ID).
				Msg("failed to find or create project")

			continue
		}

		backup, err := mapInspectToContainerBackup(inspect)
		if err != nil {
			log.Warn().
				Ctx(ctx).
				Err(err).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", ctnr.ID).
				Msg("failed to map inspect to container backup")

			continue
		}

		if log.Debug().Enabled() {
			if _, found := projects[project.ProjectName]; !found {
				projectJson, _ := json.Marshal(project)
				log.Debug().
					Ctx(ctx).
					Str("engine", (string)(model.ContainerEngineDocker)).
					RawJSON("project", projectJson).
					Str("projectName", project.ProjectName).
					Msg("found container backup project")
			}

			if log.Debug().Enabled() {
				backupJson, _ := json.Marshal(backup)
				log.Debug().
					Ctx(ctx).
					Str("engine", (string)(model.ContainerEngineDocker)).
					RawJSON("backup", backupJson).
					Str("container", ctnr.ID).
					Msg("found container backup")
			}
		}

		project.Containers[backup.ServiceName] = *backup
		projects[project.ProjectName] = project
	}

	c.cache = projects

	return slices.Collect(maps.Values(projects)), nil
}

func findOrCreateProject(projects map[string]model.ContainerBackupProject, inspect container.InspectResponse) (model.ContainerBackupProject, error) {
	newProject, err := mapInspectToProject(inspect)
	if err != nil {
		return model.ContainerBackupProject{}, err
	}

	project, found := projects[newProject.ProjectName]
	if !found {
		return *newProject, nil
	} else {
		return project, nil
	}
}

func isBorgdEnabled(inspect container.InspectResponse) bool {
	borgdEnabledRaw, found := inspect.Config.Labels[model.LabelBorgdEnabled]
	return found && borgdEnabledRaw == "true"
}
