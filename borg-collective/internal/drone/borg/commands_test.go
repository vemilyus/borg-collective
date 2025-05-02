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
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/vemilyus/borg-collective/internal/drone"
	"testing"
)

func TestRunInit(t *testing.T) {
	unsecureBorg := createTmpBorg(t, nil, nil)

	err := unsecureBorg.runInit()
	assert.Nil(t, err)

	info, err := unsecureBorg.runInfo()
	assert.Nil(t, err)
	assert.NotNil(t, info)

	b, _ := json.MarshalIndent(info, "", "  ")
	println(string(b))
}

func TestRunCreateWithPaths(t *testing.T) {
	unsecureBorg := createTmpBorg(t, nil, nil)
	err := unsecureBorg.runInit()
	assert.Nil(t, err)

	// TODO
}

func TestRunCreateWithInput(t *testing.T) {
	unsecureBorg := createTmpBorg(t, nil, nil)
	err := unsecureBorg.runInit()
	assert.Nil(t, err)

	// TODO
}

func createTmpBorg(t *testing.T, backups []*drone.BackupConfig, secret *string) *Borg {
	tmpDir := t.TempDir()
	repo := t.TempDir()

	var crypt *drone.EncryptionConfig
	if secret != nil {
		crypt = &drone.EncryptionConfig{
			Secret: secret,
		}
	}

	b, err := New(&drone.Config{
		Options: &drone.OptionsConfig{TempDir: tmpDir},
		Repo: drone.RepositoryConfig{
			Location: repo,
		},
		Encryption: crypt,
		Backups:    backups,
	})

	assert.Nil(t, err)

	return b
}
