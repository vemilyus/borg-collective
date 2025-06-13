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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/docker/docker/client"
	"github.com/vemilyus/borg-collective/internal/utils"
)

func absPath(path string) string {
	_, b, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(b), path)
}

func newClient() *Client {
	dc, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}

	return NewClient(dc)
}

func composeUp(project, file string) error {
	path := filepath.Join("testdata", file)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		panic(err)
	}

	ctx := context.WithValue(context.Background(), "test", true)
	return utils.Exec(ctx, []string{"docker", "compose", "-f", absPath(path), "-p", project, "up", "-d"})
}

func composeDown(project string) error {
	ctx := context.WithValue(context.Background(), "test", true)
	return utils.Exec(ctx, []string{"docker", "compose", "-p", project, "down", "-v"})
}

func containerName(project, containerName string) string {
	return fmt.Sprintf("%s-%s-1", project, containerName)
}

func dockerStop(containerName string) error {
	ctx := context.WithValue(context.Background(), "test", true)
	return utils.Exec(ctx, []string{"docker", "stop", containerName})
}
