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
	"context"
	"encoding/json"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/drone/container/docker"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
)

type Worker struct {
	configPath     string
	borgClient     *borg.Client
	dockerClient   *docker.Client
	scheduler      *cron.Cron
	schedulerMutex sync.Mutex
	ctx            context.Context
	ctxCancel      context.CancelFunc
	compactJobId   cron.EntryID
	staticJobIds   []cron.EntryID
	dockerJobIds   map[string]cron.EntryID
}

func NewWorker(
	parentCtx context.Context,
	configPath string,
	borgClient *borg.Client,
	dockerClient *docker.Client,
	scheduler *cron.Cron,
) *Worker {
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	wCtx, cancel := context.WithCancel(parentCtx)
	s := &Worker{
		configPath:   configPath,
		borgClient:   borgClient,
		dockerClient: dockerClient,
		scheduler:    scheduler,
		ctx:          wCtx,
		ctxCancel:    cancel,
		staticJobIds: make([]cron.EntryID, 0),
		dockerJobIds: make(map[string]cron.EntryID),
	}

	return s
}

func (w *Worker) Run() error {
	defer w.ctxCancel()

	configWatch, err := config.NewWatch(w.ctx, w.configPath)
	if err != nil {
		return err
	}

	var dockerUpdates <-chan model.ContainerBackupProject
	var dockerErrors <-chan error
	if w.dockerClient != nil {
		dockerWatch, err := w.dockerClient.Watch(w.ctx)
		if err != nil {
			return err
		}

		defer func() { _ = dockerWatch.Close() }()

		dockerUpdates = dockerWatch.Updates()
		dockerErrors = dockerWatch.Errors()
	}

	log.Info().Ctx(w.ctx).Msg("starting cron scheduler")

	w.scheduler.Start()
	defer w.scheduler.Stop()

	for {
		select {
		case cfg := <-configWatch.Updates():
			w.borgClient.SetConfig(cfg)
			w.ScheduleRepoCompaction(cfg)
			w.ScheduleStaticBackups(cfg.Backups)
		case err = <-configWatch.Errors():
			return err
		case proj := <-dockerUpdates:
			if proj.Engine == model.ContainerEngineDocker {
				err = w.scheduleDockerBackup(proj)
				if err != nil {
					log.Warn().
						Ctx(w.ctx).
						Err(err).
						Msg("failed to schedule Docker backup project")
				}
			}
		case err = <-dockerErrors:
			return err
		case <-w.ctx.Done():
			return nil
		}
	}
}

func (w *Worker) RunOnce() error {
	defer w.ctxCancel()

	log.Info().Ctx(w.ctx).Msg("executing all backup jobs once")

	for _, entry := range w.scheduler.Entries() {
		entry.WrappedJob.Run()
	}

	_ = w.borgClient.Compact()

	return nil
}

func (w *Worker) ScheduleRepoCompaction(cfg config.Config) {
	w.schedulerMutex.Lock()
	defer w.schedulerMutex.Unlock()

	if w.compactJobId != 0 {
		w.scheduler.Remove(w.compactJobId)
		w.compactJobId = 0
	}

	compactionSchedule := cfg.Repo.CompactionSchedule()
	if compactionSchedule != nil {
		w.compactJobId = w.scheduler.Schedule(compactionSchedule, newRepoCompactionJob(w.borgClient))
	}
}

func (w *Worker) ScheduleStaticBackups(backups []config.BackupConfig) {
	w.schedulerMutex.Lock()
	defer w.schedulerMutex.Unlock()

	for _, jobId := range w.staticJobIds[:] {
		w.scheduler.Remove(jobId)
	}

	w.staticJobIds = make([]cron.EntryID, 0)

	for _, backup := range backups {
		job := w.newStaticBackupJob(backup)

		backupJson, _ := json.Marshal(backup)
		log.Info().
			Ctx(w.ctx).
			RawJSON("backup", backupJson).
			Msg("scheduling static backup")

		jobId := w.scheduler.Schedule(backup.Schedule(), job)
		w.staticJobIds = append(w.staticJobIds, jobId)
	}
}

func (w *Worker) ScheduleContainerBackups(backups []model.ContainerBackupProject) error {
	for _, cbp := range backups {
		if cbp.Engine == model.ContainerEngineDocker {
			err := w.scheduleDockerBackup(cbp)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (w *Worker) scheduleDockerBackup(cbp model.ContainerBackupProject) error {
	w.schedulerMutex.Lock()
	defer w.schedulerMutex.Unlock()

	jobId, found := w.dockerJobIds[cbp.ProjectName]
	if found {
		log.Info().
			Ctx(w.ctx).
			Str("projectName", cbp.ProjectName).
			Msg("unscheduling container backup project")

		w.scheduler.Remove(jobId)
		delete(w.dockerJobIds, cbp.ProjectName)
	}

	if len(cbp.Containers) > 0 {
		job, err := w.newContainerProjectBackupJob(cbp)
		if err != nil {
			return err
		}

		cbpJson, _ := json.Marshal(cbp)
		log.Info().
			Ctx(w.ctx).
			RawJSON("project", cbpJson).
			Msg("scheduling container backup project")

		jobId = w.scheduler.Schedule(cbp.Schedule, job)
		w.dockerJobIds[cbp.ProjectName] = jobId
	}

	return nil
}
