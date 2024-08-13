package utils

import (
	"sync"

	"github.com/sirupsen/logrus"
)

var (
	logger *logrus.Logger
	once   sync.Once
)

// Initialize sets up the logger with the appropriate level.
func InitializeLog(isDebug bool) {
	once.Do(func() {
		logger = logrus.New()
		if isDebug {
			logger.SetLevel(logrus.DebugLevel)
		} else {
			logger.SetLevel(logrus.InfoLevel)
		}
	})
}

// Logger returns the configured Logrus logger instance.
func Logger() *logrus.Logger {
	if logger == nil {
		logger = logrus.New()
		logger.SetLevel(logrus.InfoLevel)
		logger.Warn("Logger is not initialized properly, using default logger.")
	}
	return logger
}
