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

package api

import (
	"github.com/rs/zerolog"
)

type ReturnCode int

const (
	ReturnCodeSuccess                  ReturnCode = 0
	ReturnCodeWarning                  ReturnCode = 1
	ReturnCodeError                    ReturnCode = 2
	ReturnCodeRepositoryDoesNotExist   ReturnCode = 13
	ReturnCodeRepositoryIsInvalid      ReturnCode = 15
	ReturnCodePasscommandFailure       ReturnCode = 51
	ReturnCodePassphraseWrong          ReturnCode = 52
	ReturnCodeConnectionClosed         ReturnCode = 80
	ReturnCodeConnectionClosedWithHint ReturnCode = 81
)

type LogMessage interface {
	Level() zerolog.Level
	Msg() *string
}

type LogMessageType string

const (
	LogMessageTypeArchiveProgress LogMessageType = "archive_progress"
	LogMessageTypeProgressMessage LogMessageType = "progress_message"
	LogMessageTypeProgressPercent LogMessageType = "progress_percent"
	LogMessageTypeFileStatus      LogMessageType = "file_status"
	LogMessageTypeLogMessage      LogMessageType = "log_message"
)

type LogMessageArchiveProgress struct {
	OriginalSize     *int64  `json:"original_size"`
	CompressedSize   *int64  `json:"compressed_size"`
	DeduplicatedSize *int64  `json:"deduplicated_size"`
	Nfiles           *int64  `json:"nfiles"`
	Path             *string `json:"path"`
	Time             float64 `json:"time"`
	Finished         bool    `json:"finished"`
}

func (m LogMessageArchiveProgress) Level() zerolog.Level {
	return zerolog.TraceLevel
}

func (m LogMessageArchiveProgress) Msg() *string {
	return nil
}

type LogMessageProgressMessage struct {
	Operation int64   `json:"operation"`
	Msgid     *string `json:"msgid"`
	Finished  bool    `json:"finished"`
	Message   *string `json:"message"`
	Time      float64 `json:"time"`
}

func (m LogMessageProgressMessage) Level() zerolog.Level {
	return zerolog.TraceLevel
}

func (m LogMessageProgressMessage) Msg() *string {
	return m.Message
}

type LogMessageProgressPercent struct {
	Operation int64   `json:"operation"`
	Msgid     *string `json:"msgid"`
	Finished  bool    `json:"finished"`
	Message   *string `json:"message"`
	Current   *int64  `json:"current"`
	Info      *string `json:"info"`
	Total     *int64  `json:"total"`
	Time      float64 `json:"time"`
}

func (m LogMessageProgressPercent) Level() zerolog.Level {
	return zerolog.TraceLevel
}

func (m LogMessageProgressPercent) Msg() *string {
	return m.Message
}

type LogMessageFileStatus struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

func (m LogMessageFileStatus) Level() zerolog.Level {
	return zerolog.TraceLevel
}

func (m LogMessageFileStatus) Msg() *string {
	return nil
}

type LogMessageLogMessage struct {
	Time      float64 `json:"time"`
	Levelname string  `json:"levelname"`
	Name      string  `json:"name"`
	Message   string  `json:"message"`
	Msgid     *string `json:"msgid"`
}

func (m LogMessageLogMessage) Level() zerolog.Level {
	switch m.Levelname {
	case "DEBUG":
		return zerolog.TraceLevel
	case "INFO":
		return zerolog.DebugLevel
	case "WARNING":
		return zerolog.InfoLevel
	case "ERROR":
		return zerolog.WarnLevel
	case "CRITICAL":
		return zerolog.ErrorLevel
	}

	return zerolog.TraceLevel
}

func (m LogMessageLogMessage) Msg() *string {
	return &m.Message
}

type BaseInfoOutput struct {
	Cache      *CacheInfo      `json:"cache"`
	Encryption *EncryptionInfo `json:"encryption"`
	Repository RepositoryInfo  `json:"repository"`
}

type CreateOutput struct {
	BaseInfoOutput
	Archive ArchiveInfo `json:"archive"`
}

type InfoListOutput struct {
	BaseInfoOutput
	Archives []ArchiveInfo `json:"archives"`
}

type CacheInfo struct {
	Path  string         `json:"path"`
	Stats CacheInfoStats `json:"stats"`
}

type CacheInfoStats struct {
	TotalChunks       int64 `json:"total_chunks"`
	TotalCsize        int64 `json:"total_csize"`
	TotalSize         int64 `json:"total_size"`
	TotalUniqueChunks int64 `json:"total_unique_chunks"`
	UniqueCsize       int64 `json:"unique_csize"`
	UniqueSize        int64 `json:"unique_size"`
}

type EncryptionInfo struct {
	Mode    string  `json:"mode"`
	Keyfile *string `json:"keyfile"`
}

type RepositoryInfo struct {
	Id           string `json:"id"`
	LastModified string `json:"last_modified"`
	Location     string `json:"location"`
}

type ArchiveInfo struct {
	Name          string         `json:"name"`
	Id            string         `json:"id"`
	Start         string         `json:"start"`
	End           *string        `json:"end"`
	Duration      *float64       `json:"duration"`
	Stats         *ArchiveStats  `json:"stats"`
	Limits        *ArchiveLimits `json:"limits"`
	CommandLine   []string       `json:"command_line"`
	ChunkerParams []string       `json:"chunker_params"`
	Hostname      *string        `json:"hostname"`
	Username      *string        `json:"username"`
	Comment       *string        `json:"comment"`
}

type ArchiveStats struct {
	OriginalSize     int64 `json:"original_size"`
	CompressedSize   int64 `json:"compressed_size"`
	DeduplicatedSize int64 `json:"deduplicated_size"`
	Nfiles           int64 `json:"nfiles"`
}

type ArchiveLimits struct {
	MaxArchiveSize float64 `json:"max_archive_size"`
}
