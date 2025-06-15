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
	"testing"

	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/drone/container/docker"
)

func TestContainerProjectBackupJob(t *testing.T) {
	config.Verbose = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dc, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	assert.NoError(t, err)

	engine := docker.NewClient(dc)

	borgClient, err := borg.NewClient(config.Config{Repo: config.RepositoryConfig{Location: t.TempDir()}})
	assert.NoError(t, err)

	err = borgClient.Init()
	assert.NoError(t, err)

	err = composeUp("test-paperless", "project.yml")
	assert.NoError(t, err)

	defer composeDown("test-paperless")

	projects, err := engine.ReadProjects(ctx)
	assert.NoError(t, err)

	paperlessProject := projects[0]

	worker := Worker{
		ctx:          ctx,
		borgClient:   borgClient,
		dockerClient: engine,
	}

	job, err := worker.newContainerProjectBackupJob(paperlessProject)
	assert.NoError(t, err)

	backupJob, _ := job.(*containerProjectBackupJob)

	backupOrder := make([]string, 0, len(backupJob.plan))
	for _, backup := range backupJob.plan {
		backupOrder = append(backupOrder, backup.ServiceName)
	}

	assert.Equal(t, []string{"server", "redis", "db"}, backupOrder)

	backupJob.Run()
}
