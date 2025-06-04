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

package borg

import (
	"context"
	"crypto/rand"
	"os"
	"os/exec"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/utils"
)

func TestBorgNewClient(t *testing.T) {
	borgClient, err := NewClient(config.Config{})
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	version, err := borgClient.Version()
	assert.NoError(t, err)
	assert.NotNil(t, version)

	assert.True(t, version.GreaterThanEqual(supportedVersionMin))
}

func TestBorgInfo(t *testing.T) {
	cfg := config.Config{Repo: config.RepositoryConfig{Location: "/tmp/" + rand.Text()}}
	borgClient, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	info, err := borgClient.Info()
	assert.Error(t, err)

	existingDir := t.TempDir()
	cfg.Repo.Location = existingDir
	borgClient.SetConfig(cfg)

	info, err = borgClient.Info()
	assert.Error(t, err)

	validDir := t.TempDir()
	err = exec.Command("borg", "init", "--encryption=none", validDir).Run()
	assert.NoError(t, err)

	cfg.Repo.Location = validDir
	borgClient.SetConfig(cfg)

	info, err = borgClient.Info()
	assert.NoError(t, err)

	assert.NotEmpty(t, info.Repository.Id)
	assert.Equal(t, validDir, info.Repository.Location)
}

func TestBorgInit(t *testing.T) {
	cfg := config.Config{Repo: config.RepositoryConfig{Location: "/tmp/" + rand.Text() + "/" + rand.Text()}}
	borgClient, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	err = borgClient.Init()
	assert.NoError(t, err)

	err = borgClient.Init()
	assert.Error(t, err)

	_ = os.RemoveAll(cfg.Repo.Location)
}

func TestBorgCreateWithPaths(t *testing.T) {
	cfg := config.Config{Repo: config.RepositoryConfig{Location: t.TempDir()}}
	borgClient, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	err = borgClient.Init()
	assert.NoError(t, err)

	dir := t.TempDir()
	file := path.Join(dir, "some-random-data.bin")
	randomData, err := exec.Command("bash", "-c", "cat /dev/random | head -n 1024").Output()
	assert.NoError(t, err)
	err = os.WriteFile(file, randomData, 0644)
	assert.NoError(t, err)

	result, err := borgClient.CreateWithPaths("some-backup", []string{dir})
	assert.NoError(t, err)
	assert.NotNil(t, result.Archive.Stats)
}

func TestBorgCreateWithInput(t *testing.T) {
	cfg := config.Config{Repo: config.RepositoryConfig{Location: t.TempDir()}}
	borgClient, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	err = borgClient.Init()
	assert.NoError(t, err)

	ctx := context.Background()
	input, err := utils.ExecWithOutput(ctx, []string{"bash", "-c", "cat /dev/random | head -n 1024"})
	assert.NoError(t, err)

	result, err := borgClient.CreateWithInput(ctx, "some-data", input)
	assert.NoError(t, input.Error())
	assert.NoError(t, err)

	assert.NotNil(t, result.Archive.Stats)
}

func TestBorgCompact(t *testing.T) {
	cfg := config.Config{Repo: config.RepositoryConfig{Location: t.TempDir()}}
	borgClient, err := NewClient(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, borgClient)

	err = borgClient.Init()
	assert.NoError(t, err)

	err = borgClient.Compact()
	assert.NoError(t, err)
}
