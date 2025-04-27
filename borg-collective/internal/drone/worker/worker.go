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

package worker

import (
	"github.com/docker/docker/client"
	"github.com/robfig/cron/v3"
	"github.com/vemilyus/borg-collective/internal/drone"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/docker"
	"github.com/vemilyus/borg-collective/internal/drone/model"
)

type Worker struct {
	config    *drone.Config
	b         *borg.Borg
	scheduler *cron.Cron
	d         *docker.Docker
}

func New(scheduler *cron.Cron, config *drone.Config) (*Worker, error) {
	dc, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		return nil, err
	}

	b, err := borg.New(config)
	if err != nil {
		return nil, err
	}

	d := docker.New(dc, b, scheduler)

	return &Worker{config, b, scheduler, d}, nil
}

func (w *Worker) Start() error {
	var configuredBackups []drone.BackupConfig
	for _, backup := range w.config.Backups {
		configuredBackups = append(configuredBackups, *backup)
	}

	containerBackupProjects, err := w.d.CreateBackupProjects()
	if err != nil {
		return err
	}

	err = w.b.ScheduleBackups(configuredBackups, w.scheduler)
	if err != nil {
		return err
	}

	err = w.d.ScheduleBackups(containerBackupProjects)
	if err != nil {
		return err
	}

	backups := model.NewBackups(containerBackupProjects, configuredBackups)

	c := make(chan bool, 1)
	go func() {
		err = w.d.Listen(backups)
		c <- true
	}()

	if w.config.Repo.CompactionScheduleParsed() != nil {
		w.scheduler.Schedule(w.config.Repo.CompactionScheduleParsed(), borg.Wrap(w.b.Compact()))
	}

	_, ok := <-c
	if !ok {
		panic("channel closed")
	}

	return err
}
