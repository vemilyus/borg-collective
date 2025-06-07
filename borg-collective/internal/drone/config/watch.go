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
	"fmt"
	"github.com/rs/zerolog"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

type Watch struct {
	updates chan Config
	err     chan error
}

func (w *Watch) Updates() <-chan Config {
	return w.updates
}

func (w *Watch) Errors() <-chan error {
	return w.err
}

func NewWatch(ctx context.Context, path string) (*Watch, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watch := &Watch{
		updates: make(chan Config),
		err:     make(chan error),
	}

	go func() {
		watchingParentDir := false
		var lastOp fsnotify.Op

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					_ = watcher.Close()
					return
				}

				if log.Trace().Enabled() {
					log.Trace().
						Str("path", event.Name).
						Str("op", event.Op.String()).
						Msg("received config watch event")
				}

				if event.Has(fsnotify.Remove) {
					if watchingParentDir {
						watch.err <- fmt.Errorf("config file parent directory was removed: %s", filepath.Dir(path))
						return
					}

					log.Debug().Msg("config file was removed")
					lastOp = fsnotify.Remove

					// need to add dir-watch due to removal
					err = watcher.Add(filepath.Dir(event.Name))
					if err != nil {
						watch.err <- err
						return
					}

					watchingParentDir = true

					continue
				}

				lastOp = 0

				if event.Has(fsnotify.Write) && event.Name == path {
					if watchingParentDir {
						err = watcher.Remove(filepath.Dir(event.Name))
						if err != nil {
							watch.err <- err
							return
						}
						watchingParentDir = false

						err = watcher.Add(path)
						if err != nil {
							watch.err <- err
							return
						}
					}

					log.Info().Msg("config file changed")
					config, err := LoadConfig(path)
					if err != nil {
						var evt *zerolog.Event
						if strings.HasPrefix(err.Error(), "toml:") {
							evt = log.Debug()
						} else {
							evt = log.Warn()
						}

						if evt.Enabled() {
							evt.Err(err).Str("path", path).Msg("failed to load config file")
						}

						continue
					}

					watch.updates <- *config
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				watch.err <- err
				return
			case <-time.After(500 * time.Millisecond):
				if lastOp > 0 {
					watch.err <- fmt.Errorf("timed out after last op %s", lastOp.String())
					return
				}
			case <-ctx.Done():
				_ = watcher.Close()
				return
			}
		}
	}()

	err = watcher.Add(path)
	if err != nil {
		_ = watcher.Close()
		return nil, err
	}

	log.Info().Msgf("watching config file for changes: %s", path)

	return watch, nil
}
