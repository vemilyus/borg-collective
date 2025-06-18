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

package service

import (
	"github.com/vemilyus/borg-collective/credentials/internal/proto"
	"github.com/vemilyus/borg-collective/credentials/internal/store"
	"github.com/vemilyus/borg-collective/credentials/internal/store/vault"
)

type State struct {
	config       *store.Config
	vault        *vault.Vault
	isProduction bool
}

func NewState(config *store.Config, vault *vault.Vault, prod bool) *State {
	return &State{
		config:       config,
		vault:        vault,
		isProduction: prod,
	}
}

func (s *State) Config() *store.Config {
	return s.config
}

func (s *State) IsProduction() bool {
	return s.isProduction
}

func (s *State) StoreInfo() *proto.StoreInfo {
	return &proto.StoreInfo{
		Version:       store.Version.String(),
		IsVaultLocked: s.vault.IsLocked(),
		IsProduction:  s.IsProduction(),
	}
}

func (s *State) Unlock(request *proto.AdminCredentials) error {
	return s.vault.Unlock(request.Passphrase)
}

func (s *State) Lock() bool {
	err := s.vault.Lock()
	if err != nil {
		return false
	}

	return true
}
