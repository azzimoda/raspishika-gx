package logger

import (
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

func Init(logLevelStr, logDir string) {
	logLevel, err := zerolog.ParseLevel(logLevelStr)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to parse configured log level; fallback to DEBUG")
		logLevel = zerolog.DebugLevel
	}

	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}

	if logDir == "" {
		setupConsoleLogger(consoleWriter, logLevel)
	} else {
		logFile := filepath.Join(logDir, "log.json")

		if _, err := os.Stat(logDir); err == nil {
			// Directory already exists
			setupConsoleFileLogger(consoleWriter, logFile, logLevel)
		} else if err = os.MkdirAll(logDir, 0755); err == nil { // Try to create log directory
			setupConsoleFileLogger(consoleWriter, logFile, logLevel)
		} else {
			log.Error().Msg("Failed to create log directory; logging to file disabled")
			setupConsoleLogger(consoleWriter, logLevel)
		}
	}

	log.Debug().Msgf("Log level: %s", logLevel)
	zerolog.SetGlobalLevel(logLevel)
}

func setupConsoleLogger(consoleWriter zerolog.ConsoleWriter, logLevel zerolog.Level) {
	loggerContext := zerolog.New(consoleWriter).With().Timestamp()
	if logLevel <= zerolog.TraceLevel {
		loggerContext = loggerContext.Caller()
	}
	log.Logger = loggerContext.Logger()
}

func setupConsoleFileLogger(consoleWriter zerolog.ConsoleWriter, logFileName string, logLevel zerolog.Level) {
	loggerContext := zerolog.New(zerolog.MultiLevelWriter(
		consoleWriter,
		&lumberjack.Logger{Filename: logFileName, MaxSize: 16, MaxBackups: 16, MaxAge: 30},
	)).With().Timestamp()
	if logLevel <= zerolog.TraceLevel {
		loggerContext = loggerContext.Caller()
	}
	log.Logger = loggerContext.Logger()
}
