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

package store

import (
	"errors"
	"os"

	"github.com/Masterminds/semver/v3"
	"github.com/pelletier/go-toml/v2"
)

var Version = semver.MustParse("0.0.0+devel")

type Config struct {
	StoragePath          string
	ListenAddress        string
	MetricsListenAddress *string
	Tls                  *TlsConfig
}

type TlsConfig struct {
	CertFile string
	KeyFile  string
}

func LoadConfig(path string) (*Config, error) {
	configReader, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = configReader.Close()
	}()

	decoder := toml.NewDecoder(configReader)

	var conf Config
	if err = decoder.Decode(&conf); err != nil {
		return nil, err
	}

	return &conf, nil
}

func InitStoragePath(config *Config) error {
	if config.StoragePath == "" {
		return errors.New("no storage path configured")
	}

	return os.MkdirAll(config.StoragePath, 0o700)
}
