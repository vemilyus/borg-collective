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
	"slices"

	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/borg/api"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
	"github.com/vemilyus/borg-collective/internal/utils"
)

type containerPlan []model.ContainerBackup

func (p containerPlan) Len() int {
	return len(p)
}

func (p containerPlan) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

func (p containerPlan) Less(i, j int) bool {
	a := p[i]
	b := p[j]

	return slices.Contains(a.Dependencies, b.ServiceName) || a.Mode < b.Mode
}

func backupPaths(ctx context.Context, borgClient *borg.Client, backupName string, paths []string) error {
	if len(paths) == 0 {
		return errors.New("no paths specified")
	}

	result, err := borgClient.CreateWithPaths(utils.ArchiveName(backupName), paths)
	if err != nil {
		return err
	}

	logBackupComplete(ctx, backupName, result)

	return nil
}

func logBackupComplete(ctx context.Context, backupName string, result api.CreateOutput) {
	resultLog := log.Info().
		Ctx(ctx).
		Str("backup", backupName)

	if config.Verbose {
		resultJson, _ := json.Marshal(result)
		resultLog.RawJSON("result", resultJson)
	}

	resultLog.Msg("backup complete")
}
