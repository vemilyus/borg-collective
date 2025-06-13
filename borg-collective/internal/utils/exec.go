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

package utils

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/config"
)

func Exec(ctx context.Context, command []string) error {
	log.Info().
		Ctx(ctx).
		Strs("command", command).
		Msg("executing command")

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	if ctx.Value("test") == true {
		cmd.Stderr = os.Stderr
	}

	output, err := cmd.Output()

	if err != nil {
		exitEvent := log.Warn().
			Ctx(ctx).
			Err(err).
			Strs("command", command)

		if config.Verbose && len(output) > 0 {
			exitEvent.Strs("output", strings.Split(string(output), "\n"))
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitEvent.
				Int("exitCode", exitErr.ExitCode()).
				Msg("command finished with non-zero exit code")
		} else {
			exitEvent.Msg("error executing command")
		}

		return errors.Wrap(err, "command execution failed")
	}

	return nil
}

type execOutputWrapper struct {
	delegate    io.ReadCloser
	errMutex    sync.Mutex
	err         chan error
	gotErrValue bool
	returnedErr error
}

func (e *execOutputWrapper) Read(p []byte) (n int, err error) {
	return e.delegate.Read(p)
}

func (e *execOutputWrapper) Error() error {
	if !e.gotErrValue {
		e.errMutex.Lock()
		defer e.errMutex.Unlock()

		if !e.gotErrValue {
			retErr := <-e.err
			e.returnedErr = retErr
			e.gotErrValue = true
		}
	}

	return e.returnedErr
}

func ExecWithOutput(ctx context.Context, command []string) (ErrorReader, error) {
	log.Info().
		Ctx(ctx).
		Strs("command", command).
		Msg("executing command with output")

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	stderr := new(bytes.Buffer)
	cmd.Stderr = stderr

	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	wrapper := execOutputWrapper{
		delegate: output,
		err:      make(chan error, 1),
	}

	go func() {
		err = cmd.Wait()
		if err != nil {
			wrapper.err <- errors.Wrap(err, "command execution failed")

			exitEvent := log.Warn().
				Ctx(ctx).
				Err(err).
				Strs("command", command)

			if config.Verbose && stderr.Len() > 0 {
				exitEvent.Strs("output", strings.Split(string(stderr.Bytes()), "\n"))
			}

			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitEvent.
					Int("exitCode", exitErr.ExitCode()).
					Msg("command finished with non-zero exit code")
			} else {
				exitEvent.Msg("error executing command")
			}
		} else {
			wrapper.err <- nil
		}
	}()

	return &wrapper, nil
}
