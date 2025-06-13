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
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vemilyus/borg-collective/internal/drone/container/model"
)

func TestDockerWatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newClient()
	watch, err := client.Watch(ctx)
	assert.NoError(t, err)

	updates := make([]model.ContainerBackupProject, 0, 5)

	go func() {
		for {
			select {
			case project := <-watch.Updates():
				updates = append(updates, project)
			case <-ctx.Done():
				return
			}
		}
	}()

	err = composeUp("test-paperless", "project.yml")
	assert.NoError(t, err)

	defer func() { _ = composeDown("test-paperless") }()

	time.Sleep(1 * time.Second)

	assert.Equal(t, 5, len(updates))
	assert.Equal(t, 5, len(updates[4].Containers))

	updates = make([]model.ContainerBackupProject, 0, 5)

	_ = composeDown("test-paperless")

	time.Sleep(1 * time.Second)

	assert.Equal(t, 5, len(updates))
	assert.Equal(t, 0, len(updates[4].Containers))
}

func TestDockerWatch_IgnoreUnconfigured(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := newClient()
	watch, err := client.Watch(ctx)
	assert.NoError(t, err)

	go func() {
		for {
			select {
			case <-watch.Updates():
				assert.Fail(t, "received container backup project")
			case <-ctx.Done():
				return
			}
		}
	}()

	proj := strings.ToLower(rand.Text())

	err = composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer func() { _ = composeDown(proj) }()

	time.Sleep(1 * time.Second)
}
