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
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
	"strings"
	"time"
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

func NewWatch(path string) (*Watch, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	watch := &Watch{
		updates: make(chan Config),
		err:     make(chan error),
	}

	go func() {
		var lastOp fsnotify.Op

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Has(fsnotify.Remove) {
					log.Debug().Msg("config file was removed")
					lastOp = fsnotify.Remove

					// need to re-add watch due to removal
					err = watcher.Add(path)
					if err != nil {
						watch.err <- err
						return
					}

					continue
				}

				lastOp = 0

				if strings.Contains(event.Op.String(), "CLOSE_WRITE") {
					log.Info().Msg("config file changed, reloading...")
					config, err := LoadConfig(path)
					if err != nil {
						log.Warn().Err(err).Str("path", path).Msg("failed to load config file")
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
