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
	"errors"

	"filippo.io/age"
	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/vemilyus/borg-collective/credentials/internal/proto"
	"github.com/vemilyus/borg-collective/credentials/internal/store/vault"
	"google.golang.org/grpc/peer"
)

func (s *State) SetRecoveryRecipient(request *proto.RecoveryRecipient) error {
	err := s.vault.VerifyPassphrase(request.GetCredentials().Passphrase)
	if err != nil {
		return err
	}

	var recipient *age.X25519Recipient
	recipient, err = age.ParseX25519Recipient(request.Recipient)
	if err != nil {
		return err
	}

	err = s.vault.SetRecoveryRecipient(*recipient)
	if err != nil {
		return err
	}

	return nil
}

func (s *State) CreateVaultItem(request *proto.ItemCreation) (*proto.Item, error) {
	err := s.vault.VerifyPassphrase(request.GetCredentials().Passphrase)
	if err != nil {
		return nil, err
	}

	item, err := s.vault.CreateItem(request.Description)
	if err != nil {
		return nil, err
	}

	itemValue := memguard.NewBufferFromBytes(request.GetValue())

	err = s.vault.SetItemValue(item.Id, itemValue)
	if err != nil {
		_ = s.vault.DeleteItem(item.Id)
		return nil, err
	}

	return &proto.Item{
		Id:          item.Id.String(),
		Description: item.Description,
		Checksum:    item.Checksum,
		CreatedAt:   item.ModifiedAt.UnixMilli(),
	}, nil
}

func (s *State) ListVaultItems(request *proto.ItemSearch) ([]vault.Item, error) {
	err := s.vault.VerifyPassphrase(request.GetCredentials().Passphrase)
	if err != nil {
		return nil, err
	}

	items := s.vault.Items()

	return items, nil
}

func (s *State) DeleteVaultItems(request *proto.ItemDeletion) ([]uuid.UUID, error) {
	err := s.vault.VerifyPassphrase(request.GetCredentials().Passphrase)
	if err != nil {
		return nil, err
	}

	deletedItemIds := make([]uuid.UUID, 0, len(request.Id))

	for _, idRaw := range request.Id {
		id, err := uuid.Parse(idRaw)
		if err != nil {
			return nil, err
		}

		err = s.vault.DeleteItem(id)
		if err != nil {
			return nil, err
		}

		deletedItemIds = append(deletedItemIds, id)
	}

	return deletedItemIds, nil
}

func (s *State) ReadVaultItem(request *proto.ItemRequest, p *peer.Peer) (*proto.ItemValue, error) {
	var err error
	if request.GetAdmin() != nil {
		err = s.vault.VerifyPassphrase(request.GetAdmin().GetPassphrase())
	} else if request.GetClient() != nil {
		err = s.verifyClientCredentials(request.GetClient())
	} else {
		err = errors.New("invalid request: no credentials provided")
	}

	itemId, err := uuid.Parse(request.GetItemId())
	if err != nil {
		return nil, err
	}

	var value *memguard.LockedBuffer
	if request.GetClient() != nil {
		value, err = s.vault.GetItemForPeer(itemId, p.Addr.String())
	} else {
		value, err = s.vault.GetItem(itemId)
	}

	if err != nil {
		return nil, err
	}

	defer value.Destroy()

	valueBytes := make([]byte, len(value.Bytes()))
	copy(valueBytes, value.Bytes())

	return &proto.ItemValue{Value: valueBytes}, nil
}
