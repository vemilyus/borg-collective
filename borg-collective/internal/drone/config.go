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

package drone

import (
	"errors"
	"fmt"
	"github.com/pelletier/go-toml/v2"
	"github.com/robfig/cron/v3"
	"os"
)

type Config struct {
	Options    *OptionsConfig
	Repo       RepositoryConfig
	Encryption *EncryptionConfig
	Backups    []*BackupConfig
}

type OptionsConfig struct {
	TempDir string
}

type RepositoryConfig struct {
	Location                 string
	IdentityFile             *string
	CompactionSchedule       *string
	compactionScheduleParsed cron.Schedule
}

func (rc *RepositoryConfig) CompactionScheduleParsed() cron.Schedule {
	return rc.compactionScheduleParsed
}

type EncryptionConfig struct {
	Secret   *string
	SecretId *string
}

type BackupConfig struct {
	Name           string
	Schedule       string
	scheduleParsed cron.Schedule
	Exec           *ExecBackupConfig
	Paths          *PathsBackupConfig
	PreCommand     []string
	PostCommand    []string
	FinallyCommand []string
}

func (bc *BackupConfig) ScheduleParsed() cron.Schedule {
	return bc.scheduleParsed
}

type ExecBackupConfig struct {
	Command []string
	Stdout  *bool
	Paths   []string
}

type PathsBackupConfig struct {
	Paths []string
}

func LoadConfig(path string) (*Config, error) {
	configReader, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = configReader.Close() }()

	decoder := toml.NewDecoder(configReader)

	var conf Config
	if err = decoder.Decode(&conf); err != nil {
		return nil, err
	}

	if conf.Repo.CompactionSchedule != nil {
		schedule, err := cron.ParseStandard(*conf.Repo.CompactionSchedule)
		if err != nil {
			return nil, fmt.Errorf("invalid compaction schedule %s: %v", *conf.Repo.CompactionSchedule, err)
		}

		conf.Repo.compactionScheduleParsed = schedule
	}

	if conf.Encryption != nil {
		if conf.Encryption.Secret == nil && conf.Encryption.SecretId == nil {
			return nil, errors.New("encryption config must specify either Secret or SecretId")
		}
	}

	for _, backup := range conf.Backups {
		schedule, err := cron.ParseStandard(backup.Schedule)
		if err != nil {
			return nil, fmt.Errorf("invalid backup schedule for %s (%s): %v", backup.Name, backup.Schedule, err)
		}

		backup.scheduleParsed = schedule
	}

	return &conf, nil
}
