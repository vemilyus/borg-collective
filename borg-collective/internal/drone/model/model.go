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
	"github.com/vemilyus/borg-collective/internal/drone"
	"strconv"
	"sync"
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

type Backups struct {
	lock              sync.RWMutex
	ContainerBackups  []*ContainerBackupProject
	ConfiguredBackups []drone.BackupConfig
}

func NewBackups(containerBackups []*ContainerBackupProject, configuredBackups []drone.BackupConfig) *Backups {
	return &Backups{
		lock:              sync.RWMutex{},
		ContainerBackups:  containerBackups,
		ConfiguredBackups: configuredBackups,
	}
}

func (b *Backups) ReadAction(action func(*Backups)) {
	b.lock.RLock()
	defer b.lock.RUnlock()

	action(b)
}

func (b *Backups) WriteAction(action func(*Backups)) {
	b.lock.Lock()
	defer b.lock.Unlock()

	action(b)
}

type ContainerBackupProject struct {
	ProjectName string
	Schedule    cron.Schedule
	JobId       *cron.EntryID
	Containers  []ContainerBackup
}

type ContainerBackup struct {
	ContainerName string
	Mode          BackupMode
	Exec          *ContainerExecBackup
	Volumes       []container.MountPoint
	Dependencies  []string
}

type ContainerExecBackup struct {
	Command []string
	Stdout  bool
	Paths   []string
}
