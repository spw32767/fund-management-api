package config

import (
	"io"
	"log"
	"os"
	"path/filepath"
)

// LogWriter is the writer used for application and database logs.
var LogWriter io.Writer = os.Stdout

// LogFilePath returns the path to the backend log file.
func LogFilePath() string {
	return filepath.Join("logs", "fund-api.log")
}

// InitLogging prepares the log file and configures the standard logger output.
func InitLogging() (*os.File, io.Writer) {
	logPath := filepath.Dir(LogFilePath())
	if err := os.MkdirAll(logPath, os.ModePerm); err != nil {
		log.Printf("Warning: Failed to create logs directory: %v", err)
	}

	logFile, err := os.OpenFile(LogFilePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.Printf("Warning: Failed to open log file: %v", err)
		LogWriter = os.Stdout
		log.SetOutput(LogWriter)
		return nil, LogWriter
	}

	LogWriter = io.MultiWriter(os.Stdout, logFile)
	log.SetOutput(LogWriter)
	return logFile, LogWriter
}
