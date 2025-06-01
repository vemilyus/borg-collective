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
	"context"
	"encoding/json"

	"github.com/docker/docker/client"
	"github.com/integrii/flaggy"
	"github.com/pkg/errors"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/internal/drone/borg"
	"github.com/vemilyus/borg-collective/internal/drone/borg/api"
	"github.com/vemilyus/borg-collective/internal/drone/config"
	"github.com/vemilyus/borg-collective/internal/drone/container/docker"
	"github.com/vemilyus/borg-collective/internal/drone/worker"
	"github.com/vemilyus/borg-collective/internal/logging"
)

var (
	version = "unknown"

	configPath string
)

func main() {
	parseArgs()
	logging.InitLogging()

	if config.Verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	initialConfig, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to load config file")
	}

	borgClient, err := borg.NewClient(*initialConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to create Borg client")
	}

	var dockerClient *docker.Client
	rawDockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Warn().Err(err).Msg("Docker not available")
	} else {
		dockerClient = docker.NewClient(rawDockerClient)
	}

	cronLogger := logging.NewZerologCronLogger(config.Verbose)

	scheduler := cron.New(
		cron.WithLogger(cronLogger),
		cron.WithChain(cron.SkipIfStillRunning(cronLogger), cron.Recover(cronLogger)),
	)

	wrk := worker.NewWorker(configPath, borgClient, dockerClient, scheduler)
	err = wrk.ScheduleStaticBackups(initialConfig.Backups)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to schedule backups")
	}

	if dockerClient != nil {
		dockerBackups, err := dockerClient.ReadProjects(context.Background())
		if err != nil {
			log.Fatal().Err(err).Msg("failed to load docker container state")
		}

		err = wrk.ScheduleContainerBackups(dockerBackups)
		if err != nil {
			log.Fatal().Err(err).Msg("failed to schedule container backups")
		}
	}

	info, err := borgClient.Info()
	var borgError api.Error
	if err != nil {
		if !errors.As(err, &borgError) || !borgError.IsRecoverable() {
			log.Fatal().Err(err).Msg("failed to retrieve borg repository info")
		}
	}

	initializeRepository := false
	if info.Repository.Id != "" {
		infoJson, _ := json.Marshal(info)
		log.Info().
			RawJSON("info", infoJson).
			Msg("retrieved borg repository info")
	} else if err != nil && borgError.ReturnCode() == api.ReturnCodeRepositoryDoesNotExist {
		initializeRepository = true
		log.Info().Msg("borg repository does not exist")
	}

	if !config.DryRun {
		if initializeRepository {
			err = borgClient.Init()
			if err != nil {
				log.Fatal().Err(err).Msg("failed to initialize borg repository")
			}
		}

		if config.Once {
			err = wrk.RunOnce()
		} else {
			err = wrk.Run()
		}
	}

	if err != nil {
		log.Fatal().Err(err).Msg("scheduler failed")
	}
}

func parseArgs() {
	flaggy.SetName("borgd")
	flaggy.SetDescription("Schedules and controls the execution of borg backups")
	flaggy.SetVersion(version)

	flaggy.AddPositionalValue(&configPath, "CONFIG-PATH", 1, true, "Path to the configuration file")
	flaggy.Bool(&config.DryRun, "", "dry-run", "Configure all backups without actually running them")
	flaggy.Bool(&config.Once, "", "once", "Run all configured backups once and exit")
	flaggy.Bool(&config.Verbose, "", "verbose", "Enable verbose log output")

	flaggy.Parse()
}
