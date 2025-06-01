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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

func Run(ctx context.Context, command []string, env map[string]string, input io.Reader, result any) (returnCode ReturnCode, logMessages []LogMessage, err error) {
	logTag := rand.Text()

	finalCommand := []string{"--log-json"}
	finalCommand = append(finalCommand, command...)

	log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("command: borg %v", finalCommand)

	var cmd *exec.Cmd
	if ctx != nil {
		cmd = exec.CommandContext(ctx, "borg", finalCommand...)
	} else {
		cmd = exec.Command("borg", finalCommand...)
	}

	if input != nil {
		log.Debug().Str("tag", logTag).Msg("providing data to stdin")
		cmd.Stdin = input
	}

	finalEnv := cmd.Env
	for i, keyVal := range finalEnv {
		split := strings.SplitN(keyVal, "=", 2)
		key := split[0]

		newValue, found := env[key]
		if !found {
			continue
		}

		if strings.Contains(strings.ToLower(key), "_pass") {
			log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("env: %s = ******", key)
		} else {
			log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("env: %s = %s", key, newValue)
		}

		finalEnv[i] = key + "=" + newValue
		delete(env, key)
	}

	for k, v := range env {
		if strings.Contains(strings.ToLower(k), "_pass") {
			log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("env: %s = ******", k)
		} else {
			log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("env: %s = %s", k, v)
		}

		finalEnv = append(finalEnv, k+"="+v)
	}

	cmd.Env = finalEnv
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	var stdout []byte

	if result != nil {
		stdout, err = cmd.Output()
	} else {
		log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("ignoring stdout")
		err = cmd.Run()
	}

	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		log.Debug().Ctx(ctx).Str("tag", logTag).Msg("context canceled")
		return -1, nil, ctx.Err()
	}

	log.Debug().Ctx(ctx).Str("tag", logTag).Err(err).Msgf("command exited with code %d", cmd.ProcessState.ExitCode())

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
		log.Debug().Ctx(ctx).Str("tag", logTag).Msgf("reading stdout as %T", result)
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
	err := NewError(returnCode)
	switch returnCode {
	case ReturnCodeSuccess:
		return nil
	case ReturnCodeWarning:
		HandleBorgLogMessages(logMessages)
		return nil
	case ReturnCodeError:
		HandleBorgLogMessages(logMessages)
		return errors.Wrap(err, "borg command failed, check log")
	case ReturnCodeRepositoryDoesNotExist:
		return errors.Wrap(err, "configured repository does not exist")
	case ReturnCodeRepositoryIsInvalid:
		return errors.Wrap(err, "configured location doesn't point to a valid repository")
	case ReturnCodePasscommandFailure:
		HandleBorgLogMessages(logMessages)
		return errors.Wrap(err, "borg passcommand failed, check log")
	case ReturnCodePassphraseWrong:
		return errors.Wrap(err, "configured passphrase is wrong")
	case ReturnCodeConnectionClosed:
		HandleBorgLogMessages(logMessages)
		return errors.Wrap(err, "borg connection closed, check log")
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
			return errors.Wrap(err, *lm.Msg())
		} else {
			HandleBorgLogMessages(logMessages)
			return errors.Wrap(err, "borg connection closed, check log")
		}
	}

	HandleBorgLogMessages(logMessages)
	return fmt.Errorf("unknown returncode: %d", returnCode)
}
