package logging

import (
	"os"

	"github.com/sirupsen/logrus"
)

// Configure creates a JSON logger and ensures every log entry contains service_name.
func Configure(serviceName string) *logrus.Logger {
	logger := logrus.New()
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.SetOutput(os.Stdout)
	logger.SetLevel(logrus.InfoLevel)
	logger.AddHook(&serviceHook{serviceName: serviceName})
	return logger
}

type serviceHook struct {
	serviceName string
}

func (h *serviceHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *serviceHook) Fire(entry *logrus.Entry) error {
	if _, ok := entry.Data["service_name"]; !ok {
		entry.Data["service_name"] = h.serviceName
	}
	return nil
}
