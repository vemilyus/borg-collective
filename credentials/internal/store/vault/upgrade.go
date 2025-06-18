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

package vault

import (
	"fmt"

	"filippo.io/age"
	"github.com/Masterminds/semver/v3"
	"github.com/awnumar/memguard"
	"github.com/vemilyus/borg-collective/credentials/internal/store"
)

var versionBeforeRecoveryVerified = semver.MustParse("0.2.0")

func (v *Vault) upgrade(secret *memguard.LockedBuffer) error {
	currentVersionBytes, err := v.backend().ReadFile(".version")
	if err != nil {
		return err
	}

	var currentVersion *semver.Version
	if currentVersionBytes != nil {
		currentVersion, err = semver.NewVersion(string(currentVersionBytes))
		if err != nil {
			return err
		}
	}

	if currentVersion == nil || currentVersion.LessThanEqual(versionBeforeRecoveryVerified) {
		existingRecoveryHash, err := v.backend().ReadFile(".recovery.sum")
		if err != nil {
			return err
		} else if existingRecoveryHash != nil {
			return fmt.Errorf(".recovery.sum exists")
		}

		recBytes, err := v.backend().ReadFile(".recovery")
		if err != nil {
			return err
		} else if recBytes != nil {
			recipient, err := age.ParseX25519Recipient(string(recBytes))
			if err != nil {
				return err
			}

			recoveryHash := createRecoveryHash(*recipient, secret)
			err = v.backend().WriteFile(".recovery.sum", []byte(recoveryHash))
			if err != nil {
				return err
			}
		}
	}

	return v.backend().WriteFile(".version", []byte(store.Version.String()))
}
