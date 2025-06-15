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
	"os"
	"path/filepath"
	"runtime"

	"github.com/vemilyus/borg-collective/internal/utils"
)

func absPath(path string) string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(b), "../container/docker", path)
}

func composeUp(project, file string) error {
	path := absPath(filepath.Join("testdata", file))
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panic(err)
	}

	ctx := context.WithValue(context.Background(), "test", true)
	return utils.Exec(ctx, []string{"docker", "compose", "-f", path, "-p", project, "up", "-d"})
}

func composeDown(project string) {
	ctx := context.WithValue(context.Background(), "test", true)
	_ = utils.Exec(ctx, []string{"docker", "compose", "-p", project, "down", "-v"})
}
