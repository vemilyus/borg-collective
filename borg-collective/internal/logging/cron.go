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

package logging

import (
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

type zerologCronLogger struct {
	verbose bool
}

func NewZerologCronLogger(verbose bool) cron.Logger {
	return zerologCronLogger{verbose}
}

func (z zerologCronLogger) Info(msg string, keysAndValues ...interface{}) {
	if z.verbose {
		log.Info().Fields(keysAndValues).Msg(msg)
	}
}

func (z zerologCronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	log.Error().Err(err).Fields(keysAndValues).Msg(msg)
}
