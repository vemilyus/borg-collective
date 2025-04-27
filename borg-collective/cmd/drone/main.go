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

package main

import (
	"github.com/integrii/flaggy"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone"
	"github.com/vemilyus/borg-collective/internal/drone/worker"
	"github.com/vemilyus/borg-collective/internal/logging"
)

var (
	version = "unknown"

	configPath string
	verbose    = false
)

func main() {
	parseArgs()
	logging.InitLogging()

	config, err := drone.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}

	cronLogger := logging.NewZerologCronLogger(verbose)

	scheduler := cron.New(
		cron.WithLogger(cronLogger),
		cron.WithChain(cron.SkipIfStillRunning(cronLogger), cron.Recover(cronLogger)),
	)

	w, err := worker.New(scheduler, config)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create worker")
	}

	err = w.Start()
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}

func parseArgs() {
	flaggy.SetName("borgd")
	flaggy.SetDescription("Schedules and controls the execution of borg backups")
	flaggy.SetVersion(version)

	flaggy.AddPositionalValue(&configPath, "CONFIG-PATH", 1, true, "Path to the configuration file")
	flaggy.Bool(&verbose, "", "verbose", "Enable verbose log output")

	flaggy.Parse()
}
