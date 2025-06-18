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
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/awnumar/memguard"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewVault(t *testing.T) {
	// Define a temporary storage path for testing
	tempDir := t.TempDir()
	// Test successful creation of a new vault
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(tempDir),
	})
	assert.NoError(t, err)
	assert.NotNil(t, vault)
	assert.True(t, vault.IsLocked()) // Should be locked initially

	// Check if the necessary directories were created
	identityPath := filepath.Join(tempDir, ".identity")

	_, err = os.Stat(identityPath)
	assert.True(t, os.IsNotExist(err), "Identity file should not exist yet")
}

func TestNewVault_ErrorCreatingStoragePath(t *testing.T) {
	// Attempt to create a vault with an invalid storage path
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend("/invalid/path/to/storage"), // Invalid path
	})
	assert.Error(t, err)
	assert.Nil(t, vault)
}

func TestUnlock_Local(t *testing.T) {
	// Create a new vault
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testUnlock(t, vault)
}

func TestUnlock_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testUnlock(t, vault)
}

//goland:noinspection GoRedundantConversion
func testUnlock(t *testing.T, vault *Vault) {
	// Test unlocking with the correct passphrase
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)
	assert.False(t, vault.IsLocked())        // Vault should be unlocked
	assert.NotNil(t, vault.primaryRecipient) // Primary recipient should be set

	// Lock the vault again
	err = vault.Lock()
	assert.NoError(t, err)
	assert.True(t, vault.IsLocked()) // Vault should be locked

	// Test repeated unlocking of the locked vault
	err = vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)                   // Should succeed
	assert.False(t, vault.IsLocked())        // Vault should be unlocked again
	assert.NotNil(t, vault.primaryRecipient) // Primary recipient should be set

	// Test repeated calls to unlocked vault
	err = vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)                   // Should succeed
	assert.False(t, vault.IsLocked())        // Vault should still be unlocked
	assert.NotNil(t, vault.primaryRecipient) // Primary recipient should be set

	// Lock the vault again
	err = vault.Lock()
	assert.NoError(t, err)
	assert.True(t, vault.IsLocked()) // Vault should be locked

	// Test unlocking with a wrong passphrase
	invalidPassphrase := string([]byte("wrong_passphrase"))
	err = vault.Unlock(invalidPassphrase)
	assert.Error(t, err)                  // Should return an error for invalid passphrase
	assert.True(t, vault.IsLocked())      // Vault should still be locked
	assert.Nil(t, vault.primaryRecipient) // Primary recipient should be nil
}

func TestVerifyPassphrase_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testVerifyPassphrase(t, vault)
}

func TestVerifyPassphrase_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testVerifyPassphrase(t, vault)
}

func testVerifyPassphrase(t *testing.T, vault *Vault) {
	// Define a passphrase
	// Unlock the vault with the correct passphrase
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Test verifying the passphrase with the correct passphrase
	//goland:noinspection GoRedundantConversion
	err = vault.VerifyPassphrase(string([]byte("correct_passphrase")))
	assert.NoError(t, err) // Should succeed

	// Test verifying the passphrase with an incorrect passphrase
	//goland:noinspection GoRedundantConversion
	err = vault.VerifyPassphrase(string([]byte("wrong_passphrase")))
	assert.Error(t, err) // Should return an error for invalid passphrase

	// Lock the vault
	err = vault.Lock()
	assert.NoError(t, err)

	// Test verifying the passphrase when the vault is locked
	//goland:noinspection GoRedundantConversion
	err = vault.VerifyPassphrase(string([]byte("correct_passphrase")))
	assert.Error(t, err) // Should return an error since the vault is locked
}

func TestVerifyPassphrase_EmptyPassphrase(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	// Define a passphrase
	//goland:noinspection GoRedundantConversion
	passphrase := string([]byte("correct_passphrase"))

	// Unlock the vault with the correct passphrase
	err = vault.Unlock(passphrase)
	assert.NoError(t, err)

	// Test verifying with an empty passphrase
	err = vault.VerifyPassphrase("")
	assert.Error(t, err) // Should return an error for empty passphrase
}

func TestSetRecoveryRecipient_Local(t *testing.T) {
	// Create a new vault
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testSetRecoveryRecipient(t, vault)
}

func TestSetRecoveryRecipient_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testSetRecoveryRecipient(t, vault)
}

func testSetRecoveryRecipient(t *testing.T, vault *Vault) {
	// Define a valid passphrase and unlock the vault
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Create a new recovery recipient
	recoveryIdentity, err := age.GenerateX25519Identity()
	assert.NoError(t, err)

	// Test setting the recovery recipient
	err = vault.SetRecoveryRecipient(*recoveryIdentity.Recipient())
	assert.NoError(t, err)

	secret, err := vault.metadataHmacSecret.Open()
	assert.NoError(t, err)

	// Verify that the recovery recipient is correctly set
	storedRecipient, err := loadRecoveryRecipient(vault.backend(), secret)
	assert.NoError(t, err)
	assert.Equal(t, recoveryIdentity.Recipient(), storedRecipient)

	// Lock the vault
	err = vault.Lock()
	assert.NoError(t, err)

	// Attempt to set a recovery recipient when the vault is locked
	err = vault.SetRecoveryRecipient(*recoveryIdentity.Recipient())
	assert.Error(t, err) // Should return an error since the vault is locked

	// Verify that the recovery recipient is still the same
	storedRecipient, err = loadRecoveryRecipient(vault.backend(), secret)
	assert.NoError(t, err)
	assert.Equal(t, recoveryIdentity.Recipient(), storedRecipient)
}

