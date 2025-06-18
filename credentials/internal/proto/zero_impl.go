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

package proto

import (
	"unsafe"

	"github.com/awnumar/memguard"
)

func (c *AdminCredentials) PbZero() {
	memguard.WipeBytes(*(*[]byte)(unsafe.Pointer(&c.Passphrase)))
	memguard.WipeBytes(c.unknownFields)
}

func (r *RecoveryRecipient) PbZero() {
	r.Credentials.PbZero()
	memguard.WipeBytes(r.unknownFields)
}

func (i *ItemCreation) PbZero() {
	i.Credentials.PbZero()
	memguard.WipeBytes(i.unknownFields)
}

func (i *ItemSearch) PbZero() {
	i.Credentials.PbZero()
	memguard.WipeBytes(i.unknownFields)
}

func (i *ItemDeletion) PbZero() {
	i.Credentials.PbZero()
	memguard.WipeBytes(i.unknownFields)
}

func (i *ItemRequest) PbZero() {
	if i.GetAdmin() != nil {
		i.GetAdmin().PbZero()
	}

	if i.GetClient() != nil {
		i.GetClient().PbZero()
	}

	memguard.WipeBytes(i.unknownFields)
}

func (c *ClientCreation) PbZero() {
	c.Credentials.PbZero()
	memguard.WipeBytes(c.unknownFields)
}

func (c *ClientCredentials) PbZero() {
	memguard.WipeBytes(*(*[]byte)(unsafe.Pointer(&c.Id)))
	memguard.WipeBytes(*(*[]byte)(unsafe.Pointer(&c.Secret)))
	memguard.WipeBytes(c.unknownFields)
}
