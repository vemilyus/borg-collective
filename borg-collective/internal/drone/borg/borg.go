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
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg/api"
	"github.com/vemilyus/borg-collective/internal/drone/config"
)

var (
	supportedVersionMin   = semver.MustParse("1.2.5")
	supportedVersionUpper = semver.MustParse("2.0.0")
)

type Client struct {
	configLock sync.RWMutex
	config     config.Config
}

func NewClient(config config.Config) (*Client, error) {
	b := &Client{config: config}

	version, err := b.Version()
	if err != nil {
		return nil, fmt.Errorf("failed to check borg version: %v", err)
	}

	if version.LessThan(supportedVersionMin) || version.GreaterThanEqual(supportedVersionUpper) {
		return nil, fmt.Errorf(
			"unsupported borg version (must be >= %v and < %v): %v",
			supportedVersionMin,
			supportedVersionUpper,
			version,
		)
	}

	log.Info().
		Str("version", version.String()).
		Msgf("borg version: %v", version)

	return b, nil
}

func (b *Client) SetConfig(config config.Config) {
	b.configLock.Lock()
	defer b.configLock.Unlock()

	b.config = config
}

func (b *Client) Version() (*semver.Version, error) {
	log.Debug().Msg("determining borg version")

	cmd := exec.Command("borg", "--version")
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("failed to get borg version: %w", err)
	}

	split := strings.Split(strings.TrimSpace(string(output)), " ")
	if len(split) != 2 {
		return nil, fmt.Errorf("failed to parse borg version: %s", output)
	}

	return semver.NewVersion(split[1])
}

func (b *Client) Info() (api.InfoListOutput, error) {
	args := []string{"info", "--json"}

	b.configLock.RLock()
	args = b.setRsh(args)
	args = append(args, b.config.Repo.Location)

	env := b.env()
	b.configLock.RUnlock()

	var info api.InfoListOutput
	returnCode, logMessages, err := api.Run(nil, args, env, nil, &info)
	if err != nil {
		return api.InfoListOutput{}, fmt.Errorf("failed to run borg info: %w", err)
	}

	return info, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Client) Init() error {
	args := []string{"init", "--make-parent-dirs"}

	b.configLock.RLock()
	if b.config.Encryption != nil {
		args = append(args, "--encryption=keyfile")
	} else {
		args = append(args, "--encryption=none")
	}

	args = b.setRsh(args)
	args = append(args, b.config.Repo.Location)

	env := b.env()
	b.configLock.RUnlock()

	log.Info().Msgf("initializing repository: %v", b.config.Repo.Location)

	returnCode, logMessages, err := api.Run(nil, args, env, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to run borg init: %w", err)
	}

	return api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Client) CreateWithPaths(archiveName string, paths []string) (api.CreateOutput, error) {
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return api.CreateOutput{}, fmt.Errorf("path %s is not an absolute path", path)
		}
	}

	args := []string{"create", "--json", "--compression", "zlib,6"}

	b.configLock.RLock()
	args = b.setRsh(args)
	args = append(args, fmt.Sprintf("%s::%s", b.config.Repo.Location, archiveName))
	args = append(args, paths...)

	env := b.env()
	b.configLock.RUnlock()

	log.Info().Strs("paths", paths).Msgf("creating archive: %v", archiveName)

	var stats api.CreateOutput
	returnCode, logMessages, err := api.Run(nil, args, env, nil, &stats)
	if err != nil {
		return api.CreateOutput{}, fmt.Errorf("failed to run borg create with paths: %w", err)
	}

	return stats, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Client) CreateWithInput(ctx context.Context, archiveName string, input io.Reader) (api.CreateOutput, error) {
	if input == nil {
		panic("input cannot be nil")
	}

	args := []string{"create", "--json", "--compression", "zlib,6"}

	b.configLock.RLock()
	args = b.setRsh(args)
	args = append(args, fmt.Sprintf("%s::%s", b.config.Repo.Location, archiveName))
	args = append(args, "-")

	env := b.env()
	b.configLock.RUnlock()

	log.Info().Ctx(ctx).Msgf("creating archive from input: %v", archiveName)

	var stats api.CreateOutput
	returnCode, logMessages, err := api.Run(ctx, args, env, input, &stats)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return api.CreateOutput{}, err
		}

		return api.CreateOutput{}, fmt.Errorf("failed to run borg create with stdin: %w", err)
	}

	return stats, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Client) Compact() error {
	args := []string{"compact"}

	b.configLock.RLock()
	args = b.setRsh(args)

	repoLocation := b.config.Repo.Location
	args = append(args, repoLocation)

	env := b.env()
	b.configLock.RUnlock()

	log.Info().Msgf("compacting repository: %v", repoLocation)

	returnCode, logMessages, err := api.Run(nil, args, env, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to run borg compact: %w", err)
	}

	return api.HandleBorgReturnCode(returnCode, logMessages)
}

func defaultEnv() map[string]string {
	return map[string]string{
		"LANG":            "en_US.UTF-8",
		"LC_CTYPE":        "en_US.UTF-8",
		"BORG_EXIT_CODES": "modern",
	}
}

func (b *Client) env() map[string]string {
	env := defaultEnv()
	if b.config.Encryption != nil {
		if b.config.Encryption.SecretCommand != nil {
			env["BORG_PASSCOMMAND"] = *b.config.Encryption.SecretCommand
		} else {
			env["BORG_PASSPHRASE"] = *b.config.Encryption.Secret
		}
	}

	return env
}

func (b *Client) setRsh(args []string) []string {
	if b.config.Repo.IdentityFile != nil {
		log.Debug().Msgf("using identity file %s", *b.config.Repo.IdentityFile)
		args = append(args, "--rsh", "ssh -i "+*b.config.Repo.IdentityFile)
	}

	return args
}
