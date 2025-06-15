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
	"errors"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/utils"
)

type staticBackupJob struct {
	ctx        context.Context
	borgClient *borg.Client
	backup     config.BackupConfig
}

func (w *Worker) newStaticBackupJob(backup config.BackupConfig) cron.Job {
	return &staticBackupJob{w.ctx, w.borgClient, backup}
}

func (s staticBackupJob) Run() {
	startEvent := log.Info().Ctx(s.ctx)
	if config.Verbose {
		backupJson, _ := json.Marshal(s.backup)
		startEvent.RawJSON("backup", backupJson)
	} else {
		startEvent.Str("backup", s.backup.Name)
	}
	startEvent.Msg("starting static backup")

	var err error
	if len(s.backup.PreCommand) > 0 {
		err = utils.Exec(s.ctx, s.backup.PreCommand)
	}

	if err == nil {
		if s.backup.Exec != nil {
			err = s.runExecBackup()
		} else {
			err = s.runPathsBackup()
		}
	}

	if err != nil {
		log.Warn().
			Ctx(s.ctx).
			Err(err).
			Str("backup", s.backup.Name).
			Msg("backup failed")
	} else if len(s.backup.PostCommand) > 0 {
		_ = utils.Exec(s.ctx, s.backup.PostCommand)
	}

	if len(s.backup.FinallyCommand) > 0 {
		_ = utils.Exec(s.ctx, s.backup.FinallyCommand)
	}
}

func (s staticBackupJob) runExecBackup() error {
	if log.Debug().Enabled() {
		log.Debug().
			Ctx(s.ctx).
			Str("backup", s.backup.Name).
			Msg("backing up exec result")
	}

	if len(s.backup.Exec.Command) == 0 {
		return errors.New("no exec command specified")
	}

	if s.backup.Exec.Stdout != nil && *s.backup.Exec.Stdout {
		output, err := utils.ExecWithOutput(s.ctx, s.backup.Exec.Command)
		if err != nil {
			return err
		}

		result, err := s.borgClient.CreateWithInput(s.ctx, utils.ArchiveName(s.backup.Name), output)
		if err != nil {
			return err
		}

		if output.Error() != nil {
			log.Warn().
				Ctx(s.ctx).
				Err(output.Error()).
				Msg("exec command failed, backup may be incomplete")
		}

		logBackupComplete(s.ctx, s.backup.Name, result)
	} else {
		if len(s.backup.Exec.Paths) == 0 {
			return errors.New("no paths configured")
		}

		err := utils.Exec(s.ctx, s.backup.Exec.Command)
		if err != nil {
			return err
		}

		return backupPaths(s.ctx, s.borgClient, s.backup.Name, s.backup.Exec.Paths)
	}

	return nil
}

func (s staticBackupJob) runPathsBackup() error {
	if log.Debug().Enabled() {
		log.Debug().
			Ctx(s.ctx).
			Str("backup", s.backup.Name).
			Msg("backing up static paths")
	}

	if s.backup.Paths == nil {
		return errors.New("no paths configured")
	}

	return backupPaths(s.ctx, s.borgClient, s.backup.Name, s.backup.Paths.Paths)
}
