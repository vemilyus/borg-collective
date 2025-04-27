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
	"github.com/docker/docker/client"
	"github.com/robfig/cron/v3"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/model"
)

type Docker struct {
	dc        *client.Client
	b         *borg.Borg
	scheduler *cron.Cron
}

func New(dc *client.Client, b *borg.Borg, scheduler *cron.Cron) *Docker {
	return &Docker{dc, b, scheduler}
}

func (c *Docker) CreateBackupProjects() ([]*model.ContainerBackupProject, error) {
	return []*model.ContainerBackupProject{}, nil
}

func (c *Docker) ScheduleBackups(backups []*model.ContainerBackupProject) error {
	for _, backup := range backups {
		if backup.JobId != nil {
			c.scheduler.Remove(*backup.JobId)
			backup.JobId = nil
		}

		action, err := newDockerBackupAction(backup)
		if err != nil {
			return err
		}

		jobId := c.scheduler.Schedule(backup.Schedule, borg.Wrap(action))
		backup.JobId = &jobId
	}

	return nil
}

func (c *Docker) Listen(backups *model.Backups) error {
	return nil
}

func newDockerBackupAction(backup *model.ContainerBackupProject) (borg.Action, error) {
	// TODO
	return nil, nil
}
