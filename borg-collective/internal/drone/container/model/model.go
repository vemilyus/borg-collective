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
	"strconv"

	"github.com/robfig/cron/v3"
)

const (
	LabelBorgdEnabled = "io.v47.borgd.enabled"

	LabelProjectName = "io.v47.borgd.project_name"
	LabelProjectWhen = "io.v47.borgd.when"

	LabelBackupMode      = "io.v47.borgd.service.mode"
	LabelDependenciesPfx = "io.v47.borgd.service.dependencies."
	LabelExec            = "io.v47.borgd.service.exec"
	LabelExecStdout      = "io.v47.borgd.service.stdout"
	LabelExecPathsPfx    = "io.v47.borgd.service.paths."
	LabelServiceName     = "io.v47.borgd.service_name"
	LabelVolumesPfx      = "io.v47.borgd.service.volumes."
)

type BackupMode uint8

const (
	BackupModeDefault BackupMode = 1 + iota
	BackupModeDependentOffline
	BackupModeOffline
)

//goland:noinspection GoMixedReceiverTypes
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

	return 0, errors.New("unrecognized backup mode: " + s)
}

//goland:noinspection GoMixedReceiverTypes
func (b BackupMode) MarshalJSON() ([]byte, error) {
	return []byte(b.String()), nil
}

//goland:noinspection GoMixedReceiverTypes
func (b *BackupMode) UnmarshalJSON(bytes []byte) error {
	bm, err := BackupModeFromString(string(bytes))
	if err != nil {
		return err
	}

	*b = bm

	return nil
}

type ContainerEngine string

const (
	ContainerEngineDocker ContainerEngine = "docker"
)

type ContainerBackupProject struct {
	Engine      ContainerEngine
	ProjectName string
	Schedule    cron.Schedule
	Containers  map[string]ContainerBackup `json:",omitempty"`
}

type ContainerBackup struct {
	ID            string
	ServiceName   string
	Mode          BackupMode
	UpperDirPath  string
	Exec          *ContainerExecBackup
	BackupVolumes []Volume `json:",omitempty"`
	AllVolumes    []Volume `json:",omitempty"`
	Dependencies  []string `json:",omitempty"`
}

func (b *ContainerBackup) NeedsBackup() bool {
	return b.Exec != nil || len(b.BackupVolumes) > 0
}

type ContainerExecBackup struct {
	Command []string
	Stdout  bool
	Paths   []string `json:",omitempty"`
}

type Volume struct {
	Type        string
	Name        string
	Source      string
	Destination string
}
