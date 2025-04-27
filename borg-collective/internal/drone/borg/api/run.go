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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/rs/zerolog/log"
	"io"
	"os/exec"
	"strings"
)

func Run(ctx context.Context, command []string, env map[string]string, input io.Reader, result any) (returnCode ReturnCode, logMessages []LogMessage, err error) {
	finalCommand := []string{"--log-json"}
	finalCommand = append(finalCommand, command...)

	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, "borg", finalCommand...)
	} else {
		cmd = exec.Command("borg", finalCommand...)
	}

	if input != nil {
		cmd.Stdin = input
	}

	finalEnv := cmd.Env
	for i, keyVal := range finalEnv {
		split := strings.SplitN(keyVal, "=", 2)
		key := split[0]

		newValue, ok := env[key]
		if !ok {
			continue
		}

		finalEnv[i] = key + "=" + newValue
		delete(env, key)
	}

	for k, v := range env {
		finalEnv = append(finalEnv, k+"="+v)
	}

	cmd.Env = finalEnv
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	var stdout []byte

	if result != nil {
		stdout, err = cmd.Output()
	} else {
		err = cmd.Run()
	}

	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		return -1, nil, ctx.Err()
	}

	if err != nil {
		var exiterr *exec.ExitError
		if errors.As(err, &exiterr) {
			ll, e := parseLogLines(stderr.Bytes())
			if e != nil {
				return -1, nil, e
			}

			return (ReturnCode)(exiterr.ExitCode()), ll, nil
		} else {
			return -1, nil, err
		}
	}

	if result != nil {
		err = json.Unmarshal(stdout, result)
		if err != nil {
			return -1, nil, err
		}
	}

	return 0, nil, nil
}

var (
	searchArchiveProgress = []byte("type\": \"" + LogMessageTypeArchiveProgress)
	searchLogMessage      = []byte("type\": \"" + LogMessageTypeLogMessage)
	searchFileStatus      = []byte("type\": \"" + LogMessageTypeFileStatus)
	searchProgressMessage = []byte("type\": \"" + LogMessageTypeProgressMessage)
	searchProgressPercent = []byte("type\": \"" + LogMessageTypeProgressPercent)
)

func parseLogLines(stderr []byte) ([]LogMessage, error) {
	var result []LogMessage
	for {
		if len(stderr) == 0 {
			break
		}

		newLinesI := bytes.IndexByte(stderr, '\n')
		var line []byte
		if newLinesI == -1 {
			line = stderr
		} else {
			line = stderr[:newLinesI]
			stderr = stderr[newLinesI+1:]
		}

		if len(line) == 0 {
			continue
		}

		var parsedLine LogMessage
		if bytes.Index(line, searchArchiveProgress) > -1 {
			parsedLine = LogMessageArchiveProgress{}
		} else if bytes.Index(line, searchLogMessage) > -1 {
			parsedLine = LogMessageLogMessage{}
		} else if bytes.Index(line, searchFileStatus) > -1 {
			parsedLine = LogMessageFileStatus{}
		} else if bytes.Index(line, searchProgressMessage) > -1 {
			parsedLine = LogMessageProgressMessage{}
		} else if bytes.Index(line, searchProgressPercent) > -1 {
			parsedLine = LogMessageProgressPercent{}
		} else {
			log.Debug().Str("line", string(line)).Msg("Unknown log message type")
			continue
		}

		err := json.Unmarshal(line, &parsedLine)
		if err != nil {
			log.Debug().Err(err).Str("line", string(line)).Msg("Failed to unmarshal log message line")
			continue
		}

		result = append(result, parsedLine)
	}

	return result, nil
}

func HandleBorgLogMessages(logMessages []LogMessage) {
	for _, logMessage := range logMessages {
		msg := logMessage.Msg()
		if msg != nil && *msg != "" {
			log.WithLevel(logMessage.Level()).Msgf("[BORG] %s", *msg)
		}
	}
}

func HandleBorgReturnCode(returnCode ReturnCode, logMessages []LogMessage) error {
	switch returnCode {
	case ReturnCodeSuccess:
		return nil
	case ReturnCodeWarning:
		HandleBorgLogMessages(logMessages)
		return nil
	case ReturnCodeError:
		HandleBorgLogMessages(logMessages)
		return errors.New("borg command failed, check log")
	case ReturnCodeRepositoryDoesNotExist:
		return errors.New("configured repository does not exist")
	case ReturnCodeRepositoryIsInvalid:
		return errors.New("configured location doesn't point to a valid repository")
	case ReturnCodePasscommandFailure:
		HandleBorgLogMessages(logMessages)
		return errors.New("borg passcommand failed, check log")
	case ReturnCodePassphraseWrong:
		return errors.New("configured passphrase is wrong")
	case ReturnCodeConnectionClosed:
		HandleBorgLogMessages(logMessages)
		return errors.New("borg connection closed, check log")
	case ReturnCodeConnectionClosedWithHint:
		var lm *LogMessageLogMessage
		for _, logMessage := range logMessages {
			if lmlm, ok := logMessage.(*LogMessageLogMessage); ok {
				if lmlm.Msgid != nil && *lmlm.Msgid == "ConnectionClosedWithHint" {
					lm = lmlm
				}
			}
		}

		if lm != nil {
			return errors.New(*lm.Msg())
		} else {
			HandleBorgLogMessages(logMessages)
			return errors.New("borg connection closed, check log")
		}
	}

	HandleBorgLogMessages(logMessages)
	return errors.New("unknown returncode")
}
