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
	"fmt"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/container"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
	"github.com/vemilyus/borg-collective/internal/utils"
	"golang.org/x/sync/errgroup"
)

type containerProjectBackupJob struct {
	ctx        context.Context
	engine     container.Engine
	borgClient *borg.Client
	project    model.ContainerBackupProject
	plan       containerPlan
}

func (w *Worker) newContainerProjectBackupJob(project model.ContainerBackupProject) (cron.Job, error) {
	if len(project.Containers) == 0 {
		return nil, fmt.Errorf("nothing to do")
	}

	plan := containerPlan(slices.Collect(maps.Values(project.Containers)))
	sort.Sort(plan)

	for _, ctnr := range project.Containers {
		if !ctnr.Exec.Stdout {
			for _, cPath := range ctnr.Exec.Paths {
				_, found := findSourceForInContainerPath(&ctnr, cPath)
				if !found {
					return nil, fmt.Errorf("no source for in-container path %s", cPath)
				}
			}
		}

		for _, dep := range ctnr.Dependencies {
			_, found := project.Containers[dep]
			if !found {
				return nil, fmt.Errorf("dependency %s of %s not found", dep, ctnr.ServiceName)
			}
		}
	}

	job := &containerProjectBackupJob{
		ctx:        w.ctx,
		borgClient: w.borgClient,
		project:    project,
		plan:       plan,
	}

	switch project.Engine {
	case model.ContainerEngineDocker:
		job.engine = w.dockerClient
	default:
		return nil, fmt.Errorf("unknown container engine %s", project.Engine)
	}

	return job, nil
}

func (d *containerProjectBackupJob) Run() {
	for _, backupCtnr := range d.plan {
		if !backupCtnr.NeedsBackup() {
			if log.Debug().Enabled() {
				log.Debug().
					Ctx(d.ctx).
					Fields(d.logFields(backupCtnr)).
					Msg("skipping container, backup not needed")
			}
			continue
		}

		backupName := fmt.Sprintf("%s-%s", d.project.ProjectName, backupCtnr.ServiceName)

		switch backupCtnr.Mode {
		case model.BackupModeDefault:
			d.runOnlineBackup(backupCtnr, backupName)
		case model.BackupModeDependentOffline:
			d.runDependentOfflineBackup(backupCtnr, backupName)
		case model.BackupModeOffline:
			d.runOfflineBackup(backupCtnr, backupName)
		default:
			log.Error().
				Ctx(d.ctx).
				Fields(d.logFields(backupCtnr)).
				Str("mode", backupCtnr.Mode.String()).
				Msg("unknown backup mode")
		}
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(d.project.Containers))
	for _, ctnr := range d.project.Containers {
		go func() {
			defer wg.Done()

			err := d.engine.EnsureContainerRunning(d.ctx, ctnr.ID)
			if err != nil {
				log.Warn().
					Ctx(d.ctx).
					Err(err).
					Fields(d.logFields(ctnr)).
					Msg("failed to ensure container running after backup")
			}
		}()
	}

	wg.Wait()
}

func (d *containerProjectBackupJob) runOnlineBackup(backupCtnr model.ContainerBackup, backupName string) {
	log.Info().
		Ctx(d.ctx).
		Fields(d.logFields(backupCtnr)).
		Msg("starting online backup")

	err := d.engine.EnsureContainerRunning(d.ctx, backupCtnr.ID)
	if err != nil {
		log.Warn().
			Ctx(d.ctx).
			Err(err).
			Fields(d.logFields(backupCtnr)).
			Msg("failed to ensure container running for online backup")

		return
	}

	if backupCtnr.Exec != nil {
		d.runExecBackup(backupCtnr, backupName)
	} else {
		d.runVolumeBackup(backupCtnr, backupName)
	}
}

func (d *containerProjectBackupJob) runDependentOfflineBackup(backupCtnr model.ContainerBackup, backupName string) {
	log.Info().
		Ctx(d.ctx).
		Fields(d.logFields(backupCtnr)).
		Msg("starting online backup (dependents offline)")

	err := d.engine.EnsureContainerRunning(d.ctx, backupCtnr.ID)
	if err != nil {
		log.Warn().
			Ctx(d.ctx).
			Err(err).
			Fields(d.logFields(backupCtnr)).
			Msg("failed to ensure container running for online backup (dependents offline)")

		return
	}

	dependents := d.findDependents(backupCtnr)
	if len(dependents) > 0 {
		eg, egCtx := errgroup.WithContext(d.ctx)
		eg.SetLimit(len(dependents))
		for _, dependent := range dependents {
			eg.Go(func() error {
				return d.engine.EnsureContainerStopped(egCtx, dependent.ID)
			})
		}

		err = eg.Wait()
		if err != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(err).
				Fields(d.logFields(backupCtnr)).
				Msg("failed to ensure dependent containers stopped")

			return
		}
	}

	if backupCtnr.Exec != nil {
		d.runExecBackup(backupCtnr, backupName)
	} else {
		d.runVolumeBackup(backupCtnr, backupName)
	}
}

