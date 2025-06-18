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
	"github.com/Masterminds/semver/v3"
	"github.com/awnumar/memguard"
	"github.com/integrii/flaggy"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/credentials/internal/logging"
	"github.com/vemilyus/borg-collective/credentials/internal/store"
	"github.com/vemilyus/borg-collective/credentials/internal/store/metrics"
	"github.com/vemilyus/borg-collective/credentials/internal/store/server"
	"github.com/vemilyus/borg-collective/credentials/internal/store/service"
	"github.com/vemilyus/borg-collective/credentials/internal/store/vault"
)

var (
	version = "0.0.0+devel"

	prod       bool
	configPath string
)

func main() {
	store.Version = semver.MustParse(version)

	memguard.CatchInterrupt()
	defer memguard.Purge()

	parseArgs()
	logging.InitLogging(prod)

	config := loadConfig(configPath)

	vaultInstance, err := vault.NewVault(&vault.Options{
		Backend: vault.NewLocalStorageBackend(config.StoragePath),
		Secure:  prod,
	})

	if err != nil {
		log.Fatal().Err(err).Send()
	}

	state := service.NewState(
		config,
		vaultInstance,
		prod,
	)

	srv, err := server.NewServer(state)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	if prod {
		log.Info().Msg("Running in production mode")
	}

	log.Info().Msgf("Listening on %s", config.ListenAddress)

	asyncErr := make(chan error)
	go func() { asyncErr <- srv.Serve() }()
	if config.MetricsListenAddress != nil {
		log.Info().Msgf("Metrics available at %s/metrics", *config.MetricsListenAddress)
		go func() { asyncErr <- metrics.Serve(config) }()
	}

	err = <-asyncErr
	if err != nil {
		log.Fatal().Err(err).Send()
	}
}

func parseArgs() {
	flaggy.SetName("credstore")
	flaggy.SetDescription("Securely stores and provides credentials over the network")
	flaggy.SetVersion(store.Version.String())

	flaggy.Bool(&prod, "p", "production", "Indicates whether to run in production mode (requires TLS config)")
	flaggy.AddPositionalValue(&configPath, "CONFIG-PATH", 1, true, "Path to the configuration file")

	flaggy.Parse()
}

func loadConfig(configPath string) *store.Config {
	config, err := store.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	err = store.InitStoragePath(config)
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	return config
}
