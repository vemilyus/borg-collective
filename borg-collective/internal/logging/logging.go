package logging

import (
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	golog "log"
	"os"
	"strings"
	"time"
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
	logWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339, NoColor: true}
	logWriter.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %5s |", i))
	}

	initLogging(logWriter)
}

func initLogging(logWriter zerolog.ConsoleWriter) {
	log.Logger = log.Output(logWriter)

	golog.SetFlags(0)
	golog.SetOutput(&zerologLogger{logger: log.Logger})
}