func (d *containerProjectBackupJob) runOfflineBackup(backupCtnr model.ContainerBackup, backupName string) {
	log.Info().
		Ctx(d.ctx).
		Fields(d.logFields(backupCtnr)).
		Msg("starting offline backup")

	err := d.engine.EnsureContainerStopped(d.ctx, backupCtnr.ID)
	if err != nil {
		log.Warn().
			Ctx(d.ctx).
			Err(err).
			Fields(d.logFields(backupCtnr)).
			Msg("failed to ensure container stopped for offline backup")

		return
	}

	d.runVolumeBackup(backupCtnr, backupName)
}

func (d *containerProjectBackupJob) runExecBackup(backupCtnr model.ContainerBackup, backupName string) {
	if log.Debug().Enabled() {
		log.Debug().
			Ctx(d.ctx).
			Fields(d.logFields(backupCtnr)).
			Msg("backing up exec result")
	}

	if backupCtnr.Exec.Stdout {
		output, err := d.engine.ExecWithOutput(d.ctx, backupCtnr.ID, backupCtnr.Exec.Command)
		if err != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(err).
				Fields(d.logFields(backupCtnr)).
				Msg("failed to execute exec command")

			return
		}

		result, err := d.borgClient.CreateWithInput(d.ctx, utils.ArchiveName(backupName), output)
		if err != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(err).
				Fields(d.logFields(backupCtnr)).
				Msg("backup failed")

			return
		}

		if output.Error() != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(output.Error()).
				Fields(d.logFields(backupCtnr)).
				Msg("exec command failed, backup may be incomplete")
		}

		logBackupComplete(d.ctx, backupName, result)
	} else {
		err := d.engine.Exec(d.ctx, backupCtnr.ID, backupCtnr.Exec.Command)
		if err != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(err).
				Fields(d.logFields(backupCtnr)).
				Msg("failed to execute exec command")

			return
		}

		paths := make([]string, 0, len(backupCtnr.Exec.Paths))
		for _, cPath := range backupCtnr.Exec.Paths {
			sPath, found := findSourceForInContainerPath(&backupCtnr, cPath)
			if !found {
				log.Warn().
					Ctx(d.ctx).
					Fields(d.logFields(backupCtnr)).
					Str("path", cPath).
					Msg("no source for path")

				continue
			}

			paths = append(paths, sPath)
		}

		result, err := d.borgClient.CreateWithPaths(utils.ArchiveName(backupName), paths)
		if err != nil {
			log.Warn().
				Ctx(d.ctx).
				Err(err).
				Fields(d.logFields(backupCtnr)).
				Msg("backup failed")
		}

		logBackupComplete(d.ctx, backupName, result)
	}
}

func (d *containerProjectBackupJob) runVolumeBackup(backupCtnr model.ContainerBackup, backupName string) {
	paths := make([]string, len(backupCtnr.BackupVolumes))
	for _, vol := range backupCtnr.BackupVolumes {
		paths = append(paths, vol.Source)
	}

	result, err := d.borgClient.CreateWithPaths(utils.ArchiveName(backupName), paths)
	if err != nil {
		log.Warn().
			Ctx(d.ctx).
			Err(err).
			Fields(d.logFields(backupCtnr)).
			Msg("backup failed")

		return
	}

	logBackupComplete(d.ctx, backupName, result)
}

func findSourceForInContainerPath(ctnr *model.ContainerBackup, cPath string) (string, bool) {
	lowerCPath := strings.ToLower(cPath)
	for _, vol := range ctnr.AllVolumes {
		if strings.HasPrefix(strings.ToLower(vol.Destination), lowerCPath) {
			return path.Join(vol.Source, cPath[len(vol.Destination):]), true
		}
	}

	if ctnr.UpperDirPath != "" {
		if strings.HasPrefix(strings.ToLower(ctnr.UpperDirPath), lowerCPath) {
			return path.Join(ctnr.UpperDirPath, cPath), true
		}
	}

	return "", false
}

func (d *containerProjectBackupJob) findDependents(backup model.ContainerBackup) []model.ContainerBackup {
	result := make([]model.ContainerBackup, 0)

	for _, ctnr := range d.project.Containers {
		if slices.Contains(ctnr.Dependencies, backup.ServiceName) {
			result = append(result, ctnr)
		}
	}

	return result
}

func (d *containerProjectBackupJob) logFields(backupCtnr model.ContainerBackup) map[string]interface{} {
	result := make(map[string]interface{})
	result["engine"] = d.project.Engine
	result["container"] = backupCtnr.ID
	result["projectName"] = d.project.ProjectName
	result["serviceName"] = backupCtnr.ServiceName

	return result
}
