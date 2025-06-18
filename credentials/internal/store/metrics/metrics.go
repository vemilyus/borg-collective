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

package metrics

import (
	"crypto/tls"
	"errors"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vemilyus/borg-collective/credentials/internal/store"
	"github.com/vemilyus/borg-collective/credentials/internal/store/cert"
)

func Serve(config *store.Config) error {
	http.Handle("/metrics", promhttp.Handler())

	if config.Tls != nil {
		var certReloader *cert.X509KeyPairReloader
		certReloader, err := cert.NewX509KeyPairReloader(config.Tls.CertFile, config.Tls.KeyFile)
		if err != nil {
			return errors.New("Failed to load TLS certificate: " + err.Error())
		}

		tlsConfig := &tls.Config{
			GetCertificate: certReloader.GetCertificate,
		}

		listener, err := tls.Listen("tcp", *config.MetricsListenAddress, tlsConfig)
		if err != nil {
			return err
		}

		return http.Serve(listener, nil)
	} else {
		return http.ListenAndServe(*config.MetricsListenAddress, nil)
	}
}
