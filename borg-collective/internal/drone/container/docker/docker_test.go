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
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDockerEnsureContainerRunning(t *testing.T) {
	proj := strings.ToLower(rand.Text())

	err := composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer composeDown(proj)

	cn := containerName(proj, "test-container")
	err = dockerStop(cn)
	assert.NoError(t, err)

	client := newClient()
	err = client.EnsureContainerRunning(context.Background(), cn)
	assert.NoError(t, err)
}

func TestDockerEnsureContainerRunning_WithHealth(t *testing.T) {
	proj := strings.ToLower(rand.Text())

	err := composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer composeDown(proj)

	cn := containerName(proj, "test-container-with-health")
	err = dockerStop(cn)
	assert.NoError(t, err)

	client := newClient()
	err = client.EnsureContainerRunning(context.Background(), cn)
	assert.NoError(t, err)
}

func TestDockerEnsureContainerStopped(t *testing.T) {
	proj := strings.ToLower(rand.Text())

	err := composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer composeDown(proj)

	cn := containerName(proj, "test-container")
	client := newClient()
	err = client.EnsureContainerStopped(context.Background(), cn)
	assert.NoError(t, err)
}

func TestDockerExec(t *testing.T) {
	proj := strings.ToLower(rand.Text())

	err := composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer composeDown(proj)

	cn := containerName(proj, "test-container")
	client := newClient()
	err = client.Exec(context.Background(), cn, []string{"ash", "-c", "sleep 1; exit 0"})
	assert.NoError(t, err)

	err = client.Exec(context.Background(), cn, []string{"ash", "-c", "sleep 1; exit 1"})
	assert.Error(t, err)
}

func TestDockerExecWithOutput(t *testing.T) {
	proj := strings.ToLower(rand.Text())

	err := composeUp(proj, "simple.yml")
	assert.NoError(t, err)

	defer composeDown(proj)

	cn := containerName(proj, "test-container")
	expectedCount := 524288
	client := newClient()

	err = client.EnsureContainerRunning(context.Background(), cn)
	assert.NoError(t, err)

	output, err := client.ExecWithOutput(context.Background(), cn, []string{"echo", "hello world"})
	assert.NoError(t, err)

	var buf bytes.Buffer
	_, err = buf.ReadFrom(output)

	assert.NoError(t, err)
	assert.NoError(t, output.Error())
	assert.Equal(t, "hello world\n", buf.String())

	longOutput, err := client.ExecWithOutput(
		context.Background(),
		cn,
		[]string{"ash", "-c", fmt.Sprintf("cat /dev/random | head -c %d", expectedCount)},
	)
	assert.NoError(t, err)

	buf.Reset()
	_, err = buf.ReadFrom(longOutput)
	assert.NoError(t, err)
	assert.NoError(t, longOutput.Error())
	assert.Equal(t, expectedCount, buf.Len())

	failingOutput, err := client.ExecWithOutput(
		context.Background(),
		cn,
		[]string{"ash", "-c", fmt.Sprintf("cat /dev/random | head -c %d; exit 1", expectedCount)},
	)
	assert.NoError(t, err)

	buf.Reset()
	_, err = buf.ReadFrom(failingOutput)
	assert.NoError(t, err)

	assert.Error(t, failingOutput.Error())
}

func TestDockerReadProjects(t *testing.T) {
	err := composeUp("test-paperless", "project.yml")
	assert.NoError(t, err)

	defer composeDown("test-paperless")

	client := newClient()
	projects, err := client.ReadProjects(context.Background())
	assert.NoError(t, err)
	assert.Len(t, projects, 1)
}
