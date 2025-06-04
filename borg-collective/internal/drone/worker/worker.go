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

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/drone/container/docker"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
)

type Worker struct {
	configPath   string
	borgClient   *borg.Client
	dockerClient *docker.Client
	scheduler    *cron.Cron
	ctx          context.Context
	ctxCancel    context.CancelFunc
	compactJobId cron.EntryID
	staticJobIds []cron.EntryID
	dockerJobIds map[string]cron.EntryID
}

func NewWorker(
	configPath string,
	borgClient *borg.Client,
	dockerClient *docker.Client,
	scheduler *cron.Cron,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Worker{
		configPath:   configPath,
		borgClient:   borgClient,
		dockerClient: dockerClient,
		scheduler:    scheduler,
		ctx:          ctx,
		ctxCancel:    cancel,
		staticJobIds: make([]cron.EntryID, 0),
		dockerJobIds: make(map[string]cron.EntryID),
	}

	return s
}

func (w *Worker) Run() error {
	defer w.ctxCancel()

	configWatch, err := config.NewWatch(w.configPath)
	if err != nil {
		return err
	}

	var dockerWatch *docker.Watch
	if w.dockerClient != nil {
		dockerWatch, err = w.dockerClient.Watch(w.ctx)
		if err != nil {
			return err
		}
	}

	log.Info().Ctx(w.ctx).Msg("starting cron scheduler")

	w.scheduler.Start()
	defer w.scheduler.Stop()

	for {
		select {
		case cfg := <-configWatch.Updates():
			w.borgClient.SetConfig(cfg)
			w.ScheduleRepoCompaction(cfg)

			err = w.ScheduleStaticBackups(cfg.Backups)
			if err != nil {
				log.Warn().
					Ctx(w.ctx).
					Err(err).
					Msg("failed to (re)schedule static backups")
			}
		case err = <-configWatch.Errors():
			return err
		case proj := <-dockerWatch.Updates():
			if proj.Engine == model.ContainerEngineDocker {
				err = w.scheduleDockerBackup(proj)
				if err != nil {
					log.Warn().
						Ctx(w.ctx).
						Err(err).
						Msg("failed to (re)schedule Docker backup project")
				}
			}
		case err = <-dockerWatch.Errors():
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
	if w.compactJobId != 0 {
		w.scheduler.Remove(w.compactJobId)
		w.compactJobId = 0
	}

	compactionSchedule := cfg.Repo.CompactionSchedule()
	if compactionSchedule != nil {
		w.compactJobId = w.scheduler.Schedule(compactionSchedule, newRepoCompactionJob(w.borgClient))
	}
}

func (w *Worker) ScheduleStaticBackups(backups []config.BackupConfig) error {
	prevEntries := w.scheduler.Entries()
	oldStaticJobIds := w.staticJobIds
	w.staticJobIds = make([]cron.EntryID, 0)

	for _, jobId := range oldStaticJobIds {
		w.scheduler.Remove(jobId)
	}

	for _, backup := range backups {
		job, err := w.newStaticBackupJob(backup)
		if err != nil {
			w.restorePreviousStaticBackups(oldStaticJobIds, prevEntries)
			return err
		}

		backupJson, _ := json.Marshal(backup)
		log.Info().
			Ctx(w.ctx).
			RawJSON("backup", backupJson).
			Msg("scheduling static backup")

		jobId := w.scheduler.Schedule(backup.Schedule(), job)
		w.staticJobIds = append(w.staticJobIds, jobId)
	}

	return nil
}

func (w *Worker) restorePreviousStaticBackups(prevJobIds []cron.EntryID, prevEntries []cron.Entry) {
	for _, jobId := range w.staticJobIds {
		w.scheduler.Remove(jobId)
	}

	w.staticJobIds = make([]cron.EntryID, 0)

	for _, oldJobId := range prevJobIds {
		for _, entry := range prevEntries {
			if entry.ID == oldJobId {
				jobId := w.scheduler.Schedule(entry.Schedule, entry.Job)
				w.staticJobIds = append(w.staticJobIds, jobId)
			}
		}
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
	jobId, found := w.dockerJobIds[cbp.ProjectName]
	var prevEntry cron.Entry
	if found {
		prevEntry = w.scheduler.Entry(jobId)

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
			if found {
				if log.Debug().Enabled() {
					log.Info().
						Ctx(w.ctx).
						Err(err).
						Str("projectName", cbp.ProjectName).
						Msg("restoring container backup project")
				}

				jobId = w.scheduler.Schedule(prevEntry.Schedule, prevEntry.Job)

				w.dockerJobIds[cbp.ProjectName] = jobId
			}

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