func TestCreateItem_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testCreateItem(t, vault)
}

func TestCreateItem_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testCreateItem(t, vault)
}

func testCreateItem(t *testing.T, vault *Vault) {

	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Test creating an item
	description := "Test Item"
	item, err := vault.CreateItem(description)
	assert.NoError(t, err)
	assert.NotNil(t, item)
	assert.Equal(t, description, item.Description)
	assert.True(t, item.ModifiedAt.After(time.Time{})) // ModifiedAt should be set
	assert.Equal(t, 1, len(vault.items))               // There should be one item in the vault
}

func TestDeleteItem_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testDeleteItem(t, vault)
}

func TestDeleteItem_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testDeleteItem(t, vault)
}

func testDeleteItem(t *testing.T, vault *Vault) {
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Create an item to delete
	description := "Item to Delete"
	item, err := vault.CreateItem(description)
	assert.NoError(t, err)

	// Test deleting the item
	err = vault.DeleteItem(item.Id)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(vault.items)) // There should be no items in the vault

	// Attempt to delete a non-existent item
	err = vault.DeleteItem(item.Id)
	assert.NoError(t, err) // Should not return an error
}

func TestGetItem_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testGetItem(t, vault)
}

func TestGetItem_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testGetItem(t, vault)
}

func testGetItem(t *testing.T, vault *Vault) {
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Create an item
	description := "Test Item"
	item, err := vault.CreateItem(description)
	assert.NoError(t, err)

	// Test getting the item
	retrievedItem, err := vault.GetItem(item.Id)
	assert.NoError(t, err)
	assert.Nil(t, retrievedItem) // Item has no content

	// Test getting a non-existent item
	nonExistentID := uuid.New()
	retrievedItem, err = vault.GetItem(nonExistentID)
	assert.Error(t, err)         // Should return an error
	assert.Nil(t, retrievedItem) // Should be nil
}

func TestSetItemValue_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testSetItemValue(t, vault)
}

func TestSetItemValue_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testSetItemValue(t, vault)
}

func testSetItemValue(t *testing.T, vault *Vault) {
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Create an item
	description := "Test Item"
	item, err := vault.CreateItem(description)
	assert.NoError(t, err)

	// Create a value to set
	value := memguard.NewBufferFromBytes([]byte("test value"))

	// Set the item value
	err = vault.SetItemValue(item.Id, value)
	assert.NoError(t, err)

	// Retrieve the item to verify the value was set correctly
	retrievedValue, err := vault.GetItem(item.Id)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedValue)                              // Item should exist
	assert.Equal(t, "test value", string(retrievedValue.Bytes())) // Check the value

	// Verify that the value is stored in encrypted form on disk
	encryptedData, err := vault.backend().ReadFile(valuePath(*item))
	assert.NoError(t, err)
	assert.NotEmpty(t, encryptedData) // Ensure that the file is not empty

	// Check that the encrypted data is not equal to the plain text value
	assert.NotEqual(t, "test value", string(encryptedData)) // Ensure the stored data is not plain text
}

func TestWriteItemValue_Local(t *testing.T) {
	// Create a new vault and unlock it
	vault, err := NewVault(&Options{
		Backend: NewLocalStorageBackend(t.TempDir()),
	})
	assert.NoError(t, err)

	testWriteItemValue(t, vault)
}

func TestWriteItemValue_InMemory(t *testing.T) {
	vault, err := NewVault(&Options{Backend: &inMemoryBackend{}})
	assert.NoError(t, err)

	testWriteItemValue(t, vault)
}

func testWriteItemValue(t *testing.T, vault *Vault) {
	//goland:noinspection GoRedundantConversion
	err := vault.Unlock(string([]byte("correct_passphrase")))
	assert.NoError(t, err)

	// Create an item
	description := "Test Item"
	item, err := vault.CreateItem(description)
	assert.NoError(t, err)

	// Write the item value
	err = vault.WriteItemValue(item.Id, bytes.NewReader([]byte("test value")))
	assert.NoError(t, err)

	// Retrieve the item to verify the value was written correctly
	retrievedValue, err := vault.GetItem(item.Id)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedValue)                              // Item should exist
	assert.Equal(t, "test value", string(retrievedValue.Bytes())) // Check the value

	// Verify that the value is stored in encrypted form on disk
	encryptedData, err := vault.backend().ReadFile(valuePath(*item))
	assert.NoError(t, err)
	assert.NotEmpty(t, encryptedData) // Ensure that the file is not empty

	// Check that the encrypted data is not equal to the plain text value
	assert.NotEqual(t, "test value", string(encryptedData)) // Ensure the stored data is not plain text
}
