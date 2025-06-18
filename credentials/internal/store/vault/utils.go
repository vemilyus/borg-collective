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
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"filippo.io/age"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"time"
	"unsafe"

	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

func createRecoveryHash(recipient age.X25519Recipient, secret *memguard.LockedBuffer) string {
	bytesToHash := []byte(recipient.String())
	bytesToHash = append(bytesToHash, secret.Bytes()...)

	defer memguard.WipeBytes(bytesToHash)

	return sum(bytesToHash)
}

func loadRecoveryRecipient(backend Backend, secret *memguard.LockedBuffer) (*age.X25519Recipient, error) {
	recBytes, err := backend.ReadFile(".recovery")
	if err != nil {
		return nil, err
	} else if recBytes == nil {
		return nil, nil
	}

	recSumBytes, err := backend.ReadFile(".recovery.sum")
	if err != nil {
		return nil, err
	} else if recSumBytes == nil {
		return nil, fmt.Errorf(".recovery.sum is missing")
	}

	recipient, err := age.ParseX25519Recipient(string(recBytes))
	if err != nil {
		return nil, err
	}

	checkSum := createRecoveryHash(*recipient, secret)
	if string(recSumBytes) != checkSum {
		return nil, fmt.Errorf(".recovery.sum does not match")
	}

	return recipient, nil
}

func writeRecoveryRecipient(backend Backend, recipient age.X25519Recipient, secret *memguard.LockedBuffer) error {
	err := backend.WriteFile(".recovery", []byte(recipient.String()))
	if err != nil {
		return err
	}

	recoveryHash := createRecoveryHash(recipient, secret)
	err = backend.WriteFile(".recovery.sum", []byte(recoveryHash))
	if err != nil {
		_, _ = backend.DeleteFile(".recovery")
		return err
	}

	return nil
}

func readIdentity(backend Backend, identityKey *memguard.LockedBuffer) (*age.X25519Identity, error) {
	cryptBytes, err := backend.ReadFile(".identity")
	if err != nil {
		return nil, err
	} else if cryptBytes == nil {
		panic(errors.New("identity file not found"))
	}

	c, err := aes.NewCipher(identityKey.Bytes())
	identityKey.Destroy()

	if err != nil {
		panic(err.Error())
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		panic(err.Error())
	}

	nonce := cryptBytes[:12]
	cryptBytes = cryptBytes[12:]

	rawIdentity, err := gcm.Open(nil, nonce, cryptBytes, nil)
	defer memguard.WipeBytes(rawIdentity)

	if err != nil {
		return nil, err
	}

	return age.ParseX25519Identity(*(*string)(unsafe.Pointer(&rawIdentity)))
}

func deriveMetadataHmacSecret(identity age.X25519Identity) *memguard.Enclave {
	identityString := identity.String()
	identityBytes := []byte(identityString)
	memguard.WipeBytes(*(*[]byte)(unsafe.Pointer(&identityString)))
	rawHmacSecret := sha256.Sum256(identityBytes)

	return memguard.NewEnclave(rawHmacSecret[:])
}

func writeIdentity(backend Backend, identityKey *memguard.LockedBuffer, identity *age.X25519Identity) error {
	identityString := identity.String()
	identityBytes := *(*[]byte)(unsafe.Pointer(&identityString))
	defer memguard.WipeBytes(identityBytes)

	c, err := aes.NewCipher(identityKey.Bytes())
	if err != nil {
		return err
	}

	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		panic(err.Error())
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		panic(err.Error())
	}

	cryptBytes := gcm.Seal(nil, nonce, identityBytes, nil)

	result := make([]byte, 0)
	result = append(result, nonce...)
	result = append(result, cryptBytes...)

	return backend.WriteFile(".identity", result)
}

func readAllMetadataUnsafe(backend Backend, hmacSecret *memguard.LockedBuffer) (map[uuid.UUID]Item, error) {
	listing, err := backend.ListFiles("")
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}

	items := make(map[uuid.UUID]Item)

	for _, entry := range listing {
		if filepath.Ext(entry) == ".json" {
			metadata, err := readItemMetadataUnsafe(backend, entry, hmacSecret)
			if err != nil {
				log.Warn().Err(err).Str("source", entry).Msg("error reading item metadata")
				continue
			}

			items[metadata.Id] = *metadata
		}
	}

	return items, nil
}

func readItemMetadataUnsafe(backend Backend, path string, hmacSecret *memguard.LockedBuffer) (*Item, error) {
	metadataBytes, err := backend.ReadFile(path)
	if err != nil {
		return nil, err
	} else if metadataBytes == nil {
		return nil, errors.New("metadata file not found: " + path)
	}

	h := hmac.New(sha256.New, hmacSecret.Bytes())
	h.Write(metadataBytes[:len(metadataBytes)-32])
	checkHmac := h.Sum(nil)
	if !bytes.Equal(checkHmac, metadataBytes[len(metadataBytes)-32:]) {
		return nil, errors.New("invalid metadata: checksum mismatch")
	}

	var metadata Item
	err = json.Unmarshal(metadataBytes[:len(metadataBytes)-32], &metadata)
	if err != nil {
		return nil, err
	}

	if path != metadataPath(metadata) {
		return nil, errors.New("metadata path doesn't match item id: " + metadata.Id.String())
	}

	return &metadata, nil
}

func writeItemMetadataUnsafe(backend Backend, item Item, hmacSecret *memguard.LockedBuffer) error {
	metadataBytes, err := json.Marshal(item)
	if err != nil {
		return err
	}

	h := hmac.New(sha256.New, hmacSecret.Bytes())
	h.Write(metadataBytes)

	result := make([]byte, 32+len(metadataBytes))
	copy(result, metadataBytes)
	copy(result[len(metadataBytes):], h.Sum(nil))

	return backend.WriteFile(metadataPath(item), result)
}

func copyFile(backend Backend, src, dest string) error {
	srcBytes, err := backend.ReadFile(src)
	if err != nil {
		return err
	} else if srcBytes == nil {
		return fmt.Errorf("file does not exist: %s", src)
	}

	return backend.WriteFile(dest, srcBytes)
}

func sum(data []byte) string {
	raw := sha256.Sum256(data)
	return hex.EncodeToString(raw[:])
}

func wipeBuffer(buf *bytes.Buffer, length int) {
	// NOTE: Yes it may miss some data if the buffer was forced to allocate a bigger byte slice,
	//       but any left-over secret value in memory will only be a partial value, so it's
	//       not as bad as it seems.

	buf.Truncate(0)
	for range length {
		buf.WriteByte(0)
	}

	runtime.KeepAlive(buf)
}

func backupPath(item Item) string {
	return filepath.Join(".bak", fmt.Sprintf("%s.%d.json", item.Id.String(), time.Now().UnixMilli()))
}

func metadataPath(item Item) string {
	return item.Id.String() + ".json"
}

func valuePath(item Item) string {
	return item.Id.String() + ".age"
}
