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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
	"github.com/vemilyus/borg-collective/internal/utils"
)

func mapInspectToProject(inspect container.InspectResponse) (*model.ContainerBackupProject, error) {
	projectName, found := inspect.Config.Labels[model.LabelProjectName]
	if !found || projectName == "" {
		return nil, fmt.Errorf("project name not found in container %s", inspect.ID)
	}

	scheduleRaw, found := inspect.Config.Labels[model.LabelProjectWhen]
	if !found {
		return nil, fmt.Errorf("project schedule not found in container %s", inspect.ID)
	}

	schedule, err := cron.ParseStandard(scheduleRaw)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("failed to parse project schedule in container %s", inspect.ID))
	}

	return &model.ContainerBackupProject{
		Engine:      model.ContainerEngineDocker,
		ProjectName: projectName,
		Schedule:    schedule,
		Containers:  make(map[string]model.ContainerBackup),
	}, nil
}

var ampEnvEscape = regexp.MustCompile(`&\{`)

func mapInspectToContainerBackup(inspect container.InspectResponse) (*model.ContainerBackup, error) {
	upperDir := ""
	if inspect.GraphDriver.Name == "overlay2" {
		upperDir = inspect.GraphDriver.Data["UpperDir"]
	} else {
		log.Warn().
			Str("engine", (string)(model.ContainerEngineDocker)).
			Str("container", inspect.ID).
			Str("graphDriver", inspect.GraphDriver.Name).
			Msg("graph driver not supported, backed up data may be incomplete")
	}

	result := &model.ContainerBackup{
		ID:            inspect.ID,
		Mode:          model.BackupModeDefault,
		UpperDirPath:  upperDir,
		BackupVolumes: make([]model.Volume, 0, 3),
		AllVolumes:    mapVolumes(inspect.Mounts, inspect.ID),
		Dependencies:  make([]string, 0, 3),
	}

	exec := model.ContainerExecBackup{
		Paths: make([]string, 0, 1),
	}

	for key, value := range inspect.Config.Labels {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}

		if key == model.LabelBackupMode {
			mode, err := model.BackupModeFromString(value)
			if err != nil {
				return nil, err
			}

			result.Mode = mode
		} else if strings.HasPrefix(key, model.LabelDependenciesPfx) {
			result.Dependencies = append(result.Dependencies, value)
		} else if key == model.LabelExec {
			value = ampEnvEscape.ReplaceAllString(value, "${")
			exec.Command = utils.SplitCommandLine(value)
		} else if key == model.LabelExecStdout {
			exec.Stdout = true
		} else if strings.HasPrefix(key, model.LabelExecPathsPfx) {
			exec.Paths = append(exec.Paths, value)
		} else if key == model.LabelServiceName {
			result.ServiceName = value
		} else if strings.HasPrefix(key, model.LabelVolumesPfx) {
			m := findVolumeByDestination(value, inspect)
			if m == nil {
				return nil, fmt.Errorf("volume for destination %s not found in %s", value, result.ID)
			}

			result.BackupVolumes = append(result.BackupVolumes, *m)
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

	if result.Exec != nil && len(result.BackupVolumes) > 0 {
		return nil, fmt.Errorf("container must not have both exec and volumes: %s", result.ID)
	}

	if result.ServiceName == "" {
		return nil, fmt.Errorf("container must have a service name: %s", result.ID)
	}

	if result.Mode == model.BackupModeOffline && result.Exec != nil {
		return nil, fmt.Errorf("container cannot have exec with offline backup mode: %s", result.ID)
	}

	return result, nil
}

func findVolumeByDestination(target string, inspect container.InspectResponse) *model.Volume {
	for _, m := range inspect.Mounts {
		if m.Destination == target {
			mapped, err := mapVolume(m)
			if err != nil {
				mountJson, _ := json.Marshal(m)
				log.Error().
					Err(err).
					RawJSON("mount", mountJson).
					Str("container", inspect.ID).
					Msg("failed to map volume")

				return nil
			}

			return &mapped
		}
	}

	return nil
}

func mapVolumes(mounts []container.MountPoint, containerID string) []model.Volume {
	result := make([]model.Volume, 0, len(mounts))
	for _, m := range mounts {
		mapped, err := mapVolume(m)
		if err != nil {
			mountJson, _ := json.Marshal(m)
			log.Warn().
				Err(err).
				RawJSON("mount", mountJson).
				Str("container", containerID).
				Msg("failed to map volume")

			continue
		}

		result = append(result, mapped)
	}

	return result
}

func mapVolume(m container.MountPoint) (model.Volume, error) {
	if m.Type != mount.TypeBind && m.Type != mount.TypeVolume {
		return model.Volume{}, fmt.Errorf("volume mount type not supported: %s", m.Type)
	}

	return model.Volume{
		Type:        (string)(m.Type),
		Name:        m.Name,
		Source:      m.Source,
		Destination: m.Destination,
	}, nil
}
