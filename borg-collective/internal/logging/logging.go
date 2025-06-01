package logging

import (
	"fmt"
	golog "log"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type zerologLogger struct {
	logger zerolog.Logger
}

func (z *zerologLogger) Write(p []byte) (n int, err error) {
	output := strings.TrimSpace(string(p))

	z.logger.Debug().Msg(output)
	return len(p), nil
}

func InitLogging() {
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339, NoColor: true}
	consoleWriter.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %5s |", i))
	}

	initLogging(consoleWriter)
}

func initLogging(logWriter zerolog.ConsoleWriter) {
	log.Logger = log.Output(logWriter)

	golog.SetFlags(0)
	golog.SetOutput(&zerologLogger{logger: log.Logger})
}
