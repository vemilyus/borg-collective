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
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
	"github.com/vemilyus/borg-collective/internal/utils"
)

const loopBackoff = 100 * time.Millisecond
const defaultTimeout = 30 * time.Second

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

		if inspect.State.Health != nil {
			timeout, cancel := context.WithTimeout(ctx, defaultTimeout)
			defer cancel()

			started := make(chan error)
			go func() {
				for {
					inspect, err = c.dc.ContainerInspect(timeout, containerID)
					if err != nil {
						started <- err
						break
					}

					if inspect.State.Health.Status == container.Unhealthy {
						started <- errors.New("container is unhealthy: " + containerID)
						break
					} else if inspect.State.Health.Status == container.Healthy {
						started <- nil
						break
					}

					time.Sleep(loopBackoff)
				}
			}()

			select {
			case err = <-started:
				return err
			case <-timeout.Done():
				return errors.New("timed out waiting for container to become healthy: " + containerID)
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
		if inspect.State.Status != container.StateExited {
			log.Info().
				Ctx(ctx).
				Str("engine", (string)(model.ContainerEngineDocker)).
				Str("container", containerID).
				Msg("stopping container")

			timeout := (int)(defaultTimeout / time.Second)
			err = c.dc.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
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

	inspect, err := c.dc.ContainerInspect(ctx, containerID)
	if err != nil {
		return err
	}

	envMap := utils.ToMap(inspect.Config.Env)
	cmd = expandCmd(cmd, envMap)

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
	io.Reader
	response    types.HijackedResponse
	errMutex    sync.Mutex
	err         chan error
	gotErrValue bool
	returnedErr error
}

func (e *execAttachWrapper) Error() error {
	if !e.gotErrValue {
		e.errMutex.Lock()
		defer e.errMutex.Unlock()

		if !e.gotErrValue {
			retErr := <-e.err
			e.returnedErr = retErr
			e.gotErrValue = true
		}
	}

	return e.returnedErr
}

func (c *Client) ExecWithOutput(ctx context.Context, containerID string, cmd []string) (utils.ErrorReader, error) {
	log.Info().
		Ctx(ctx).
		Str("engine", (string)(model.ContainerEngineDocker)).
		Strs("command", cmd).
		Str("container", containerID).
		Msg("executing command (for output) in container")

	inspect, err := c.dc.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	envMap := utils.ToMap(inspect.Config.Env)
	cmd = expandCmd(cmd, envMap)

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

	attach, err := c.dc.ContainerExecAttach(ctx, exec.ID, container.ExecAttachOptions{})
	if err != nil {
		return nil, err
	}

	// pipe is required because Docker multiplexes stdout and stderr into one stream
	// and stdCopy is required to separate those streams (look to goroutine below)
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	wrapper := &execAttachWrapper{
		Reader: reader,
		err:    make(chan error, 1),
	}

	go func() {
		defer func() { _ = writer.Close() }()

		_, err = stdcopy.StdCopy(writer, nil, attach.Reader)
		if err != nil {
			wrapper.err <- err
			return
		}

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
				err = fmt.Errorf("exec container exited with %d", execInspect.ExitCode)
				log.Error().
					Ctx(ctx).
					Err(err).
					Str("engine", (string)(model.ContainerEngineDocker)).
					Str("container", execInspect.ContainerID).
					Int("exitCode", execInspect.ExitCode).
					Msg("container exec failed")

				return err
			}

			break
		}

		time.Sleep(loopBackoff)
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

var envVarRegex = regexp.MustCompile(`&\{(\S+?)}|&\S+?`)

func expandCmd(cmd []string, env map[string]string) []string {
	for i := range cmd {
		cmd[i] = envVarRegex.ReplaceAllStringFunc(cmd[i], func(s string) string {
			var name string
			if s[1] == '{' {
				name = s[2 : len(s)-1]
			} else {
				name = s[1:]
			}
			value, found := env[name]
			if !found {
				return s
			} else {
				return value
			}
		})
	}

	return cmd
}
