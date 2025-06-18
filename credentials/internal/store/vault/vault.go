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
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"time"
	"unsafe"

	"filippo.io/age"
	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog/log"
)

const sentinel = "sentinel"

var vaultOpenGauge = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "credstore",
	Subsystem: "vault",
	Name:      "unlocked",
})

var vaultItemsCount = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: "credstore",
	Subsystem: "vault",
	Name:      "items_count",
})

var vaultItemReads = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "credstore",
	Subsystem: "vault",
	Name:      "item_reads",
})

var vaultItemReadAttempts = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: "credstore",
	Subsystem: "vault",
	Name:      "item_read_attempts",
})

type Options struct {
	Backend
	Secure bool
}

type Item struct {
	Id          uuid.UUID `json:"id"`
	Description string    `json:"description"`
	Peer        *string   `json:"peer"`
	Checksum    string    `json:"checksum"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type Vault struct {
	lock               sync.RWMutex
	options            *Options
	identityKey        *memguard.Enclave
	metadataHmacSecret *memguard.Enclave
	primaryRecipient   *age.X25519Recipient
	items              map[uuid.UUID]Item
}

func (v *Vault) backend() Backend {
	return v.options.Backend
}

func (v *Vault) Options() *Options {
	return v.options
}

func (v *Vault) IsLocked() bool {
	return v.identityKey == nil
}

func NewVault(options *Options) (*Vault, error) {
	err := options.Backend.Init()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize backend: %w", err)
	}

	return &Vault{
		lock:               sync.RWMutex{},
		options:            options,
		identityKey:        nil,
		metadataHmacSecret: nil,
		primaryRecipient:   nil,
		items:              nil,
	}, nil
}

func (v *Vault) Unlock(passphrase string) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	if !v.IsLocked() {
		return nil
	}

	passphraseBytes := *(*[]byte)(unsafe.Pointer(&passphrase))
	hasher := sha256.New()
	hasher.Write(passphraseBytes)
	if v.Options().Secure {
		hasher.Write([]byte(sentinel))
	}

	rawSum := hasher.Sum(nil)
	memguard.WipeBytes(passphraseBytes)

	v.identityKey = memguard.NewEnclave(rawSum)

	identityBytes, err := v.backend().ReadFile(".identity")
	if err != nil {
		v.identityKey = nil

		log.Error().Err(err).Msg("failed to read identity file")
		return errors.New("failed to verify passphrase")
	} else if identityBytes != nil {
		memguard.WipeBytes(identityBytes)

		identityKey, _ := v.identityKey.Open()
		defer identityKey.Destroy()

		identity, err := readIdentity(v.backend(), identityKey)
		if err != nil {
			v.identityKey = nil

			log.Error().Err(err).Msg("failed to read identity file")
			return errors.New("failed to verify passphrase")
		}

		v.metadataHmacSecret = deriveMetadataHmacSecret(*identity)
		v.primaryRecipient = identity.Recipient()
	} else {
		identity, err := age.GenerateX25519Identity()
		if err != nil {
			v.identityKey = nil

			log.Error().Err(err).Msg("failed to generate primary identity")
			return errors.New("failed to verify passphrase")
		}

		identityKey, _ := v.identityKey.Open()
		defer identityKey.Destroy()

		err = writeIdentity(v.backend(), identityKey, identity)
		if err != nil {
			log.Err(err).Msg("failed to write identity")
			return errors.New("failed to verify passphrase")
		}

		v.metadataHmacSecret = deriveMetadataHmacSecret(*identity)
		v.primaryRecipient = identity.Recipient()
	}

	metadataHmacSecret, err := v.metadataHmacSecret.Open()
	if err != nil {
		v.identityKey = nil
		v.metadataHmacSecret = nil
		v.primaryRecipient = nil

		log.Error().Err(err).Msg("failed to access metadata HMAC secret")
		return errors.New("failed to verify passphrase")
	}

	defer metadataHmacSecret.Destroy()

	err = v.upgrade(metadataHmacSecret)
	if err != nil {
		return fmt.Errorf("failed to upgrade vault: %w", err)
	}

	v.items, err = readAllMetadataUnsafe(v.backend(), metadataHmacSecret)
	if err != nil {
		v.identityKey = nil
		v.metadataHmacSecret = nil
		v.primaryRecipient = nil
		v.items = nil

		log.Error().Err(err).Msg("failed to read all item metadata")
		return errors.New("failed to verify passphrase")
	}

	vaultOpenGauge.Set(1)
	vaultItemsCount.Set(float64(len(v.items)))

	return nil
}

func (v *Vault) VerifyPassphrase(passphrase string) error {
	v.lock.RLock()
	defer v.lock.RUnlock()

	if v.IsLocked() {
		return errors.New("vault is locked")
	}

	passphraseBytes := *(*[]byte)(unsafe.Pointer(&passphrase))
	hasher := sha256.New()
	hasher.Write(passphraseBytes)
	if v.Options().Secure {
		hasher.Write([]byte(sentinel))
	}

	rawSum := hasher.Sum(nil)
	memguard.WipeBytes(passphraseBytes)

	checkKey := memguard.NewBufferFromBytes(rawSum)

	defer checkKey.Destroy()

	identityKey, err := v.identityKey.Open()
	if err != nil {
		log.Error().Err(err).Msg("failed to open identity key")
		return errors.New("failed to verify passphrase")
	}

	defer identityKey.Destroy()

	if !bytes.Equal(checkKey.Bytes(), identityKey.Bytes()) {
		log.Info().Msg("incorrect passphrase specified")
		return errors.New("failed to verify passphrase")
	}

	return nil
}

func (v *Vault) Lock() error {
	v.lock.Lock()
	defer v.lock.Unlock()

	if v.IsLocked() {
		return errors.New("vault is locked")
	}

	v.identityKey = nil
	v.metadataHmacSecret = nil
	v.primaryRecipient = nil
	v.items = nil

	vaultOpenGauge.Set(0)

	return nil
}

func (v *Vault) Items() []Item {
	v.lock.RLock()
	defer v.lock.RUnlock()

	return slices.Collect(maps.Values(v.items))
}

func (v *Vault) SetRecoveryRecipient(recipient age.X25519Recipient) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	if v.IsLocked() {
		return errors.New("vault is locked")
	}

	metadataHmacSecret, err := v.metadataHmacSecret.Open()
	if err != nil {
		log.Error().Err(err).Msg("failed to access metadata HMAC secret")
		return errors.New("failed to set recovery recipient")
	}

	defer metadataHmacSecret.Destroy()

	oldRecoveryRecipient, err := loadRecoveryRecipient(v.backend(), metadataHmacSecret)
	if err != nil {
		return err
	}

	if err := writeRecoveryRecipient(v.backend(), recipient, metadataHmacSecret); err != nil {
		log.Error().Err(err).Msg("failed to write recovery recipient")

		if oldRecoveryRecipient != nil {
			for i := 0; i < 3; i++ {
				time.Sleep(time.Second)

				err = writeRecoveryRecipient(v.backend(), *oldRecoveryRecipient, metadataHmacSecret)
				if err == nil {
					break
				}
			}

			if err != nil {
				log.Fatal().Err(err).Msg("failed to restore previous recovery recipient")
			}
		}

		return errors.New("failed to set recovery recipient")
	}

	items, err := readAllMetadataUnsafe(v.backend(), metadataHmacSecret)
	metadataHmacSecret.Destroy()

	if err != nil {
		log.Error().Err(err).Msg("failed to read all item metadata")
		return errors.New("failed to set recovery recipient")
	}

	for _, item := range items {
		func() {
			value, err := v.readItemValueUnsafe(item)
			if err != nil {
				log.Error().Err(err).Str("item", item.Id.String()).Msg("failed to read item value")
				return
			}

			defer value.Destroy()

			err = v.writeItemValueUnsafe(item, value)
			if err != nil {
				log.Error().Err(err).Str("item", item.Id.String()).Msg("failed to write item value")
			}
		}()
	}

	return nil
}

func (v *Vault) CreateItem(description string) (*Item, error) {
	v.lock.Lock()
	defer v.lock.Unlock()

	if v.IsLocked() {
		return nil, errors.New("vault is locked")
	}

	id := uuid.New()
	item := Item{
		Id:          id,
		Description: description,
		Checksum:    "",
		ModifiedAt:  time.Now(),
	}

	metadataHmacSecret, err := v.metadataHmacSecret.Open()
	if err != nil {
		log.Error().Err(err).Msg("failed to access metadata HMAC secret")
		return nil, errors.New("failed to create item")
	}

	defer metadataHmacSecret.Destroy()

	if err = writeItemMetadataUnsafe(v.backend(), item, metadataHmacSecret); err != nil {
		log.Error().Err(err).Str("item", item.Id.String()).Msg("failed to write item metadata")
		return nil, errors.New("failed to create item")
	}

	v.items[id] = item

	vaultItemsCount.Set(float64(len(v.items)))

	return &item, nil
}

func (v *Vault) DeleteItem(id uuid.UUID) error {
	v.lock.Lock()
	defer v.lock.Unlock()

	if v.IsLocked() {
		return errors.New("vault is locked")
	}

	ok := v.deleteItemUnsafe(id)
	if !ok {
		log.Warn().Str("item", id.String()).Msg("no such item")
	}

	vaultItemsCount.Set(float64(len(v.items)))

	return nil
}

func (v *Vault) GetItem(id uuid.UUID) (*memguard.LockedBuffer, error) {
	v.lock.RLock()
	defer v.lock.RUnlock()

	vaultItemReadAttempts.Inc()

	if v.IsLocked() {
		return nil, errors.New("vault is locked")
	}

	item, ok := v.items[id]
	if !ok {
		return nil, errors.New("item not found")
	}

	if item.Checksum == "" {
		return nil, nil
	}

	return v.readItemValueUnsafe(item)
}

func (v *Vault) GetItemForPeer(id uuid.UUID, peer string) (*memguard.LockedBuffer, error) {
	// need RW lock here because verifyPeerUnsafe potentially modifies item metadata
	v.lock.Lock()
	defer v.lock.Unlock()

	vaultItemReadAttempts.Inc()

	if v.IsLocked() {
		return nil, errors.New("vault is locked")
	}

	item, ok := v.items[id]
	if !ok {
		return nil, errors.New("item not found")
	}

	err := v.verifyPeerUnsafe(item, peer)
	if err != nil {
		return nil, err
	}

	if item.Checksum == "" {
		return nil, nil
	}

	return v.readItemValueUnsafe(item)
}

func (v *Vault) SetItemValue(id uuid.UUID, value *memguard.LockedBuffer) error {
	if len(value.Bytes()) == 0 {
		return errors.New("value is empty")
	}
	defer value.Destroy()

	v.lock.Lock()
	defer v.lock.Unlock()

	if v.IsLocked() {
		return errors.New("vault is locked")
	}

	item, ok := v.items[id]
	if !ok {
		return errors.New("item not found")
	}

	return v.writeItemValueUnsafe(item, value)
}

func (v *Vault) WriteItemValue(id uuid.UUID, r io.Reader) error {
	buf, err := memguard.NewBufferFromEntireReader(r)
	if err != nil {
		return err
	}

	return v.SetItemValue(id, buf)
}

func (v *Vault) readItemValueUnsafe(item Item) (*memguard.LockedBuffer, error) {
	ageBytes, err := v.backend().ReadFile(valuePath(item))
	if err != nil {
		return nil, fmt.Errorf("failed to read item value (%s): %v", item.Id, err)
	} else if ageBytes == nil {
		return nil, errors.New("item value file not found: " + item.Id.String())
	}

	value, err := v.decryptFromRestUnsafe(ageBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read item value (%s): %v", item.Id, err)
	}

	decryptSum := sum(value.Bytes())
	if decryptSum != item.Checksum {
		value.Destroy()
		return nil, fmt.Errorf("failed to read item value (%s): checksum mismatch", item.Id)
	}

	vaultItemReads.Inc()

	return value, nil
}

func (v *Vault) verifyPeerUnsafe(item Item, peer string) error {
	metadataHmacSecret, err := v.metadataHmacSecret.Open()
	if err != nil {
		log.Error().Err(err).Msg("failed to access metadata HMAC secret")
		return fmt.Errorf("failed to verify peer (%s): %s", peer, item.Id.String())
	}

	defer metadataHmacSecret.Destroy()

	if item.Peer == nil {
		item.Peer = &peer
		err = writeItemMetadataUnsafe(v.backend(), item, metadataHmacSecret)
		if err != nil {
			return fmt.Errorf("failed to write item metadata (%s): %v", item.Id, err)
		}

		v.items[item.Id] = item

		return nil
	} else if *item.Peer != peer {
		log.Warn().Msgf("invalid peer for item (%s): %s", item.Id.String(), peer)
		return errors.New("client credentials mismatch")
	}

	return nil
}

func (v *Vault) writeItemValueUnsafe(item Item, value *memguard.LockedBuffer) error {
	ageBytes, err := v.encryptForRestUnsafe(value)
	if err != nil {
		return fmt.Errorf("failed to encrypt item value (%s): %v", item.Id, err)
	}

	vPath := valuePath(item)

	if item.Checksum != "" {
		bPath := backupPath(item)
		if err := copyFile(v.backend(), vPath, bPath); err != nil {
			return fmt.Errorf("failed to create backup of previous value (%s): %v", item.Id, err)
		}
	}

	checksum := sum(value.Bytes())
	item.Checksum = checksum
	item.ModifiedAt = time.Now()

	err = v.backend().WriteFile(valuePath(item), ageBytes)
	if err != nil {
		return fmt.Errorf("failed to write item value (%s): %v", item.Id, err)
	}

	metadataHmacSecret, err := v.metadataHmacSecret.Open()
	if err != nil {
		log.Error().Err(err).Msg("failed to access metadata HMAC secret")
		return fmt.Errorf("failed to write item value (%s): %v", item.Id, err)
	}

	defer metadataHmacSecret.Destroy()

	err = writeItemMetadataUnsafe(v.backend(), item, metadataHmacSecret)
	if err != nil {
		return fmt.Errorf("failed to write item metadata (%s): %v", item.Id, err)
	}

	v.items[item.Id] = item

	return nil
}

func (v *Vault) deleteItemUnsafe(id uuid.UUID) bool {
	item, ok := v.items[id]
	if !ok {
		return false
	}

	delete(v.items, id)

	removed := false

	ok, err := v.backend().DeleteFile(metadataPath(item))
	if err != nil {
		log.Debug().
			Err(err).
			Str("item", item.Id.String()).
			Msg("failed to delete item metadata file")
	} else if ok {
		removed = true
	}

	ok, err = v.backend().DeleteFile(valuePath(item))
	if err != nil {
		log.Debug().
			Err(err).
			Str("item", item.Id.String()).
			Msg("failed to delete item value file")
	} else if ok {
		removed = true
	}

	if removed {
		log.Info().Str("item", item.Id.String()).Msg("removed files for item")
	}

	return true
}

func (v *Vault) decryptFromRestUnsafe(data []byte) (*memguard.LockedBuffer, error) {
	identityKey, _ := v.identityKey.Open()
	defer identityKey.Destroy()

	identity, err := readIdentity(v.backend(), identityKey)
	if err != nil {
		log.Fatal().Err(err).Msg("error reading identity")
	}

	reader, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		log.Fatal().Err(err).Msg("error decrypting data")
	}

	out := bytes.NewBuffer(make([]byte, 0, len(data)))
	defer wipeBuffer(out, out.Len())

	if _, err := io.Copy(out, reader); err != nil {
		log.Fatal().Err(err).Msg("error decrypting data")
	}

	result := make([]byte, out.Len())
	copy(result, out.Bytes())

	return memguard.NewBufferFromBytes(result), nil
}

func (v *Vault) encryptForRestUnsafe(data *memguard.LockedBuffer) ([]byte, error) {
	var recipients []age.Recipient
	recipients = append(recipients, v.primaryRecipient)

	secret, err := v.metadataHmacSecret.Open()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to access metadata HMAC secret")
	}

	defer secret.Destroy()

	recoveryRecipient, err := loadRecoveryRecipient(v.backend(), secret)
	if err != nil {
		log.Fatal().Err(err).Msg("error loading recovery recipient")
	}

	if recoveryRecipient != nil {
		recipients = append(recipients, recoveryRecipient)
	}

	out := &bytes.Buffer{}
	wc, err := age.Encrypt(out, recipients...)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to encrypt data")
	}

	_, err = io.Copy(wc, bytes.NewReader(data.Bytes()))
	if err != nil {
		log.Fatal().Err(err).Msg("error writing data")
	}

	err = wc.Close()
	if err != nil {
		log.Fatal().Err(err).Msg("error closing writer")
	}

	return out.Bytes(), nil
}
