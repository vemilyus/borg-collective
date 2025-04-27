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

package borg

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Masterminds/semver/v3"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone"
	"io"
	"os/exec"
	"regexp"
	"time"
)

var (
	supportedVersionMin   = semver.MustParse("1.2.5")
	supportedVersionUpper = semver.MustParse("2.0.0")
)

type Borg struct {
	config        *drone.Config
	scheduledJobs map[string]cron.EntryID
}

func New(config *drone.Config) (*Borg, error) {
	b := &Borg{
		config:        config,
		scheduledJobs: make(map[string]cron.EntryID),
	}

	version, err := b.runVersionLocal()
	if err != nil {
		return nil, fmt.Errorf("failed to check borg version: %v", err)
	}

	if version.LessThan(supportedVersionMin) || version.GreaterThanEqual(supportedVersionUpper) {
		return nil, fmt.Errorf("unsupported borg version (must be >= %v and < %v): %v", supportedVersionMin, supportedVersionUpper, version)
	}

	log.Info().Msgf("Borg version: %v", version)

	return b, nil
}

func (b *Borg) Compact() Action {
	return &closureAction{
		id: fmt.Sprintf("compact-%s", rand.Text()),
		action: func(Action) error {
			return b.runCompact()
		},
	}
}

func (b *Borg) ScheduleBackups(backups []drone.BackupConfig, scheduler *cron.Cron) error {
	for _, backup := range backups {
		existing, ok := b.scheduledJobs[backup.Name]
		if ok {
			scheduler.Remove(existing)
			delete(b.scheduledJobs, backup.Name)
		}

		backupAction, err := b.buildBackupAction(backup)
		if err != nil {
			return err
		}

		action := NewComposedAction(backupAction)
		if len(backup.PreCommand) > 0 {
			action.Pre(NewExecAction(backup.PreCommand))
		}

		if len(backup.PostCommand) > 0 {
			action.Post(NewExecAction(backup.PostCommand))
		}

		if len(backup.FinallyCommand) > 0 {
			action.Finally(NewExecAction(backup.FinallyCommand))
		}

		action.SetId(backup.Name)

		entryId := scheduler.Schedule(backup.ScheduleParsed(), Wrap(action))
		b.scheduledJobs[backup.Name] = entryId
	}

	return nil
}

func (b *Borg) buildBackupAction(backup drone.BackupConfig) (Action, error) {
	if backup.Exec == nil && backup.Paths == nil {
		return nil, fmt.Errorf("backup specifies neither exec nor paths: %s", backup.Name)
	}

	var delegate Action
	var err error

	if backup.Exec != nil {
		delegate, err = b.buildExecAction(backup.Name, *backup.Exec)
	} else {
		delegate, err = b.BuildArchivePathsAction(backup.Name, backup.Paths.Paths)
	}

	if err != nil {
		return nil, err
	}

	result := NewComposedAction(delegate)
	if backup.PreCommand != nil && len(backup.PreCommand) > 0 {
		result.Pre(NewExecAction(backup.PreCommand))
	}

	if backup.PostCommand != nil && len(backup.PostCommand) > 0 {
		result.Post(NewExecAction(backup.PostCommand))
	}

	if backup.FinallyCommand != nil && len(backup.FinallyCommand) > 0 {
		result.Finally(NewExecAction(backup.FinallyCommand))
	}

	return result, nil
}

func (b *Borg) buildExecAction(baseName string, backup drone.ExecBackupConfig) (Action, error) {
	if backup.Command == nil || len(backup.Command) == 0 {
		return nil, errors.New("exec backup has no command")
	}

	actualStdout := false
	if backup.Stdout != nil {
		actualStdout = *backup.Stdout
	}

	if actualStdout {
		return b.BuildArchiveStdoutAction(
			baseName,
			func() (io.Reader, error, chan error) {
				cmd := exec.Command(backup.Command[0], backup.Command[1:]...)
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					return nil, err, nil
				}

				errChan := make(chan error, 1)

				go func() {
					err = cmd.Wait()
					if err != nil {
						errChan <- err
					}
				}()

				return stdout, nil, errChan
			},
		)
	} else {
		if backup.Paths == nil || len(backup.Paths) == 0 {
			return nil, errors.New("exec backup defines no paths")
		}

		result := NewSequenceAction()
		result.Push(NewExecAction(backup.Command))

		backupAction, err := b.BuildArchivePathsAction(baseName, backup.Paths)
		if err != nil {
			return nil, err
		}

		result.Push(backupAction)

		return result, nil
	}
}

func (b *Borg) BuildArchiveStdoutAction(baseName string, stdout func() (io.Reader, error, chan error)) (Action, error) {
	return &closureAction{
		id: rand.Text(),
		action: func(self Action) error {
			input, err, errChan := stdout()
			if err != nil {
				return err
			}

			ctx, cancelContext := context.WithCancel(context.Background())
			defer cancelContext()

			done := make(chan error, 1)

			go func() {
				stats, err := b.runCreateWithInput(ctx, CreateArchiveName(baseName), input)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
				} else {
					infoJson, _ := json.Marshal(stats)

					log.Info().
						Str("actionId", self.Id()).
						RawJSON("info", infoJson).
						Msg("borg execution succeeded")
				}

				done <- err
			}()

			select {
			case err = <-errChan:
				cancelContext()
				return fmt.Errorf("process creating backup data failed: %w", err)
			case err = <-done:
				return err
			}
		},
	}, nil
}

func (b *Borg) BuildArchivePathsAction(baseName string, sourcePaths []string) (Action, error) {
	if len(sourcePaths) == 0 {
		return nil, fmt.Errorf("no paths specified for backup %s", baseName)
	}

	return &closureAction{
		id: rand.Text(),
		action: func(self Action) error {
			stats, err := b.runCreateWithPaths(CreateArchiveName(baseName), sourcePaths)
			if err != nil {
				return err
			}

			infoJson, _ := json.Marshal(stats)

			log.Info().
				Str("actionId", self.Id()).
				RawJSON("info", infoJson).
				Msg("borg execution succeeded")

			return nil
		},
	}, nil
}

var normalizationRegexp = regexp.MustCompile("[^_a-zA-Z0-9]+")

func CreateArchiveName(baseName string) string {
	normalizedName := normalizationRegexp.ReplaceAllString(baseName, "_")
	return fmt.Sprintf("%s-%s", normalizedName, time.Now().Format("20060102150405"))
}
