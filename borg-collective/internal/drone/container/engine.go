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

package container

import (
	"context"

	"github.com/vemilyus/borg-collective/internal/utils"
)

type Engine interface {
	EnsureContainerRunning(ctx context.Context, containerID string) error
	EnsureContainerStopped(ctx context.Context, containerID string) error

	Exec(ctx context.Context, containerID string, cmd []string) error
	ExecWithOutput(ctx context.Context, containerID string, cmd []string) (utils.ErrorReadCloser, error)
}
