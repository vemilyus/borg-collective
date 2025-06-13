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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUtilsSplitCommandLine(t *testing.T) {
	simpleCommand := "borg init"
	assert.Equal(t, []string{"borg", "init"}, SplitCommandLine(simpleCommand))

	complexCommand := `echo "this is a \"value\"" my dudes`
	assert.Equal(t, []string{"echo", `this is a "value"`, "my", "dudes"}, SplitCommandLine(complexCommand))

	anotherComplexOne := `echo 'this is a \'value\'' my brother`
	assert.Equal(t, []string{"echo", "this is a 'value'", "my", "brother"}, SplitCommandLine(anotherComplexOne))

	complexIncludingEnvVar := `echo "this is ${ENV_VAR} value"`
	assert.Equal(t, []string{"echo", "this is ${ENV_VAR} value"}, SplitCommandLine(complexIncludingEnvVar))
}
