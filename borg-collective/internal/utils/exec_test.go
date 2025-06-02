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
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Exec(t *testing.T) {
	err := Exec(context.Background(), []string{"bash", "-c", "echo \"hello world\""})
	assert.NoError(t, err)

	err = Exec(context.Background(), []string{"bash", "-c", "exit 2"})
	assert.Error(t, err)

	var exitErr *exec.ExitError
	assert.ErrorAs(t, errors.Unwrap(err), &exitErr)

	assert.Equal(t, 2, exitErr.ExitCode())
}

func Test_ExecWithOutput(t *testing.T) {
	output, err := ExecWithOutput(context.Background(), []string{"bash", "-c", "echo \"hello world\""})
	assert.NoError(t, err)

	var buf bytes.Buffer
	_, err = buf.ReadFrom(output)

	assert.NoError(t, err)
	assert.Equal(t, "hello world\n", buf.String())

	expectedCount := 524288
	longOutput, err := ExecWithOutput(context.Background(), []string{"bash", "-c", fmt.Sprintf("cat /dev/random | head -c %d", expectedCount)})
	assert.NoError(t, err)

	buf.Reset()
	_, err = buf.ReadFrom(longOutput)

	assert.NoError(t, longOutput.Error())
	assert.Equal(t, expectedCount, buf.Len())

	failingOutput, err := ExecWithOutput(context.Background(), []string{"bash", "-c", fmt.Sprintf("cat /dev/random | head -c %d; exit 1", expectedCount)})
	assert.NoError(t, err)

	buf.Reset()
	_, err = buf.ReadFrom(failingOutput)

	assert.Error(t, failingOutput.Error())

	var exitErr *exec.ExitError
	assert.ErrorAs(t, errors.Unwrap(failingOutput.Error()), &exitErr)
}
