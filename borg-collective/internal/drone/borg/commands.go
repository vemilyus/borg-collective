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
	"github.com/Masterminds/semver/v3"
	"github.com/vemilyus/borg-collective/internal/drone/borg/api"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

func defaultEnv() map[string]string {
	return map[string]string{
		"LANG":            "en_US.UTF-8",
		"LC_CTYPE":        "en_US.UTF-8",
		"BORG_EXIT_CODES": "modern",
	}
}

func (b *Borg) env() map[string]string {
	env := defaultEnv()
	if b.config.Encryption != nil {
		if b.config.Encryption.SecretId != nil {
			env["BORG_PASSCOMMAND"] = "cred item read " + *b.config.Encryption.SecretId
		} else {
			env["BORG_PASSPHRASE"] = *b.config.Encryption.Secret
		}
	}

	return env
}

func (b *Borg) setRsh(args []string) []string {
	if b.config.Repo.IdentityFile != nil {
		args = append(args, "--rsh", "ssh -i "+*b.config.Repo.IdentityFile)
	}

	return args
}

func (b *Borg) runInfo() (api.InfoListOutput, error) {
	args := []string{"info", "--json"}
	args = b.setRsh(args)
	args = append(args, b.config.Repo.Location)

	var info api.InfoListOutput
	returnCode, logMessages, err := api.Run(nil, args, b.env(), nil, &info)
	if err != nil {
		return api.InfoListOutput{}, fmt.Errorf("failed to run borg info: %w", err)
	}

	return info, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Borg) runInit() error {
	args := []string{"init", "--make-parent-dirs"}
	if b.config.Encryption != nil {
		args = append(args, "--encryption=keyfile")
	} else {
		args = append(args, "--encryption=none")
	}

	args = b.setRsh(args)
	args = append(args, b.config.Repo.Location)

	returnCode, logMessages, err := api.Run(nil, args, b.env(), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to run borg init: %w", err)
	}

	return api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Borg) runCreateWithPaths(archiveName string, paths []string) (api.CreateOutput, error) {
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return api.CreateOutput{}, fmt.Errorf("path %s is not an absolute path", path)
		}
	}

	args := []string{"create", "--json", "--compression", "zlib,6"}
	args = b.setRsh(args)
	args = append(args, fmt.Sprintf("%s::%s", b.config.Repo.Location, archiveName))
	args = append(args, paths...)

	var stats api.CreateOutput
	returnCode, logMessages, err := api.Run(nil, args, b.env(), nil, &stats)
	if err != nil {
		return api.CreateOutput{}, fmt.Errorf("failed to run borg create with paths: %w", err)
	}

	return stats, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Borg) runCreateWithInput(ctx context.Context, archiveName string, input io.Reader) (api.CreateOutput, error) {
	if input == nil {
		panic("input cannot be nil")
	}

	args := []string{"create", "--json", "--compression", "zlib,6"}
	args = b.setRsh(args)
	args = append(args, fmt.Sprintf("%s::%s", b.config.Repo.Location, archiveName))
	args = append(args, "-")

	var stats api.CreateOutput
	returnCode, logMessages, err := api.Run(ctx, args, b.env(), input, &stats)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return api.CreateOutput{}, err
		}

		return api.CreateOutput{}, fmt.Errorf("failed to run borg create with stdin: %w", err)
	}

	return stats, api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Borg) runCompact() error {
	args := []string{"compact"}
	args = b.setRsh(args)
	args = append(args, b.config.Repo.Location)

	returnCode, logMessages, err := api.Run(nil, args, b.env(), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to run borg compact: %w", err)
	}

	return api.HandleBorgReturnCode(returnCode, logMessages)
}

func (b *Borg) runVersionLocal() (*semver.Version, error) {
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
