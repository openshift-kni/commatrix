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
	return logger
}
