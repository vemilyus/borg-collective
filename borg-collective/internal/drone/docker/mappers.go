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

package docker

import (
	"fmt"
	"github.com/docker/docker/api/types/container"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/model"
	"github.com/vemilyus/borg-collective/internal/utils"
	"strings"
)

const (
	LabelBackupMode      = "io.v47.borgd.service.mode"
	LabelDependenciesPfx = "io.v47.borgd.service.dependencies."
	LabelExec            = "io.v47.borgd.service.exec"
	LabelExecStdout      = "io.v47.borgd.service.stdout"
	LabelExecPathsPfx    = "io.v47.borgd.service.paths."
	LabelServiceName     = "io.v47.borgd.service_name"
	LabelVolumesPfx      = "io.v47.borgd.service.volumes."
)

func mapLabelsToProject(labels map[string]string) (*model.ContainerBackupProject, error) {
	var projectName string
	var schedule cron.Schedule
	var err error

	for key, value := range labels {
		if key == "io.v47.borgd.project_name" {
			projectName = value
		} else if key == "io.v47.borgd.when" {
			schedule, err = cron.ParseStandard(value)
			if err != nil {
				return nil, err
			}
		}
	}

	if projectName == "" {
		return nil, fmt.Errorf("project name not found in labels")
	}

	if schedule == nil {
		return nil, fmt.Errorf("schedule not found in labels")
	}

	return &model.ContainerBackupProject{
		ProjectName: projectName,
		Schedule:    schedule,
		JobId:       nil,
		Containers:  make(map[string]model.ContainerBackup),
	}, nil
}

func mapInspectToContainerBackup(inspect container.InspectResponse) (*model.ContainerBackup, error) {
	upperDir := ""
	if inspect.GraphDriver.Name == "overlay2" {
		upperDir = inspect.GraphDriver.Data["UpperDir"]
	} else {
		log.Warn().
			Str("GraphDriver.Name", inspect.GraphDriver.Name).
			Msg("graph driver not supported, exec paths backup from non-volume dir will fail")
	}

	result := &model.ContainerBackup{
		ID:           inspect.ID,
		Mode:         model.BackupModeDefault,
		UpperDirPath: upperDir,
		Volumes:      make([]container.MountPoint, 3),
		AllVolumes:   inspect.Mounts,
		Dependencies: make([]string, 3),
	}

	exec := model.ContainerExecBackup{
		Paths: make([]string, 1),
	}

	for key, value := range inspect.Config.Labels {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if key == LabelBackupMode {
			mode, err := model.BackupModeFromString(value)
			if err != nil {
				return nil, err
			}

			result.Mode = mode
		} else if strings.HasPrefix(key, LabelDependenciesPfx) {
			result.Dependencies = append(result.Dependencies, value)
		} else if key == LabelExec {
			exec.Command = utils.SplitCommandLine(value)
		} else if key == LabelExecStdout {
			exec.Stdout = true
		} else if strings.HasPrefix(key, LabelExecPathsPfx) {
			exec.Paths = append(exec.Paths, value)
		} else if key == LabelServiceName {
			result.ServiceName = value
		} else if strings.HasPrefix(key, LabelVolumesPfx) {
			mount := findMountByTarget(value, inspect)
			if mount == nil {
				return nil, fmt.Errorf("mount point %s not found in %s", value, result.ID)
			}

			result.Volumes = append(result.Volumes, *mount)
		}
	}

	if len(exec.Command) > 0 {
		if len(exec.Paths) == 0 && !exec.Stdout {
			return nil, fmt.Errorf("exec must have either paths or stdout: %s", result.ID)
		} else if len(exec.Paths) > 0 && exec.Stdout {
			return nil, fmt.Errorf("exec must not have both paths and stdout: %s", result.ID)
		}

		result.Exec = &exec
	}

	if result.Exec == nil && len(result.Volumes) == 0 {
		return nil, fmt.Errorf("container must have either exec or volumes: %s", result.ID)
	} else if result.Exec != nil && len(result.Volumes) > 0 {
		return nil, fmt.Errorf("container must not have both exec and volumes: %s", result.ID)
	}

	if result.ServiceName == "" {
		return nil, fmt.Errorf("container must have a service name: %s", result.ID)
	}

	return result, nil
}

func findMountByTarget(target string, inspect container.InspectResponse) *container.MountPoint {
	for _, mount := range inspect.Mounts {
		if mount.Destination == target {
			return &mount
		}
	}

	return nil
}
