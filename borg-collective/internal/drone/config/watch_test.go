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

package config

import (
	"context"
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
)

func writeConfig(file string, cfg Config) error {
	bytes, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(file, bytes, 0644)
}

func TestConfigWatch(t *testing.T) {
	cfgFile := t.TempDir() + "/config.toml"
	cfg := Config{Repo: RepositoryConfig{Location: "/tmp/" + rand.Text()}}

	err := writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	watch, err := NewWatch(ctx, cfgFile)
	assert.NoError(t, err)

	cfgs := make([]Config, 0)
	errs := make([]error, 0)

	go func() {
		defer cancel()

		for i := 0; i < 2; i++ {
			select {
			case <-watch.Updates():
				cfgs = append(cfgs, cfg)
			case err = <-watch.Errors():
				errs = append(errs, err)
			}
		}
	}()

	err = writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	// delay is required, otherwise it shows up as just one write event
	time.Sleep(50 * time.Millisecond)

	err = writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	<-ctx.Done()

	assert.Empty(t, errs)
	assert.Equal(t, 2, len(cfgs))
}

func TestConfigWatch_ConfigFileRemoved(t *testing.T) {
	cfgFile := t.TempDir() + "/config.toml"
	cfg := Config{Repo: RepositoryConfig{Location: "/tmp/" + rand.Text()}}

	err := writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	watch, err := NewWatch(ctx, cfgFile)
	assert.NoError(t, err)

	cfgs := make([]Config, 0)
	errs := make([]error, 0)

	go func() {
		defer cancel()

		for i := 0; i < 1; i++ {
			select {
			case <-watch.Updates():
				cfgs = append(cfgs, cfg)
			case err = <-watch.Errors():
				errs = append(errs, err)
			}
		}
	}()

	err = os.Remove(cfgFile)
	assert.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	err = writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	<-ctx.Done()

	assert.Empty(t, errs)
	assert.Equal(t, 1, len(cfgs))
}

func TestConfigWatch_ConfigFileRemovedTimeout(t *testing.T) {
	cfgFile := t.TempDir() + "/config.toml"
	cfg := Config{Repo: RepositoryConfig{Location: "/tmp/" + rand.Text()}}

	err := writeConfig(cfgFile, cfg)
	assert.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	watch, err := NewWatch(ctx, cfgFile)
	assert.NoError(t, err)

	cfgs := make([]Config, 0)
	errs := make([]error, 0)

	go func() {
		defer cancel()

		for i := 0; i < 1; i++ {
			select {
			case <-watch.Updates():
				cfgs = append(cfgs, cfg)
			case err = <-watch.Errors():
				errs = append(errs, err)
			}
		}
	}()

	err = os.Remove(cfgFile)
	assert.NoError(t, err)

	<-ctx.Done()

	assert.Empty(t, cfgs)
	assert.Equal(t, 1, len(errs))
}
