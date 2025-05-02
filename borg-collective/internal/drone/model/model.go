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

package model

import (
	"errors"
	"github.com/docker/docker/api/types/container"
	"github.com/robfig/cron/v3"
	"strconv"
)

type BackupMode int8

const (
	BackupModeDefault BackupMode = iota
	BackupModeDependentOffline
	BackupModeOffline
)

func (b BackupMode) String() string {
	switch b {
	case BackupModeDefault:
		return "default"
	case BackupModeDependentOffline:
		return "dependent-offline"
	case BackupModeOffline:
		return "offline"
	}

	panic("invalid backup mode: " + strconv.Itoa(int(b)))
}

func BackupModeFromString(s string) (BackupMode, error) {
	switch s {
	case "default":
		return BackupModeDefault, nil
	case "dependent-offline":
		return BackupModeDependentOffline, nil
	case "offline":
		return BackupModeOffline, nil
	}

	return -1, errors.New("unrecognized backup mode: " + s)
}

type ContainerBackupProject struct {
	ProjectName string
	Schedule    cron.Schedule
	JobId       *cron.EntryID
	Containers  map[string]ContainerBackup
}

type ContainerBackup struct {
	ID           string
	ServiceName  string
	Mode         BackupMode
	UpperDirPath string
	Exec         *ContainerExecBackup
	Volumes      []container.MountPoint
	AllVolumes   []container.MountPoint
	Dependencies []string
}

type ContainerExecBackup struct {
	Command []string
	Stdout  bool
	Paths   []string
}
