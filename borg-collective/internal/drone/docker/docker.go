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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/model"
)

type Docker struct {
	dc        *client.Client
	b         *borg.Borg
	scheduler *cron.Cron
	projects  map[string]*model.ContainerBackupProject
}

func New(dc *client.Client, b *borg.Borg, scheduler *cron.Cron) *Docker {
	return &Docker{
		dc:        dc,
		b:         b,
		scheduler: scheduler,
		projects:  make(map[string]*model.ContainerBackupProject),
	}
}

func (d *Docker) Start() error {
	containerList, err := d.dc.ContainerList(
		context.Background(),
		container.ListOptions{
			All:     true,
			Filters: filters.NewArgs(filters.Arg("label", "io.v47.borgd.enabled=true")),
		},
	)

	if err != nil {
		return err
	}

	for _, containerSummary := range containerList {
		err = d.handleContainerUpdated(containerSummary.ID)
	}

	dcEvents, errChan := d.dc.Events(context.Background(), events.ListOptions{
		Since: "",
		Until: "",
		Filters: filters.NewArgs(
			filters.Arg("label", "io.v47.borgd.enabled"),
			filters.Arg("event", "create"),
			filters.Arg("event", "update"),
			filters.Arg("event", "destroy"),
			filters.Arg("event", "mount"),
			filters.Arg("event", "umount"),
		),
	})

	for event := range dcEvents {
		eventHandled := false

		if event.Type == events.ContainerEventType {
			if event.Action == "create" || event.Action == "update" {
				eventHandled = true
				err = d.handleContainerUpdated(event.Actor.ID)
			} else if event.Action == "destroy" {
				eventHandled = true
				err = d.handleContainerDestroyed(event.Actor.ID)
			}
		} else if event.Type == events.VolumeEventType {
			if event.Action == "mount" {
				eventHandled = true
				err = d.handleContainerUpdated(event.Actor.Attributes["container"])
			} else if event.Action == "umount" {
				eventHandled = true
				err = d.handleContainerUpdated(event.Actor.Attributes["container"])
			}
		}

		if !eventHandled {
			evtJson, _ := json.Marshal(event)
			log.Debug().RawJSON("event", evtJson).Msg("received unhandled event from docker daemon")
		} else if err != nil {
			evtJson, _ := json.Marshal(event)
			log.Warn().
				Err(err).
				RawJSON("event", evtJson).
				Msg("failed to handle event")
		}
	}

	return <-errChan
}

func (d *Docker) findProjectForContainer(id string) *model.ContainerBackupProject {
	var result *model.ContainerBackupProject
	for _, project := range d.projects {
		for _, bc := range project.Containers {
			if bc.ID == id {
				result = project
			}
		}
	}

	return result
}

func (d *Docker) handleContainerUpdated(id string) error {
	inspect, err := d.dc.ContainerInspect(context.Background(), id)
	if err != nil {
		return err
	}

	project := d.findProjectForContainer(id)
	if project == nil {
		project, err = mapLabelsToProject(inspect.Config.Labels)
		if err != nil {
			return err
		}

		if existingProject, ok := d.projects[project.ProjectName]; ok {
			project = existingProject
		} else {
			d.projects[project.ProjectName] = project
		}
	}

	containerBackup, err := mapInspectToContainerBackup(inspect)
	if err != nil {
		return err
	}

	project.Containers[containerBackup.ID] = *containerBackup

	return d.rescheduleProject(project)
}

func (d *Docker) handleContainerDestroyed(id string) error {
	project := d.findProjectForContainer(id)
	if project == nil {
		return nil
	}

	delete(project.Containers, id)
	if len(project.Containers) == 0 {
		if project.JobId != nil {
			d.scheduler.Remove(*project.JobId)
		}

		delete(d.projects, project.ProjectName)
	}

	return d.rescheduleProject(project)
}

func (d *Docker) rescheduleProject(project *model.ContainerBackupProject) error {
	newBackupAction, err := d.createProjectBackupAction(*project)
	if err != nil {
		return err
	}

	if project.JobId != nil {
		d.scheduler.Remove(*project.JobId)
	}

	jobId := d.scheduler.Schedule(project.Schedule, borg.Wrap(newBackupAction))
	project.JobId = &jobId

	return nil
}

func (d *Docker) createProjectBackupAction(project model.ContainerBackupProject) (borg.Action, error) {
	nodes, err := createBackupNodes(&project)
	if err != nil {
		return nil, err
	}

	sortedNodes := topologicalSort(nodes)

	backupSequence := borg.NewSequenceAction()
	for _, node := range sortedNodes {
		backupAction, err := d.createBackupAction(node)
		if err != nil {
			return nil, err
		}

		backupSequence.Push(backupAction)
	}

	finalAction := borg.NewComposedAction(backupSequence)
	finalAction.Finally(d.ensureAllContainersRunningAction(nodes))

	return finalAction, nil
}

func (d *Docker) createBackupAction(node *backupNode) (borg.Action, error) {
	composedAction := borg.NewComposedAction(d.createBackupWorkAction(node))
	switch node.Backup.Mode {
	case model.BackupModeDefault:
		composedAction.Pre(d.ensureContainerRunningAction(node))
		break
	case model.BackupModeDependentOffline:
		neededStopped, err := d.ensureNeededStoppedAction(node)
		if err != nil {
			return nil, err
		}

		composedAction.Pre(
			borg.NewSequenceAction().
				Push(neededStopped).
				Push(d.ensureContainerRunningAction(node)),
		)

		break
	case model.BackupModeOffline:
		composedAction.Pre(d.ensureContainerStoppedAction(node))
		break
	}

	return composedAction, nil
}

func (d *Docker) waitForExec(ctx context.Context, execId string) error {
	for {
		execInspect, err := d.dc.ContainerExecInspect(ctx, execId)
		if err != nil {
			return err
		}

		if !execInspect.Running {
			if execInspect.ExitCode != 0 {
				newErr := fmt.Errorf("exec container exited with %d", execInspect.ExitCode)
				log.Error().
					Err(newErr).
					AnErr("original_err", err).
					Str("container_id", execInspect.ContainerID).
					Int("exit_code", execInspect.ExitCode).
					Msg("exec container finished unsuccessfully")

				return newErr
			}

			break
		}
	}

	return nil
}
